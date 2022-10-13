package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	log2 "log"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/version"

	"github.com/vulpemventures/go-elements/network"

	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/elementsproject/glightning/gbitcoin"
	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/glightning/glightning"
	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/messages"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/poll"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/txwatcher"
	"github.com/elementsproject/peerswap/wallet"
	"go.etcd.io/bbolt"
)

var supportedAssets = []string{}

var GitCommit string

const (
	minClnVersion = float64(10.2)
)

func main() {
	// In order to receive panics, we write to stderr to a file
	closeFileFunc, err := setPanicLogger()
	if err != nil {
		log.Infof("Error setting panic log file: %s", err)
		os.Exit(1)
	}
	defer closeFileFunc()

	if err := run(); err != nil {
		log.Infof("plugin quitting, error: %s", err)
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
			log2.Fatal(err)
		}
		quitChan <- true
	}()
	<-initChan
	log.SetLogger(clightning.NewGlightninglogger(lightningPlugin.Plugin))
	log.Infof("PeerSwap CLN starting up with commit %s", GitCommit)
	config, err := lightningPlugin.GetConfig()
	if err != nil {
		return err
	}
	err = validateConfig(config)
	if err != nil {
		return err
	}
	// setup services
	nodeInfo, err := lightningPlugin.GetLightningRpc().GetInfo()
	if err != nil {
		return err
	}
	err = checkClnVersion(nodeInfo.Network, nodeInfo.Version)
	if err != nil {
		return err
	}

	// We want to make sure that cln is synced and ready to use before we
	// continue to start services.
	log.Infof("Waiting for cln to be synced...")
	err = waitForClnSynced(lightningPlugin.GetLightningRpc(), 10*time.Second)
	if err != nil {
		return err
	}
	log.Infof("Cln synced, continue...")

	// liquid
	var liquidOnChainService *onchain.LiquidOnChain
	var liquidTxWatcher *txwatcher.BlockchainRpcTxWatcher
	var liquidRpcWallet *wallet.ElementsRpcWallet
	var liquidCli *gelements.Elements
	var liquidEnabled bool

	if config.LiquidEnabled {
		liquidOnChainService, liquidTxWatcher, liquidRpcWallet, liquidCli, err = setupLiquid(ctx, lightningPlugin.GetLightningRpc(), config)
		if err != nil && liquidWanted(config) {
			return err
		}
		if err != nil {
			log.Infof("Error setting up liquid %v", err)
		}
		if err == nil {
			liquidEnabled = true
		}
	}

	if liquidEnabled {
		log.Infof("Liquid swaps enabled")
	} else {
		log.Infof("Liquid swaps disabled")
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
		log.Infof("Bitcoin swaps enabled")
		bitcoinEnabled = true
		bitcoinTxWatcher = txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewBitcoinRpc(bitcoinCli), onchain.BitcoinMinConfs, onchain.BitcoinCsv)

		// We set the default Estimator to the static regtest estimator.
		var bitcoinEstimator onchain.Estimator
		bitcoinEstimator, _ = onchain.NewRegtestFeeEstimator()

		// If we use a network different than regtest we override the Estimator
		// with the useful GBitcoindEstimator.
		if chain.Name != "regtest" {
			log.Infof("Using gbitcoind estimator")

			// Initiate the GBitcoinEstimator with the "ECONOMICAL" estimation
			// rule and a fallback fee rate of 6250 sat/kw which converts to
			// 25 sat/vbyte as this is the hardcoded fallback fee that lnd uses.
			// See https://github.com/lightningnetwork/lnd/blob/5c36d96c9cbe8b27c29f9682dcbdab7928ae870f/chainreg/chainregistry.go#L481
			fallbackFeeRateSatPerKw := btcutil.Amount(6250)
			bitcoinEstimator, err = onchain.NewGBitcoindEstimator(
				bitcoinCli,
				"ECONOMICAL",
				fallbackFeeRateSatPerKw,
			)
			if err != nil {
				return err
			}
		}

		if err = bitcoinEstimator.Start(); err != nil {
			return err
		}

		// Create the bitcoin onchain service with a fallback fee rate of
		// 253 sat/kw. (This should be useless in this case).
		// TODO: This fee rate does not matter right now but we might want to
		// add a config flag to set this higher than the assumed floor fee rate
		// of 275 sat/kw (1.1 sat/vb).
		bitcoinOnChainService = onchain.NewBitcoinOnChain(
			bitcoinEstimator,
			btcutil.Amount(253),
			chain,
		)
	} else {
		log.Infof("Bitcoin swaps disabled")
	}

	if !bitcoinEnabled && !liquidEnabled {
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
	log.Infof("using policy:\n%s", pol)

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
		liquidEnabled,
		liquidOnChainService,
		liquidOnChainService,
		liquidTxWatcher,
	)
	swapService := swap.NewSwapService(swapServices)

	if liquidTxWatcher != nil && liquidEnabled {
		go func() {
			err := liquidTxWatcher.StartWatchingTxs()
			if err != nil {
				log.Infof("%v", err)
				os.Exit(1)
			}
		}()
	}

	if bitcoinTxWatcher != nil {
		go func() {
			err := bitcoinTxWatcher.StartWatchingTxs()
			if err != nil {
				log.Infof("%v", err)
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

	// We are ready to accept and handle requests.
	// FIXME: Once we reworked the recovery service (non-blocking) we want to
	// set ready after the recovery to avoid race conditions.
	lightningPlugin.SetReady()

	// Try to upgrade version if needed
	versionService, err := version.NewVersionService(swapDb)
	if err != nil {
		return err
	}
	err = versionService.SafeUpgrade(swapService)
	if err != nil {
		return err
	}

	// Check for active swaps and compare with version
	err = swapService.RecoverSwaps()
	if err != nil {
		return err
	}

	log.Infof("peerswap initialized")
	<-quitChan
	return nil
}

func setupLiquid(ctx context.Context, li *glightning.Lightning,
	config *clightning.PeerswapClightningConfig) (*onchain.LiquidOnChain, *txwatcher.BlockchainRpcTxWatcher, *wallet.ElementsRpcWallet, *gelements.Elements, error) {
	var err error
	supportedAssets = append(supportedAssets, "lbtc")

	// blockchaincli
	liquidCli, err := getElementsClient(li, config)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Wallet
	liquidWalletCli, err := getElementsClient(li, config)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	liquidRpcWallet, err := wallet.NewRpcWallet(liquidWalletCli, config.LiquidRpcWallet)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// txwatcher
	liquidTxWatcher := txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewElementsCli(liquidCli), onchain.LiquidConfs, onchain.LiquidCsv)

	// LiquidChain
	liquidChain, err := getLiquidChain(liquidCli)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	liquidOnChainService := onchain.NewLiquidOnChain(liquidCli, liquidRpcWallet, liquidChain)
	return liquidOnChainService, liquidTxWatcher, liquidRpcWallet, liquidCli, nil
}

func liquidWanted(cfg *clightning.PeerswapClightningConfig) bool {
	return !(cfg.LiquidRpcUser == "" && cfg.LiquidRpcPasswordFile == "")
}

func validateConfig(cfg *clightning.PeerswapClightningConfig) error {
	if cfg.LiquidEnabled {
		if cfg.LiquidRpcPasswordFile != "" {
			passBytes, err := ioutil.ReadFile(cfg.LiquidRpcPasswordFile)
			if err != nil {
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

func getLiquidFolderNameForBitcoinChain(btcChain *chaincfg.Params) (string, error) {
	switch btcChain.Name {
	case "mainnet":
		return "liquidv1", nil
	case "testnet3":
	case "simnet":
	case "signet":
		return "liquidtestnet", nil
	case "regtest":
		return "liquidregtest", nil
	default:
		return "", errors.New("unknown bitcoin network")

	}
	return "", errors.New("unknown bitcoin network")
}
func getElementsClient(li *glightning.Lightning, pluginConfig *clightning.PeerswapClightningConfig) (*gelements.Elements, error) {
	var elementsCli *gelements.Elements
	var rpcUser, rpcPass string

	// get bitcoin chain
	bitcoinChain, err := getBitcoinChain(li)
	if err != nil {
		return nil, err
	}

	// if no user and pass is specified try to find the cookie file
	if pluginConfig.LiquidRpcUser == "" && pluginConfig.LiquidRpcPassword == "" {
		// get liquid Chain
		liquidChain, err := getLiquidFolderNameForBitcoinChain(bitcoinChain)
		if err != nil {
			return nil, err
		}
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		cookiePath := filepath.Join(homeDir, ".elements", liquidChain, ".cookie")
		if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
			log.Infof("cannot find liquid cookie file at %s", cookiePath)
			return nil, err
		}
		cookieBytes, err := os.ReadFile(cookiePath)
		if err != nil {
			return nil, err
		}

		cookie := strings.Split(string(cookieBytes), ":")

		rpcUser = cookie[0]
		rpcPass = cookie[1]

	} else if pluginConfig.LiquidRpcUser != "" && pluginConfig.LiquidRpcPassword != "" {
		rpcUser = pluginConfig.LiquidRpcUser
		rpcPass = pluginConfig.LiquidRpcPassword
	} else {
		// incorrect config
		return nil, errors.New("Either both liquid-rpcuser and liquid-rpcpassword must be set, or none")
	}

	elementsCli = gelements.NewElements(rpcUser, rpcPass)

	err = elementsCli.StartUp(pluginConfig.LiquidRpcHost, pluginConfig.LiquidRpcPort)
	if err != nil {
		return nil, err
	}

	return elementsCli, nil
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

	if pluginConfig.BitcoinRpcHost == "" {
		rpcHost = "http://127.0.0.1"
	}
	if pluginConfig.BitcoinRpcPort == 0 {
		rpcPort = int(getNetworkPort(gi.Network))
	}

	if pluginConfig.BitcoinRpcUser != "" || pluginConfig.BitcoinCookieFilePath != "" {
		if pluginConfig.BitcoinCookieFilePath != "" {
			// look for cookie file
			cookiePath := filepath.Join(pluginConfig.BitcoinCookieFilePath)
			if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
				log.Infof("cannot find bitcoin cookie file at %s", cookiePath)
				return nil, nil
			}
			cookieBytes, err := os.ReadFile(cookiePath)
			if err != nil {
				return nil, err
			}

			cookie := strings.Split(string(cookieBytes), ":")

			rpcUser = cookie[0]
			rpcPassword = cookie[1]
		} else {
			rpcUser = pluginConfig.BitcoinRpcUser
			if pluginConfig.BitcoinRpcPassword == "" {
				return nil, errors.New("if peerswap-bitcoin-rpcuser is set, peerswap-bitcoin-rpcpassword must be set as well")
			}
			rpcPassword = pluginConfig.BitcoinRpcPassword
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

		if rpcPortStr, ok := bcliConfig.Options["bitcoin-rpcport"]; ok && rpcPortStr != nil {
			rpcPort, err = strconv.Atoi(rpcPortStr.(string))
			if err != nil {
				return nil, err
			}
		}

		if rpcConn, ok := bcliConfig.Options["bitcoin-rpcconnect"]; ok {
			var rpcConnStr string
			if rpcConn == nil {
				rpcConnStr = "127.0.0.1"
			} else {
				rpcConnStr = rpcConn.(string)
			}

			rpcHost = "http://" + rpcConnStr
		}

		bclirpcUser, ok := bcliConfig.Options["bitcoin-rpcuser"]
		if bclirpcUser == nil || !ok {
			// look for cookie file
			bitcoinDir, ok := bcliConfig.Options["bitcoin-datadir"].(string)
			if !ok {
				// if no bitcoin dir is set, default to $HOME/.bitcoin
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				bitcoinDir = filepath.Join(homeDir, ".bitcoin")
			}

			cookiePath := filepath.Join(bitcoinDir, getNetworkFolder(gi.Network), ".cookie")
			if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
				log.Infof("cannot find bitcoin cookie file at %s", cookiePath)
				return nil, nil
			}
			cookieBytes, err := os.ReadFile(cookiePath)
			if err != nil {
				return nil, err
			}

			cookie := strings.Split(string(cookieBytes), ":")

			rpcUser = cookie[0]
			rpcPassword = cookie[1]

		} else {
			rpcUser = bclirpcUser.(string)
			// assume auth authentication
			rpcPassBcli, ok := bcliConfig.Options["bitcoin-rpcpassword"]
			if !ok || rpcPassBcli == nil {
				log.Infof("`bitcoin-rpcpassword` not set in lightning config")
				return nil, nil
			}

			rpcPassword = rpcPassBcli.(string)

		}
	}

	log.Debugf("connecting with %s, %s to %s:%v", rpcUser, rpcPassword, rpcHost, rpcPort)
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
		return 18443
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

// setPanicLogger duplicates calls to Stderr to a file in the lightning peerswap directory
func setPanicLogger() (func() error, error) {

	// Get working directory ("default is ~/.lightning/<network>")
	wd, err := os.Getwd()
	if err != nil {
		log.Infof("Cannot get working directory, error: %s", err)
		os.Exit(1)
	}

	newpath := filepath.Join(wd, "peerswap")

	err = os.MkdirAll(newpath, os.ModePerm)
	if err != nil {
		return nil, err
	}

	panicLogFile, err := os.OpenFile(filepath.Join(wd, "peerswap/peerswap-panic-log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	_, err = panicLogFile.WriteString("\n\nServer started " + time.Now().UTC().Format(time.RFC3339) + "\n")
	if err != nil {
		return nil, err
	}
	err = panicLogFile.Sync()
	if err != nil {
		return nil, err
	}

	err = syscall.Dup3(int(panicLogFile.Fd()), int(os.Stderr.Fd()), 0)
	if err != nil {
		return nil, err
	}

	return panicLogFile.Close, nil
}

// waitForClnSynced waits until cln is synced to the blockchain and the network.
// This call is blocking.
func waitForClnSynced(cln *glightning.Lightning, tick time.Duration) error {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			info, err := cln.GetInfo()
			if err != nil {
				return err
			}
			if info.IsBitcoindSync() && info.IsLightningdSync() {
				return nil
			}
		}
	}
}
