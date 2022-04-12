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
	liquidEnabledOption             = "peerswap-liquid-enabled"

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
	LiquidEnabled         bool

	PolicyPath string
}

// RegisterOptions adds options to clightning
func (cl *ClightningClient) RegisterOptions() error {
	err := cl.Plugin.RegisterNewOption(dbOption, "path to boltdb", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(bitcoinRpcHostOption, "bitcoind rpchost", "localhost")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(bitcoinRpcPortOption, "bitcoind rpcport", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(bitcoinRpcUserOption, "bitcoind rpcuser", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(bitcoinRpcPasswordOption, "bitcoind rpcpassword", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(bitcoinCookieFilePath, "path to bitcoin cookie file", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidRpcHostOption, "elementsd rpchost", "localhost")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidRpcPortOption, "elementsd rpcport", "18888")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidRpcUserOption, "elementsd rpcuser", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidRpcPasswordOption, "elementsd rpcpassword", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidNetworkOption, "liquid-network", "regtest")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(rpcWalletOption, "liquid-rpcwallet", "peerswap")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidRpcPasswordFilepathOption, "elementsd rpcpassword filepath", "")
	if err != nil {
		return err
	}

	err = cl.Plugin.RegisterNewBoolOption(liquidEnabledOption, "enable/disable liquid", true)
	if err != nil {
		return err
	}

	// register policy options
	err = cl.Plugin.RegisterNewOption(policyPathOption, "Path to the policy file. If empty the default policy is used", "")
	if err != nil {
		return err
	}
	return nil
}

// GetConfig returns the peerswap config
func (cl *ClightningClient) GetConfig() (*PeerswapClightningConfig, error) {

	dbpath, err := cl.Plugin.GetOption(dbOption)
	if err != nil {
		return nil, err
	}
	if dbpath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dbpath = filepath.Join(wd, "peerswap")
	}
	err = os.MkdirAll(dbpath, 0755)
	if err != nil && err != os.ErrExist {
		return nil, err
	}
	// bitcoin rpc settings
	bitcoinRpcHost, err := cl.Plugin.GetOption(bitcoinRpcHostOption)
	if err != nil {
		return nil, err
	}
	bitcoinRpcPortString, err := cl.Plugin.GetOption(bitcoinRpcPortOption)
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
	bitcoinRpcUser, err := cl.Plugin.GetOption(bitcoinRpcUserOption)
	if err != nil {
		return nil, err
	}
	bitcoinRpcPassword, err := cl.Plugin.GetOption(bitcoinRpcPasswordOption)
	if err != nil {
		return nil, err
	}
	bitcoinCookieFilePath, err := cl.Plugin.GetOption(bitcoinCookieFilePath)
	if err != nil {
		return nil, err
	}
	// liquid rpc settings
	liquidRpcHost, err := cl.Plugin.GetOption(liquidRpcHostOption)
	if err != nil {
		return nil, err
	}
	liquidRpcPortString, err := cl.Plugin.GetOption(liquidRpcPortOption)
	if err != nil {
		return nil, err
	}

	var liquidRpcPort int
	if liquidRpcPortString != "" {
		liquidRpcPort, err = strconv.Atoi(liquidRpcPortString)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("%s is not an int", liquidRpcPortOption))
		}
	}
	liquidRpcUser, err := cl.Plugin.GetOption(liquidRpcUserOption)
	if err != nil {
		return nil, err
	}

	liquidRpcPass, err := cl.Plugin.GetOption(liquidRpcPasswordOption)
	if err != nil {
		return nil, err
	}
	liquidRpcPassFile, err := cl.Plugin.GetOption(liquidRpcPasswordFilepathOption)
	liquidRpcWallet, err := cl.Plugin.GetOption(rpcWalletOption)
	if err != nil {
		return nil, err
	}
	if liquidRpcWallet == "dev_test" {
		idBytes := make([]byte, 8)
		_, _ = rand.Read(idBytes[:])
		liquidRpcWallet = hex.EncodeToString(idBytes)
	}

	liquidEnabled, err := cl.Plugin.GetBoolOption(liquidEnabledOption)
	if err != nil {
		return nil, err
	}

	// get policy path
	policyPath, err := cl.Plugin.GetOption(policyPathOption)
	if err != nil {
		return nil, err
	}

	if policyPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		policyPath = filepath.Join(wd, "peerswap", "policy.conf")
	}

	return &PeerswapClightningConfig{
		DbPath:                dbpath,
		LiquidRpcHost:         liquidRpcHost,
		LiquidRpcPort:         uint(liquidRpcPort),
		LiquidRpcUser:         liquidRpcUser,
		LiquidRpcPassword:     liquidRpcPass,
		LiquidRpcPasswordFile: liquidRpcPassFile,
		LiquidRpcWallet:       liquidRpcWallet,
		LiquidEnabled:         liquidEnabled,
		BitcoinRpcHost:        bitcoinRpcHost,
		BitcoinRpcPort:        uint(bitcoinRpcPort),
		BitcoinRpcUser:        bitcoinRpcUser,
		BitcoinRpcPassword:    bitcoinRpcPassword,
		BitcoinCookieFilePath: bitcoinCookieFilePath,
		PolicyPath:            policyPath,
	}, nil
}
