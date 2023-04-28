package clightning

import (
	"encoding/json"
)

const (
	liquidRpcHostOption             = "peerswap-elementsd-rpchost"
	liquidRpcPortOption             = "peerswap-elementsd-rpcport"
	liquidRpcUserOption             = "peerswap-elementsd-rpcuser"
	liquidRpcPasswordOption         = "peerswap-elementsd-rpcpassword"
	liquidRpcPasswordFilepathOption = "peerswap-elementsd-rpcpasswordfile"
	liquidDisabledOption            = "peerswap-elementsd-disabled"
	liquidRpcWalletOption           = "peerswap-elementsd-rpcwallet"

	bitcoinRpcHostOption     = "peerswap-bitcoin-rpchost"
	bitcoinRpcPortOption     = "peerswap-bitcoin-rpcport"
	bitcoinRpcUserOption     = "peerswap-bitcoin-rpcuser"
	bitcoinRpcPasswordOption = "peerswap-bitcoin-rpcpassword"
	bitcoinCookieFilePath    = "peerswap-bitcoin-cookiefilepath"

	policyPathOption = "peerswap-policy-path"
)

var legacyOptions = []string{
	liquidRpcHostOption,
	liquidRpcPortOption,
	liquidRpcUserOption,
	liquidRpcPasswordOption,
	liquidRpcPasswordFilepathOption,
	liquidDisabledOption,
	liquidRpcWalletOption,
	bitcoinRpcHostOption,
	bitcoinRpcPortOption,
	bitcoinRpcUserOption,
	bitcoinRpcPasswordOption,
	bitcoinCookieFilePath,
	policyPathOption,
}

// PeerswapClightningConfig contains relevant config params for peerswap
type PeerswapClightningConfig struct {
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
	LiquidDisabled        bool   `json:"liquid.disabled"`

	PeerswapDir string `json:"peerswap-dir"`
}

func (c PeerswapClightningConfig) String() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// RegisterOptions adds options to core-lightning. All these options
// are deprecated, we just keep them to notify people if an option was
// passed to core-lightning
func (cl *ClightningClient) RegisterOptions() error {
	err := cl.Plugin.RegisterNewOption(bitcoinRpcHostOption, "bitcoind rpchost", "")
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
	err = cl.Plugin.RegisterNewOption(liquidRpcHostOption, "elementsd rpchost", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidRpcPortOption, "elementsd rpcport", "")
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
	err = cl.Plugin.RegisterNewOption(liquidRpcWalletOption, "liquid-rpcwallet", "")
	if err != nil {
		return err
	}
	err = cl.Plugin.RegisterNewOption(liquidRpcPasswordFilepathOption, "elementsd rpcpassword filepath", "")
	if err != nil {
		return err
	}

	err = cl.Plugin.RegisterNewBoolOption(liquidDisabledOption, "enable/disable liquid", false)
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
