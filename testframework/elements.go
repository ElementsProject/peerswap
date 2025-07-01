package testframework

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	// Addresses to generate to
	LBTC_BURN = "ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep"
	BTC_BURN  = "2N61yGL5ZBy3yaiEM8312CuG78CBNQMWE4Y"
)

func getBitcoindConfig() map[string]string {
	return map[string]string{
		"regtest":     "1",
		"rpcuser":     "rpcuser",
		"rpcpassword": "rpcpass",
		"fallbackfee": "0.00001",
	}
}

func getLiquiddConfig() map[string]string {
	return map[string]string{
		"listen":           "1",
		"debug":            "1",
		"rpcuser":          "rpcuser",
		"rpcpassword":      "rpcpass",
		"fallbackfee":      "0.00001",
		"initialfreecoins": "2100000000000000",
		"validatepegin":    "0",
		"chain":            "liquidregtest",
		"acceptdiscountct": "1",
		"creatediscountct": "1",
		"minrelaytxfee":    "0.00000001",
		"mintxfee":         "0.00000001",
		"blockmintxfee":    "0.00000001",
	}
}

type BitcoinNode struct {
	*DaemonProcess
	*RpcProxy

	DataDir     string
	ConfigFile  string
	RpcPort     int
	RpcUser     string
	RpcPassword string
	WalletName  string

	ZmqPubRawTx    string
	ZmqPubRawBlock string
}

func NewBitcoinNode(testDir string, id int) (*BitcoinNode, error) {
	rpcPort, err := GetFreePort()
	if err != nil {
		return nil, err
	}

	zmqpubrawblockPort := rpcPort
	for zmqpubrawblockPort == rpcPort {
		zmqpubrawblockPort, err = GetFreePort()
		if err != nil {
			return nil, err
		}
	}

	zmqpubrawtxPort := rpcPort
	for zmqpubrawtxPort == rpcPort || zmqpubrawtxPort == zmqpubrawblockPort {
		zmqpubrawtxPort, err = GetFreePort()
		if err != nil {
			return nil, err
		}
	}

	zmqpubrawblock := fmt.Sprintf("tcp://127.0.0.1:%d", zmqpubrawblockPort)
	zmqpubrawtx := fmt.Sprintf("tcp://127.0.0.1:%d", zmqpubrawtxPort)

	rngDirExtension, err := GenerateRandomString(5)
	dataDir := filepath.Join(testDir, fmt.Sprintf("bitcoin-%s", rngDirExtension))

	err = os.MkdirAll(dataDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, err
	}

	cmdLine := []string{
		"bitcoind",
		fmt.Sprintf("-datadir=%s", dataDir),
		"-printtoconsole",
		"-server",
		"-logtimestamps",
		"-nolisten",
		"-txindex",
		"-nowallet",
		"-addresstype=bech32",
		// https://github.com/lightningnetwork/lnd/issues/9163
		"-deprecatedrpc=warnings",
	}

	bitcoinConfig := getBitcoindConfig()
	bitcoinConfig["zmqpubrawblock"] = zmqpubrawblock
	bitcoinConfig["zmqpubrawtx"] = zmqpubrawtx
	regtestConfig := map[string]string{"rpcport": strconv.Itoa(rpcPort)}
	configFile := filepath.Join(dataDir, "bitcoin.conf")
	WriteConfig(configFile, bitcoinConfig, regtestConfig, "regtest")

	proxy, err := NewRpcProxy(configFile)
	if err != nil {
		return nil, fmt.Errorf("NewRpcProxy(configFile) %w", err)
	}

	return &BitcoinNode{
		DaemonProcess:  NewDaemonProcess(cmdLine, fmt.Sprintf("bitcoind-%d", id)),
		RpcProxy:       proxy,
		DataDir:        dataDir,
		ConfigFile:     configFile,
		RpcPort:        rpcPort,
		RpcUser:        bitcoinConfig["rpcuser"],
		RpcPassword:    bitcoinConfig["rpcpassword"],
		WalletName:     "lightningd-tests",
		ZmqPubRawBlock: zmqpubrawblock,
		ZmqPubRawTx:    zmqpubrawtx,
	}, nil
}

func (n *BitcoinNode) Run(generateInitialBlocks bool) error {
	n.DaemonProcess.Run()

	// Wait for daemon process to be ready
	err := n.WaitForLog("Done loading", TIMEOUT)
	if err != nil {
		return err
	}

	// Add RPC client
	n.RpcProxy, err = NewRpcProxy(n.ConfigFile)
	if err != nil {
		return fmt.Errorf("NewRpcProxy(configFile) %w", err)
	}

	// Create and open wallet
	_, err = n.Call("createwallet", n.WalletName)
	if err != nil {
		return fmt.Errorf("can not create wallet: %w", err)
	}

	_, err = n.Call("loadwallet", n.WalletName)
	if err != nil {
		return fmt.Errorf("can not load wallet: %w", err)
	}

	// Check for 101 blocks
	blockchainInfo := struct {
		Blocks int `json:"blocks"`
	}{}

	r, err := n.Rpc.Call("getblockchaininfo")
	if err != nil {
		return fmt.Errorf("Call(\"getblockchaininfo\") %w", err)
	}

	err = r.GetObject(&blockchainInfo)
	if err != nil {
		return fmt.Errorf("GetObject() %w", err)
	}

	if blockchainInfo.Blocks < 101 {
		n.GenerateBlocks(101 - blockchainInfo.Blocks)
	}

	// Check for walletbalance
	walletInfo := struct {
		Balance float32 `json:"balance"`
	}{}

	r, err = n.Rpc.Call("getwalletinfo")
	if err != nil {
		return fmt.Errorf("Call(\"getwalletinfo\") %w", err)
	}

	err = r.GetObject(&walletInfo)
	if err != nil {
		return fmt.Errorf("GetObject() %w", err)
	}

	if walletInfo.Balance < 1 {
		n.GenerateBlocks(1)
	}

	return nil
}

func (n *BitcoinNode) GenerateBlocks(b int) error {
	_, err := n.Rpc.Call("getrawmempool")
	if err != nil {
		return fmt.Errorf("getrawmempool %w", err)
	}

	r, err := n.Rpc.Call("getnewaddress")
	if err != nil {
		return fmt.Errorf("getnewaddress %w", err)
	}

	address, err := r.GetString()
	if err != nil {
		return fmt.Errorf("GetObject() %w", err)
	}

	_, err = n.Rpc.Call("generatetoaddress", b, address)
	if err != nil {
		return fmt.Errorf("Call(\"generate\") %w", err)
	}
	return nil
}

func (n *BitcoinNode) ReturnAsset() string {
	return "btc"
}

type LiquidNode struct {
	*DaemonProcess
	*RpcProxy

	bitcoin     *BitcoinNode
	DataDir     string
	ConfigFile  string
	Port        int
	RpcPort     int
	RpcUser     string
	RpcPassword string
	WalletName  string
	Network     string
}

func NewLiquidNode(testDir string, bitcoin *BitcoinNode, id int) (*LiquidNode, error) {
	rpcPort, err := GetFreePort()
	if err != nil {
		return nil, err
	}

	port := rpcPort
	for port == rpcPort {
		port, err = GetFreePort()
		if err != nil {
			return nil, err
		}
	}

	rngDirExtension, err := GenerateRandomString(5)
	if err != nil {
		return nil, err
	}

	dataDir := filepath.Join(testDir, fmt.Sprintf("liquid-%s", rngDirExtension))

	err = os.MkdirAll(dataDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, err
	}

	cmdLine := []string{
		"elementsd",
		fmt.Sprintf("-datadir=%s", dataDir),
	}

	config := getLiquiddConfig()
	bitcoindConfig := getBitcoindConfig()
	config["mainchainrpcport"] = strconv.Itoa(bitcoin.RpcPort)
	config["mainchainrpcuser"] = bitcoindConfig["rpcuser"]
	config["mainchainrpcpassword"] = bitcoindConfig["rpcpassword"]

	regtestConfig := map[string]string{"rpcport": strconv.Itoa(rpcPort), "port": strconv.Itoa(port)}
	configFile := filepath.Join(dataDir, "elements.conf")
	WriteConfig(configFile, config, regtestConfig, config["chain"])

	return &LiquidNode{
		DaemonProcess: NewDaemonProcess(cmdLine, fmt.Sprintf("elements-%d", id)),
		DataDir:       dataDir,
		ConfigFile:    configFile,
		RpcPort:       rpcPort,
		Port:          port,
		WalletName:    "liquidwallet",
		RpcUser:       config["rpcuser"],
		RpcPassword:   config["rpcpassword"],
		Network:       config["chain"],
		bitcoin:       bitcoin,
	}, nil
}

func NewLiquidNodeFromConfig(testDir string, bitcoin *BitcoinNode, config map[string]string, id int) (*LiquidNode, error) {
	rpcPort, err := GetFreePort()
	if err != nil {
		return nil, err
	}

	port := rpcPort
	for port == rpcPort {
		port, err = GetFreePort()
		if err != nil {
			return nil, err
		}
	}

	rngDirExtension, err := GenerateRandomString(5)
	if err != nil {
		return nil, err
	}

	dataDir := filepath.Join(testDir, fmt.Sprintf("liquid-%s", rngDirExtension))

	err = os.MkdirAll(dataDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, err
	}

	cmdLine := []string{
		"elementsd",
		fmt.Sprintf("-datadir=%s", dataDir),
	}

	bitcoindConfig := getBitcoindConfig()
	config["mainchainrpcport"] = strconv.Itoa(bitcoin.RpcPort)
	config["mainchainrpcuser"] = bitcoindConfig["rpcuser"]
	config["mainchainrpcpassword"] = bitcoindConfig["rpcpassword"]

	regtestConfig := map[string]string{"rpcport": strconv.Itoa(rpcPort), "port": strconv.Itoa(port)}
	configFile := filepath.Join(dataDir, "elements.conf")
	WriteConfig(configFile, config, regtestConfig, config["chain"])

	return &LiquidNode{
		DaemonProcess: NewDaemonProcess(cmdLine, fmt.Sprintf("elements-%d", id)),
		DataDir:       dataDir,
		ConfigFile:    configFile,
		RpcPort:       rpcPort,
		Port:          port,
		WalletName:    "liquidwallet",
		RpcUser:       config["rpcuser"],
		RpcPassword:   config["rpcpassword"],
		Network:       config["chain"],
		bitcoin:       bitcoin,
	}, nil
}

func (n *LiquidNode) Run(generateInitialBlocks bool) error {
	n.DaemonProcess.Run()

	// Wait for daemon process to be ready
	err := n.WaitForLog("Done loading", TIMEOUT)
	if err != nil {
		return err
	}

	n.RpcProxy, err = NewRpcProxy(n.ConfigFile)
	if err != nil {
		return fmt.Errorf("NewRpcProxy(configFile) %w", err)
	}

	// Create and open wallet
	_, err = n.Call("createwallet", n.WalletName)
	if err != nil {
		return fmt.Errorf("can not create wallet: %w", err)
	}

	// Change to wallet url
	n.RpcProxy.UpdateServiceUrl(fmt.Sprintf("http://127.0.0.1:%d/wallet/%s", n.RpcPort, n.WalletName))

	// Rescan blockchain to "add" outputs to new wallet
	_, err = n.Rpc.Call("rescanblockchain")
	if err != nil {
		return fmt.Errorf("Call(\"rescanblockchain\") %w", err)
	}

	// Check for 101 blocks
	r, err := n.Rpc.Call("getblockchaininfo")
	if err != nil {
		return fmt.Errorf("Call(\"getblockchaininfo\") %w", err)
	}

	blockchainInfo := struct {
		Blocks int `json:"blocks"`
	}{}
	err = r.GetObject(&blockchainInfo)
	if err != nil {
		return fmt.Errorf("GetObject() %w", err)
	}

	if blockchainInfo.Blocks < 101 {
		n.GenerateBlocks(101 - blockchainInfo.Blocks)
	}

	return nil
}

func (n *LiquidNode) GenerateBlocks(b int) error {
	_, err := n.Rpc.Call("generatetoaddress", b, LBTC_BURN)
	if err != nil {
		return fmt.Errorf("Call(\"generate\") %w", err)
	}
	return nil
}

func (n *LiquidNode) ReturnAsset() string {
	return "lbtc"
}

func (n *LiquidNode) SwitchWallet(wallet string) error {
	_, err := n.Rpc.Call("loadwallet", wallet)
	if err != nil {
		return fmt.Errorf("Call(\"loadwallet\") %w", err)
	}

	n.RpcProxy.UpdateServiceUrl(fmt.Sprintf("http://127.0.0.1:%d/wallet/%s", n.RpcPort, wallet))
	return nil
}
