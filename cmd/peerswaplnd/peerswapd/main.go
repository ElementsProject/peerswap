package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	log2 "log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/version"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	lnd2 "github.com/elementsproject/peerswap/lnd"
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
	"github.com/lightningnetwork/lnd/lnrpc/chainrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/vulpemventures/go-elements/network"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

const (
	minLndVersion = float64(14.1)
)

var GitCommit string

func main() {
	err := run()
	if err != nil {
		log2.Fatal(err)
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
	log.Infof("Starting peerswap commit %s with cfg: %s", GitCommit, cfg)

	// setup lnd connection
	lndConn, err := getLndClientConnection(ctx, cfg)
	if err != nil {
		return err
	}
	defer lndConn.Close()
	lnrpcClient := lnrpc.NewLightningClient(lndConn)

	timeOutCtx, timeoutCancel := context.WithTimeout(ctx, time.Minute)
	defer timeoutCancel()

	getInfo, err := lnrpcClient.GetInfo(timeOutCtx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return err
	}

	log.Infof("Running with lnd node: %s", getInfo.IdentityPubkey)

	err = checkLndVersion(getInfo.Version)
	if err != nil {
		return err
	}

	var supportedAssets = []string{}

	var bitcoinOnChainService *onchain.BitcoinOnChain
	var lndTxWatcher *lnd2.LndTxWatcher
	// setup bitcoin stuff
	if cfg.BitcoinEnabled {
		// bitcoin
		chain, err := getBitcoinChain(ctx, lnrpcClient)
		if err != nil {
			return err
		}

		supportedAssets = append(supportedAssets, "btc")
		lndFeeEstimator := lnd2.NewLndFeeEstimator(ctx, walletrpc.NewWalletKitClient(lndConn))

		lndTxWatcher = lnd2.NewLndTxWatcher(ctx, chainrpc.NewChainNotifierClient(lndConn), lnrpcClient, chain)
		bitcoinOnChainService = onchain.NewBitcoinOnChain(lndFeeEstimator, chain)

		log.Infof("Bitcoin swaps enabled on network %s", chain.Name)
	} else {
		log.Infof("Bitcoin swaps disabled")
	}

	// setup liquid stuff
	var liquidOnChainService *onchain.LiquidOnChain
	var liquidTxWatcher *txwatcher.BlockchainRpcTxWatcher
	var liquidRpcWallet *wallet.ElementsRpcWallet
	var liquidCli *gelements.Elements
	if cfg.LiquidEnabled {
		supportedAssets = append(supportedAssets, "lbtc")
		log.Infof("Liquid swaps enabled")
		// blockchaincli
		liquidConfig := cfg.ElementsConfig
		liquidCli = gelements.NewElements(liquidConfig.RpcUser, liquidConfig.RpcPassword)
		err = liquidCli.StartUp(liquidConfig.RpcHost, liquidConfig.RpcPort)
		if err != nil {
			return err
		}
		// Wallet
		liquidWalletCli := gelements.NewElements(liquidConfig.RpcUser, liquidConfig.RpcPassword)
		err = liquidWalletCli.StartUp(liquidConfig.RpcHost, liquidConfig.RpcPort)
		if err != nil {
			return err
		}
		liquidRpcWallet, err = wallet.NewRpcWallet(liquidWalletCli, liquidConfig.RpcWallet)
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
		log.Infof("Liquid swaps disabled")
	}

	if !cfg.BitcoinEnabled && !cfg.LiquidEnabled {
		return errors.New("bad config, either liquid or bitcoin settings must be set")
	}
	// setup lnd
	lnd, err := lnd2.NewLnd(ctx, cfg.LndConfig.TlsCertPath, cfg.LndConfig.MacaroonPath, cfg.LndConfig.LndHost, bitcoinOnChainService)
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
		go func() {
			err := liquidTxWatcher.StartWatchingTxs()
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

	lnd.PollService = pollService
	lnd.StartListening()

	// setup grpc server
	sp := swap.NewRequestedSwapsPrinter(requestedSwapStore)
	peerswaprpcServer := peerswaprpc.NewPeerswapServer(liquidRpcWallet, swapService, sp, pollService, pol, liquidCli, lnrpc.NewLightningClient(lndConn), sigChan)

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
			log2.Fatal(err)
		}
	}()
	defer grpcSrv.Stop()
	log.Infof("peerswapd listening on %v", cfg.Host)
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
	bitcoin := gbitcoin.NewBitcoin(cfg.RpcUser, cfg.RpcPassword)
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

func getLndClientConnection(ctx context.Context, cfg *peerswaplnd.PeerSwapConfig) (*grpc.ClientConn, error) {
	creds, err := credentials.NewClientTLSFromFile(cfg.LndConfig.TlsCertPath, "")
	if err != nil {
		return nil, err
	}
	macBytes, err := ioutil.ReadFile(cfg.LndConfig.MacaroonPath)
	if err != nil {
		return nil, err
	}
	mac := &macaroon.Macaroon{}
	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, err
	}
	cred, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, err
	}
	maxMsgRecvSize := grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 500)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(cred),
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
	}
	conn, err := grpc.DialContext(ctx, cfg.LndConfig.LndHost, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
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
	log2.SetOutput(w)

	return &LndLogger{loglevel: cfg.LogLevel}, logFile.Close, nil
}

func (l *LndLogger) Infof(format string, v ...interface{}) {
	log2.Printf("[INFO] "+format, v...)
}

func (l *LndLogger) Debugf(format string, v ...interface{}) {
	if l.loglevel == peerswaplnd.LOGLEVEL_DEBUG {
		log2.Printf("[DEBUG] "+format, v...)
	}
}
