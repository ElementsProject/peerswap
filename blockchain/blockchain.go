package blockchain

import "github.com/sputn1ck/glightning/gelements"

type Blockchain interface {
	GetBlockHeight() (uint64, error)
	GetTxOut(txid string, vout uint32) (*gelements.TxOutResp, error)
	SendRawTx(txstring string) (string, error)
	GetRawtransaction(txId string) (string, error)
	DecodeRawTx(txstring string) (*gelements.Tx, error)
}

type ElementsRpc struct {
	gelements *gelements.Elements
}

func (e *ElementsRpc) GetRawtransaction(txId string) (string, error) {
	return e.gelements.GetRawtransaction(txId)
}

func (e *ElementsRpc) GetBlockHeight() (u uint64, err error) {
	return e.gelements.GetBlockHeight()
}

func (e *ElementsRpc) GetTxOut(txid string, vout uint32) (*gelements.TxOutResp, error) {
	return e.GetTxOut(txid, vout)
}

func (e *ElementsRpc) SendRawTx(txHex string) (string, error) {
	return e.SendRawTx(txHex)
}

func NewElementsRpc(gelements *gelements.Elements) *ElementsRpc {
	return &ElementsRpc{gelements: gelements}
}
