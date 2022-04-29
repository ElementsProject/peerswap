package peerswaplnd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/btcsuite/btcutil"
)

type LogLevel uint8

const (
	LOGLEVEL_INFO = LogLevel(iota + 1)
	LOGLEVEL_DEBUG
)

var (
	DefaultPeerswapHost   = "localhost:42069"
	DefaultLndHost        = "localhost:10009"
	DefaultTlsCertPath    = filepath.Join(defaultLndDir, "tls.cert")
	DefaultMacaroonPath   = filepath.Join(defaultLndDir, "data", "chain", "bitcoin", DefaultNetwork, "admin.macaroon")
	DefaultNetwork        = "signet"
	DefaultConfigFile     = filepath.Join(DefaultDatadir, "peerswap.conf")
	DefaultDatadir        = btcutil.AppDataDir("peerswap", false)
	DefaultLiquidwallet   = "swap"
	DefaultBitcoinEnabled = true
	DefaultLogLevel       = LOGLEVEL_DEBUG
	DefaultPolicyFile     = filepath.Join(DefaultDatadir, "policy.conf")

	defaultLndDir = btcutil.AppDataDir("lnd", false)
)

type PeerSwapConfig struct {
	Host       string   `long:"host" description:"host to listen on for grpc connections"`
	ConfigFile string   `long:"configfile" description:"path to configfile"`
	PolicyFile string   `long:"policyfile" description:"path to policyfile"`
	DataDir    string   `long:"datadir" description:"peerswap datadir"`
	LogLevel   LogLevel `long:"loglevel" description:"loglevel (1=Info, 2=Debug)"`

	LndConfig      *LndConfig     `group:"Lnd Grpc config" namespace:"lnd"`
	ElementsConfig *OnchainConfig `group:"Elements Rpc Config" namespace:"elementsd"`

	LiquidEnabled  bool
	BitcoinEnabled bool `long:"bitcoinswaps" description:"enable bitcoin peerswaps"`
}

func (p *PeerSwapConfig) String() string {
	var liquidString string
	if p.ElementsConfig != nil {
		liquidString = fmt.Sprintf("elements: rpcuser: %s, rpchost: %s, rpcport %v, rpcwallet: %s", p.ElementsConfig.RpcUser, p.ElementsConfig.RpcHost, p.ElementsConfig.RpcPort, p.ElementsConfig.RpcWallet)
	}
	var lndString string
	if p.LndConfig != nil {
		lndString = fmt.Sprintf("host: %s, macaroonpath %s, tlspath %s", p.LndConfig.LndHost, p.LndConfig.MacaroonPath, p.LndConfig.TlsCertPath)
	}

	if p.DataDir != DefaultDatadir && p.PolicyFile == DefaultPolicyFile {
		p.PolicyFile = filepath.Join(p.DataDir, "policy.conf")
	}

	return fmt.Sprintf("Host %s, ConfigFile %s, Datadir %s, Bitcoin enabled: %v, Lnd Config: %s, elements: %s", p.Host, p.ConfigFile, p.DataDir, p.BitcoinEnabled, lndString, liquidString)
}

func (p *PeerSwapConfig) Validate() error {
	if p.ElementsConfig.RpcHost != "" {
		err := p.ElementsConfig.Validate()
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
	RpcWallet         string `long:"rpcwallet" description:"wallet to use for swaps (elements only)"`
}

func (o *OnchainConfig) Validate() error {
	if (o.RpcCookieFilePath == "" && o.RpcUser == "") && o.RpcHost != "" {
		return errors.New("either rpcuser or cookie file must be set")
	}
	if o.RpcUser == "" {
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
		PolicyFile: DefaultPolicyFile,
		DataDir:    DefaultDatadir,
		LndConfig: &LndConfig{
			LndHost:      DefaultLndHost,
			TlsCertPath:  DefaultTlsCertPath,
			MacaroonPath: DefaultMacaroonPath,
		},
		BitcoinEnabled: DefaultBitcoinEnabled,
		ElementsConfig: defaultLiquidConfig(),
		LogLevel:       DefaultLogLevel,
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
