package wallet

import (
	"errors"
	"fmt"
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
}

type rpcWallet struct {
	walletName string
	rpcClient  RpcClient
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
