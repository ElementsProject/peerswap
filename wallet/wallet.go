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

type Wallet interface {
	GetAddress() (string, error)
	SendToAddress(string, uint64) (string, error)
	GetBalance() (uint64, error)
	CreateAndBroadcastTransaction(swapParams *swap.OpeningParams, asset []byte) (txid, rawTx string, fee uint64, err error)
	SendRawTx(rawTx string) (txid string, err error)
	GetFee(txSize int64) (uint64, error)
	SetLabel(txID, address, label string) error
	Ping() (bool, error)
}
