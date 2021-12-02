package testframework

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/lightningnetwork/lnd/lnrpc"
)

var LND_CONFIG = map[string]string{
	"bitcoin.active":  "true",
	"bitcoin.regtest": "true",
	"bitcoin.node":    "bitcoind",
	"noseedbackup":    "true",
}

type LndNode struct {
	*DaemonProcess
	*LndRpcClient

	DataDir    string
	ConfigFile string
	RpcPort    int
	ListenPort int
	Info       *lnrpc.GetInfoResponse

	bitcoin *BitcoinNode
}

func NewLndNode(testDir string, bitcoin *BitcoinNode, id int) (*LndNode, error) {
	listen, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("getFreePort() %w", err)
	}

	rpcListen, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("getFreePort() %w", err)
	}

	rngDirExtension, err := generateRandomString(5)
	if err != nil {
		return nil, fmt.Errorf("generateRandomString(5) %w", err)
	}

	dataDir := filepath.Join(testDir, fmt.Sprintf("lnd-%s", rngDirExtension), "data")
	err = os.MkdirAll(dataDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("os.MkdirAll() %w", err)
	}

	regtestConfig := LND_CONFIG
	regtestConfig["datadir"] = dataDir
	regtestConfig["listen"] = fmt.Sprintf("localhost:%d", listen)
	regtestConfig["rpclisten"] = fmt.Sprintf("localhost:%d", rpcListen)
	regtestConfig["bitcoind.dir"] = bitcoin.DataDir
	regtestConfig["bitcoind.rpchost"] = fmt.Sprintf("%s:%d", bitcoin.rpcHost, bitcoin.rpcPort)
	regtestConfig["bitcoind.rpcuser"] = bitcoin.RpcUser
	regtestConfig["bitcoind.rpcpass"] = bitcoin.RpcPassword
	regtestConfig["bitcoind.zmqpubrawblock"] = bitcoin.ZmqPubRawBlock
	regtestConfig["bitcoind.zmqpubrawtx"] = bitcoin.ZmqPubRawTx

	configFile := filepath.Join(dataDir, "lnd.conf")
	writeConfig(configFile, regtestConfig, nil, "")

	cmdLine := []string{
		"lnd",
		fmt.Sprintf("--configfile=%s", configFile),
	}

	return &LndNode{
		DaemonProcess: NewDaemonProcess(cmdLine, fmt.Sprintf("lnd-%d", id)),
		LndRpcClient:  nil,
		DataDir:       dataDir,
		ConfigFile:    configFile,
		RpcPort:       rpcListen,
		ListenPort:    listen,
		bitcoin:       bitcoin,
	}, nil
}

func (n *LndNode) Run(waitForReady, waitForBitcoinSynced bool) error {
	n.DaemonProcess.Run()
	if waitForReady {
		err := n.WaitForLog("LightningWallet opened", TIMEOUT)
		if err != nil {
			return fmt.Errorf("CLightningNode.Run() %w", err)
		}
	}

	lndRpcClient, err := NewLndRpcClient(fmt.Sprintf("localhost:%d", n.RpcPort), filepath.Join(n.DataDir, "..", "tls.cert"), filepath.Join(n.DataDir, "chain", "bitcoin", "regtest", "admin.macaroon"))
	if err != nil {
		return fmt.Errorf("NewLndClientProxy() %w", err)
	}
	n.LndRpcClient = lndRpcClient

	// Cache info
	n.Info, err = n.Rpc.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
	if err != nil {
		return fmt.Errorf("GetInfo() %w", err)
	}

	// Wait for sync with bitcoin network
	if waitForBitcoinSynced {
		// Wait for sync with bitcoin network
		return WaitForWithErr(func() (bool, error) {
			info, err := n.Rpc.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
			if err != nil {
				log.Printf("rpc.GetInfo() %v", err)
				return false, fmt.Errorf("rpc.GetInfo() %w", err)
			}

			r, err := n.bitcoin.Rpc.Call("getblockcount")
			if err != nil {
				return false, fmt.Errorf("bitcoin.rpc.Call(\"getblockcount\") %w", err)
			}
			chainHeight, err := r.GetFloat()
			if err != nil {
				return false, fmt.Errorf("GetFloat() %w", err)
			}

			return info.SyncedToChain && info.SyncedToGraph && chainHeight == float64(info.BlockHeight), nil
		}, TIMEOUT)
	}
	return nil
}
