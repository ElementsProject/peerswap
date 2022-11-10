package clightning

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/elementsproject/glightning/glightning"
	"github.com/elementsproject/peerswap/log"
	"github.com/pelletier/go-toml/v2"
)

const (
	defaultDbName         = "peerswap"
	defaultPolicyFileName = "policy.conf"
)

const (
	dbOption                        = "peerswap-db-path"
	liquidRpcHostOption             = "peerswap-elementsd-rpchost"
	liquidRpcPortOption             = "peerswap-elementsd-rpcport"
	liquidRpcUserOption             = "peerswap-elementsd-rpcuser"
	liquidRpcPasswordOption         = "peerswap-elementsd-rpcpassword"
	liquidRpcPasswordFilepathOption = "peerswap-elementsd-rpcpasswordfile"
	liquidEnabledOption             = "peerswap-elementsd-enabled"
	rpcWalletOption                 = "peerswap-elementsd-rpcwallet"

	bitcoinRpcHostOption     = "peerswap-bitcoin-rpchost"
	bitcoinRpcPortOption     = "peerswap-bitcoin-rpcport"
	bitcoinRpcUserOption     = "peerswap-bitcoin-rpcuser"
	bitcoinRpcPasswordOption = "peerswap-bitcoin-rpcpassword"
	bitcoinCookieFilePath    = "peerswap-bitcoin-cookiefilepath"

	policyPathOption = "peerswap-policy-path"
)

// PeerswapClightningConfig contains relevant config params for peerswap
type PeerswapClightningConfig struct {
	DbPath string `json:"dbpath"`

	BitcoinRpcUser         string `json:"bitcoin.rpcuser"`
	BitcoinRpcPassword     string `json:"bitcoin.rpcpassword"`
	BitcoinRpcPasswordFile string `json:"bitcoin.rpcpasswordfile"`
	BitcoinRpcHost         string `json:"bitcoin.rpchost"`
	BitcoinRpcPort         uint   `json:"bitcoin.rpcport"`
	BitcoinCookieFilePath  string `json:"bitcoin.rpccookiefilepath"`

	LiquidRpcUser         string `json:"liquid.rpcuser"`
	LiquidRpcPassword     string `json:"liquid.rpcpassword"`
	LiquidRpcPasswordFile string `json:"liquid.rpcpasswordfile"`
	LiquidRpcHost         string `json:"liquid.rpchost"`
	LiquidRpcPort         uint   `json:"liquid.rpcport"`
	LiquidRpcWallet       string `json:"liquid.rpcwallet"`
	LiquidEnabled         bool   `json:"liquid.enabled"`

	PolicyPath     string `json:"policypath"`
	ConfigFilePath string
}

func (c PeerswapClightningConfig) String() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// RegisterOptions adds options to clightning
func (cl *ClightningClient) RegisterOptions() error {
	err := cl.Plugin.RegisterNewOption(dbOption, "path to boltdb", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(bitcoinRpcHostOption, "bitcoind rpchost", "")
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
	err = cl.Plugin.RegisterNewOption(liquidRpcHostOption, "elementsd rpchost", "http://localhost")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidRpcPortOption, "elementsd rpcport", "7041")
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

// parseConfigFromFile parses the peerswap-plugin config from a *toml file.
func parseConfigFromFile(configPath string) (*PeerswapClightningConfig, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var fileConf struct {
		DbPath     string
		PolicyPath string
		Bitcoin    struct {
			RpcUser         string
			RpcPassword     string
			RpcPasswordFile string
			RpcHost         string
			RpcPort         uint
			CookieFilePath  string
		}
		Liquid struct {
			RpcUser         string
			RpcPassword     string
			RpcPasswordFile string
			RpcHost         string
			RpcPort         uint
			RpcWallet       string
			Enabled         bool
		}
	}

	err = toml.Unmarshal(data, &fileConf)
	if err != nil {
		return nil, err
	}

	return &PeerswapClightningConfig{
		DbPath:                 fileConf.DbPath,
		BitcoinRpcUser:         fileConf.Bitcoin.RpcUser,
		BitcoinRpcPassword:     fileConf.Bitcoin.RpcPassword,
		BitcoinRpcPasswordFile: fileConf.Bitcoin.RpcPasswordFile,
		BitcoinRpcHost:         fileConf.Bitcoin.RpcHost,
		BitcoinRpcPort:         fileConf.Bitcoin.RpcPort,
		BitcoinCookieFilePath:  fileConf.Bitcoin.CookieFilePath,
		LiquidRpcUser:          fileConf.Liquid.RpcUser,
		LiquidRpcPassword:      fileConf.Liquid.RpcPassword,
		LiquidRpcPasswordFile:  fileConf.Liquid.RpcPasswordFile,
		LiquidRpcHost:          fileConf.Liquid.RpcHost,
		LiquidRpcPort:          fileConf.Liquid.RpcPort,
		LiquidRpcWallet:        fileConf.Liquid.RpcWallet,
		LiquidEnabled:          fileConf.Liquid.Enabled,
		PolicyPath:             fileConf.PolicyPath,
	}, nil
}

func parseConfigFromInitMsg(plugin *glightning.Plugin) (*PeerswapClightningConfig, error) {
	dbpath, err := plugin.GetOption(dbOption)
	if err != nil {
		return nil, err
	}

	// bitcoin rpc settings
	bitcoinRpcHost, err := plugin.GetOption(bitcoinRpcHostOption)
	if err != nil {
		return nil, err
	}
	bitcoinRpcPortString, err := plugin.GetOption(bitcoinRpcPortOption)
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
	bitcoinRpcUser, err := plugin.GetOption(bitcoinRpcUserOption)
	if err != nil {
		return nil, err
	}
	bitcoinRpcPassword, err := plugin.GetOption(bitcoinRpcPasswordOption)
	if err != nil {
		return nil, err
	}
	bitcoinCookieFilePath, err := plugin.GetOption(bitcoinCookieFilePath)
	if err != nil {
		return nil, err
	}
	// liquid rpc settings
	liquidRpcHost, err := plugin.GetOption(liquidRpcHostOption)
	if err != nil {
		return nil, err
	}
	liquidRpcPortString, err := plugin.GetOption(liquidRpcPortOption)
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
	liquidRpcUser, err := plugin.GetOption(liquidRpcUserOption)
	if err != nil {
		return nil, err
	}

	liquidRpcPass, err := plugin.GetOption(liquidRpcPasswordOption)
	if err != nil {
		return nil, err
	}
	liquidRpcPassFile, err := plugin.GetOption(liquidRpcPasswordFilepathOption)
	liquidRpcWallet, err := plugin.GetOption(rpcWalletOption)
	if err != nil {
		return nil, err
	}
	if liquidRpcWallet == "dev_test" {
		idBytes := make([]byte, 8)
		_, _ = rand.Read(idBytes[:])
		liquidRpcWallet = hex.EncodeToString(idBytes)
	}

	liquidEnabled, err := plugin.GetBoolOption(liquidEnabledOption)
	if err != nil {
		return nil, err
	}

	// get policy path
	policyPath, err := plugin.GetOption(policyPathOption)
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
		LiquidEnabled:         liquidEnabled,
		BitcoinRpcHost:        bitcoinRpcHost,
		BitcoinRpcPort:        uint(bitcoinRpcPort),
		BitcoinRpcUser:        bitcoinRpcUser,
		BitcoinRpcPassword:    bitcoinRpcPassword,
		BitcoinCookieFilePath: bitcoinCookieFilePath,
		PolicyPath:            policyPath,
	}, nil
}

// GetConfig returns the peerswap config
func (cl *ClightningClient) GetConfig() (*PeerswapClightningConfig, error) {
	// If we have a config file path set, use this path to parse the config from
	// instead of the config that is returned by the `init` method of core
	// lightning.
	configFilePath, err := cl.Plugin.GetOption(configFilePathOption)
	if err != nil {
		return nil, err
	}

	var config *PeerswapClightningConfig
	if configFilePath != "" {
		log.Infof("Trying to parse config from file %s", configFilePath)
		config, err = parseConfigFromFile(configFilePath)
		if err != nil {
			return nil, err
		}
	} else {
		log.Infof("Trying to parse config from init msg")
		config, err = parseConfigFromInitMsg(cl.Plugin)
		if err != nil {
			return nil, err
		}
	}

	// Normalize config.
	// Todo: Move validation from main function to the config.

	// If the db path is not set we create a new database at the default
	// location that is in the same dir as the config file (if this was set),
	// otherwise we assume the database is in the working directory.
	//
	// It is recommended to create the db file separately to have control over
	// the file permissions.
	if config.DbPath == "" {
		var dir string
		if configFilePath != "" {
			// First option next to the config file if config file path is set:
			fp := filepath.FromSlash(configFilePath)
			dir = filepath.Dir(fp)
		} else {
			// Second option in the working directory
			dir, err = os.Getwd()
			if err != nil {
				return nil, err
			}
		}
		config.DbPath = filepath.Join(dir, defaultDbName)

		err = os.MkdirAll(config.DbPath, 0755)
		if err != nil && err != os.ErrExist {
			return nil, err
		}
	}

	// If the policy path is not set we create a new policy file at the default
	// location that is in the same dir as the config file (if this was set),
	// otherwise we assume the policy file in the working directory.
	//
	// It is recommended to create the db file separately to have control over
	// the file permissions.
	if config.PolicyPath == "" {
		var dir string
		if configFilePath != "" {
			// First option next to the config file if config file path is set:
			fp := filepath.FromSlash(configFilePath)
			dir = filepath.Dir(fp)
		} else {
			// Second option in the working directory
			dir, err = os.Getwd()
			if err != nil {
				return nil, err
			}
		}
		config.DbPath = filepath.Join(dir, defaultPolicyFileName)

		err = os.MkdirAll(config.DbPath, 0755)
		if err != nil && err != os.ErrExist {
			return nil, err
		}
	}

	// If no bitcoin config is set at all we use the config that core lightning
	// provides.
	// TODO: I don't like this kind of behavior, we should have a flag to
	// indicate if we want to use the lnd bitcoin config or a separate config.
	// As I don't want to break the current behavior I stick to the following:
	// If no bitcoin config is set at all -> use cln bitcoin config and assume
	// that bitcoin is a swap possibility.
	if config.BitcoinCookieFilePath == "" && config.BitcoinRpcHost == "" &&
		config.BitcoinRpcPassword == "" && config.BitcoinRpcPasswordFile == "" &&
		config.BitcoinRpcPort == 0 && config.BitcoinRpcUser == "" {
		log.Debugf("No bitcoin config set, injecting from cln")
		err = cl.injectBitcoinConfig(config)
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}

func (cl *ClightningClient) injectBitcoinConfig(conf *PeerswapClightningConfig) error {
	clnConf, err := cl.glightning.ListConfigs()
	if err != nil {
		return err
	}

	info, err := cl.glightning.GetInfo()
	if err != nil {
		return err
	}

	bconf, err := getBitcoinConfig(clnConf, info)
	if err != nil {
		return err
	}

	conf.BitcoinCookieFilePath = bconf.CookiePath
	conf.BitcoinRpcHost = bconf.RpcHost
	conf.BitcoinRpcPassword = bconf.RpcPassword
	conf.BitcoinRpcPort = bconf.RpcPort
	conf.BitcoinRpcUser = bconf.RpcUser

	return nil
}

func getBitcoinConfig(conf map[string]interface{}, info *glightning.NodeInfo) (*BitcoinConfig, error) {
	data, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}

	var listConfigResponse struct {
		ImportantPlugins []*struct {
			Path    string
			Name    string
			Options map[string]interface{}
		} `json:"important-plugins"`
	}
	err = json.Unmarshal(data, &listConfigResponse)
	if err != nil {
		return nil, err
	}

	bconf := &BitcoinConfig{}
	// Search the bcli plugin
	for _, plugin := range listConfigResponse.ImportantPlugins {
		if plugin.Name == "bcli" {
			// Read the configuration
			if v, ok := plugin.Options["bitcoin-datadir"]; ok {
				if v != nil {
					bconf.DataDir = v.(string)
				}
			}
			if v, ok := plugin.Options["bitcoin-rpcuser"]; ok {
				if v != nil {
					bconf.RpcUser = v.(string)
				}
			}
			if v, ok := plugin.Options["bitcoin-rpcpassword"]; ok {
				if v != nil {
					bconf.RpcPassword = v.(string)
				}
			}
			if v, ok := plugin.Options["bitcoin-rpcconnect"]; ok {
				if v != nil {
					bconf.RpcHost = v.(string)
				}
			}
			if v, ok := plugin.Options["bitcoin-rpcport"]; ok {
				if v != nil {
					port, err := strconv.Atoi(v.(string))
					if err != nil {
						return nil, err
					}
					bconf.RpcPort = uint(port)
				}
			}
			if v, ok := plugin.Options["network"]; ok {
				if v != nil {
					bconf.Network = v.(string)
				}
			}
			if _, ok := plugin.Options["mainnet"]; ok {
				bconf.Network = "bitcoin"
			}
			if _, ok := plugin.Options["testnet"]; ok {
				bconf.Network = "testnet"
			}
			if _, ok := plugin.Options["signet"]; ok {
				bconf.Network = "signet"
			}

			// Check if we know the network
			if bconf.Network == "" {
				// If not, try to get the network from the info call
				if info.Network == "" {
					return nil, fmt.Errorf("could not figure out which network to use")
				}
				bconf.Network = info.Network
			}

			// Normalize bconf. Set standard values if config is not set.
			if bconf.RpcHost == "" {
				bconf.RpcHost = "http://127.0.0.1"
			} else {
				addr, err := url.Parse(bconf.RpcHost)
				if err != nil {
					return nil, err
				}
				if addr.Scheme == "" {
					addr.Scheme = "http"
				}
				bconf.RpcHost = addr.String()
			}

			if bconf.DataDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				bconf.DataDir = filepath.Join(homeDir, ".bitcoin")
			}

			// If user and password are unset we might have a cookie file in the
			// datadir.
			if bconf.RpcUser == "" && bconf.RpcPassword == "" {
				// Look for the network dir
				var netdir string
				switch bconf.Network {
				case "bitcoin":
					netdir = ""
				case "regtest":
					netdir = "regtest"
				case "signet":
					netdir = "signet"
				case "testnet":
					netdir = "testnet3"
				default:
					return nil, fmt.Errorf("unknown network %s", netdir)
				}

				cookiePath := filepath.Join(bconf.DataDir, netdir, ".cookie")
				bconf.CookiePath = cookiePath
				rpcuser, rpcpass, err := readCookie(cookiePath)
				if err != nil {
					return nil, err
				}

				bconf.RpcUser = rpcuser
				bconf.RpcPassword = rpcpass
			}

			return bconf, nil
		}
	}

	return nil, fmt.Errorf("bcli configuration not found")
}

func readCookie(path string) (string, string, error) {
	cookieBytes, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	cookie := strings.Split(string(cookieBytes), ":")
	if len(cookie) != 2 {
		return "", "", fmt.Errorf("malformed cookie %v", cookieBytes)
	}

	return cookie[0], cookie[1], nil
}

// BitcoinConfig is an internally used struct that represents the data that can
// be fetched from core lightning.
type BitcoinConfig struct {
	DataDir     string
	RpcUser     string
	RpcPassword string
	RpcHost     string
	RpcPort     uint
	Cookie      string
	CookiePath  string
	Network     string
}
