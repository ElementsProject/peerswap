package clightning

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
)

const (
	dbOption                        = "peerswap-db-path"
	liquidRpcHostOption             = "peerswap-liquid-rpchost"
	liquidRpcPortOption             = "peerswap-liquid-rpcport"
	liquidRpcUserOption             = "peerswap-liquid-rpcuser"
	liquidRpcPasswordOption         = "peerswap-liquid-rpcpassword"
	liquidRpcPasswordFilepathOption = "peerswap-liquid-rpcpasswordfile"

	bitcoinRpcHostOption     = "peerswap-bitcoin-rpchost"
	bitcoinRpcPortOption     = "peerswap-bitcoin-rpcport"
	bitcoinRpcUserOption     = "peerswap-bitcoin-rpcuser"
	bitcoinRpcPasswordOption = "peerswap-bitcoin-rpcpassword"
	bitcoinCookieFilePath    = "peerswap-bitcoin-cookiefilepath"

	rpcWalletOption     = "peerswap-liquid-rpcwallet"
	liquidNetworkOption = "peerswap-liquid-network"
	policyPathOption    = "peerswap-policy-path"
)

// PeerswapClightningConfig contains relevant config params for peerswap
type PeerswapClightningConfig struct {
	DbPath string

	BitcoinRpcUser         string
	BitcoinRpcPassword     string
	BitcoinRpcPasswordFile string
	BitcoinRpcHost         string
	BitcoinRpcPort         uint
	BitcoinCookieFilePath  string

	LiquidRpcUser         string
	LiquidRpcPassword     string
	LiquidRpcPasswordFile string
	LiquidRpcHost         string
	LiquidRpcPort         uint
	LiquidRpcWallet       string

	PolicyPath string

	LiquidEnabled bool
}

// RegisterOptions adds options to clightning
func (cl *ClightningClient) RegisterOptions() error {
	err := cl.plugin.RegisterNewOption(dbOption, "path to boltdb", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(bitcoinRpcHostOption, "bitcoind rpchost", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(bitcoinRpcPortOption, "bitcoind rpcport", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(bitcoinRpcUserOption, "bitcoind rpcuser", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(bitcoinRpcPasswordOption, "bitcoind rpcpassword", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(bitcoinCookieFilePath, "path to bitcoin cookie file", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(liquidRpcHostOption, "elementsd rpchost", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(liquidRpcPortOption, "elementsd rpcport", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(liquidRpcUserOption, "elementsd rpcuser", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(liquidRpcPasswordOption, "elementsd rpcpassword", "")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(liquidNetworkOption, "liquid-network", "regtest")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(rpcWalletOption, "liquid-rpcwallet", "swap")
	if err != nil {
		return err
	}
	err = cl.plugin.RegisterNewOption(liquidRpcPasswordFilepathOption, "elementsd rpcpassword filepath", "")
	if err != nil {
		return err
	}
	// register policy options
	err = cl.plugin.RegisterNewOption(policyPathOption, "Path to the policy file. If empty the default policy is used", "")
	if err != nil {
		return err
	}
	return nil
}

// GetConfig returns the peerswap config
func (cl *ClightningClient) GetConfig() (*PeerswapClightningConfig, error) {

	dbpath, err := cl.plugin.GetOption(dbOption)
	if err != nil {
		return nil, err
	}
	if dbpath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dbpath = filepath.Join(wd, "swaps")
	}
	err = os.MkdirAll(dbpath, 0700)
	if err != nil && err != os.ErrExist {
		return nil, err
	}
	// bitcoin rpc settings
	bitcoinRpcHost, err := cl.plugin.GetOption(bitcoinRpcHostOption)
	if err != nil {
		return nil, err
	}
	bitcoinRpcPortString, err := cl.plugin.GetOption(bitcoinRpcPortOption)
	if err != nil {
		return nil, err
	}
	var bitcoinRpcPort int
	if bitcoinRpcPortString != "" {
		bitcoinRpcPort, err = strconv.Atoi(bitcoinRpcPortString)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("%s is not an int", liquidRpcPortOption))
		}
	}
	bitcoinRpcUser, err := cl.plugin.GetOption(bitcoinRpcUserOption)
	if err != nil {
		return nil, err
	}
	bitcoinRpcPassword, err := cl.plugin.GetOption(bitcoinRpcPasswordOption)
	if err != nil {
		return nil, err
	}
	bitcoinCookieFilePath, err := cl.plugin.GetOption(bitcoinCookieFilePath)
	if err != nil {
		return nil, err
	}
	// liquid rpc settings
	liquidRpcHost, err := cl.plugin.GetOption(liquidRpcHostOption)
	if err != nil {
		return nil, err
	}
	liquidRpcPortString, err := cl.plugin.GetOption(liquidRpcPortOption)
	if err != nil {
		return nil, err
	}
	if liquidRpcHost != "" && liquidRpcPortString == "" {
		return nil, errors.New(fmt.Sprintf("%s need to be set", liquidRpcPortOption))
	}
	var liquidRpcPort int
	if liquidRpcPortString != "" {
		liquidRpcPort, err = strconv.Atoi(liquidRpcPortString)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("%s is not an int", liquidRpcPortOption))
		}
	}
	liquidRpcUser, err := cl.plugin.GetOption(liquidRpcUserOption)
	if err != nil {
		return nil, err
	}
	if liquidRpcHost != "" && liquidRpcUser == "" {
		return nil, errors.New(fmt.Sprintf("%s need to be set", liquidRpcUserOption))
	}
	liquidRpcPass, err := cl.plugin.GetOption(liquidRpcPasswordOption)
	if err != nil {
		return nil, err
	}
	liquidRpcPassFile, err := cl.plugin.GetOption(liquidRpcPasswordFilepathOption)
	if liquidRpcHost != "" && liquidRpcPass == "" && liquidRpcPassFile == "" {
		return nil, errors.New(fmt.Sprintf("%s or %s need to be set", liquidRpcPasswordOption, liquidRpcPasswordFilepathOption))
	}
	liquidRpcWallet, err := cl.plugin.GetOption(rpcWalletOption)
	if err != nil {
		return nil, err
	}
	if liquidRpcWallet == "dev_test" {
		idBytes := make([]byte, 8)
		_, _ = rand.Read(idBytes[:])
		liquidRpcWallet = hex.EncodeToString(idBytes)
	}

	// get policy path
	policyPath, err := cl.plugin.GetOption(policyPathOption)
	if err != nil {
		return nil, err
	}

	return &PeerswapClightningConfig{
		DbPath:                dbpath,
		LiquidRpcHost:         liquidRpcHost,
		LiquidRpcPort:         uint(liquidRpcPort),
		LiquidRpcUser:         liquidRpcUser,
		LiquidRpcPassword:     liquidRpcPass,
		LiquidRpcPasswordFile: liquidRpcPassFile,
		LiquidRpcWallet:       liquidRpcWallet,
		BitcoinRpcHost:        bitcoinRpcHost,
		BitcoinRpcPort:        uint(bitcoinRpcPort),
		BitcoinRpcUser:        bitcoinRpcUser,
		BitcoinRpcPassword:    bitcoinRpcPassword,
		BitcoinCookieFilePath: bitcoinCookieFilePath,
		PolicyPath:            policyPath,
	}, nil
}
