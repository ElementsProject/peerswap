package lightning

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/niftynei/glightning/jrpc2"
)

type GetAddressMethod struct {
	wallet WalletService `json:"-"`
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
	return res, nil
}

type GetBalanceMethod struct {
	wallet WalletService `json:"-"`
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
	wallet WalletService `json:"-"`
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

type LoopIn struct {
	Amt  int64  `json:"amt"`
	Peer string `json:"peer"`

	wallet WalletService    `json:"-"`
	pc     PeerCommunicator `json:"-"`
}

type LoopInData struct {
	Amt int64  `json:"amt"`
	Msg string `json:"msg"`
}

func (l *LoopIn) New() interface{} {
	return &LoopIn{
		wallet: l.wallet,
		pc:     l.pc,
	}
}

func (l *LoopIn) Name() string {
	return "loop-in"
}

func (l *LoopIn) Call() (jrpc2.Result, error) {
	if l.Amt <= 0 {
		return nil, errors.New("Missing required amt parameter")
	}
	if l.Peer == "" {
		return nil, errors.New("Missing required peer parameter")
	}
	bytes, err := json.Marshal(&LoopInData{
		Amt: l.Amt,
		Msg: "Gudee",
	})
	if err != nil {
		return nil, err
	}
	err = l.pc.SendMessage(l.Peer, bytes)
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("loop-in called!"), nil
}
