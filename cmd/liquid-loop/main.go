package main

import (
	"errors"
	"github.com/btcsuite/btcd/btcec"
	"github.com/sputn1ck/liquid-loop/liquid"
	"github.com/sputn1ck/liquid-loop/swap"
	"github.com/sputn1ck/liquid-loop/wallet"
	"github.com/vulpemventures/go-elements/network"
	"log"
	"os"
)

const (
	dataType = "aaff"
)

// ok, let's try plugging this into c-lightning
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}

}
func run() error {
	if len(os.Args) > 1 && (os.Args[1] == "--lnd") {
		// make lnd handler
		return errors.New("lnd is not yet supported")
	}

	privkey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return err
	}

	esplora := liquid.NewEsploraClient("http://localhost:3001")
	walletStore := &wallet.DummyWalletStore{PrivKey: privkey}
	walletService := &wallet.LiquiddWallet{Store: walletStore, Blockchain: esplora}

	clightning, err := NewClightningClient()
	if err != nil {
		return err
	}
	swapStore := swap.NewInMemStore()
	swapService := swap.NewService(swapStore, walletService, clightning,esplora, clightning, &network.Regtest)
	err = clightning.RegisterOptions()
	if err != nil {
		return err
	}
	err = clightning.RegisterMethods(walletService, swapService)
	if err != nil {
		return err
	}

	messageHandler := swap.NewMessageHandler(clightning, swapService)
	err = messageHandler.Start()
	if err != nil {
		return err
	}
	addr, err := walletService.ListAddresses()
	if err != nil {
		return err
	}
	_, err = esplora.DEV_Fundaddress(addr[0])
	if err != nil {
		return err
	}
	return clightning.Start()
}


