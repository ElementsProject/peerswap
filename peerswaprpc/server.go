package peerswaprpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/peersync"
	"github.com/elementsproject/peerswap/peersync/format"
	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/premium"
	"github.com/samber/lo"

	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/wallet"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
)

type PeerswapServer struct {
	liquidWallet   wallet.Wallet
	swaps          *swap.SwapService
	requestedSwaps *swap.RequestedSwapsPrinter
	peerSync       *peersync.PeerSync
	peerStore      *peersync.Store
	policy         *policy.Policy
	ps             *premium.Setting

	lnd lnrpc.LightningClient

	sigchan chan os.Signal

	UnimplementedPeerSwapServer
}

func (p *PeerswapServer) AddPeer(ctx context.Context, request *AddPeerRequest) (*Policy, error) {
	err := p.policy.AddToAllowlist(request.PeerPubkey)
	if err != nil {
		return nil, err
	}
	pol := p.policy.Get()
	return GetPolicyMessage(pol), nil

}

func (p *PeerswapServer) AddSusPeer(ctx context.Context, request *AddPeerRequest) (*Policy, error) {
	err := p.policy.AddToSuspiciousPeerList(request.PeerPubkey)
	if err != nil {
		return nil, err
	}
	pol := p.policy.Get()
	return GetPolicyMessage(pol), nil

}

func (p *PeerswapServer) RemovePeer(ctx context.Context, request *RemovePeerRequest) (*Policy, error) {
	err := p.policy.RemoveFromAllowlist(request.PeerPubkey)
	if err != nil {
		return nil, err
	}
	pol := p.policy.Get()
	return GetPolicyMessage(pol), nil
}

func (p *PeerswapServer) RemoveSusPeer(ctx context.Context, request *RemovePeerRequest) (*Policy, error) {
	err := p.policy.RemoveFromSuspiciousPeerList(request.PeerPubkey)
	if err != nil {
		return nil, err
	}
	pol := p.policy.Get()
	return GetPolicyMessage(pol), nil
}

func (p *PeerswapServer) Stop(ctx context.Context, empty *Empty) (*Empty, error) {
	p.sigchan <- os.Interrupt
	return &Empty{}, nil
}

// NewPeerswapServer wires together the RPC server with its dependencies.
func NewPeerswapServer(
	liquidWallet wallet.Wallet,
	swaps *swap.SwapService,
	requestedSwaps *swap.RequestedSwapsPrinter,
	peerSync *peersync.PeerSync,
	peerStore *peersync.Store,
	policy *policy.Policy,
	gelements *gelements.Elements,
	lnd lnrpc.LightningClient,
	ps *premium.Setting,
	sigchan chan os.Signal,
) *PeerswapServer {
	return &PeerswapServer{
		liquidWallet:   liquidWallet,
		swaps:          swaps,
		requestedSwaps: requestedSwaps,
		peerSync:       peerSync,
		peerStore:      peerStore,
		policy:         policy,
		lnd:            lnd,
		ps:             ps,
		sigchan:        sigchan,
	}
}

func (p *PeerswapServer) SwapOut(ctx context.Context, request *SwapOutRequest) (*SwapResponse, error) {
	if request.SwapAmount <= 0 {
		return nil, errors.New("Missing required swap_amount parameter")
	}
	if request.ChannelId == 0 {
		return nil, errors.New("Missing required channel_id parameter")
	}
	var swapchan *lnrpc.Channel
	chans, err := p.lnd.ListChannels(ctx, &lnrpc.ListChannelsRequest{ActiveOnly: true})
	if err != nil {
		return nil, err
	}
	for _, v := range chans.Channels {
		if v.ChanId == request.ChannelId {
			swapchan = v
		}
	}
	if swapchan == nil {
		return nil, errors.New("channel not found")
	}

	if uint64(swapchan.LocalBalance) < (request.SwapAmount + 5000) {
		return nil, errors.New("not enough local balance on channel to perform swap out")
	}

	if !swapchan.Active {
		return nil, errors.New("channel is not connected")
	}

	if strings.Compare(request.Asset, "lbtc") == 0 {
		if !p.swaps.LiquidEnabled {
			return nil, errors.New("liquid swaps are not enabled")
		}
		if ok, perr := p.liquidWallet.Ping(); perr != nil || !ok {
			return nil, fmt.Errorf("liquid wallet not reachable: %v", perr)
		}

	} else if strings.Compare(request.Asset, "btc") == 0 {
		if !p.swaps.BitcoinEnabled {
			return nil, errors.New("bitcoin swaps are not enabled")
		}
	} else {
		return nil, errors.New("invalid asset (btc or lbtc)")
	}
	gi, err := p.lnd.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	pk := gi.IdentityPubkey
	intermediaryPeerId := swapchan.RemotePubkey
	peerId := intermediaryPeerId
	if request.GetPeerPubkey() != "" {
		peerId = request.GetPeerPubkey()
	}

	shortId := lnwire.NewShortChanIDFromInt(swapchan.ChanId)

	// Skip this test if force flag is set.
	if !request.Force {
		if p.peerSync == nil || !p.peerSync.HasCompatiblePeer(peerId) {
			return nil, fmt.Errorf("peer does not run peerswap")
		}
	}

	if !p.isPeerConnected(ctx, peerId) {
		return nil, fmt.Errorf("peer is not connected")
	}

	var swapOut *swap.SwapStateMachine
	if request.GetPeerPubkey() != "" && request.GetPeerPubkey() != intermediaryPeerId {
		swapOut, err = p.swaps.SwapOutTwoHop(peerId, request.Asset, shortId.String(), pk, request.SwapAmount, request.GetPremiumLimitRatePpm(), intermediaryPeerId)
	} else {
		swapOut, err = p.swaps.SwapOut(peerId, request.Asset, shortId.String(), pk, request.SwapAmount, request.GetPremiumLimitRatePpm())
	}
	if err != nil {
		return nil, err
	}
	// In order to be responsive we wait for the `opening_tx` to be sent before
	// we return. Time out if we wait too long.
	if !swapOut.WaitForStateChange(func(st swap.StateType) bool {
		switch st {
		case swap.State_SwapOutSender_AwaitTxConfirmation:
			return true
		case swap.State_SwapCanceled:
			err = fmt.Errorf(swapOut.Data.GetCancelMessage())
			return true
		default:
			return false
		}
	}, 30*time.Second) {
		// Timeout.
		return nil, errors.New("rpc timeout reached, use peerswap-listswaps for info")
	}
	if err != nil {
		return nil, err
	}
	return &SwapResponse{Swap: PrettyprintFromServiceSwap(swapOut)}, nil
}

// isPeerConnected returns true if the peer is connected to the lnd node.
func (p *PeerswapServer) isPeerConnected(ctx context.Context, peerId string) bool {
	peers, err := p.lnd.ListPeers(ctx, &lnrpc.ListPeersRequest{})
	if err != nil {
		log.Infof("Could not get peer: %v", err)
		return false
	}

	for _, peer := range peers.Peers {
		if peer.PubKey == peerId {
			return true
		}
	}

	return false
}

func (p *PeerswapServer) SwapIn(ctx context.Context, request *SwapInRequest) (*SwapResponse, error) {
	var swapchan *lnrpc.Channel
	chans, err := p.lnd.ListChannels(ctx, &lnrpc.ListChannelsRequest{ActiveOnly: true})
	if err != nil {
		return nil, err
	}
	for _, v := range chans.Channels {
		if v.ChanId == request.ChannelId {
			swapchan = v
		}
	}
	if swapchan == nil {
		return nil, errors.New("channel not found")
	}

	if uint64(swapchan.RemoteBalance) < (request.SwapAmount) {
		return nil, errors.New("not enough remote balance on channel to perform swap in")
	}

	if !swapchan.Active {
		return nil, errors.New("channel is not connected")
	}

	switch request.Asset {
	case "lbtc":
		if !p.swaps.LiquidEnabled {
			return nil, errors.New("liquid swaps are not enabled")
		}
	case "btc":
		if !p.swaps.BitcoinEnabled {
			return nil, errors.New("bitcoin swaps are not enabled")
		}
	default:
		return nil, errors.New("invalid asset (btc or lbtc)")
	}

	gi, err := p.lnd.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	pk := gi.IdentityPubkey
	intermediaryPeerId := swapchan.RemotePubkey
	peerId := intermediaryPeerId
	if request.GetPeerPubkey() != "" {
		peerId = request.GetPeerPubkey()
	}

	shortId := lnwire.NewShortChanIDFromInt(swapchan.ChanId)

	// Skip this test if force flag is set.
	if !request.Force {
		if p.peerSync == nil || !p.peerSync.HasCompatiblePeer(peerId) {
			return nil, fmt.Errorf("peer does not run peerswap")
		}
	}

	if !p.isPeerConnected(ctx, peerId) {
		return nil, fmt.Errorf("peer is not connected")
	}

	var swapIn *swap.SwapStateMachine
	if request.GetPeerPubkey() != "" && request.GetPeerPubkey() != intermediaryPeerId {
		swapIn, err = p.swaps.SwapInTwoHop(peerId, request.Asset, shortId.String(), pk, request.SwapAmount, request.GetPremiumLimitRatePpm(), intermediaryPeerId)
	} else {
		swapIn, err = p.swaps.SwapIn(peerId, request.Asset, shortId.String(), pk, request.SwapAmount, request.GetPremiumLimitRatePpm())
	}
	if err != nil {
		return nil, err
	}
	// In order to be responsive we wait for the `opening_tx` to be sent before
	// we return. Time out if we wait too long.
	if !swapIn.WaitForStateChange(func(st swap.StateType) bool {
		switch st {
		case swap.State_SwapInSender_SendTxBroadcastedMessage:
			return true
		case swap.State_SwapCanceled:
			err = fmt.Errorf(swapIn.Data.GetCancelMessage())
			return true
		default:
			return false
		}
	}, 30*time.Second) {
		// Timeout.
		return nil, errors.New("rpc timeout reached, use peerswap-listswaps for info")
	}
	if err != nil {
		return nil, err
	}
	return &SwapResponse{Swap: PrettyprintFromServiceSwap(swapIn)}, nil
}

func (p *PeerswapServer) GetSwap(ctx context.Context, request *GetSwapRequest) (*SwapResponse, error) {
	if request.SwapId == "" {
		return nil, errors.New("SwapId required")
	}
	swapRes, err := p.swaps.GetSwap(request.SwapId)
	if err != nil {
		return nil, err
	}
	return &SwapResponse{Swap: PrettyprintFromServiceSwap(swapRes)}, nil
}

func (p *PeerswapServer) ListSwaps(ctx context.Context, request *ListSwapsRequest) (*ListSwapsResponse, error) {
	swaps, err := p.swaps.ListSwaps()
	if err != nil {
		return nil, err
	}
	sort.Slice(swaps, func(i, j int) bool {
		if swaps[i].Data != nil && swaps[j].Data != nil {
			return swaps[i].Data.CreatedAt < swaps[j].Data.CreatedAt
		}
		return false
	})
	var resSwaps []*PrettyPrintSwap
	for _, v := range swaps {
		resSwaps = append(resSwaps, PrettyprintFromServiceSwap(v))
	}
	return &ListSwapsResponse{Swaps: resSwaps}, nil
}

func (p *PeerswapServer) ListPeers(ctx context.Context, request *ListPeersRequest) (*ListPeersResponse, error) {
	peersRes, err := p.lnd.ListPeers(ctx, &lnrpc.ListPeersRequest{})
	if err != nil {
		return nil, err
	}

	channelRes, err := p.lnd.ListChannels(ctx, &lnrpc.ListChannelsRequest{ActiveOnly: true})
	if err != nil {
		return nil, err
	}

	compatiblePeers := make(map[string]*peersync.Peer)
	if p.peerSync != nil {
		var err error
		compatiblePeers, err = p.peerSync.CompatiblePeers()
		if err != nil {
			return nil, err
		}
	}

	var peerSwapPeers []*PeerSwapPeer
	for _, v := range peersRes.Peers {
		if peerState, ok := compatiblePeers[v.PubKey]; ok {
			capability := peerState.Capability()
			swaps, err := p.swaps.ListSwapsByPeer(v.PubKey)
			if err != nil {
				return nil, err
			}

			view := format.BuildPeerView(v.PubKey, capability, swaps)
			peer := NewPeerSwapPeerFromView(view)
			peer.ChannelAdjacency = ChannelAdjacencyFromPeerState(peerState)
			peer.Channels = getPeerSwapChannels(v.PubKey, channelRes.Channels)
			peerSwapPeers = append(peerSwapPeers, peer)
		}

	}
	return &ListPeersResponse{Peers: peerSwapPeers}, nil
}

func getPeerSwapChannels(peerId string, channelList []*lnrpc.Channel) []*PeerSwapPeerChannel {
	var peerswapchannels []*PeerSwapPeerChannel
	for _, v := range findChannels(peerId, channelList) {
		peerswapchannels = append(peerswapchannels, lnrpcChannelToPeerswapChannel(v))
	}
	return peerswapchannels
}
func lnrpcChannelToPeerswapChannel(channel *lnrpc.Channel) *PeerSwapPeerChannel {
	return &PeerSwapPeerChannel{
		ChannelId:      channel.ChanId,
		ShortChannelId: lnwire.NewShortChanIDFromInt(channel.ChanId).String(),
		LocalBalance:   uint64(channel.LocalBalance),
		RemoteBalance:  uint64(channel.RemoteBalance),
		Active:         channel.Active,
	}
}

func findChannels(peerId string, channelList []*lnrpc.Channel) []*lnrpc.Channel {
	var channels []*lnrpc.Channel
	for _, v := range channelList {
		if v.RemotePubkey == peerId {
			channels = append(channels, v)
		}
	}
	return channels
}

func findPeer(peerId string, peerlist []*lnrpc.Peer) *lnrpc.Peer {
	for _, v := range peerlist {
		if v.PubKey == peerId {
			return v
		}
	}
	return nil
}

func (p *PeerswapServer) ReloadPolicyFile(ctx context.Context, request *ReloadPolicyFileRequest) (*Policy, error) {
	err := p.policy.ReloadFile()
	if err != nil {
		return nil, err
	}
	if p.peerSync != nil {
		p.peerSync.ForcePollAllPeers(context.Background())
	}
	pol := p.policy.Get()
	return GetPolicyMessage(pol), nil
}

func (p *PeerswapServer) ListRequestedSwaps(ctx context.Context, request *ListRequestedSwapsRequest) (*ListRequestedSwapsResponse, error) {
	requestedSwaps, err := p.requestedSwaps.GetRaw()
	if err != nil {
		return nil, err
	}
	swapMap := make(map[string]*RequestSwapList)
	for k, v := range requestedSwaps {
		var swapList []*RequestedSwap
		for _, reqSwap := range v {
			swapList = append(swapList, &RequestedSwap{
				Asset:           reqSwap.Asset,
				AmountSat:       reqSwap.AmountSat,
				SwapType:        RequestedSwap_SwapType(reqSwap.Type),
				RejectionReason: reqSwap.RejectionReason,
			})
		}
		swapMap[k] = &RequestSwapList{RequestedSwaps: swapList}
	}
	return &ListRequestedSwapsResponse{RequestedSwaps: swapMap}, nil
}

func (p *PeerswapServer) LiquidGetAddress(ctx context.Context, request *GetAddressRequest) (*GetAddressResponse, error) {
	if !p.swaps.LiquidEnabled {
		return nil, errors.New("liquid swaps are not enabled")
	}

	res, err := p.liquidWallet.GetAddress()
	if err != nil {
		return nil, err
	}
	return &GetAddressResponse{Address: res}, nil
}

func (p *PeerswapServer) LiquidGetBalance(ctx context.Context, request *GetBalanceRequest) (*GetBalanceResponse, error) {
	if !p.swaps.LiquidEnabled {
		return nil, errors.New("liquid swaps are not enabled")
	}

	res, err := p.liquidWallet.GetBalance()
	if err != nil {
		return nil, err
	}
	return &GetBalanceResponse{SatAmount: res}, nil

}

func (p *PeerswapServer) LiquidSendToAddress(ctx context.Context, request *SendToAddressRequest) (*SendToAddressResponse, error) {
	if !p.swaps.LiquidEnabled {
		return nil, errors.New("liquid swaps are not enabled")
	}

	if request.Address == "" {
		return nil, errors.New("address not set")
	}
	if request.SatAmount < 5000 {
		return nil, errors.New("amount is dust")
	}

	res, err := p.liquidWallet.SendToAddress(request.Address, request.SatAmount)
	if err != nil {
		return nil, err
	}
	return &SendToAddressResponse{TxId: res}, nil
}

func (p *PeerswapServer) ListActiveSwaps(ctx context.Context, request *ListSwapsRequest) (*ListSwapsResponse, error) {
	swaps, err := p.swaps.ListActiveSwaps()
	if err != nil {
		return nil, err
	}
	sort.Slice(swaps, func(i, j int) bool {
		if swaps[i].Data != nil && swaps[j].Data != nil {
			return swaps[i].Data.CreatedAt < swaps[j].Data.CreatedAt
		}
		return false
	})
	var resSwaps []*PrettyPrintSwap
	for _, v := range swaps {
		resSwaps = append(resSwaps, PrettyprintFromServiceSwap(v))
	}
	return &ListSwapsResponse{Swaps: resSwaps}, nil
}

func (p *PeerswapServer) AllowSwapRequests(ctx context.Context, request *AllowSwapRequestsRequest) (*Policy, error) {
	if request.Allow {
		p.policy.EnableSwaps()
	} else {
		p.policy.DisableSwaps()
	}

	pol := p.policy.Get()
	return GetPolicyMessage(pol), nil
}

func toPremiumAssetType(assetType AssetType) premium.AssetType {
	switch assetType {
	case AssetType_BTC:
		return premium.BTC
	case AssetType_LBTC:
		return premium.LBTC
	default:
		return premium.AsserUnspecified
	}
}

func toPremiumOperationType(operationType OperationType) premium.OperationType {
	switch operationType {
	case OperationType_SWAP_IN:
		return premium.SwapIn
	case OperationType_SWAP_OUT:
		return premium.SwapOut
	default:
		return premium.OperationUnspecified
	}
}

func ToAssetType(assetType premium.AssetType) AssetType {
	switch assetType {
	case premium.BTC:
		return AssetType_BTC
	case premium.LBTC:
		return AssetType_LBTC
	default:
		return AssetType_ASSET_UNSPECIFIED
	}
}

func ToOperationType(operationType premium.OperationType) OperationType {
	switch operationType {
	case premium.SwapIn:
		return OperationType_SWAP_IN
	case premium.SwapOut:
		return OperationType_SWAP_OUT
	default:
		return OperationType_OPERATION_UNSPECIFIED
	}
}

func (p *PeerswapServer) GetGlobalPremiumRate(ctx context.Context,
	request *GetGlobalPremiumRateRequest) (*PremiumRate, error) {
	if request.GetAsset() != AssetType_BTC && request.GetAsset() != AssetType_LBTC {
		return nil, fmt.Errorf("invalid asset type: %s", request.Asset)
	}
	if request.GetOperation() != OperationType_SWAP_IN &&
		request.GetOperation() != OperationType_SWAP_OUT {
		return nil, fmt.Errorf("invalid operation type: %s", request.Operation)
	}
	r, err := p.ps.GetDefaultRate(toPremiumAssetType(request.GetAsset()),
		toPremiumOperationType(request.GetOperation()))
	if err != nil {
		return nil, err
	}
	return &PremiumRate{
		Asset:          ToAssetType(r.Asset()),
		Operation:      ToOperationType(r.Operation()),
		PremiumRatePpm: r.PremiumRatePPM().Value(),
	}, nil
}

func (p *PeerswapServer) UpdateGlobalPremiumRate(ctx context.Context,
	request *UpdateGlobalPremiumRateRequest) (*PremiumRate, error) {
	rate, err := premium.NewPremiumRate(
		toPremiumAssetType(request.GetRate().GetAsset()),
		toPremiumOperationType(request.GetRate().GetOperation()),
		premium.NewPPM(request.GetRate().GetPremiumRatePpm()),
	)
	if err != nil {
		return nil, fmt.Errorf("could not create rate: %v", err)
	}
	err = p.ps.SetDefaultRate(ctx, rate)
	if err != nil {
		return nil, fmt.Errorf("could not set default rate: %v", err)
	}
	if p.peerSync != nil {
		p.peerSync.ForcePollAllPeers(ctx)
	}
	return request.GetRate(), nil
}

func (p *PeerswapServer) GetPremiumRate(ctx context.Context,
	request *GetPremiumRateRequest) (*PremiumRate, error) {
	if request.GetAsset() != AssetType_BTC && request.GetAsset() != AssetType_LBTC {
		return nil, fmt.Errorf("invalid asset type: %s", request.Asset)
	}
	if request.GetOperation() != OperationType_SWAP_IN &&
		request.GetOperation() != OperationType_SWAP_OUT {
		return nil, fmt.Errorf("invalid operation type: %s", request.Operation)
	}
	r, err := p.ps.GetRate(request.GetNodeId(),
		toPremiumAssetType(request.GetAsset()),
		toPremiumOperationType(request.GetOperation()))
	if err != nil {
		return nil, err
	}
	return &PremiumRate{
		Asset:          ToAssetType(r.Asset()),
		Operation:      ToOperationType(r.Operation()),
		PremiumRatePpm: r.PremiumRatePPM().Value(),
	}, nil
}

func (p *PeerswapServer) UpdatePremiumRate(ctx context.Context,
	request *UpdatePremiumRateRequest) (*PremiumRate, error) {
	rate, err := premium.NewPremiumRate(
		toPremiumAssetType(request.GetRate().GetAsset()),
		toPremiumOperationType(request.GetRate().GetOperation()),
		premium.NewPPM(request.GetRate().GetPremiumRatePpm()),
	)
	if err != nil {
		return nil, fmt.Errorf("could not create rate: %v", err)
	}
	err = p.ps.SetRate(ctx, request.GetNodeId(), rate)
	if err != nil {
		return nil, fmt.Errorf("could not set rate: %v", err)
	}
	if p.peerSync != nil {
		p.peerSync.ForcePollAllPeers(ctx)
	}
	return request.GetRate(), nil
}

func (p *PeerswapServer) DeletePremiumRate(ctx context.Context,
	request *DeletePremiumRateRequest) (*PremiumRate, error) {
	err := p.ps.DeleteRate(ctx, request.GetNodeId(),
		toPremiumAssetType(request.GetAsset()),
		toPremiumOperationType(request.GetOperation()))
	if err != nil {
		return nil, fmt.Errorf("could not delete rate: %v", err)
	}
	if p.peerSync != nil {
		p.peerSync.ForcePollAllPeers(ctx)
	}
	return &PremiumRate{
		Asset:     request.GetAsset(),
		Operation: request.GetOperation(),
	}, nil
}

func PrettyprintFromServiceSwap(swp *swap.SwapStateMachine) *PrettyPrintSwap {
	scid, err := NewScidFromString(swp.Data.GetScid())
	if err != nil {
		log.Debugf("Could not parse scid from %s: %v", scid, err)
	}

	var lnd_chan_id uint64
	if scid != nil {
		lnd_chan_id = scid.ToUint64()
	}

	return &PrettyPrintSwap{
		Id:              swp.SwapId.String(),
		CreatedAt:       swp.Data.CreatedAt,
		Asset:           swp.Data.GetChain(),
		Type:            swp.Type.String(),
		Role:            swp.Role.String(),
		State:           string(swp.Current),
		InitiatorNodeId: swp.Data.InitiatorNodeId,
		PeerNodeId:      swp.Data.PeerNodeId,
		Amount:          swp.Data.GetAmount(),
		ChannelId:       swp.Data.GetScid(),
		OpeningTxId:     swp.Data.GetOpeningTxId(),
		ClaimTxId:       swp.Data.ClaimTxId,
		CancelMessage:   swp.Data.GetCancelMessage(),
		LndChanId:       lnd_chan_id,
		// Reversing sign if role=sender because sender pays premium to peer
		PremiumAmount: lo.Ternary(swp.Role == swap.SWAPROLE_SENDER,
			-swp.Data.GetPremium(),
			swp.Data.GetPremium(),
		),
	}
}

func NewScidFromString(scid string) (*lnwire.ShortChannelID, error) {
	scid = strings.ReplaceAll(scid, "x", ":")
	parts := strings.Split(scid, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected scid to be composed of 3 blocks")
	}

	blockHeight, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, err
	}

	txIndex, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, err
	}

	txPosition, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, err
	}

	return &lnwire.ShortChannelID{
		BlockHeight: uint32(blockHeight),
		TxIndex:     uint32(txIndex),
		TxPosition:  uint16(txPosition),
	}, nil
}
