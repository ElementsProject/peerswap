package testframework

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
)

var LND_CONFIG = map[string]string{
	"bitcoin.active":              "true",
	"bitcoin.regtest":             "true",
	"bitcoin.node":                "bitcoind",
	"bitcoin.defaultchanconfs":    "1",
	"noseedbackup":                "true",
	"norest":                      "true",
	"debuglevel":                  "debug",
	"max-commit-fee-rate-anchors": "1",
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
	listen, err := GetFreePort()
	if err != nil {
		return nil, fmt.Errorf("getFreePort() %w", err)
	}

	rpcListen, err := GetFreePort()
	if err != nil {
		return nil, fmt.Errorf("getFreePort() %w", err)
	}

	rngDirExtension, err := GenerateRandomString(5)
	if err != nil {
		return nil, fmt.Errorf("generateRandomString(5) %w", err)
	}

	lndDir := filepath.Join(testDir, fmt.Sprintf("lnd-%s", rngDirExtension))
	dataDir := filepath.Join(lndDir, "data")
	err = os.MkdirAll(dataDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("os.MkdirAll() %w", err)
	}

	regtestConfig := LND_CONFIG
	regtestConfig["lnddir"] = lndDir
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
	WriteConfig(configFile, regtestConfig, nil, "")

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
	var err error
	n.DaemonProcess.Run()
	if waitForReady {
		err := n.WaitForLog("LightningWallet opened", TIMEOUT)
		if err != nil {
			return fmt.Errorf("CLightningNode.Run() %w", err)
		}

		err = n.WaitForLog(fmt.Sprintf("RPC server listening on 127.0.0.1:%d", n.RpcPort), TIMEOUT)
		if err != nil {
			return fmt.Errorf("CLightningNode.Run() %w", err)
		}
	}

	n.LndRpcClient, err = NewLndRpcClient(fmt.Sprintf("localhost:%d", n.RpcPort), filepath.Join(n.DataDir, "..", "tls.cert"), filepath.Join(n.DataDir, "chain", "bitcoin", "regtest", "admin.macaroon"))
	if err != nil {
		return fmt.Errorf("NewLndClientProxy() %w", err)
	}

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

			return info.SyncedToChain && chainHeight == float64(info.BlockHeight), nil
		}, TIMEOUT)
	}
	return nil
}

func (n *LndNode) Address() string {
	return fmt.Sprintf("%s@127.0.0.1:%d", n.Info.IdentityPubkey, n.ListenPort)
}

func (n *LndNode) Id() (id string) {
	return n.Info.IdentityPubkey
}

func (n *LndNode) GetBtcBalanceSat() (uint64, error) {
	r, err := n.Rpc.WalletBalance(context.Background(), &lnrpc.WalletBalanceRequest{})
	if err != nil {
		return 0, fmt.Errorf("WalletBalance() %w", err)
	}
	return uint64(r.TotalBalance), nil
}

func (n *LndNode) GetScid(peer LightningNode) (scid string, err error) {
	res, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return "", fmt.Errorf("ListChannels() %w", err)
	}

	for _, ch := range res.Channels {
		if ch.RemotePubkey == peer.Id() {
			return scidFromLndChanId(ch.ChanId), nil
		}
	}

	return "", fmt.Errorf("peer not found")
}

func (n *LndNode) Connect(peer LightningNode, waitForConnection bool) error {
	pk, host, port, err := SplitLnAddr(peer.Address())
	if err != nil {
		return fmt.Errorf("SplitLnAddr() %w", err)
	}

	_, err = n.Rpc.ConnectPeer(context.Background(),
		&lnrpc.ConnectPeerRequest{Addr: &lnrpc.LightningAddress{
			Pubkey: pk,
			Host:   fmt.Sprintf("%s:%d", host, port),
		}})
	if err != nil {
		return fmt.Errorf("ConnectPeer() %w", err)
	}

	if waitForConnection {
		if waitForConnection {
			return WaitForWithErr(func() (bool, error) {
				localIsConnected, err := n.IsConnected(peer)
				if err != nil {
					return false, fmt.Errorf("IsConnected() %w", err)
				}
				peerIsConnected, err := peer.IsConnected(n)
				if err != nil {
					return false, fmt.Errorf("IsConnected() %w", err)
				}
				return localIsConnected && peerIsConnected, nil
			}, TIMEOUT)
		}
	}

	return nil
}

func (n *LndNode) FundWallet(sats uint64, mineBlock bool) (string, error) {
	addr, err := n.Rpc.NewAddress(context.Background(), &lnrpc.NewAddressRequest{})
	if err != nil {
		return "", fmt.Errorf("NewAddress() %w", err)
	}

	r, err := n.bitcoin.Call("sendtoaddress", addr.Address, float64(sats)/math.Pow(10., 8))
	if err != nil {
		return "", fmt.Errorf("sendtoaddress %w", err)
	}

	txId, err := r.GetString()
	if err != nil {
		return "", err
	}

	if mineBlock {
		err = n.bitcoin.GenerateBlocks(1)
		if err != nil {
			return "", fmt.Errorf("bitcoin.GenerateBlocks() %w", err)
		}
		err = n.WaitForLog(fmt.Sprintf("Marking unconfirmed transaction %s", txId), TIMEOUT)
		if err != nil {
			return "", err
		}
	}

	return addr.Address, nil
}

func (n *LndNode) OpenChannel(peer LightningNode, capacity uint64, connect, confirm, waitForChannelActive bool) (string, error) {
	// fund wallet 10*cap
	_, err := n.FundWallet(10*capacity, true)
	if err != nil {
		return "", fmt.Errorf("FundWallet() %w", err)
	}

	isConnected, err := n.IsConnected(peer)
	if err != nil {
		return "", fmt.Errorf("IsConnected() %w", err)
	}

	if !isConnected && connect {
		err = n.Connect(peer, true)
		if err != nil {
			return "", fmt.Errorf("Connect() %w", err)
		}
	}

	pk, err := hex.DecodeString(peer.Id())
	if err != nil {
		return "", fmt.Errorf("DecodeString() %w", err)
	}
	stream, err := n.Rpc.OpenChannel(context.Background(), &lnrpc.OpenChannelRequest{
		NodePubkey:         pk,
		LocalFundingAmount: int64(capacity),
	})
	if err != nil {
		return "", fmt.Errorf("OpenChannel() %w", err)
	}

	// Wait for channel pending
	u, err := stream.Recv()
	if err != nil {
		return "", fmt.Errorf("stream.Recv() %w", err)
	}
	chp := u.GetChanPending()
	if chp == nil {
		return "", fmt.Errorf("GetChanPending() was nil")
	}

	if waitForChannelActive || confirm {
		n.bitcoin.GenerateBlocks(3)
	}

	if waitForChannelActive {
		err = WaitForWithErr(func() (bool, error) {
			u, err := stream.Recv()
			if err != nil {
				return false, fmt.Errorf("stream.Recv() %w", err)
			}

			open := u.GetChanOpen()
			return open != nil, nil
		}, TIMEOUT)
		if err != nil {
			return "", fmt.Errorf("error waiting for active channel: %w", err)
		}

		err = WaitForWithErr(func() (bool, error) {
			scid, err := n.GetScid(peer)
			if err != nil {
				return false, fmt.Errorf("GetScid() %w", err)
			}
			if scid == "" {
				return false, nil
			}

			localActive, err := n.IsChannelActive(scid)
			if err != nil {
				return false, fmt.Errorf("IsChannelActive() %w", err)
			}
			remoteActive, err := peer.IsChannelActive(scid)
			if err != nil {
				return false, fmt.Errorf("IsChannelActive() %w", err)
			}
			return remoteActive && localActive, nil
		}, TIMEOUT)
		if err != nil {
			return "", fmt.Errorf("error waiting for active channel: %w", err)
		}
	}

	scid, err := n.GetScid(peer)
	if err != nil {
		return "", fmt.Errorf("GetScid() %w", err)
	}

	return scid, nil
}

func (n *LndNode) IsBlockHeightSynced() (bool, error) {
	r, err := n.bitcoin.Rpc.Call("getblockcount")
	if err != nil {
		return false, fmt.Errorf("bitcoin.rpc.Call(\"getblockcount\") %w", err)
	}

	chainHeight, err := r.GetFloat()
	if err != nil {
		return false, fmt.Errorf("GetFloat() %w", err)
	}

	nodeInfo, err := n.Rpc.GetInfo(context.Background(), &lnrpc.GetInfoRequest{})
	if err != nil {
		return false, fmt.Errorf("GetInfo() %w", err)
	}
	return uint(nodeInfo.BlockHeight) >= uint(chainHeight), nil
}

func (n *LndNode) IsChannelActive(scid string) (bool, error) {
	r, err := n.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	if err != nil {
		return false, fmt.Errorf("ListChannels() %w", err)
	}

	for _, ch := range r.Channels {
		chScid := scidFromLndChanId(ch.ChanId)
		if chScid == scid {
			return ch.Active, nil
		}
	}

	return false, nil
}

func (n *LndNode) IsConnected(remote LightningNode) (bool, error) {
	peers, err := n.Rpc.ListPeers(context.Background(), &lnrpc.ListPeersRequest{})
	if err != nil {
		return false, fmt.Errorf("rpc.ListPeers() %w", err)
	}

	for _, peer := range peers.Peers {
		if peer.PubKey == remote.Id() {
			return true, nil
		}
	}

	return false, nil
}

func scidFromLndChanId(id uint64) string {
	lndScid := lnwire.NewShortChanIDFromInt(id)
	return fmt.Sprintf("%dx%dx%d", lndScid.BlockHeight, lndScid.TxIndex, lndScid.TxPosition)
}
