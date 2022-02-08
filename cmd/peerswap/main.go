package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/vulpemventures/go-elements/network"

	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/glightning/glightning"
	"github.com/sputn1ck/peerswap/clightning"
	"github.com/sputn1ck/peerswap/messages"
	"github.com/sputn1ck/peerswap/onchain"
	"github.com/sputn1ck/peerswap/policy"
	"github.com/sputn1ck/peerswap/poll"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/txwatcher"
	"github.com/sputn1ck/peerswap/wallet"
	"go.etcd.io/bbolt"
)

var supportedAssets = []string{}

const (
	minClnVersion = float64(10.2)
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
	lightningPlugin, initChan, err := clightning.NewClightningClient(ctx)
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
	log.Printf("PeerswapClightningConfig: Db:%s, Rpc: %s %s,", config.DbPath, config.LiquidRpcHost, config.LiquidRpcUser)
	// setup services
	nodeInfo, err := lightningPlugin.GetLightningRpc().GetInfo()
	if err != nil {
		return err
	}
	err = checkClnVersion(nodeInfo.Network, nodeInfo.Version)
	if err != nil {
		return err
	}
	// liquid
	var liquidOnChainService *onchain.LiquidOnChain
	var liquidTxWatcher *txwatcher.BlockchainRpcTxWatcher
	var liquidRpcWallet *wallet.ElementsRpcWallet
	var liquidCli *gelements.Elements
	if config.LiquidEnabled {
		supportedAssets = append(supportedAssets, "l-btc")
		log.Printf("Liquid swaps enabled")
		// blockchaincli
		liquidCli = gelements.NewElements(config.LiquidRpcUser, config.LiquidRpcPassword)
		liquidCli.SetTimeout(120)
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
		liquidTxWatcher = txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewElementsCli(liquidCli), onchain.LiquidConfs, onchain.LiquidCsv)

		// LiquidChain
		liquidChain, err := getLiquidChain(liquidCli)
		if err != nil {
			return err
		}
		liquidOnChainService = onchain.NewLiquidOnChain(liquidCli, liquidRpcWallet, liquidChain)
	} else {
		log.Printf("Liquid swaps disabled")
	}

	// bitcoin
	chain, err := getBitcoinChain(lightningPlugin.GetLightningRpc())
	if err != nil {
		return err
	}
	bitcoinCli, err := getBitcoinClient(lightningPlugin.GetLightningRpc(), config)
	if err != nil {
		return err
	}
	var bitcoinTxWatcher *txwatcher.BlockchainRpcTxWatcher
	var bitcoinOnChainService *onchain.BitcoinOnChain
	var bitcoinEnabled bool
	if bitcoinCli != nil {
		supportedAssets = append(supportedAssets, "btc")
		log.Printf("Bitcoin swaps enabled")
		bitcoinEnabled = true
		bitcoinTxWatcher = txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewBitcoinRpc(bitcoinCli), onchain.BitcoinMinConfs, onchain.BitcoinCsv)
		bitcoinOnChainService = onchain.NewBitcoinOnChain(lightningPlugin, chain)
	} else {
		log.Printf("Bitcoin swaps disabled")
	}

	if !bitcoinEnabled && !config.LiquidEnabled {
		return errors.New("bad config, either liquid or bitcoin settings must be set")
	}

	// db
	swapDb, err := bbolt.Open(filepath.Join(config.DbPath, "swaps"), 0700, nil)
	if err != nil {
		return err
	}

	// policy
	pol, err := policy.CreateFromFile(config.PolicyPath)
	if err != nil {
		return err
	}
	log.Printf("using policy:\n%s", pol)

	// Swap store.
	swapStore, err := swap.NewBboltStore(swapDb)
	if err != nil {
		return err
	}

	requestedSwapStore, err := swap.NewRequestedSwapsStore(swapDb)
	if err != nil {
		return err
	}

	// Manager for send message retry.
	mesmgr := messages.NewManager()

	swapServices := swap.NewSwapServices(swapStore,
		requestedSwapStore,
		lightningPlugin,
		lightningPlugin,
		mesmgr,
		pol,
		bitcoinEnabled,
		lightningPlugin,
		bitcoinOnChainService,
		bitcoinTxWatcher,
		config.LiquidEnabled,
		liquidOnChainService,
		liquidOnChainService,
		liquidTxWatcher,
	)
	swapService := swap.NewSwapService(swapServices)

	if liquidTxWatcher != nil {
		go func() {
			err := liquidTxWatcher.StartWatchingTxs()
			if err != nil {
				log.Printf("%v", err)
				os.Exit(1)
			}
		}()
	}

	if bitcoinTxWatcher != nil {
		go func() {
			err := bitcoinTxWatcher.StartWatchingTxs()
			if err != nil {
				log.Printf("%v", err)
				os.Exit(1)
			}
		}()
	}

	err = swapService.Start()
	if err != nil {
		return err
	}

	pollStore, err := poll.NewStore(swapDb)
	if err != nil {
		return err
	}
	pollService := poll.NewService(1*time.Hour, 2*time.Hour, pollStore, lightningPlugin, pol, lightningPlugin, supportedAssets)
	pollService.Start()
	defer pollService.Stop()

	sp := swap.NewRequestedSwapsPrinter(requestedSwapStore)
	lightningPlugin.SetupClients(liquidRpcWallet, swapService, pol, sp, liquidCli, bitcoinCli, bitcoinOnChainService, pollService)
	err = swapService.RecoverSwaps()
	if err != nil {
		return err
	}

	log.Printf("peerswap initialized")
	<-quitChan
	return nil
}

func validateConfig(cfg *clightning.PeerswapClightningConfig) error {
	if cfg.LiquidRpcUser == "" {
		cfg.LiquidEnabled = false
	} else {
		cfg.LiquidEnabled = true
	}
	if cfg.LiquidEnabled {
		if cfg.LiquidRpcPasswordFile != "" {
			passBytes, err := ioutil.ReadFile(cfg.LiquidRpcPasswordFile)
			if err != nil {
				log.Printf("error reading file: %v", err)
				return err
			}
			passString := strings.TrimRight(string(passBytes), "\r\n")
			cfg.LiquidRpcPassword = passString
		}

	}

	return nil
}

func getLiquidChain(li *gelements.Elements) (*network.Network, error) {
	bi, err := li.GetChainInfo()
	if err != nil {
		return nil, err
	}
	switch bi.Chain {
	case "liquidv1":
		return &network.Liquid, nil
	case "liquidregtest":
		return &network.Regtest, nil
	case "liquidtestnet":
		return &network.Testnet, nil
	default:
		return &network.Testnet, nil
	}
}
func getBitcoinChain(li *glightning.Lightning) (*chaincfg.Params, error) {
	gi, err := li.GetInfo()
	if err != nil {
		return nil, err
	}
	switch gi.Network {
	case "regtest":
		return &chaincfg.RegressionNetParams, nil
	case "testnet":
		return &chaincfg.TestNet3Params, nil
	case "signet":
		return &chaincfg.SigNetParams, nil
	case "bitcoin":
		return &chaincfg.MainNetParams, nil
	default:
		return nil, errors.New("unknown bitcoin network")
	}
}
func getBitcoinClient(li *glightning.Lightning, pluginConfig *clightning.PeerswapClightningConfig) (*gbitcoin.Bitcoin, error) {
	configs, err := li.ListConfigs()
	if err != nil {
		return nil, err
	}
	gi, err := li.GetInfo()
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
	var rpcHost, rpcUser, rpcPassword string
	var rpcPort int
	if pluginConfig.BitcoinRpcUser != "" || pluginConfig.BitcoinCookieFilePath != "" {
		if pluginConfig.BitcoinCookieFilePath != "" {
			log.Printf("looking for bitcoin cookie")
			// look for cookie file
			cookiePath := filepath.Join(pluginConfig.BitcoinCookieFilePath)
			if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
				log.Printf("cannot find bitcoin cookie file at %s", cookiePath)
				return nil, nil
			}
			cookieBytes, err := os.ReadFile(cookiePath)
			if err != nil {
				return nil, err
			}

			cookie := strings.Split(string(cookieBytes), ":")

			if pluginConfig.BitcoinRpcHost == "" || pluginConfig.BitcoinRpcPort == 0 {
				return nil, errors.New("if peerswap-bitcoin-cookiefilepath is set, peerswap-bitcoin-rpchost and peerswap-bitcoin-rpcport must be set as well")
			}
			rpcHost = pluginConfig.BitcoinRpcHost
			rpcPort = int(pluginConfig.BitcoinRpcPort)

			rpcUser = cookie[0]
			rpcPassword = cookie[1]
		} else {
			rpcUser = pluginConfig.BitcoinRpcUser
			if pluginConfig.BitcoinRpcPassword == "" || pluginConfig.BitcoinRpcHost == "" || pluginConfig.BitcoinRpcPort == 0 {
				return nil, errors.New("if peerswap-bitcoin-rpcuser is set, peerswap-bitcoin-rpcpassword peerswap-bitcoin-rpchost and peerswap-bitcoin-rpcport must be set as well")
			}
			rpcPassword = pluginConfig.BitcoinRpcPassword
			rpcPort = int(pluginConfig.BitcoinRpcPort)
			rpcHost = pluginConfig.BitcoinRpcHost
		}
	} else {
		var bcliConfig *ImportantPlugin
		for _, v := range listconfigRes.ImportantPlugins {
			if v.Name == "bcli" {
				bcliConfig = v
			}
		}
		if bcliConfig == nil {
			return nil, errors.New("bcli config not found")
		}
		bclirpcUser, ok := bcliConfig.Options["bitcoin-rpcuser"]
		if bclirpcUser == nil || !ok {
			log.Printf("looking for bitcoin cookie")
			// look for cookie file
			bitcoinDir, ok := bcliConfig.Options["bitcoin-datadir"].(string)
			if !ok {
				log.Printf("no `bitcoin-datadir` config set")
				return nil, nil
			}

			cookiePath := filepath.Join(bitcoinDir, getNetworkFolder(gi.Network), ".cookie")
			if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
				log.Printf("cannot find bitcoin cookie file at %s", cookiePath)
				return nil, nil
			}
			cookieBytes, err := os.ReadFile(cookiePath)
			if err != nil {
				return nil, err
			}

			cookie := strings.Split(string(cookieBytes), ":")

			rpcUser = cookie[0]
			rpcPassword = cookie[1]
			// assume localhost and standard network ports
			rpcHost = "http://127.0.0.1"
			rpcPort = int(getNetworkPort(gi.Network))
		} else {

			rpcUser = bclirpcUser.(string)
			// assume auth authentication
			rpcPassBcli, ok := bcliConfig.Options["bitcoin-rpcpassword"]
			if !ok {
				log.Printf("`bitcoin-rpcpassword` not set in lightning config")
				return nil, nil
			}

			rpcPassword = rpcPassBcli.(string)
			rpcPortStr, ok := bcliConfig.Options["bitcoin-rpcport"]
			if !ok {
				log.Printf("`bitcoin-rpcport` not set in lightning config")
				return nil, nil
			}

			rpcPort, err = strconv.Atoi(rpcPortStr.(string))
			if err != nil {
				return nil, err
			}

			rpcConn, ok := bcliConfig.Options["bitcoin-rpcconnect"]
			var rpcConnStr string
			/* We default to localhost */
			if rpcConn == nil {
				rpcConnStr = "localhost"
			} else {
				rpcConnStr = rpcConn.(string)
			}
			rpcHost = "http://" + rpcConnStr

		}
	}

	log.Printf("connecting with %s, %s to %s:%v", rpcUser, rpcPassword, rpcHost, rpcPort)
	bitcoin := gbitcoin.NewBitcoin(rpcUser, rpcPassword)
	bitcoin.SetTimeout(10)
	err = bitcoin.StartUp(rpcHost, "", uint(rpcPort))
	if err != nil {
		return nil, err
	}
	return bitcoin, nil
}

func getNetworkFolder(network string) string {
	switch network {
	case "regtest":
		return "regtest"
	case "testnet":
		return "testnet3"
	case "signet":
		return "signet"
	default:
		return ""
	}
}

func getNetworkPort(network string) uint {
	switch network {
	case "regtest":
		return 18332
	case "testnet":
		return 18332
	case "signet":
		return 38332
	default:
		return 8332
	}
}

type ListConfigRes struct {
	ImportantPlugins []*ImportantPlugin `json:"important-plugins"`
}

type ImportantPlugin struct {
	Path    string
	Name    string
	Options map[string]interface{}
}

func checkClnVersion(network string, fullVersionString string) error {
	// skip version check if running signet as it needs a custom build
	if network == "signet" {
		return nil
	}
	versionString := fullVersionString[3:7]
	versionFloat, err := strconv.ParseFloat(versionString, 64)
	if err != nil {
		return err
	}
	if versionFloat < minClnVersion {
		return errors.New(fmt.Sprintf("clightning version unsupported, requires %v", minClnVersion))
	}
	return nil
}
