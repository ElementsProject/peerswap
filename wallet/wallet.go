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
	// PrecheckTransaction funds and signs (but never broadcasts) a throwaway
	// version of the opening transaction to verify that the wallet can
	// construct it right now.
	PrecheckTransaction(swapParams *swap.OpeningParams, asset []byte) error
	SendRawTx(rawTx string) (txid string, err error)
	GetFee(txSize int64) (uint64, error)
	SetLabel(txID, address, label string) error
	Ping() (bool, error)
}
