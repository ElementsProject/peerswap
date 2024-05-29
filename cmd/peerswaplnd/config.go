package peerswaplnd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/elementsproject/peerswap/lwk"
	"github.com/jessevdk/go-flags"
)

type LogLevel uint8

const (
	LOGLEVEL_INFO = LogLevel(iota + 1)
	LOGLEVEL_DEBUG
)

var (
	DefaultPeerswapHost   = "localhost:42069"
	DefaultRestHost       = "localhost:42070"
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
	RestHost   string   `long:"resthost" description:"host to listen for rest connection"`
	ConfigFile string   `long:"configfile" description:"path to configfile"`
	PolicyFile string   `long:"policyfile" description:"path to policyfile"`
	DataDir    string   `long:"datadir" description:"peerswap datadir"`
	LogLevel   LogLevel `long:"loglevel" description:"loglevel (1=Info, 2=Debug)"`

	LndConfig      *LndConfig     `group:"Lnd Grpc config" namespace:"lnd"`
	ElementsConfig *OnchainConfig `group:"Elements Rpc Config" namespace:"elementsd"`
	LWKConfig      *lwk.Conf

	LiquidEnabled  bool `long:"liquidswaps" description:"enable bitcoin peerswaps"`
	BitcoinEnabled bool `long:"bitcoinswaps" description:"enable bitcoin peerswaps"`
}

func (p *PeerSwapConfig) String() string {
	var liquidString string
	if p.ElementsConfig != nil {
		liquidString = fmt.Sprintf("elements: rpcuser: %s, rpchost: %s, rpcport %v, rpcwallet: %s, liquidswaps: %v", p.ElementsConfig.RpcUser, p.ElementsConfig.RpcHost, p.ElementsConfig.RpcPort, p.ElementsConfig.RpcWallet, p.ElementsConfig.LiquidSwaps)
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
	if p.ElementsConfig.RpcHost != "" && p.ElementsConfig.LiquidSwaps != false {
		err := p.ElementsConfig.Validate()
		if err != nil {
			return err
		}
		p.LiquidEnabled = true
	} else if p.LWKConfig.Enabled() {
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
	LiquidSwaps       bool   `long:"liquidswaps" description:"set to false to disable L-BTC swaps"`
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
		RestHost:   DefaultRestHost,
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
		LiquidSwaps:       true,
	}
}

func LWKFromIniFileConfig(filePath string) (*lwk.Conf, error) {
	type LWK struct {
		SignerName       string `long:"signername" description:"name of the signer"`
		WalletName       string `long:"walletname" description:"name of the wallet"`
		LWKEndpoint      string `long:"lwkendpoint" description:"endpoint for the liquid wallet kit"`
		ElectrumEndpoint string `long:"elementsendpoint" description:"endpoint for the elements rpc"`
		Network          string `long:"network" description:"network to use"`
		LiquidSwaps      bool   `long:"liquidswaps" description:"enable liquid swaps"`
	}
	type IniConf struct {
		LWK *LWK `group:"Elements Rpc Config" namespace:"lwk"`
	}
	cfg := &IniConf{}
	if _, err := os.Stat(filePath); err == nil {
		fileParser := flags.NewParser(cfg, flags.Default|flags.IgnoreUnknown)
		err = flags.NewIniParser(fileParser).ParseFile(filePath)
		if err != nil {
			return nil, err
		}
	}
	flagParser := flags.NewParser(cfg, flags.Default|flags.IgnoreUnknown)
	if _, err := flagParser.Parse(); err != nil {
		return nil, err
	}

	ln, err := lwk.NewlwkNetwork("liquid-testnet")
	if err != nil {
		return nil, err
	}

	if cfg.LWK.Network != "" {
		n, e := lwk.NewlwkNetwork(cfg.LWK.Network)
		if e != nil {
			return nil, e
		}
		ln = n
	}
	c, err := lwk.NewConfBuilder(ln).DefaultConf()
	if err != nil {
		return nil, err
	}
	if cfg.LWK.WalletName != "" {
		c.SetWalletName(lwk.NewConfName(cfg.LWK.WalletName))
	}
	if cfg.LWK.SignerName != "" {
		c.SetSignerName(lwk.NewConfName(cfg.LWK.SignerName))
	}
	if cfg.LWK.LWKEndpoint != "" {
		lwkEndpoint, err := lwk.NewLWKURL(cfg.LWK.LWKEndpoint)
		if err != nil {
			return nil, err
		}
		c.SetLWKEndpoint(*lwkEndpoint)
	}
	if cfg.LWK.ElectrumEndpoint != "" {
		electrumEndpoint, err := lwk.NewElectrsURL(cfg.LWK.ElectrumEndpoint)
		if err != nil {
			return nil, err
		}
		c.SetElectrumEndpoint(*electrumEndpoint)
	}
	return c.SetLiquidSwaps(cfg.LWK.LiquidSwaps).Build()
}
