package testframework

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/sputn1ck/glightning/glightning"
)

type CLightningNode struct {
	*DaemonProcess
	*CLightningProxy

	DataDir    string
	ConfigFile string
	Port       int
	Info       *glightning.NodeInfo

	bitcoin *BitcoinNode
}

func NewCLightningNode(testDir string, bitcoin *BitcoinNode, id int) (*CLightningNode, error) {
	port, err := GetFreePort()
	if err != nil {
		return nil, fmt.Errorf("GetFreePort() %w", err)
	}

	rngDirExtension, err := GenerateRandomString(5)
	if err != nil {
		return nil, fmt.Errorf("GenerateRandomString(5) %w", err)
	}

	dataDir := filepath.Join(testDir, fmt.Sprintf("clightning-%s", rngDirExtension))
	networkDir := filepath.Join(dataDir, "regtest")

	err = os.MkdirAll(networkDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("os.MkdirAll() %w", err)
	}

	bitcoinConf, err := ReadConfig(bitcoin.configFile)
	if err != nil {
		return nil, fmt.Errorf("ReadConfig() %w", err)
	}

	var bitcoinRpcPass string
	if pass, ok := bitcoinConf["rpcpassword"]; ok {
		bitcoinRpcPass = pass
	} else {
		return nil, fmt.Errorf("bitcoin rpcpassword not found in config %s", bitcoin.configFile)
	}

	var bitcoinRpcUser string
	if user, ok := bitcoinConf["rpcuser"]; ok {
		bitcoinRpcUser = user
	} else {
		return nil, fmt.Errorf("bitcoin rpcuser not found in config %s", bitcoin.configFile)
	}

	var bitcoinRpcPort string
	if port, ok := bitcoinConf["rpcport"]; ok {
		bitcoinRpcPort = port
	} else {
		return nil, fmt.Errorf("bitcoin rpcuser not found in config %s", bitcoin.configFile)
	}

	cmdLine := []string{
		"lightningd",
		fmt.Sprintf("--lightning-dir=%s", dataDir),
		fmt.Sprintf("--log-level=%s", "debug"),
		fmt.Sprintf("--addr=127.0.0.1:%d", port),
		fmt.Sprintf("--allow-deprecated-apis=%s", "true"),
		fmt.Sprintf("--network=%s", "regtest"),
		fmt.Sprintf("--ignore-fee-limits=%s", "false"),
		fmt.Sprintf("--bitcoin-rpcuser=%s", bitcoinRpcUser),
		fmt.Sprintf("--bitcoin-rpcpassword=%s", bitcoinRpcPass),
		fmt.Sprintf("--bitcoin-rpcport=%s", bitcoinRpcPort),
		fmt.Sprintf("--bitcoin-datadir=%s", bitcoin.DataDir),
	}

	// socketPath := filepath.Join(networkDir, "lightning-rpc")
	proxy, err := NewCLightningProxy("lightning-rpc", networkDir)
	if err != nil {
		return nil, fmt.Errorf("NewCLightningProxy(configFile) %w", err)
	}

	// Create seed file
	regex, _ := regexp.Compile("[^/]+")
	found := regex.FindAll([]byte(dataDir), -1)
	all := []byte{}
	for _, v := range found {
		all = append(all, v...)
	}
	seed := regex.Find(all)[len(all)-32:]
	seedFile := filepath.Join(networkDir, "hsm_secret")
	err = os.WriteFile(seedFile, seed, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("WriteFile() %w", err)
	}

	return &CLightningNode{
		DaemonProcess:   NewDaemonProcess(cmdLine, fmt.Sprintf("clightning-%d", id)),
		CLightningProxy: proxy,
		DataDir:         dataDir,
		Port:            port,
		bitcoin:         bitcoin,
	}, nil
}

func (n *CLightningNode) Run(waitForReady, waitForBitcoinSynced bool) error {
	n.DaemonProcess.Run()
	if waitForReady {
		err := n.WaitForLog("Server started with public key", 60*time.Second)
		if err != nil {
			return fmt.Errorf("CLightningNode.Run() %w", err)
		}
	}

	var counter int
	var err error
	for {
		if counter > 10 {
			return fmt.Errorf("to many retries: %w", err)
		}

		err = n.StartProxy()
		if err != nil {
			counter++
			time.Sleep(500 * time.Millisecond)
			continue
		}

		break
	}

	// Cache info
	n.Info, err = n.Rpc.GetInfo()
	if err != nil {
		return fmt.Errorf("rpc.GetInfo() %w", err)
	}

	if waitForBitcoinSynced {
		// Wait for sync with bitcoin network
		return WaitFor(func() bool {
			info, err := n.Rpc.GetInfo()
			if err != nil {
				log.Printf("rpc.GetInfo() %v", err)
				return false
			}

			isHeightSync, err := n.IsBlockHeightSynced()
			if err != nil {
				log.Printf("rpc.GetInfo() %v", err)
				return false
			}

			return info.IsBitcoindSync() && info.IsLightningdSync() && isHeightSync
		}, TIMEOUT)
	}

	return nil
}

func (n *CLightningNode) Shutdown() error {
	n.Rpc.Stop()
	return os.Remove(filepath.Join(n.dataDir, "lightning-rpc"))
}

func (n *CLightningNode) Id() string {
	return n.Info.Id
}

func (n *CLightningNode) Address() string {
	return fmt.Sprintf("%s@127.0.0.1:%d", n.Info.Id, n.Port)
}

func (n *CLightningNode) GetBtcBalanceSat() (sats uint64, err error) {
	r, err := n.Rpc.ListFunds()
	if err != nil {
		return 0, fmt.Errorf("ListFunds() %w", err)
	}

	var sum uint64
	for _, output := range r.Outputs {
		// Value seems to be already in sat.
		sum += output.Value
	}
	return sum, nil
}

func (n *CLightningNode) GetScid(remote LightningNode) (string, error) {
	peers, err := n.Rpc.ListPeers()
	if err != nil {
		return "", fmt.Errorf("ListPeers() %w", err)
	}

	for _, peer := range peers {
		if peer.Id == remote.Id() {
			if peer.Channels != nil {
				return peer.Channels[0].ShortChannelId, nil
			}
			return "", fmt.Errorf("no channel to peer")
		}
	}
	return "", fmt.Errorf("peer not found")
}

func (n *CLightningNode) Connect(peer LightningNode, waitForConnection bool) error {
	id, host, port, err := SplitLnAddr(peer.Address())
	if err != nil {
		return fmt.Errorf("SplitLnAddr() %w", err)
	}

	_, err = n.Rpc.Connect(id, host, uint(port))
	if err != nil {
		return fmt.Errorf("Connect() %w", err)
	}

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

	return nil
}

func (n *CLightningNode) FundWallet(sats uint64, mineBlock bool) (string, error) {
	addr, err := n.Rpc.NewAddr()
	if err != nil {
		return "", fmt.Errorf("rpc.NewAddress() %w", err)
	}

	r, err := n.bitcoin.Call("sendtoaddress", addr, float64(sats)/math.Pow(10., 8))
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
		err = n.WaitForLog(fmt.Sprintf("Owning output .* txid %s CONFIRMED", txId), TIMEOUT)
		if err != nil {
			return "", err
		}
	}

	return addr, nil
}

func (n *CLightningNode) OpenChannel(remote LightningNode, capacity uint64, connect, confirm, waitForActiveChannel bool) (string, error) {
	_, err := n.FundWallet(10*capacity, true)
	if err != nil {
		return "", fmt.Errorf("FundWallet() %w", err)
	}

	isConnected, err := n.IsConnected(remote)
	if err != nil {
		return "", fmt.Errorf("IsConnected() %w", err)
	}

	if !isConnected && connect {
		err = n.Connect(remote, true)
		if err != nil {
			return "", fmt.Errorf("Connect() %w", err)
		}
	}

	fr, err := n.Rpc.FundChannel(remote.Id(), &glightning.Sat{Value: capacity})
	if err != nil {
		return "", fmt.Errorf("FundChannel() %w", err)
	}

	// Wait for tx in mempool
	err = WaitFor(func() bool {
		r, err := n.bitcoin.Call("getrawmempool")
		if err != nil {
			log.Println("getrawmempool: %w", err)
			return false
		}

		var rawMempool []string
		err = r.GetObject(&rawMempool)
		if err != nil {
			log.Println("can not unmarshal object: %w", err)
			return false
		}

		for _, txId := range rawMempool {
			if txId == fr.FundingTxId {
				return true
			}
		}
		return false
	}, TIMEOUT)
	if err != nil {
		return "", fmt.Errorf("error waiting for tx in mempool: %w", err)
	}

	if waitForActiveChannel || confirm {
		n.bitcoin.GenerateBlocks(1)
	}

	if waitForActiveChannel {
		err = WaitForWithErr(func() (bool, error) {
			scid, err := n.GetScid(remote)
			if err != nil {
				return false, fmt.Errorf("GetScid() %w", err)
			}

			localActive, err := n.IsChannelActive(scid)
			if err != nil {
				return false, fmt.Errorf("IsChannelActive() %w", err)
			}
			remoteActive, err := remote.IsChannelActive(scid)
			if err != nil {
				return false, fmt.Errorf("IsChannelActive() %w", err)
			}
			return remoteActive && localActive, nil
		}, TIMEOUT)
		if err != nil {
			return "", fmt.Errorf("error waiting for active channel: %w", err)
		}
	}

	scid, err := n.GetScid(remote)
	if err != nil {
		return "", fmt.Errorf("GetScid() %w", err)
	}

	return scid, nil
}

func (n *CLightningNode) IsBlockHeightSynced() (bool, error) {
	r, err := n.bitcoin.Rpc.Call("getblockcount")
	if err != nil {
		return false, fmt.Errorf("bitcoin.rpc.Call(\"getblockcount\") %w", err)
	}

	chainHeight, err := r.GetFloat()
	if err != nil {
		return false, fmt.Errorf("GetFloat() %w", err)
	}

	nodeInfo, err := n.Rpc.GetInfo()
	if err != nil {
		return false, fmt.Errorf("GetInfo() %w", err)
	}
	return nodeInfo.Blockheight >= uint(chainHeight), nil
}

func (n *CLightningNode) IsChannelActive(scid string) (bool, error) {
	funds, err := n.Rpc.ListFunds()
	if err != nil {
		return false, fmt.Errorf("ListChannels() %w", err)
	}

	for _, ch := range funds.Channels {
		if ch.ShortChannelId == scid {
			return ch.State == "CHANNELD_NORMAL", nil
		}
	}
	return false, nil
}

func (n *CLightningNode) IsConnected(remote LightningNode) (bool, error) {
	peers, err := n.Rpc.ListPeers()
	if err != nil {
		return false, fmt.Errorf("rpc.ListPeers() %w", err)
	}

	for _, peer := range peers {
		if remote.Id() == peer.Id {
			return peer.Connected, nil
		}
	}
	return false, nil
}

func (n *CLightningNode) GetDataDir() string {
	return n.dataDir
}
