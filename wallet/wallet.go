package wallet

import (
	"errors"
	"github.com/vulpemventures/go-elements/transaction"
)

var (
	NotEnoughBalanceError = errors.New("Not enough balance on utxos")
)

type Wallet interface {
	GetAddress() (string, error)
	SendToAddress(string, uint64) (string, error)
	GetBalance() (uint64, error)
	CreateFundedTransaction(preparedTx *transaction.Transaction) (rawTx string, fee uint64, err error)
	FinalizeAndBroadcastFundedTransaction(rawTx string) (txId string, err error)
}
