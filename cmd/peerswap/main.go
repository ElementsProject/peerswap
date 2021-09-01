package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap"
	"github.com/sputn1ck/peerswap/onchain"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/peerswap/clightning"
	"github.com/sputn1ck/peerswap/policy"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/txwatcher"
	"github.com/sputn1ck/peerswap/wallet"
	"github.com/vulpemventures/go-elements/network"
	"go.etcd.io/bbolt"
)

func main() {
	if err := run(); err != nil {
		log.Printf("plugin quitting, error: %s", err)
		os.Exit(1)
	}

}
func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// initialize
	lightningPlugin, initChan, err := clightning.NewClightningClient()
	if err != nil {
		return err
	}

	err = lightningPlugin.RegisterOptions()
	if err != nil {
		return err
	}
	err = lightningPlugin.RegisterMethods()
	if err != nil {
		return err
	}

	// start lightningPlugin plugin
	quitChan := make(chan interface{})
	go func() {
		err := lightningPlugin.Start()
		if err != nil {
			log.Fatal(err)
		}
		quitChan <- true
	}()
	<-initChan
	log.Printf("waiting for init finished")
	config, err := lightningPlugin.GetConfig()
	if err != nil {
		return err
	}
	err = validateConfig(config)
	if err != nil {
		return err
	}
	log.Printf("Config: Db:%s, Rpc: %s %s, network: %s", config.DbPath, config.LiquidRpcHost, config.LiquidRpcUser, config.LiquidNetworkString)
	// setup services

	// liquid
	var liquidOnChainService *onchain.LiquidOnChain
	var liquidTxWatcher *txwatcher.BlockchainRpcTxWatcher
	var liquidRpcWallet *wallet.RpcWallet
	var liquidCli *gelements.Elements
	if config.LiquidEnabled {
		log.Printf("liquid enabled")
		// blockchaincli
		liquidCli = gelements.NewElements(config.LiquidRpcUser, config.LiquidRpcPassword)
		err = liquidCli.StartUp(config.LiquidRpcHost, config.LiquidRpcPort)
		if err != nil {
			return err
		}
		// Wallet
		liquidWalletCli := gelements.NewElements(config.LiquidRpcUser, config.LiquidRpcPassword)
		err = liquidWalletCli.StartUp(config.LiquidRpcHost, config.LiquidRpcPort)
		if err != nil {
			return err
		}
		liquidRpcWallet, err = wallet.NewRpcWallet(liquidWalletCli, config.LiquidRpcWallet)
		if err != nil {
			return err
		}

		// txwatcher
		liquidTxWatcher = txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewElementsCli(liquidCli), 2)

		// LiquidChain
		liquidOnChainService = onchain.NewLiquidOnChain(liquidCli, liquidTxWatcher, liquidRpcWallet, config.LiquidNetwork)
	} else {
		log.Printf("liquid disabled")
	}

	// bitcoin
	bitcoinCli, err := getBitcoinClient(lightningPlugin.GetLightningRpc())
	if err != nil {
		return err
	}
	bitcoinTxWatcher := txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewBitcoinRpc(bitcoinCli), 3)
	bitcoinOnChainService := onchain.NewBitcoinOnChain(bitcoinCli, bitcoinTxWatcher, lightningPlugin.GetLightningRpc())

	// db
	swapDb, err := bbolt.Open(filepath.Join(config.DbPath, "swaps"), 0700, nil)
	if err != nil {
		return err
	}

	// policy
	simplepolicy := &policy.SimplePolicy{}

	swapStore, err := swap.NewBboltStore(swapDb)
	if err != nil {
		return err
	}
	swapService := swap.NewSwapService(swapStore,
		liquidOnChainService,
		bitcoinOnChainService,
		lightningPlugin,
		lightningPlugin,
		simplepolicy)

	if liquidTxWatcher != nil {
		go func() {
			err := liquidTxWatcher.StartWatchingTxs()
			if err != nil {
				log.Printf("%v", err)
				os.Exit(1)
			}
		}()
	}
	go func() {
		err := bitcoinTxWatcher.StartWatchingTxs()
		if err != nil {
			log.Printf("%v", err)
			os.Exit(1)
		}
	}()

	err = swapService.Start()
	if err != nil {
		return err
	}
	err = swapService.RecoverSwaps()
	if err != nil {
		return err
	}
	lightningPlugin.SetupClients(liquidRpcWallet, swapService, liquidCli)

	log.Printf("peerswap initialized")
	<-quitChan
	return nil
}

func validateConfig(cfg *peerswap.Config) error {
	if cfg.LiquidRpcUser == "" {
		cfg.LiquidEnabled = false
	} else {
		cfg.LiquidEnabled = true
	}
	var liquidNetwork *network.Network
	if cfg.LiquidNetworkString == "regtest" {
		liquidNetwork = &network.Regtest
	} else if cfg.LiquidNetworkString == "testnet" {
		liquidNetwork = &peerswap.Testnet
	} else {
		liquidNetwork = &network.Liquid
	}

	cfg.LiquidNetwork = liquidNetwork
	return nil
}
func getBitcoinClient(li *glightning.Lightning) (*gbitcoin.Bitcoin, error) {
	configs, err := li.ListConfigs()
	if err != nil {
		return nil, err
	}
	jsonString, err := json.Marshal(configs)
	if err != nil {
		return nil, err
	}
	var listconfigRes *ListConfigRes
	err = json.Unmarshal(jsonString, &listconfigRes)
	if err != nil {
		return nil, err
	}
	var bcliConfig *ImportantPlugin
	for _, v := range listconfigRes.ImportantPlugins {
		if v.Name == "bcli" {
			bcliConfig = v
		}
	}
	if bcliConfig == nil {
		return nil, errors.New("bcli config not found")
	}

	bitcoin := gbitcoin.NewBitcoin(bcliConfig.Options["bitcoin-rpcuser"], bcliConfig.Options["bitcoin-rpcpassword"])
	bitcoin.SetTimeout(10)
	rpcPort, err := strconv.Atoi(bcliConfig.Options["bitcoin-rpcport"])
	if err != nil {
		return nil, err
	}
	bitcoin.StartUp("http://"+bcliConfig.Options["bitcoin-rpcconnect"], "", uint(rpcPort))
	return bitcoin, nil
}

type ListConfigRes struct {
	ImportantPlugins []*ImportantPlugin `json:"important-plugins"`
}

type ImportantPlugin struct {
	Path    string
	Name    string
	Options map[string]string
}
