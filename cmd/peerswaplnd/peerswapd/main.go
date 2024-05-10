package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	core_log "log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/checksum0/go-electrum/electrum"
	"github.com/elementsproject/peerswap/elements"
	"github.com/elementsproject/peerswap/isdev"
	"github.com/elementsproject/peerswap/lnd"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/lwk"

	"github.com/elementsproject/peerswap/version"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/elementsproject/glightning/gbitcoin"
	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	lnd_internal "github.com/elementsproject/peerswap/lnd"
	"github.com/elementsproject/peerswap/messages"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/poll"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/txwatcher"
	"github.com/elementsproject/peerswap/wallet"
	"github.com/jessevdk/go-flags"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/vulpemventures/go-elements/network"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	minLndVersion = float64(14.1)
)

var GitCommit string

func main() {
	err := run()
	if err != nil {
		core_log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shutdown := make(chan struct{})
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		defer close(shutdown)
		defer close(sigChan)

		select {
		case sig := <-sigChan:
			log.Infof("received signal: %v, release shutdown", sig)
			cancel()
			shutdown <- struct{}{}
		}
	}()

	// load config
	cfg, err := loadConfig()
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		return nil
	}
	if err != nil {
		return err
	}
	err = cfg.Validate()
	if err != nil {
		return err
	}

	logger, closeFunc, err := NewLndLogger(cfg)
	if err != nil {
		return err
	}
	defer closeFunc()
	log.SetLogger(logger)

	// make datadir
	err = os.MkdirAll(cfg.DataDir, 0755)
	if err != nil {
		return err
	}
	log.Infof("PeerSwap LND starting up with commit %s and cfg: %s", GitCommit, cfg)
	log.Infof("DB version: %s, Protocol version: %d", version.GetCurrentVersion(), swap.PEERSWAP_PROTOCOL_VERSION)
	if isdev.IsDev() {
		log.Infof("Dev-mode enabled.")
	}

	// setup lnd connection
	cc, err := lnd.GetClientConnection(ctx, cfg.LndConfig)
	if err != nil {
		return err
	}
	defer cc.Close()

	lnrpcClient := lnrpc.NewLightningClient(cc)

	info, err := lnrpcClient.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
	if err != nil {
		return err
	}

	log.Infof("Running with lnd node: %s", info.IdentityPubkey)
	err = checkLndVersion(info.Version)
	if err != nil {
		return err
	}

	// We want to make sure that lnd is synced and ready to use before we
	// continue to start services.
	log.Infof("Waiting for lnd to be synced...")
	err = waitForLndSynced(lnrpcClient, 10*time.Second)
	if err != nil {
		return err
	}
	log.Infof("Lnd synced, continue...")

	var supportedAssets = []string{}

	var bitcoinOnChainService *onchain.BitcoinOnChain
	var lndTxWatcher *lnd_internal.TxWatcher
	// setup bitcoin stuff
	if cfg.BitcoinEnabled {
		// bitcoin
		chain, err := getBitcoinChain(ctx, lnrpcClient)
		if err != nil {
			return err
		}

		supportedAssets = append(supportedAssets, "btc")
		lndTxWatcher, err = lnd_internal.NewTxWatcher(
			ctx,
			cc,
			chain,
			onchain.BitcoinMinConfs,
			onchain.BitcoinCsv,
		)
		if err != nil {
			return err
		}

		// Start the LndEstimator.
		lndEstimator, err := onchain.NewLndEstimator(
			walletrpc.NewWalletKitClient(cc),
			btcutil.Amount(253),
			10*time.Minute,
		)
		if err != nil {
			return err
		}
		if err = lndEstimator.Start(); err != nil {
			return err
		}

		// Create the bitcoin onchain service with a fallback fee rate of
		// 253 sat/kw.
		// TODO: This fee rate does not matter right now but we might want to
		// add a config flag to set this higher than the assumed floor fee rate
		// of 275 sat/kw (1.1 sat/vb).
		bitcoinOnChainService = onchain.NewBitcoinOnChain(
			lndEstimator,
			btcutil.Amount(253),
			chain,
		)
		log.Infof("Bitcoin swaps enabled on network %s", chain.Name)
	} else {
		log.Infof("Bitcoin swaps disabled")
	}

	// setup liquid stuff
	var liquidOnChainService *onchain.LiquidOnChain
	var liquidTxWatcher swap.TxWatcher
	var liquidRpcWallet wallet.Wallet
	var liquidCli *gelements.Elements
	if cfg.LiquidEnabled {
		if cfg.ElementsConfig.RpcUser != "" {
			supportedAssets = append(supportedAssets, "lbtc")
			log.Infof("Liquid swaps enabled")
			liquidConfig := cfg.ElementsConfig

			// This call is blocking, waiting for elements to come alive and sync.
			liquidCli, err = elements.NewClient(
				liquidConfig.RpcUser,
				liquidConfig.RpcPassword,
				liquidConfig.RpcHost,
				liquidConfig.RpcCookieFilePath,
				liquidConfig.RpcPort,
			)
			if err != nil {
				return err
			}
			liquidRpcWallet, err = wallet.NewRpcWallet(liquidCli, liquidConfig.RpcWallet)
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
			liquidOnChainService = onchain.NewLiquidOnChain(liquidRpcWallet, liquidChain)
		} else if cfg.LWKConfig.Enabled() {
			log.Infof("Liquid swaps enabled with LWK. Network: %s, wallet: %s", cfg.LWKConfig.GetNetwork(), cfg.LWKConfig.GetWalletName())
			ec, err := electrum.NewClientTCP(ctx, cfg.LWKConfig.GetElectrumEndpoint())
			if err != nil {
				return err
			}

			// This call is blocking, waiting for elements to come alive and sync.
			liquidRpcWallet, err = lwk.NewLWKRpcWallet(lwk.NewLwk(cfg.LWKConfig.GetLWKEndpoint()),
				ec, cfg.LWKConfig.GetWalletName(), cfg.LWKConfig.GetSignerName())
			if err != nil {
				return err
			}
			cfg.LiquidEnabled = true
			liquidTxWatcher, err = lwk.NewElectrumTxWatcher(ec)
			if err != nil {
				return err
			}
			liquidOnChainService = onchain.NewLiquidOnChain(liquidRpcWallet, cfg.LWKConfig.GetChain())
			supportedAssets = append(supportedAssets, "lbtc")
		} else {
			return errors.New("Liquid swaps enabled but no config found")
		}
	} else {
		log.Infof("Liquid swaps disabled")
	}

	if !cfg.BitcoinEnabled && !cfg.ElementsConfig.LiquidSwaps {
		log.Infof("Disabling both BTC and L-BTC swaps is invalid. Check PeerSwap and daemon configs. Exiting.")
		os.Exit(1)
	}

	if !cfg.BitcoinEnabled && !cfg.LiquidEnabled {
		log.Infof("Bad configuration or daemons are broken. Exiting.")
		os.Exit(1)
	}
	// Start lnd listeners and watchers.
	messageListener, err := lnd_internal.NewMessageListener(ctx, cc)
	if err != nil {
		return err
	}
	defer messageListener.Stop()

	paymentWatcher, err := lnd_internal.NewPaymentWatcher(ctx, cc)
	if err != nil {
		return err
	}
	defer paymentWatcher.Stop()

	peerListener, err := lnd_internal.NewPeerListener(ctx, cc)
	if err != nil {
		return err
	}
	defer peerListener.Stop()

	// Setup lnd client.
	lnd, err := lnd_internal.NewClient(
		ctx,
		cc,
		paymentWatcher,
		messageListener,
		bitcoinOnChainService,
	)
	if err != nil {
		return err
	}

	// db
	swapDb, err := bbolt.Open(filepath.Join(cfg.DataDir, "swaps"), 0700, nil)
	if err != nil {
		return err
	}

	// policy
	pol, err := policy.CreateFromFile(cfg.PolicyFile)
	if err != nil {
		return err
	}

	// setup swap services
	log.Infof("using policy:\n%s", pol)
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
		lnd,
		lnd,
		mesmgr,
		pol,
		cfg.BitcoinEnabled,
		lnd,
		bitcoinOnChainService,
		lndTxWatcher,
		cfg.LiquidEnabled,
		liquidOnChainService,
		liquidOnChainService,
		liquidTxWatcher,
	)
	swapService := swap.NewSwapService(swapServices)

	if liquidTxWatcher != nil {
		err := liquidTxWatcher.StartWatchingTxs()
		if err != nil {
			log.Infof("%v", err)
			os.Exit(1)
		}
	}

	err = swapService.Start()
	if err != nil {
		return err
	}

	// Try to upgrade version if needed
	versionService, err := version.NewVersionService(swapDb)
	if err != nil {
		return err
	}
	err = versionService.SafeUpgrade(swapService)
	if err != nil {
		return err
	}

	err = swapService.RecoverSwaps()
	if err != nil {
		return err
	}

	pollStore, err := poll.NewStore(swapDb)
	if err != nil {
		return err
	}
	pollService := poll.NewService(1*time.Hour, 2*time.Hour, pollStore, lnd, pol, lnd, supportedAssets)
	pollService.Start()
	defer pollService.Stop()

	// Add poll handler to peer event listener.
	err = peerListener.AddHandler(lnrpc.PeerEvent_PEER_ONLINE, pollService.Poll)
	if err != nil {
		return err
	}

	// Start internal lnd listener.
	lnd.StartListening()

	// setup grpc server
	sp := swap.NewRequestedSwapsPrinter(requestedSwapStore)
	peerswaprpcServer := peerswaprpc.NewPeerswapServer(
		liquidRpcWallet,
		swapService,
		sp,
		pollService,
		pol,
		liquidCli,
		lnrpc.NewLightningClient(cc),
		sigChan,
	)

	lis, err := net.Listen("tcp", cfg.Host)
	if err != nil {
		return err
	}
	defer lis.Close()

	grpcSrv := grpc.NewServer()

	peerswaprpc.RegisterPeerSwapServer(grpcSrv, peerswaprpcServer)

	go func() {
		err := grpcSrv.Serve(lis)
		if err != nil {
			core_log.Fatal(err)
		}
	}()
	defer grpcSrv.Stop()
	log.Infof("peerswapd grpc listening on %v", cfg.Host)
	if cfg.RestHost != "" {
		mux := runtime.NewServeMux(
			runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
				MarshalOptions: protojson.MarshalOptions{
					UseProtoNames:   true,
					EmitUnpopulated: true,
				},
				UnmarshalOptions: protojson.UnmarshalOptions{
					DiscardUnknown: true,
				},
			}),
		)
		opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
		err := peerswaprpc.RegisterPeerSwapHandlerFromEndpoint(ctx, mux, cfg.Host, opts)
		if err != nil {
			return err
		}
		go func() {
			err := http.ListenAndServe(cfg.RestHost, mux)
			if err != nil {
				core_log.Fatal(err)
			}
		}()

		log.Infof("peerswapd rest listening on %v", cfg.RestHost)
	}
	<-shutdown
	return nil
}

func getBitcoinChain(ctx context.Context, li lnrpc.LightningClient) (*chaincfg.Params, error) {
	gi, err := li.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, err
	}
	switch gi.Chains[0].Network {
	case "regtest":
		return &chaincfg.RegressionNetParams, nil
	case "testnet":
		return &chaincfg.TestNet3Params, nil
	case "signet":
		return &chaincfg.SigNetParams, nil
	case "bitcoin":
		return &chaincfg.MainNetParams, nil
	case "mainnet":
		return &chaincfg.MainNetParams, nil
	default:
		return nil, errors.New("unknown bitcoin network")
	}
}

func getBitcoinClient(cfg *peerswaplnd.OnchainConfig) (*gbitcoin.Bitcoin, error) {
	bitcoin := gbitcoin.NewBitcoin(cfg.RpcUser, cfg.RpcPassword, cfg.RpcCookieFilePath)
	err := bitcoin.StartUp(cfg.RpcHost, "", cfg.RpcPort)
	if err != nil {
		return nil, err
	}

	return bitcoin, nil
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

func loadConfig() (*peerswaplnd.PeerSwapConfig, error) {
	cfg := peerswaplnd.DefaultConfig()
	parser := flags.NewParser(cfg, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(cfg.ConfigFile); err == nil {
		fileParser := flags.NewParser(cfg, flags.Default|flags.IgnoreUnknown)
		err = flags.NewIniParser(fileParser).ParseFile(cfg.ConfigFile)
		if err != nil {
			return nil, err
		}
	}

	flagParser := flags.NewParser(cfg, flags.Default)
	if _, err := flagParser.Parse(); err != nil {
		return nil, err
	}

	err = makeDirectories(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	lc, err := peerswaplnd.LWKFromIniFileConfig(cfg.ConfigFile)
	if err != nil {
		return nil, err
	}
	cfg.LWKConfig = lc
	return cfg, nil
}

func makeDirectories(fullDir string) error {
	err := os.MkdirAll(fullDir, 0700)
	if err != nil {
		// Show a nicer error message if it's because a symlink is
		// linked to a directory that does not exist (probably because
		// it's not mounted).
		if e, ok := err.(*os.PathError); ok && os.IsExist(err) {
			if link, lerr := os.Readlink(e.Path); lerr == nil {
				str := "is symlink %s -> %s mounted?"
				err = fmt.Errorf(str, e.Path, link)
			}
		}

		err := fmt.Errorf("failed to create directory %v: %v", fullDir,
			err)
		_, _ = fmt.Fprintln(os.Stderr, err)
		return err
	}

	return nil
}

func checkLndVersion(fullVersionString string) error {
	splitString := strings.Split(fullVersionString, "-")
	// remove first two chars
	versionString := splitString[0][2:]
	versionFloat, err := strconv.ParseFloat(versionString, 64)
	if err != nil {
		return err
	}
	if versionFloat < minLndVersion {
		return errors.New(fmt.Sprintf("Lnd version unsupported, requires %v", minLndVersion))
	}
	return nil
}

type LndLogger struct {
	loglevel peerswaplnd.LogLevel
}

func NewLndLogger(cfg *peerswaplnd.PeerSwapConfig) (*LndLogger, func() error, error) {
	logFile, err := os.OpenFile(filepath.Join(cfg.DataDir, "log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, nil, err
	}
	w := io.MultiWriter(os.Stdout, logFile)
	core_log.SetFlags(core_log.LstdFlags | core_log.LUTC)
	core_log.SetOutput(w)

	return &LndLogger{loglevel: cfg.LogLevel}, logFile.Close, nil
}

func (l *LndLogger) Infof(format string, v ...interface{}) {
	core_log.Printf("[INFO] "+format, v...)
}

func (l *LndLogger) Debugf(format string, v ...interface{}) {
	if l.loglevel == peerswaplnd.LOGLEVEL_DEBUG {
		core_log.Printf("[DEBUG] "+format, v...)
	}
}

// waitForLndSynced waits until cln is synced to the blockchain and the network.
// This call is blocking.
func waitForLndSynced(lnd lnrpc.LightningClient, tick time.Duration) error {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			info, err := lnd.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
			if err != nil {
				return err
			}

			if info.SyncedToChain {
				return nil
			}
		}
	}
}
