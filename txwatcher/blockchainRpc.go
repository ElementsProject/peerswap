package txwatcher

import (
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/glightning/gelements"
)

type ElementsBlockChainRpc struct {
	ecli *gelements.Elements
}

func (e *ElementsBlockChainRpc) GetBlockHeightByHash(blockhash string) (uint32, error) {
	res, err := e.ecli.GetBlockHeader(blockhash)
	if err != nil {
		return 0, err
	}
	return res.Height, nil
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
		BestBlockHash: txout.BestBlockHash,
	}, nil
}

func (e *ElementsBlockChainRpc) String() string {
	return "lbtc"
}

func (e *ElementsBlockChainRpc) GetBlockHash(height uint32) (string, error) {
	return e.ecli.GetBlockHash(height)
}

func (e *ElementsBlockChainRpc) GetRawtransactionWithBlockHash(txId string, blockHash string) (string, error) {
	return e.ecli.GetRawtransactionWithBlockHash(txId, blockHash)
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
		BestBlockHash: txout.BestBlockHash,
	}, nil
}

func (b *BitcoinBlockchainRpc) GetBlockHeightByHash(blockhash string) (uint32, error) {
	res, err := b.bcli.GetBlockHeader(blockhash)
	if err != nil {
		return 0, err
	}
	return res.Height, nil
}

func (b *BitcoinBlockchainRpc) String() string {
	return "btc"
}
func (b *BitcoinBlockchainRpc) GetBlockHash(height uint32) (string, error) {
	return b.bcli.GetBlockHash(height)
}

func (b *BitcoinBlockchainRpc) GetRawtransactionWithBlockHash(txId string, blockHash string) (string, error) {
	return b.bcli.GetRawtransactionWithBlockHash(txId, blockHash)
}
