package peerswaplnd

import (
	"errors"
	"fmt"
	"github.com/btcsuite/btcutil"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	DefaultPeerswapHost = "localhost:42069"
	DefaultLndHost      = "localhost:10009"
	DefaultTlsCertPath  = filepath.Join(defaultLndDir, "tls.cert")
	DefaultMacaroonPath = filepath.Join(defaultLndDir, "data", "chain", "bitcoin", DefaultNetwork, "admin.macaroon")
	DefaultNetwork      = "signet"
	DefaultConfigFile   = filepath.Join(DefaultDatadir, "peerswap.conf")
	DefaultDatadir      = btcutil.AppDataDir("peerswap", false)
	DefaultLiquidwallet = "swap"

	defaultLndDir = btcutil.AppDataDir("lnd", false)
)

type PeerSwapConfig struct {
	Host       string `long:"host" description:"host to listen on for grpc connections"`
	ConfigFile string `long:"configfile" description:"path to configfile"`
	DataDir    string `long:"datadir" description:"peerswap datadir"`

	Network      string         `long:"network" description:"bitcoin network the component will run on" choice:"regtest" choice:"testnet" choice:"mainnet" choice:"simnet"`
	LndConfig    *LndConfig     `group:"Lnd Grpc config" namespace:"lnd"`
	LiquidConfig *OnchainConfig `group:"Liquid Rpc Config" namespace:"liquid"`

	LiquidEnabled  bool
	BitcoinEnabled bool `long:"bitcoinswaps" description:"enable bitcoin peerswaps"`
}

func (p *PeerSwapConfig) String() string {
	var bitcoindString, liquidString string
	if p.LiquidConfig != nil {
		liquidString = fmt.Sprintf("liquid: rpcuser: %s, rpchost: %s, rpcport %v, rpcwallet: %s", p.LiquidConfig.RpcUser, p.LiquidConfig.RpcHost, p.LiquidConfig.RpcPort, p.LiquidConfig.RpcWallet)
	}

	return fmt.Sprintf("Host %s, ConfigFile %s, Datadir %s, Network %s Bitcoind: %s, Liquid: %s", p.Host, p.ConfigFile, p.DataDir, p.Network, bitcoindString, liquidString)
}

func (p *PeerSwapConfig) Validate() error {
	if p.LiquidConfig.RpcHost != "" {
		err := p.LiquidConfig.Validate()
		if err != nil {
			return err
		}
		p.LiquidEnabled = true
	}
	return nil
}

type OnchainConfig struct {
	RpcUser           string `long:"rpcuser" description:"rpc user"`
	RpcPassword       string `long:"rpcpass" description:"password for rpc user"`
	RpcPasswordFile   string `long:"rpcpasswordfile" description:"file that cointains password for rpc user"`
	RpcCookieFilePath string `long:"rpccookiefilepath" description:"path to rpc cookie file"`
	RpcHost           string `long:"rpchost" description:"host to connect to"`
	RpcPort           uint   `long:"rpcport" description:"port to connect to"`
	RpcWallet         string `long:"rpcwallet" description:"wallet to use for swaps (liquid only)"`
}

func (o *OnchainConfig) Validate() error {
	if (o.RpcCookieFilePath == "" && o.RpcUser == "") && o.RpcHost != "" {
		return errors.New("either rpcuser or cookie file must be set")
	}
	if o.RpcUser == "" {
		log.Printf("looking for bitcoin cookie")
		// look for cookie file
		cookiePath := filepath.Join(o.RpcCookieFilePath)
		if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
			return errors.New(fmt.Sprintf("cannot find bitcoin cookie file at %s", cookiePath))
		}
		cookieBytes, err := os.ReadFile(cookiePath)
		if err != nil {
			return err
		}

		cookie := strings.Split(string(cookieBytes), ":")
		// use cookie for auth
		o.RpcUser = cookie[0]
		o.RpcPassword = cookie[1]
	} else {
		if !(o.RpcPassword == "" || o.RpcPasswordFile == "") {
			return errors.New("rpcpass or rpcpasswordfile must be set")
		}
		if o.RpcPasswordFile != "" {
			passBytes, err := ioutil.ReadFile(o.RpcPasswordFile)
			if err != nil {
				log.Printf("error reading file: %v", err)
				return err
			}
			passString := strings.TrimRight(string(passBytes), "\r\n")
			o.RpcPassword = passString
		}
	}
	return nil
}

type LndConfig struct {
	LndHost      string `long:"host" description:"host:port for lnd connection"`
	TlsCertPath  string `long:"tlscertpath" description:"path to the lnd TLS cert."`
	MacaroonPath string `long:"macaroonpath" description:"path to the macaroon (admin.macaroon or custom baked one)"`
}

func DefaultConfig() *PeerSwapConfig {
	return &PeerSwapConfig{
		Host:       DefaultPeerswapHost,
		ConfigFile: DefaultConfigFile,
		DataDir:    DefaultDatadir,
		Network:    DefaultNetwork,
		LndConfig: &LndConfig{
			LndHost:      DefaultLndHost,
			TlsCertPath:  DefaultTlsCertPath,
			MacaroonPath: DefaultMacaroonPath,
		},
		BitcoinEnabled: true,
		LiquidConfig:   defaultLiquidConfig(),
	}
}

func defaultLiquidConfig() *OnchainConfig {
	return &OnchainConfig{
		RpcUser:           "",
		RpcPassword:       "",
		RpcPasswordFile:   "",
		RpcCookieFilePath: "",
		RpcHost:           "",
		RpcPort:           0,
		RpcWallet:         DefaultLiquidwallet,
	}
}
