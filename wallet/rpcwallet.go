package wallet

import (
	"errors"
	"fmt"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/vulpemventures/go-elements/transaction"
	"strings"
)

var (
	AlreadyExistsError = errors.New("wallet already exists")
	AlreadyLoadedError = errors.New("wallet is already loaded")
)

type RpcClient interface {
	GetNewAddress(addrType int) (string, error)
	SendToAddress(address string, amount string) (string, error)
	GetBalance() (uint64, error)
	LoadWallet(filename string) (string, error)
	CreateWallet(walletname string) (string, error)
	SetRpcWallet(walletname string)
	ListWallets() ([]string, error)
	FundRawTx(txHex string) (*gelements.FundRawResult, error)
	BlindRawTransaction(txHex string) (string, error)
	SignRawTransactionWithWallet(txHex string) (gelements.SignRawTransactionWithWalletRes, error)
	SendRawTx(txHex string) (string, error)
}

type rpcWallet struct {
	walletName string
	rpcClient  RpcClient
}

func (r *rpcWallet) CreateFundedTransaction(preparedTx *transaction.Transaction) (rawTx string, fee uint64, err error) {
	txHex, err := preparedTx.ToHex()
	if err != nil {
		return "", 0, err
	}
	fundedTx, err := r.rpcClient.FundRawTx(txHex)
	if err != nil {
		return "", 0, err
	}
	return fundedTx.TxString, gelements.ConvertBtc(fundedTx.Fee), nil
}

func (r *rpcWallet) FinalizeAndBroadcastFundedTransaction(rawTx string) (txId string, err error) {
	unblinded, err := r.rpcClient.BlindRawTransaction(rawTx)
	if err != nil {
		return "", err
	}
	finalized, err := r.rpcClient.SignRawTransactionWithWallet(unblinded)
	if err != nil {
		return "", err
	}
	txId, err = r.rpcClient.SendRawTx(finalized.Hex)
	if err != nil {
		return "", err
	}
	return txId, nil
}

func NewRpcWallet(rpcClient RpcClient, walletName string) (*rpcWallet, error) {
	rpcWallet := &rpcWallet{
		walletName: walletName,
		rpcClient:  rpcClient,
	}
	err := rpcWallet.setupWallet()
	if err != nil {
		return nil, err
	}
	return rpcWallet, nil
}

func (r *rpcWallet) setupWallet() error {
	loadedWallets, err := r.rpcClient.ListWallets()
	if err != nil {
		return err
	}
	walletLoaded := false
	for _, v := range loadedWallets {
		if v == r.walletName {
			walletLoaded = true
			break
		}
	}
	if !walletLoaded {
		//todo create wallet on specific error
		_, err = r.rpcClient.LoadWallet(r.walletName)
		if err != nil && strings.Contains(err.Error(), "not found") {
			_, err = r.rpcClient.CreateWallet(r.walletName)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

	}
	r.rpcClient.SetRpcWallet(r.walletName)
	return nil
}

func (r *rpcWallet) GetBalance() (uint64, error) {
	balance, err := r.rpcClient.GetBalance()
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func (r *rpcWallet) GetAddress() (string, error) {
	address, err := r.rpcClient.GetNewAddress(0)
	if err != nil {
		return "", err
	}
	return address, nil
}

func (r *rpcWallet) SendToAddress(address string, amount uint64) (string, error) {
	txId, err := r.rpcClient.SendToAddress(address, satsToAmountString(amount))
	if err != nil {
		return "", err
	}
	return txId, nil
}

func satsToAmountString(sats uint64) string {
	bitcoinAmt := float64(sats) / 100000000
	return fmt.Sprintf("%f", bitcoinAmt)
}
