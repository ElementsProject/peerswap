package main

import (
	"context"
	"errors"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/wallet"
	"github.com/vulpemventures/go-elements/network"
	"go.etcd.io/bbolt"
	"log"
	"os"
	"path/filepath"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	go func() {
		err := clightning.Start()
		if err != nil {
			log.Fatal(err)
		}
		quitChan <- true
	}()
	<-initChan
	log.Printf("waiting for init finished")
	config, err := clightning.GetConfig()
	if err != nil {
		return err
	}
	var liquidNetwork *network.Network
	if config.Network == "regtest" {
		liquidNetwork = &network.Regtest
	} else {
		liquidNetwork = &network.Liquid
	}
	log.Printf("Config: Db:%s, Rpc: %s %s, network: %s", config.DbPath, config.RpcHost, config.RpcUser, config.Network)
	// setup services
	// blockchaincli
	ecli := gelements.NewElements(config.RpcUser, config.RpcPassword)
	err = ecli.StartUp(config.RpcHost, config.RpcPort)
	if err != nil {
		return err
	}

	// db
	swapDb, err := bbolt.Open(filepath.Join(config.DbPath, "swaps"), 0700, nil)
	if err != nil {
		return err
	}

	// Wallet
	waleltCli := gelements.NewElements(config.RpcUser, config.RpcPassword)
	err = waleltCli.StartUp(config.RpcHost, config.RpcPort)
	if err != nil {
		return err
	}
	rpcWallet, err := wallet.NewRpcWallet(waleltCli, "swap")
	if err != nil {
		return err
	}

	swapStore, err := swap.NewBboltStore(swapDb)
	if err != nil {
		return err
	}
	swapService := swap.NewService(ctx, swapStore, rpcWallet, clightning, ecli, clightning, liquidNetwork)

	messageHandler := swap.NewMessageHandler(clightning, swapService)
	err = messageHandler.Start()
	if err != nil {
		return err
	}

	go func() {
		err := swapService.StartWatchingTxs()
		if err != nil {
			log.Printf("%v", err)
			os.Exit(1)
		}
	}()

	clightning.SetupClients(rpcWallet, swapService, ecli)
	<-quitChan
	return nil
}
