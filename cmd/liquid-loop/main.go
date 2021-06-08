package main

import (
	"errors"
	"github.com/btcsuite/btcd/btcec"
	lightning2 "github.com/sputn1ck/liquid-loop/lightning"
	"github.com/sputn1ck/liquid-loop/liquid"
	"github.com/sputn1ck/liquid-loop/wallet"
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

	clightning, err := lightning2.NewClightningClient()
	if err != nil {
		return err
	}
	err = clightning.RegisterOptions()
	if err != nil {
		return err
	}
	err = clightning.RegisterMethods(walletService)
	if err != nil {
		return err
	}

	return clightning.Start()
}
