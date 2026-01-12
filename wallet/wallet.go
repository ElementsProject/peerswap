package wallet

import (
	"errors"

	"github.com/elementsproject/peerswap/swap"
)

var (
	NotEnoughBalanceError  = errors.New("Not enough balance on utxos")
	MinRelayFeeNotMetError = errors.New("MinRelayFee not met")
)

const (
	LiquidTargetBlocks = 7
)

type TxOutput struct {
	// AssetID is a 32-byte hex encoded Elements/Liquid asset id (big-endian as
	// commonly displayed).
	AssetID string
	// Amount is the amount in the on-chain asset base units.
	Amount uint64
}

type Wallet interface {
	GetAddress() (string, error)
	SendToAddress(string, uint64) (string, error)
	GetBalance() (uint64, error)
	CreateAndBroadcastTransaction(swapParams *swap.OpeningParams, outputs []TxOutput) (txid, rawTx string, fee uint64, err error)
	SendRawTx(rawTx string) (txid string, err error)
	GetFee(txSize int64) (uint64, error)
	SetLabel(txID, address, label string) error
	Ping() (bool, error)
}
