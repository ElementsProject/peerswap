package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/jessevdk/go-flags"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/sputn1ck/glightning/gbitcoin"
	"github.com/sputn1ck/glightning/gelements"
	"github.com/sputn1ck/peerswap/cmd/peerswaplnd"
	lnd2 "github.com/sputn1ck/peerswap/lnd"
	"github.com/sputn1ck/peerswap/onchain"
	"github.com/sputn1ck/peerswap/peerswaprpc"
	"github.com/sputn1ck/peerswap/policy"
	"github.com/sputn1ck/peerswap/poll"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/txwatcher"
	"github.com/sputn1ck/peerswap/wallet"
	"github.com/vulpemventures/go-elements/network"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
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
			log.Printf("received signal: %v, release shutdown", sig)
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

	closeFunc, err := setupLogger(cfg)
	if err != nil {
		return err
	}
	defer closeFunc()
	log.Printf("%s", cfg)

	// setup lnd connection
	lndConn, err := getLndClientConnection(ctx, cfg)
	if err != nil {
		return err
	}
	defer lndConn.Close()
	lnrpcClient := lnrpc.NewLightningClient(lndConn)

	var supportedAssets = []string{}

	var bitcoinTxWatcher *txwatcher.BlockchainRpcTxWatcher
	var bitcoinOnChainService *onchain.BitcoinOnChain

	// setup bitcoin stuff
	if cfg.BitcoinEnabled {
		// bitcoin
		chain, err := getBitcoinChain(ctx, lnrpcClient)
		if err != nil {
			return err
		}
		bitcoinCli, err := getBitcoinClient(cfg.BitcoinConfig)
		if err != nil {
			return err
		}

		supportedAssets = append(supportedAssets, "btc")
		bitcoinTxWatcher = txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewBitcoinRpc(bitcoinCli), 3)
		bitcoinOnChainService = onchain.NewBitcoinOnChain(bitcoinCli, bitcoinTxWatcher, chain)

		log.Printf("Bitcoin swaps enabled")
	} else {
		log.Printf("Bitcoin swaps disabled")
	}

	log.Printf("%v", bitcoinOnChainService.GetChain())
	// setup liquid stuff
	var liquidOnChainService *onchain.LiquidOnChain
	var liquidTxWatcher *txwatcher.BlockchainRpcTxWatcher
	var liquidRpcWallet *wallet.ElementsRpcWallet
	var liquidCli *gelements.Elements
	if cfg.LiquidEnabled {
		supportedAssets = append(supportedAssets, "l-btc")
		log.Printf("Liquid swaps enabled")
		// blockchaincli
		liquidConfig := cfg.LiquidConfig
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
		liquidTxWatcher = txwatcher.NewBlockchainRpcTxWatcher(ctx, txwatcher.NewElementsCli(liquidCli), 2)

		// LiquidChain
		liquidChain, err := getLiquidChain(liquidCli)
		if err != nil {
			return err
		}
		liquidOnChainService = onchain.NewLiquidOnChain(liquidCli, liquidTxWatcher, liquidRpcWallet, liquidChain)
	} else {
		log.Printf("Liquid swaps disabled")
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
	pol, err := policy.CreateFromFile(cfg.ConfigFile)
	if err != nil {
		return err
	}

	// setup swap services
	log.Printf("using policy:\n%s", pol)
	swapStore, err := swap.NewBboltStore(swapDb)
	if err != nil {
		return err
	}
	requestedSwapStore, err := swap.NewRequestedSwapsStore(swapDb)
	if err != nil {
		return err
	}

	swapServices := swap.NewSwapServices(swapStore,
		requestedSwapStore,
		lnd,
		lnd,
		pol,
		cfg.BitcoinEnabled,
		lnd,
		bitcoinOnChainService,
		cfg.LiquidEnabled,
		liquidOnChainService,
		liquidOnChainService,
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
			log.Fatal(err)
		}
	}()
	defer grpcSrv.GracefulStop()

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

func setupLogger(cfg *peerswaplnd.PeerSwapConfig) (func() error, error) {
	logFile, err := os.OpenFile(filepath.Join(cfg.DataDir, "log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	w := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(w)
	return logFile.Close, nil
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
		fileParser := flags.NewParser(cfg, flags.Default)
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
