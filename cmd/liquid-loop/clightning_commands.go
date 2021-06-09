package main

import (
	"errors"
	"fmt"
	"github.com/niftynei/glightning/glightning"
	"github.com/niftynei/glightning/jrpc2"
	"github.com/sputn1ck/sugarmama/lightning"
	"github.com/sputn1ck/sugarmama/liquid"
	"github.com/sputn1ck/sugarmama/swap"
)

type GetAddressMethod struct {
	wallet lightning.WalletService `json:"-"`
}

func (g *GetAddressMethod) New() interface{} {
	return &GetAddressMethod{
		wallet: g.wallet,
	}
}

func (g *GetAddressMethod) Name() string {
	return "liquid-wallet-getaddress"
}

func (g *GetAddressMethod) Call() (jrpc2.Result, error) {
	res, err := g.wallet.ListAddresses()
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("%s", res[0]), nil
}

type GetBalanceMethod struct {
	wallet lightning.WalletService `json:"-"`
}

func (g *GetBalanceMethod) Name() string {
	return "liquid-wallet-getbalance"
}

func (g *GetBalanceMethod) New() interface{} {
	return &GetBalanceMethod{
		wallet: g.wallet,
	}
}

func (g *GetBalanceMethod) Call() (jrpc2.Result, error) {
	res, err := g.wallet.GetBalance()
	if err != nil {
		return nil, err
	}
	return res, nil
}

type ListUtxosMethod struct {
	wallet lightning.WalletService `json:"-"`
}

func (l *ListUtxosMethod) Name() string {
	return "liquid-wallet-listutxos"
}

func (l *ListUtxosMethod) New() interface{} {
	return &ListUtxosMethod{
		wallet: l.wallet,
	}
}

func (l *ListUtxosMethod) Call() (jrpc2.Result, error) {
	utxos, err := l.wallet.ListUtxos()
	if err != nil {
		return nil, err
	}
	return utxos, nil
}

type DevFaucet struct {
	esplora *liquid.EsploraClient   `json:"-"`
	wallet  lightning.WalletService `json:"-"`
}

func (d *DevFaucet) Name() string {
	return "dev-faucet"
}

func (d *DevFaucet) New() interface{} {
	return &DevFaucet{
		wallet:  d.wallet,
		esplora: d.esplora,
	}
}

func (d *DevFaucet) Call() (jrpc2.Result, error) {
	address, err := d.wallet.ListAddresses()
	if err != nil {
		return nil, err
	}
	return d.esplora.DEV_Fundaddress(address[0])
}

type SwapOut struct {
	SatAmt         uint64 `json:"amt"`
	ShortChannelId string `json:"short_channel_id"`

	wallet    lightning.WalletService    `json:"-"`
	pc        lightning.PeerCommunicator `json:"-"`
	lightning *glightning.Lightning      `json:"-"`
	swapper   *swap.Service              `json:"-"`
}

func (l *SwapOut) New() interface{} {
	return &SwapOut{
		wallet:    l.wallet,
		pc:        l.pc,
		lightning: l.lightning,
		swapper:   l.swapper,
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

	funds, err := l.lightning.ListFunds()
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
		return nil, errors.New("not enough balance to perform swap")
	}
	if !fundingChannels.Connected {
		return nil, errors.New("fundingChannels is not connected")
	}

	err = l.swapper.StartSwapOut(fundingChannels.PeerId, l.ShortChannelId, l.SatAmt)
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("loop-in called!"), nil
}

type ListSwaps struct {
	swapper *swap.Service `json:"-"`
}

func (l *ListSwaps) New() interface{} {
	return &ListSwaps{
		swapper: l.swapper,
	}
}

func (l *ListSwaps) Name() string {
	return "swaps"
}

func (l *ListSwaps) Call() (jrpc2.Result, error) {
	swaps, err := l.swapper.ListSwaps()
	if err != nil {
		return nil, err
	}
	return swaps, nil
}
