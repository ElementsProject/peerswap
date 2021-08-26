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
	DecodeRawTx(txstring string) (*gelements.Tx, error)
}

type ElementsRpc struct {
	gelements *gelements.Elements
	network   *network.Network
}

// GetFee returns a fee for a provided txHex
func (e *ElementsRpc) GetFee(txHex string) uint64 {
	return FIXED_FEE
}

// GetAsset returns the asset of the network
func (e *ElementsRpc) GetAsset() []byte {
	return append(
		[]byte{0x01},
		elementsutil.ReverseBytes(h2b(e.network.AssetID))...,
	)
}

// GetNetwork returns the network (regtest,mainnet)
func (e *ElementsRpc) GetNetwork() *network.Network {
	return e.network
}

// GetLocktime returns a reasonable locktime for the asset/network
func (e *ElementsRpc) GetLocktime() uint64 {
	return LOCKTIME
}

func (e *ElementsRpc) GetBlockHash(blockheight uint32) (string, error) {
	return e.gelements.GetBlockHash(blockheight)
}

// GetBlockHeight returns the currentblockheight
func (e *ElementsRpc) GetBlockHeight() (u uint64, err error) {
	return e.gelements.GetBlockHeight()
}

// GetTxOut returns details about an Utxo
func (e *ElementsRpc) GetTxOut(txid string, vout uint32) (*gelements.TxOutResp, error) {
	return e.gelements.GetTxOut(txid, vout)
}

// SendRawTx broadcasts a raw transaction to the network
func (e *ElementsRpc) SendRawTx(txHex string) (string, error) {
	return e.gelements.SendRawTx(txHex)
}

// GetRawTxFromTxId returns the txhex from the txid. This only works when the tx is not spent
func (e *ElementsRpc) GetRawTxFromTxId(txId string, vout uint32) (string, error) {
	txOut, err := e.gelements.GetTxOut(txId, vout)
	if err != nil {
		return "", err
	}
	blockheight, err := e.gelements.GetBlockHeight()
	if err != nil {
		return "", err
	}
	blockhash, err := e.gelements.GetBlockHash(uint32(blockheight) - txOut.Confirmations + 1)
	if err != nil {
		return "", err
	}
	rawTxHex, err := e.gelements.GetRawtransactionWithBlockHash(txId, blockhash)
	if err != nil {
		return "", err
	}
	return rawTxHex, nil
}

// NewElementsRpc returns a new elements rpc
func NewElementsRpc(gelements *gelements.Elements, network2 *network.Network) *ElementsRpc {
	return &ElementsRpc{gelements: gelements, network: network2}
}

func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
