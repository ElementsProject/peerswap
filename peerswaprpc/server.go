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
	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/poll"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/wallet"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
)

type PeerswapServer struct {
	liquidWallet   wallet.Wallet
	swaps          *swap.SwapService
	requestedSwaps *swap.RequestedSwapsPrinter
	pollService    *poll.Service
	policy         *policy.Policy

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

func NewPeerswapServer(liquidWallet wallet.Wallet, swaps *swap.SwapService, requestedSwaps *swap.RequestedSwapsPrinter, pollService *poll.Service, policy *policy.Policy, gelements *gelements.Elements, lnd lnrpc.LightningClient, sigchan chan os.Signal) *PeerswapServer {
	return &PeerswapServer{liquidWallet: liquidWallet, swaps: swaps, requestedSwaps: requestedSwaps, pollService: pollService, policy: policy, lnd: lnd, sigchan: sigchan}
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
	peerId := swapchan.RemotePubkey

	shortId := lnwire.NewShortChanIDFromInt(swapchan.ChanId)

	// Skip this test if force flag is set.
	if !request.Force && !p.peerRunsPeerSwap(peerId) {
		return nil, fmt.Errorf("peer does not run peerswap")
	}

	if !p.isPeerConnected(ctx, peerId) {
		return nil, fmt.Errorf("peer is not connected")
	}

	swapOut, err := p.swaps.SwapOut(peerId, request.Asset, shortId.String(), pk, request.SwapAmount)
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

// peerRunsPeerSwap returns true if the peer has sent its poll info to the
// pollService showing that it is supporting peerswap.
func (p *PeerswapServer) peerRunsPeerSwap(peerId string) bool {
	pollInfo, err := p.pollService.GetPollFrom(peerId)
	if err == nil && pollInfo != nil {
		return true
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
	peerId := swapchan.RemotePubkey

	shortId := lnwire.NewShortChanIDFromInt(swapchan.ChanId)

	// Skip this test if force flag is set.
	if !request.Force && !p.peerRunsPeerSwap(peerId) {
		return nil, fmt.Errorf("peer does not run peerswap")
	}

	if !p.isPeerConnected(ctx, peerId) {
		return nil, fmt.Errorf("peer is not connected")
	}

	swapIn, err := p.swaps.SwapIn(peerId, request.Asset, shortId.String(), pk, request.SwapAmount)
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

	polls, err := p.pollService.GetCompatiblePolls()
	if err != nil {
		return nil, err
	}

	var peerSwapPeers []*PeerSwapPeer
	for _, v := range peersRes.Peers {
		if poll, ok := polls[v.PubKey]; ok {
			swaps, err := p.swaps.ListSwapsByPeer(v.PubKey)
			if err != nil {
				return nil, err
			}

			var paidFees uint64
			var ReceiverSwapsOut, ReceiverSwapsIn, ReceiverSatsOut, ReceiverSatsIn uint64
			var SenderSwapsOut, SenderSwapsIn, SenderSatsOut, SenderSatsIn uint64
			for _, s := range swaps {
				// We only list successful swaps. They all end in an
				// State_ClaimedPreimage state.
				if s.Current == swap.State_ClaimedPreimage {
					if s.Role == swap.SWAPROLE_SENDER {
						paidFees += s.Data.OpeningTxFee
						if s.Type == swap.SWAPTYPE_OUT {
							SenderSwapsOut++
							SenderSatsOut += s.Data.GetAmount()
						} else {
							SenderSwapsIn++
							SenderSatsIn += s.Data.GetAmount()
						}
					} else {
						if s.Type == swap.SWAPTYPE_OUT {
							ReceiverSwapsOut++
							ReceiverSatsOut += s.Data.GetAmount()
						} else {
							ReceiverSwapsIn++
							ReceiverSatsIn += s.Data.GetAmount()
						}
					}
				}
			}

			peerSwapPeers = append(peerSwapPeers, &PeerSwapPeer{
				NodeId:          v.PubKey,
				SwapsAllowed:    poll.PeerAllowed,
				SupportedAssets: poll.Assets,
				Channels:        getPeerSwapChannels(v.PubKey, channelRes.Channels),
				AsSender: &SwapStats{
					SwapsOut: SenderSwapsOut,
					SwapsIn:  SenderSwapsIn,
					SatsOut:  SenderSatsOut,
					SatsIn:   SenderSatsIn,
				},
				AsReceiver: &SwapStats{
					SwapsOut: ReceiverSwapsOut,
					SwapsIn:  ReceiverSwapsIn,
					SatsOut:  ReceiverSatsOut,
					SatsIn:   ReceiverSatsIn,
				},
				PaidFee: paidFees,
			})
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
		ChannelId:     channel.ChanId,
		LocalBalance:  uint64(channel.LocalBalance),
		RemoteBalance: uint64(channel.RemoteBalance),
		Active:        channel.Active,
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
	p.pollService.PollAllPeers()
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

func PrettyprintFromServiceSwap(swap *swap.SwapStateMachine) *PrettyPrintSwap {
	scid, err := newScidFromString(swap.Data.GetScid())
	if err != nil {
		log.Debugf("Could not parse scid from %s: %v", scid, err)
	}

	var lnd_chan_id uint64
	if scid != nil {
		lnd_chan_id = scid.ToUint64()
	}

	return &PrettyPrintSwap{
		Id:              swap.SwapId.String(),
		CreatedAt:       swap.Data.CreatedAt,
		Asset:           swap.Data.GetChain(),
		Type:            swap.Type.String(),
		Role:            swap.Role.String(),
		State:           string(swap.Current),
		InitiatorNodeId: swap.Data.InitiatorNodeId,
		PeerNodeId:      swap.Data.PeerNodeId,
		Amount:          swap.Data.GetAmount(),
		ChannelId:       swap.Data.GetScid(),
		OpeningTxId:     swap.Data.GetOpeningTxId(),
		ClaimTxId:       swap.Data.ClaimTxId,
		CancelMessage:   swap.Data.GetCancelMessage(),
		LndChanId:       lnd_chan_id,
	}
}

func newScidFromString(scid string) (*lnwire.ShortChannelID, error) {
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
