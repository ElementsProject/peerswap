package peerswaprpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/poll"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/wallet"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/sputn1ck/glightning/gelements"
)

type PeerswapServer struct {
	liquidWallet   wallet.Wallet
	swaps          *swap.SwapService
	requestedSwaps *swap.RequestedSwapsPrinter
	pollService    *poll.Service
	policy         *policy.Policy

	Gelements *gelements.Elements
	lnd       lnrpc.LightningClient

	sigchan chan os.Signal

	UnimplementedPeerSwapServer
}

func (p *PeerswapServer) AddPeer(ctx context.Context, request *AddPeerRequest) (*AddPeerResponse, error) {
	err := p.policy.AddToAllowlist(request.PeerPubkey)
	if err != nil {
		return nil, err
	}
	pol := p.policy.Get()
	return &AddPeerResponse{Policy: &Policy{
		ReserveOnchainMsat: pol.ReserveOnchainMsat,
		AcceptAllPeers:     pol.AcceptAllPeers,
		PeerAllowList:      pol.PeerAllowlist,
	}}, nil

}

func (p *PeerswapServer) RemovePeer(ctx context.Context, request *RemovePeerRequest) (*RemovePeerResponse, error) {
	err := p.policy.RemoveFromAllowlist(request.PeerPubkey)
	if err != nil {
		return nil, err
	}
	pol := p.policy.Get()
	return &RemovePeerResponse{Policy: &Policy{
		ReserveOnchainMsat: pol.ReserveOnchainMsat,
		AcceptAllPeers:     pol.AcceptAllPeers,
		PeerAllowList:      pol.PeerAllowlist,
	}}, nil
}

func (p *PeerswapServer) Stop(ctx context.Context, empty *Empty) (*Empty, error) {
	p.sigchan <- os.Interrupt
	return &Empty{}, nil
}

func NewPeerswapServer(liquidWallet wallet.Wallet, swaps *swap.SwapService, requestedSwaps *swap.RequestedSwapsPrinter, pollService *poll.Service, policy *policy.Policy, gelements *gelements.Elements, lnd lnrpc.LightningClient, sigchan chan os.Signal) *PeerswapServer {
	return &PeerswapServer{liquidWallet: liquidWallet, swaps: swaps, requestedSwaps: requestedSwaps, pollService: pollService, policy: policy, Gelements: gelements, lnd: lnd, sigchan: sigchan}
}

func (p *PeerswapServer) SwapOut(ctx context.Context, request *SwapOutRequest) (*SwapResponse, error) {
	if request.SwapAmount <= 0 {
		return nil, errors.New("Missing required amt parameter")
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
		if p.Gelements == nil {
			return nil, errors.New("peerswap was not started with liquid node config")
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
	err = p.PeerRunsPeerSwap(ctx, peerId)
	if err != nil {
		return nil, err
	}
	swapOut, err := p.swaps.SwapOut(peerId, request.Asset, shortId.String(), pk, request.SwapAmount)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("rpc timeout reached, use peerswap-listswaps for info")
		default:
			if swapOut.Current == swap.State_SwapOutSender_AwaitAgreement || swapOut.Current == swap.State_SwapOutSender_PayFeeInvoice || swapOut.Current == swap.State_SwapOutSender_AwaitTxBroadcastedMessage {
				continue
			}
			if swapOut.Current == swap.State_SwapCanceled {
				if swapOut.Data.CancelMessage != "" {
					return nil, errors.New(fmt.Sprintf("Swap canceled, cancel message: %s", swapOut.Data.CancelMessage))
				}
				if swapOut.Data.LastErr == nil {
					return nil, errors.New("swap canceled")
				}
				return nil, errors.New(swapOut.Data.LastErrString)

			}
			if swapOut.Current == swap.State_SwapOutSender_AwaitTxConfirmation {
				return &SwapResponse{Swap: PrettyprintFromServiceSwap(swapOut)}, nil
			}

		}
	}
}

func (p *PeerswapServer) PeerRunsPeerSwap(ctx context.Context, peerid string) error {
	// get polls
	polls, err := p.pollService.GetPolls()
	if err != nil {
		return err
	}
	peers, err := p.lnd.ListPeers(ctx, &lnrpc.ListPeersRequest{})
	if err != nil {
		return err
	}

	if _, ok := polls[peerid]; !ok {
		return errors.New("peer does not run peerswap")
	}

	for _, peer := range peers.Peers {
		if peer.PubKey == peerid {
			return nil
		}
	}
	return errors.New("peer is not connected")
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

	if request.Asset == "lbtc" {
		if !p.swaps.LiquidEnabled {
			return nil, errors.New("liquid swaps are not enabled")
		}
		if p.Gelements == nil {
			return nil, errors.New("peerswap was not started with liquid node config")
		}
		liquidBalance, err := p.liquidWallet.GetBalance()
		if err != nil {
			return nil, err
		}
		if liquidBalance < request.SwapAmount+1000 {
			return nil, errors.New("Not enough balance on liquid wallet")
		}
	} else if request.Asset == "btc" {
		if !p.swaps.BitcoinEnabled {
			return nil, errors.New("bitcoin swaps are not enabled")
		}
		walletbalance, err := p.lnd.WalletBalance(ctx, &lnrpc.WalletBalanceRequest{})
		if err != nil {
			return nil, err
		}
		if uint64(walletbalance.ConfirmedBalance) < request.SwapAmount+2000 {
			return nil, errors.New("Not enough balance on lnd onchain liquidWallet")
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

	err = p.PeerRunsPeerSwap(ctx, peerId)
	if err != nil {
		return nil, err
	}

	swapIn, err := p.swaps.SwapIn(peerId, request.Asset, shortId.String(), pk, request.SwapAmount)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("rpc timeout reached, use peerswap-listswaps for info")
		default:
			if swapIn.Current == swap.State_SwapInSender_AwaitAgreement || swapIn.Current == swap.State_SwapInSender_BroadcastOpeningTx {
				continue
			}
			if swapIn.Current == swap.State_SwapCanceled {
				if swapIn.Data.CancelMessage != "" {
					return nil, errors.New(fmt.Sprintf("Swap canceled, cancel message: %s", swapIn.Data.CancelMessage))
				}
				if swapIn.Data.LastErr == nil {
					return nil, errors.New("swap canceled")
				}
				return nil, swapIn.Data.LastErr

			}
			if swapIn.Current == swap.State_SwapInSender_SendTxBroadcastedMessage {
				return &SwapResponse{Swap: PrettyprintFromServiceSwap(swapIn)}, nil
			}

		}
	}
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

	polls, err := p.pollService.GetPolls()
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
		ChannelId:         channel.ChanId,
		LocalBalance:      uint64(channel.LocalBalance),
		RemoteBalance:     uint64(channel.RemoteBalance),
		BalancePercentage: float64(channel.LocalBalance) / float64(channel.LocalBalance+channel.RemoteBalance),
		Active:            channel.Active,
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

func (p *PeerswapServer) ListNodes(ctx context.Context, request *ListNodesRequest) (*ListNodesResponse, error) {
	return nil, errors.New("ListNodes does not work on lnd yet")
}

func (p *PeerswapServer) ReloadPolicyFile(ctx context.Context, request *ReloadPolicyFileRequest) (*ReloadPolicyFileResponse, error) {
	err := p.policy.ReloadFile()
	if err != nil {
		return nil, err
	}
	p.pollService.PollAllPeers()
	pol := p.policy.Get()
	return &ReloadPolicyFileResponse{Policy: &Policy{
		ReserveOnchainMsat: pol.ReserveOnchainMsat,
		AcceptAllPeers:     pol.AcceptAllPeers,
		PeerAllowList:      pol.PeerAllowlist,
	}}, nil
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
	if p.Gelements == nil {
		return nil, errors.New("peerswap was not started with liquid node config")
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
	if p.Gelements == nil {
		return nil, errors.New("peerswap was not started with liquid node config")
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
	if p.Gelements == nil {
		return nil, errors.New("peerswap was not started with liquid node config")
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

func (p *PeerswapServer) RejectSwaps(ctx context.Context, request *RejectSwapsRequest) (*RejectSwapsResponse, error) {
	reject := p.swaps.SetRejectSwaps(request.Reject)
	return &RejectSwapsResponse{Reject: reject}, nil
}

func PrettyprintFromServiceSwap(swap *swap.SwapStateMachine) *PrettyPrintSwap {
	timeStamp := time.Unix(swap.Data.CreatedAt, 0)
	return &PrettyPrintSwap{
		Id:              swap.Id,
		CreatedAt:       timeStamp.String(),
		Type:            swap.Type.String(),
		Role:            swap.Role.String(),
		State:           string(swap.Current),
		InitiatorNodeId: swap.Data.InitiatorNodeId,
		PeerNodeId:      swap.Data.PeerNodeId,
		Amount:          swap.Data.GetAmount(),
		ChannelId:       swap.Data.GetScid(),
		OpeningTxId:     swap.Data.GetOpeningTxId(),
		ClaimTxId:       swap.Data.ClaimTxId,
		CancelMessage:   swap.Data.CancelMessage,
	}
}
