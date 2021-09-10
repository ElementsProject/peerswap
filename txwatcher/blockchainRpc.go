package txwatcher

import (
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/glightning/gelements"
)

type ElementsBlockChainRpc struct {
	ecli *gelements.Elements
}

func NewElementsCli(ecli *gelements.Elements) *ElementsBlockChainRpc {
	return &ElementsBlockChainRpc{ecli: ecli}
}

func (e *ElementsBlockChainRpc) GetBlockHeight() (uint64, error) {
	return e.ecli.GetBlockHeight()
}

func (e *ElementsBlockChainRpc) GetTxOut(txid string, vout uint32) (*TxOutResp, error) {
	txout, err := e.ecli.GetTxOut(txid, vout)
	if err != nil {
		return nil, err
	}
	if txout == nil {
		return nil, nil
	}
	return &TxOutResp{
		Confirmations: txout.Confirmations,
		Value:         txout.Value,
	}, nil
}

func (e *ElementsBlockChainRpc) String() string {
	return "l-btc"
}

type BitcoinBlockchainRpc struct {
	bcli *gbitcoin.Bitcoin
}

func NewBitcoinRpc(bcli *gbitcoin.Bitcoin) *BitcoinBlockchainRpc {
	return &BitcoinBlockchainRpc{bcli: bcli}
}

func (b *BitcoinBlockchainRpc) GetBlockHeight() (uint64, error) {
	return b.bcli.GetBlockHeight()
}

func (b *BitcoinBlockchainRpc) GetTxOut(txid string, vout uint32) (*TxOutResp, error) {
	txout, err := b.bcli.GetTxOut(txid, vout)
	if err != nil {
		return nil, err
	}
	if txout == nil {
		return nil, nil
	}
	return &TxOutResp{
		Confirmations: txout.Confirmations,
		Value:         txout.Value,
	}, nil
}

func (e *BitcoinBlockchainRpc) String() string {
	return "btc"
}
