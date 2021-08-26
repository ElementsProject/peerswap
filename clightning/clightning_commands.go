package clightning

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
	"time"

	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/glightning/jrpc2"
	"github.com/sputn1ck/peerswap/swap"
)

// GetAddressMethod returns a new liquid address
type GetAddressMethod struct {
	cl *ClightningClient `json:"-"`
}

func (g *GetAddressMethod) New() interface{} {
	return &GetAddressMethod{
		cl: g.cl,
	}
}

func (g *GetAddressMethod) Name() string {
	return "peerswap-liquid-getaddress"
}

func (g *GetAddressMethod) Call() (jrpc2.Result, error) {
	res, err := g.cl.wallet.GetAddress()
	if err != nil {
		return nil, err
	}
	log.Printf("[Wallet] Getting address %s", res)
	return fmt.Sprintf("%s", res), nil
}

// GetBalance returns the liquid balance
type GetBalanceMethod struct {
	cl *ClightningClient
}

func (g *GetBalanceMethod) Name() string {
	return "peerswap-liquid-getbalance"
}

func (g *GetBalanceMethod) New() interface{} {
	return &GetBalanceMethod{
		cl: g.cl,
	}
}

func (g *GetBalanceMethod) Call() (jrpc2.Result, error) {
	res, err := g.cl.wallet.GetBalance()
	if err != nil {
		return nil, err
	}
	return res, nil
}

// SendToAddressMethod sends
type SendToAddressMethod struct {
	Address   string `json:"address"`
	AmountSat uint64 `json:"amount_sat"`
	cl        *ClightningClient
}

func (s *SendToAddressMethod) Name() string {
	return "peerswap-liquid-sendtoaddress"
}

func (s *SendToAddressMethod) New() interface{} {
	return &SendToAddressMethod{
		cl: s.cl,
	}
}

func (s *SendToAddressMethod) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &SendToAddressMethod{
		cl: client,
	}
}

func (s *SendToAddressMethod) Call() (jrpc2.Result, error) {
	if s.Address == "" {
		return nil, errors.New("address must be set")
	}
	res, err := s.cl.wallet.SendToAddress(s.Address, s.AmountSat)
	if err != nil {
		log.Printf("error %v", err)
		return nil, err
	}
	return res, nil
}

func (s *SendToAddressMethod) Description() string {
	return "sends lbtc to an address"
}

func (s *SendToAddressMethod) LongDescription() string {
	return "'"
}

// SwapOut starts a new swapout (paying an Invoice for onchain liquidity)
type SwapOut struct {
	SatAmt         uint64 `json:"amt"`
	ShortChannelId string `json:"short_channel_id"`

	cl *ClightningClient `json:"-"`
}

func (l *SwapOut) New() interface{} {
	return &SwapOut{
		cl: l.cl,
	}
}

func (l *SwapOut) Name() string {
	return "peerswap-swap-out"
}

func (l *SwapOut) Call() (jrpc2.Result, error) {
	if l.SatAmt <= 0 {
		return nil, errors.New("Missing required amt parameter")
	}
	if l.ShortChannelId == "" {
		return nil, errors.New("Missing required short_channel_id parameter")
	}

	funds, err := l.cl.glightning.ListFunds()
	if err != nil {
		return nil, err
	}
	var fundingChannels *glightning.FundingChannel
	for _, v := range funds.Channels {
		if v.ShortChannelId == l.ShortChannelId {
			fundingChannels = v
			break
		}
	}
	if fundingChannels == nil {
		return nil, errors.New("fundingChannels not found")
	}

	if fundingChannels.ChannelSatoshi < l.SatAmt {
		return nil, errors.New("not enough outbound capacity to perform swapOut")
	}
	if !fundingChannels.Connected {
		return nil, errors.New("fundingChannels is not connected")
	}
	liquidBalance, err := l.cl.wallet.GetBalance()
	if err != nil {
		return nil, err
	}
	// todo fix liquid fee amount
	if liquidBalance < 500 {
		return nil, errors.New("you require more than 500 lbtc sats for transaction ")
	}
	pk := l.cl.GetNodeId()
	swapOut, err := l.cl.swaps.SwapOut(fundingChannels.Id, l.ShortChannelId, pk, l.SatAmt)
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
			if swapOut.Current == swap.State_SwapOutSender_RequestSent || swapOut.Current == swap.State_SwapOutSender_FeeInvoiceReceived || swapOut.Current == swap.State_SwapOutSender_FeeInvoicePaid {
				continue
			}
			if swapOut.Current == swap.State_SwapOut_Canceled {
				if swapOut.Data.LastErr == nil {
					return nil, errors.New("swap canceled")
				}
				return nil, swapOut.Data.LastErr

			}
			if swapOut.Current == swap.State_SwapOutSender_TxBroadcasted {
				return swapOut.Data.ToPrettyPrint(), nil
			}

		}
	}
}

// SwapIn Starts a new swap in(providing onchain liquidity)
type SwapIn struct {
	SatAmt         uint64 `json:"amt"`
	ShortChannelId string `json:"short_channel_id"`

	cl *ClightningClient `json:"-"`
}

func (l *SwapIn) New() interface{} {
	return &SwapIn{
		cl: l.cl,
	}
}

func (l *SwapIn) Name() string {
	return "peerswap-swap-in"
}

func (l *SwapIn) Call() (jrpc2.Result, error) {
	if l.ShortChannelId == "" {
		return nil, errors.New("Missing required short_channel_id parameter")
	}

	funds, err := l.cl.glightning.ListFunds()
	if err != nil {
		return nil, err
	}
	var fundingChannels *glightning.FundingChannel
	for _, v := range funds.Channels {
		if v.ShortChannelId == l.ShortChannelId {
			fundingChannels = v
			break
		}
	}
	if fundingChannels == nil {
		return nil, errors.New("fundingChannels not found")
	}
	if fundingChannels.ChannelTotalSatoshi-fundingChannels.ChannelSatoshi < l.SatAmt {
		return nil, errors.New("not enough inbound capacity to perform swap")
	}
	if !fundingChannels.Connected {
		return nil, errors.New("fundingChannels is not connected")
	}

	liquidBalance, err := l.cl.wallet.GetBalance()
	if err != nil {
		return nil, err
	}
	if liquidBalance < l.SatAmt {
		return nil, errors.New("Not enough balance on liquid wallet")
	}
	pk := l.cl.GetNodeId()
	swapIn, err := l.cl.swaps.SwapIn(fundingChannels.Id, l.ShortChannelId, pk, l.SatAmt)
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
			if swapIn.Current == swap.State_SwapInSender_SwapInRequestSent || swapIn.Current == swap.State_SwapInSender_AgreementReceived {
				continue
			}
			if swapIn.Current == swap.State_SwapCanceled {
				if swapIn.Data.LastErr == nil {
					return nil, errors.New("swap canceled")
				}
				return nil, swapIn.Data.LastErr

			}
			if swapIn.Current == swap.State_SwapInSender_TxBroadcasted {
				return swapIn.Data.ToPrettyPrint(), nil
			}

		}
	}

}

// ListSwaps list all active and finished swaps
type ListSwaps struct {
	PrettyPrint bool              `json:"pretty_print,omitempty"`
	cl          *ClightningClient `json:"-"`
}

func (l *ListSwaps) New() interface{} {
	return &ListSwaps{
		cl: l.cl,
	}
}

func (l *ListSwaps) Name() string {
	return "peerswap-listswaps"
}

func (l *ListSwaps) Call() (jrpc2.Result, error) {
	swaps, err := l.cl.swaps.ListSwaps()
	if err != nil {
		return nil, err
	}
	sort.Slice(swaps, func(i, j int) bool {
		if swaps[i].Data != nil && swaps[j].Data != nil {
			return swaps[i].Data.CreatedAt < swaps[j].Data.CreatedAt
		}
		return false
	})
	if l.PrettyPrint {
		var pswasp []*swap.PrettyPrintSwapData
		for _, v := range swaps {
			if v.Data == nil {
				continue
			}
			pswasp = append(pswasp, v.Data.ToPrettyPrint())
		}
		return pswasp, nil
	}
	return swaps, nil
}

// ListPeers lists all peerswap-ready peers
type ListPeers struct {
	cl *ClightningClient
}

func (l *ListPeers) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ListPeers{cl: client}
}

func (l *ListPeers) Description() string {
	return "lists valid peerswap peers"
}

func (l *ListPeers) LongDescription() string {
	return ""
}

func (l *ListPeers) New() interface{} {
	return &ListPeers{
		cl: l.cl,
	}
}

func (l *ListPeers) Name() string {
	return "peerswap-listpeers"
}

func (l *ListPeers) Call() (jrpc2.Result, error) {
	peers, err := l.cl.glightning.ListNodes()
	if err != nil {
		return nil, err
	}

	channelMap := make(map[string]*glightning.FundingChannel)
	fundsresult, err := l.cl.glightning.ListFunds()
	if err != nil {
		return nil, err
	}

	for _, channel := range fundsresult.Channels {
		channelMap[channel.Id] = channel
	}

	peerswapPeers := []*PeerSwapPeer{}
	for _, v := range peers {
		if v.Id == l.cl.nodeId {
			continue
		}

		if v.Features == nil || !checkFeatures(v.Features.Raw, featureBit) {
			continue
		}

		peerSwapPeer := &PeerSwapPeer{NodeId: v.Id}
		var channel *glightning.FundingChannel
		var ok bool
		if channel, ok = channelMap[v.Id]; ok {
			peerSwapPeer.ChannelId = channel.ShortChannelId
			peerSwapPeer.LocalBalance = channel.ChannelSatoshi
			peerSwapPeer.RemoteBalance = uint64(channel.ChannelTotalSatoshi - channel.ChannelSatoshi)
			peerSwapPeer.Balance = fmt.Sprintf("%.2f%%", 100*(float64(channel.ChannelSatoshi)/float64(channel.ChannelTotalSatoshi)))
		}

		peerswapPeers = append(peerswapPeers, peerSwapPeer)
	}

	return peerswapPeers, nil
}

type GetSwap struct {
	SwapId string
	cl     *ClightningClient
}

func (g *GetSwap) Name() string {
	return "peerswap-getswap"
}

func (g *GetSwap) New() interface{} {
	return &GetSwap{
		cl:     g.cl,
		SwapId: g.SwapId,
	}
}

func (g *GetSwap) Call() (jrpc2.Result, error) {
	if g.SwapId == "" {
		return nil, errors.New("SwapId required")
	}
	swap, err := g.cl.swaps.GetSwap(g.SwapId)
	if err != nil {
		return nil, err
	}
	return swap, nil
}

func (g *GetSwap) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &GetSwap{
		cl: client,
	}
}

func (g *GetSwap) Description() string {
	return "returns swap by swapid"
}

func (g *GetSwap) LongDescription() string {
	return ""
}

type PeerSwapPeer struct {
	NodeId        string `json:"nodeid"`
	ChannelId     string `json:"short_channel_id"`
	LocalBalance  uint64 `json:"local_balance"`
	RemoteBalance uint64 `json:"remote_balance"`
	Balance       string `json:"balance"`
}

// checkFeatures checks if a node runs the peerswap plugin
func checkFeatures(features []byte, featureBit int64) bool {
	featuresInt := big.NewInt(0)
	featuresInt = featuresInt.SetBytes(features)
	bitInt := big.NewInt(0)
	bitInt = bitInt.Exp(big.NewInt(2), big.NewInt(featureBit), nil)
	compareInt := big.NewInt(0)
	compareInt = compareInt.And(featuresInt, bitInt)
	return compareInt.Cmp(bitInt) == 0
}

// randomString returns a random 32 byte random string
func randomString() string {
	idBytes := make([]byte, 32)
	_, _ = rand.Read(idBytes[:])
	return hex.EncodeToString(idBytes)
}
