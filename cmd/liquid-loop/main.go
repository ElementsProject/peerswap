package main

import (
	"errors"
	"github.com/sputn1ck/sugarmama/liquid"
	"github.com/sputn1ck/sugarmama/swap"
	"github.com/sputn1ck/sugarmama/wallet"
	"github.com/vulpemventures/go-elements/network"
	"go.etcd.io/bbolt"
	"log"
	"os"
)


func main() {
	if err := run(); err != nil {
		log.Printf("plugin quitting, error: %s", err)
		os.Exit(1)
	}

}
func run() error {
	if len(os.Args) > 1 && (os.Args[1] == "--lnd") {
		// make lnd handler
		return errors.New("lnd mode is not yet supported")
	}


	// initialize
	clightning, initChan, err := NewClightningClient()
	if err != nil {
		return err
	}

	err = clightning.RegisterOptions()
	if err != nil {
		return err
	}
	err = clightning.RegisterMethods()
	if err != nil {
		return err
	}

	// start clightning plugin
	quitChan := make(chan interface{})
	go func () {
		err := clightning.Start()
		if err != nil {
			log.Fatal(err)
		}
		quitChan<-true
	}()
	<-initChan
	log.Printf("waiting for init finished")
	config, err := clightning.GetConfig()
	if err != nil {
		return err
	}
	log.Printf("Config: %s, %s, wd: %s", config.DbPath, config.EsploraUrl)
	// setup services
	// esplora
	esplora := liquid.NewEsploraClient("http://localhost:3001")

	// db
	boltDb, err := bbolt.Open(config.DbPath, 0700, nil)
	if err != nil {
		return err
	}

	// Wallet
	walletStore,err := wallet.NewBboltStore(boltDb)
	if err != nil {
		return err
	}
	err = walletStore.Initialize()
	if err != nil {
		return err
	}
	walletService := &wallet.LiquiddWallet{Store: walletStore, Blockchain: esplora}

	swapStore,err := swap.NewBboltStore(boltDb)
	if err != nil {
		return err
	}
	swapService := swap.NewService(swapStore, walletService, clightning, esplora, clightning, &network.Regtest)



	messageHandler := swap.NewMessageHandler(clightning, swapService)
	err = messageHandler.Start()
	if err != nil {
		return err
	}
	// DEBUG ONLY, fund addresses
	//addr, err := walletService.ListAddresses()
	//if err != nil {
	//	return err
	//}
	//_, err = esplora.DEV_Fundaddress(addr[0])
	//if err != nil {
	//	return err
	//}

	go func() {
		err := swapService.StartWatchingTxs()
		if err != nil {
			log.Printf("%v",err)
			os.Exit(1)
		}
	}()

	clightning.SetupClients(walletService, swapService, esplora)
	<-quitChan
	return nil
}
