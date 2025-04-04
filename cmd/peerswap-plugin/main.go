package main

import (
	"context"
	"errors"
	"fmt"
	glog "log"
	"os"
	"path/filepath"
	"time"

	"github.com/elementsproject/peerswap/elements"
	"github.com/elementsproject/peerswap/isdev"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/lwk"
	"github.com/elementsproject/peerswap/version"
	"golang.org/x/sys/unix"

	"github.com/vulpemventures/go-elements/network"

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
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/txwatcher"
	"github.com/elementsproject/peerswap/wallet"
	"go.etcd.io/bbolt"
)

var supportedAssets = []string{}

var GitCommit string

const (
	minClnVersion = "23.11"
)

func main() {
	mlog := glog.New(os.Stderr, "", glog.LstdFlags|glog.LUTC)

	// In order to receive panics, we write to stderr to a file
	closeFileFunc, err := setPanicLogger()
	if err != nil {
		mlog.Println(err.Error())
		os.Exit(1)
	}
	defer closeFileFunc()

	if err := outer(); err != nil {
		mlog.Println(err.Error())
		os.Exit(1)
	}
}

func outer() error {
	// Main context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize peerswap, check in with core lightning.
	// todo: It seems that creating this initCh channel and using it is a race.
	// todo: Rework this in `NewClightningClient` (also add mutex).
	plugin, initCh, err := clightning.NewClightningClient(ctx)
	if err != nil {
		return err
	}

	err = plugin.RegisterOptions()
	if err != nil {
		return err
	}

	err = plugin.RegisterMethods()
	if err != nil {
		return err
	}

	// Glightning `Start()` is a blocking call. If this returns the server is
	// shutdown -> cancel main runtime context.
	go func() {
		ierr := plugin.Start()
		if ierr != nil {
			ctx = context.WithValue(ctx, "ierr", ierr)
		}
		cancel()
	}()
	// Wait for the plugin to be initialized.
	<-initCh

	// Now we can set the logger to the core lightning log. From here on we can
	// use the log in all inner and the rest of this routine.
	log.SetLogger(clightning.NewGlightninglogger(plugin.Plugin))

	// Start PeerSwap.
	err = run(ctx, plugin)
	if err != nil {
		log.Infof("Exited with error: %s", err.Error())
	}

	// Wait for context to be done and check if the context has collected any
	// errors, pass this error back to the main routine.
	<-ctx.Done()
	if ierr, ok := ctx.Value("ierr").(error); ok {
		return ierr
	}

	return nil
}

func run(ctx context.Context, lightningPlugin *clightning.ClightningClient) error {
	log.Infof("PeerSwap starting up with commit %s", GitCommit)
	log.Infof("DB version: %s, Protocol version: %d", version.GetCurrentVersion(), swap.PEERSWAP_PROTOCOL_VERSION)
	if isdev.IsDev() {
		log.Infof("Dev-mode enabled.")
	}

	// The working dir of the plugin is the default data dir. This should default for cln to (~/.lightning/[network-type])
	dataDir, err := os.Getwd()
	if err != nil {
		return err
	}
	log.Infof("Using data dir: %s", dataDir)

	config, err := clightning.GetConfig(lightningPlugin)
	if err != nil {
		log.Infof("Could not read config: %s", err.Error())
		return err
	}
	log.Debugf("Starting with config: %s", config)

	// Inject the config into the core lightning plugin.
	lightningPlugin.SetPeerswapConfig(config)

	// setup services
	nodeInfo, err := lightningPlugin.GetLightningRpc().GetInfo()
	if err != nil {
		return err
	}
	log.Infof("Using core-lightning version %s", lightningPlugin.Version())
	ok, err := checkClnVersion(nodeInfo.Network, lightningPlugin.Version())
	if err != nil {
		log.Debugf("Could not compare version: %s", err.Error())
		return err
	}
	if !ok {
		log.Infof("Core-lighting version %s is not supported, min version is v%s", nodeInfo.Version, minClnVersion)
		return fmt.Errorf("Core-Lightning version %s is incompatible", nodeInfo.Version)
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
	var liquidTxWatcher swap.TxWatcher
	var liquidRpcWallet wallet.Wallet
	var liquidCli *gelements.Elements
	var liquidEnabled bool

	if *config.Liquid.LiquidSwaps && elementsWanted(config) {
		liquidEnabled = true
		log.Infof("Starting elements client with rpcuser: %s, rpcpassword:******, rpccookie: %s, rpcport: %d, rpchost: %s",
			config.Liquid.RpcUser,
			config.Liquid.RpcPasswordFile,
			config.Liquid.RpcPort,
			config.Liquid.RpcHost,
		)
		// This call is blocking, waiting for elements to come alive and sync.
		liquidCli, err = elements.NewClient(
			config.Liquid.RpcUser,
			config.Liquid.RpcPassword,
			config.Liquid.RpcHost,
			config.Liquid.RpcPasswordFile,
			config.Liquid.RpcPort,
		)
		if err != nil {
			return err
		}

		liquidRpcWallet, err = wallet.NewRpcWallet(liquidCli, config.Liquid.RpcWallet)
		if err != nil {
			return err
		}

		liquidTxWatcher = txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewElementsCli(liquidCli), onchain.LiquidConfs, onchain.LiquidCsv)

		// LiquidChain
		liquidChain, err := getLiquidChain(liquidCli)
		if err != nil {
			return err
		}

		liquidOnChainService = onchain.NewLiquidOnChain(liquidRpcWallet, liquidChain)
		supportedAssets = append(supportedAssets, "lbtc")
		log.Infof("Liquid swaps enabled")
	} else if config.LWK != nil && config.LWK.Enabled() {
		liquidEnabled = true
		lc, err2 := lwk.NewLWKRpcWallet(ctx, config.LWK)
		if err2 != nil {
			return err2
		}
		liquidTxWatcher, err = lwk.NewElectrumTxWatcher(lc.GetElectrumClient())
		if err != nil {
			return err
		}
		liquidRpcWallet = lc
		liquidOnChainService = onchain.NewLiquidOnChain(liquidRpcWallet, config.LWK.GetChain())
		supportedAssets = append(supportedAssets, "lbtc")
		log.Infof("Liquid swaps enabled with LWK. Network: %s, wallet: %s",
			config.LWK.GetNetwork(), config.LWK.GetWalletName())
	} else {
		log.Infof("Liquid swaps disabled")
	}

	// bitcoin
	chain, err := getBitcoinChain(lightningPlugin.GetLightningRpc())
	if err != nil {
		return err
	}
	log.Infof(
		"Starting bitcoin client with chain:%s, rpcuser:%s, rpcpassword:******,, rpchost:%s, rpcport:%d",
		chain.Name,
		config.Bitcoin.RpcUser,
		config.Bitcoin.RpcHost,
		config.Bitcoin.RpcPort,
	)
	bitcoinCli, err := getBitcoinClient(lightningPlugin.GetLightningRpc(), config)
	if err != nil {
		return err
	}

	var bitcoinTxWatcher *txwatcher.BlockchainRpcTxWatcher
	var bitcoinOnChainService *onchain.BitcoinOnChain
	var bitcoinEnabled bool
	if bitcoinCli != nil && *config.Bitcoin.BitcoinSwaps {
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

	if !*config.Bitcoin.BitcoinSwaps && !*config.Liquid.LiquidSwaps {
		return errors.New("Disabling both BTC and L-BTC swaps is invalid.")
	}

	if !bitcoinEnabled && !liquidEnabled {
		return errors.New("Bad configuration or daemons are broken.")
	}

	// db
	swapDb, err := bbolt.Open(filepath.Join(config.DbPath), 0700, nil)
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
	ps, err := premium.NewSetting(swapDb)
	if err != nil {
		return err
	}

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
		ps,
	)
	swapService := swap.NewSwapService(swapServices)

	if liquidTxWatcher != nil && liquidEnabled {
		err := liquidTxWatcher.StartWatchingTxs()
		if err != nil {
			log.Infof("%v", err)
			os.Exit(1)
		}
	}

	if bitcoinTxWatcher != nil {
		err := bitcoinTxWatcher.StartWatchingTxs()
		if err != nil {
			log.Infof("%v", err)
			os.Exit(1)
		}
	}

	err = swapService.Start()
	if err != nil {
		return err
	}

	pollStore, err := poll.NewStore(swapDb)
	if err != nil {
		return err
	}
	pollService := poll.NewService(1*time.Hour, 2*time.Hour, pollStore, lightningPlugin, pol, lightningPlugin, supportedAssets, ps)
	pollService.Start()
	defer pollService.Stop()

	sp := swap.NewRequestedSwapsPrinter(requestedSwapStore)
	lightningPlugin.SetupClients(liquidRpcWallet, swapService, pol, sp, bitcoinCli, bitcoinOnChainService, pollService, ps)

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

	// Wait for context to finish up
	<-ctx.Done()
	return nil
}

func elementsWanted(cfg *clightning.Config) bool {
	return cfg.Liquid.RpcUser != "" && cfg.Liquid.RpcPassword != ""
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
	case "testnet4":
		return &chaincfg.TestNet4Params, nil
	case "signet":
		return &chaincfg.SigNetParams, nil
	case "bitcoin":
		return &chaincfg.MainNetParams, nil
	default:
		return nil, errors.New("unknown bitcoin network")
	}
}

func getBitcoinClient(li *glightning.Lightning, pluginConfig *clightning.Config) (*gbitcoin.Bitcoin, error) {
	rpcUser := pluginConfig.Bitcoin.RpcUser
	rpcPassword := pluginConfig.Bitcoin.RpcPassword
	rpcHost := pluginConfig.Bitcoin.RpcHost
	rpcPort := pluginConfig.Bitcoin.RpcPort
	ppcCookie := pluginConfig.Bitcoin.RpcPasswordFile

	bitcoin := gbitcoin.NewBitcoin(rpcUser, rpcPassword, ppcCookie)
	bitcoin.SetTimeout(10)
	err := bitcoin.StartUp(rpcHost, "", uint(rpcPort))
	if err != nil {
		return nil, err
	}
	return bitcoin, nil
}

func checkClnVersion(network string, fullVersionString string) (bool, error) {
	// skip version check if running signet as it needs a custom build
	// ? Can someone explain why we need this here?
	if network == "signet" {
		return true, nil
	}

	return version.CompareVersionStrings(fullVersionString, minClnVersion)
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

	err = unix.Dup2(int(panicLogFile.Fd()), int(os.Stderr.Fd()))
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
