package clightning

import (
	"errors"
	"fmt"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/glightning/jrpc2"
	"github.com/sputn1ck/peerswap/swap"
	"log"
	"math/big"
	"sort"
)

type GetAddressMethod struct {
	cl *ClightningClient `json:"-"`
}

func (g *GetAddressMethod) New() interface{} {
	return &GetAddressMethod{
		cl: g.cl,
	}
}

func (g *GetAddressMethod) Name() string {
	return "liquid-wallet-getaddress"
}

func (g *GetAddressMethod) Call() (jrpc2.Result, error) {
	res, err := g.cl.wallet.GetAddress()
	if err != nil {
		return nil, err
	}
	log.Printf("[Wallet] Getting address %s", res)
	return fmt.Sprintf("%s", res), nil
}

type GetBalanceMethod struct {
	cl *ClightningClient `json:"-"`
}

func (g *GetBalanceMethod) Name() string {
	return "liquid-wallet-getbalance"
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
	return "swap-out"
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
	pk := l.cl.GetNodeId()
	swapOut, err := l.cl.swaps.SwapOut(fundingChannels.Id, l.ShortChannelId, pk, l.SatAmt)
	if err != nil {
		return nil, err
	}
	return swapOut.Data.ToPrettyPrint(), nil
}

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
	return "swap-in"
}

func (l *SwapIn) Call() (jrpc2.Result, error) {
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
	return swapIn.Data.ToPrettyPrint(), nil
}

type ListSwaps struct {
	PrettyPrint uint64 `json:"pretty_print"`

	cl *ClightningClient `json:"-"`
}

func (l *ListSwaps) New() interface{} {
	return &ListSwaps{
		cl: l.cl,
	}
}

func (l *ListSwaps) Name() string {
	return "swaps"
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
	if l.PrettyPrint == 1 {
		var pswasp []*swap.PrettyPrintSwap
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

type ListPeers struct {
	cl *ClightningClient
}

func (l *ListPeers) Get(client *ClightningClient) jrpc2.ServerMethod {
	return &ListPeers{cl:client}
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
	return "peerswap-peers"
}

func (l *ListPeers) Call() (jrpc2.Result, error) {
	peers, err := l.cl.glightning.ListNodes()
	if err != nil {
		return nil, err
	}
	channelMap := make(map[string] *glightning.FundingChannel)
	fundsresult, err := l.cl.glightning.ListFunds()
	if err != nil {
		return nil, err
	}
	for _,channel := range fundsresult.Channels {
		channelMap[channel.Id] = channel
	}
	peerswapPeers := []*PeerSwapPeer{}
	for _,v := range peers {
		if v.Id == l.cl.nodeId {
			continue
		}
		if !checkFeatures(v.Features.Raw, featureBit) {
			continue
		}
		peerSwapPeer := &PeerSwapPeer{NodeId: v.Id}
		var channel *glightning.FundingChannel
		var ok bool
		if channel, ok = channelMap[v.Id]; ok {

			peerSwapPeer.ChannelId = channel.ShortChannelId
			peerSwapPeer.LocalBalance = channel.ChannelSatoshi
			peerSwapPeer.RemoteBalance = uint64(channel.ChannelTotalSatoshi - channel.ChannelSatoshi)
			peerSwapPeer.Balance = fmt.Sprintf("%.2f%%",100*(float64(channel.ChannelSatoshi) / float64(channel.ChannelTotalSatoshi)))
		}

		peerswapPeers = append(peerswapPeers, peerSwapPeer)
	}

	return peerswapPeers,nil
}

type PeerSwapPeer struct {
	NodeId string
	ChannelId string
	LocalBalance uint64
	RemoteBalance uint64
	Balance string
}


func checkFeatures(features []byte, featureBit int64) bool {
	featuresInt := big.NewInt(0)
	featuresInt = featuresInt.SetBytes(features)
	bitInt := big.NewInt(0)
	bitInt = bitInt.Exp(big.NewInt(2), big.NewInt(featureBit), nil)
	compareInt := big.NewInt(0)
	compareInt = compareInt.And(featuresInt, bitInt)
	return compareInt.Cmp(bitInt) == 0
}