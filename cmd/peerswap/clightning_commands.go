package main

import (
	"errors"
	"fmt"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/glightning/jrpc2"
	"github.com/sputn1ck/peerswap/swap"
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
	swapOut, err := l.cl.swaps.SwapOut(fundingChannels.Id, l.SatAmt, l.ShortChannelId, pk)
	if err != nil {
		return nil, err
	}
	return swapOut.Data.(*swap.Swap).ToPrettyPrint(), nil
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

// todo change to swap in
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
	swapOut, err := l.cl.swaps.SwapOut(fundingChannels.Id, l.SatAmt, l.ShortChannelId, pk)
	if err != nil {
		return nil, err
	}
	return swapOut.Data.(*swap.Swap).ToPrettyPrint(), nil
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

// todo reimplement
func (l *ListSwaps) Call() (jrpc2.Result, error) {
	//swaps, err := l.cl.swaps.ListSwaps()
	//if err != nil {
	//	return nil, err
	//}
	//sort.Slice(swaps, func(i, j int) bool {
	//	return swaps[i].CreatedAt < swaps[j].CreatedAt
	//})
	//if l.PrettyPrint == 1 {
	//	var pswasp []*swap.PrettyPrintSwap
	//	for _, v := range swaps {
	//		pswasp = append(pswasp, v.ToPrettyPrint())
	//	}
	//	return pswasp, nil
	//}
	//return swaps, nil
	return "not implemented yet", nil
}
