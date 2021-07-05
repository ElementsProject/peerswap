package blockchain

import (
	"encoding/hex"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
)

const (
	FIXED_FEE = 500
	LOCKTIME  = 120
)

type Blockchain interface {
	GetBlockHeight() (uint64, error)
	GetTxOut(txid string, vout uint32) (*gelements.TxOutResp, error)
	SendRawTx(txstring string) (string, error)
	GetRawtransaction(txId string) (string, error)
	DecodeRawTx(txstring string) (*gelements.Tx, error)
}

type ElementsRpc struct {
	gelements *gelements.Elements
	network   *network.Network
}

func (e *ElementsRpc) GetFee(txHex string) uint64 {
	return FIXED_FEE
}

func (e *ElementsRpc) GetAsset() []byte {
	return append(
		[]byte{0x01},
		elementsutil.ReverseBytes(h2b(e.network.AssetID))...,
	)
}

func (e *ElementsRpc) GetNetwork() *network.Network {
	return e.network
}

func (e *ElementsRpc) GetLocktime() uint64 {
	return LOCKTIME
}

func (e *ElementsRpc) GetRawtransaction(txId string) (string, error) {
	return e.gelements.GetRawtransaction(txId)
}

func (e *ElementsRpc) GetBlockHeight() (u uint64, err error) {
	return e.gelements.GetBlockHeight()
}

func (e *ElementsRpc) GetTxOut(txid string, vout uint32) (*gelements.TxOutResp, error) {
	return e.gelements.GetTxOut(txid, vout)
}

func (e *ElementsRpc) SendRawTx(txHex string) (string, error) {
	return e.gelements.SendRawTx(txHex)
}

func NewElementsRpc(gelements *gelements.Elements, network2 *network.Network) *ElementsRpc {
	return &ElementsRpc{gelements: gelements, network: network2}
}

func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
