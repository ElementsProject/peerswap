package wallet

import (
	"errors"
)

var (
	NotEnoughBalanceError = errors.New("Not enough balance on utxos")
)

type Wallet interface {
	GetAddress() (string, error)
	SendToAddress(string, uint64) (string, error)
	GetBalance() (uint64, error)
}
