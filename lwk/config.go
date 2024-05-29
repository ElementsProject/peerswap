package lwk

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/vulpemventures/go-elements/network"
)

type Conf struct {
	signerName       confname
	walletName       confname
	lwkEndpoint      lwkurl
	electrumEndpoint electsurl
	network          LwkNetwork
	liquidSwaps      bool
}

func (c *Conf) GetSignerName() string {
	return c.signerName.String()
}

func (c *Conf) GetWalletName() string {
	return c.walletName.String()
}

func (c *Conf) GetLWKEndpoint() string {
	return c.lwkEndpoint.String()
}

func (c *Conf) GetElectrumEndpoint() string {
	return c.electrumEndpoint.Host
}

func (c *Conf) IsElectrumWithTLS() bool {
	return c.electrumEndpoint.Scheme == "ssl"
}

func (c *Conf) GetNetwork() string {
	return c.network.String()
}

func (c *Conf) GetLiquidSwaps() bool {
	return c.liquidSwaps
}

func (c *Conf) GetChain() *network.Network {
	switch c.network {
	case NetworkMainnet:
		return &network.Liquid
	case NetworkRegtest:
		return &network.Regtest
	case NetworkTestnet:
		return &network.Testnet
	default:
		return &network.Testnet
	}
}

func (c *Conf) GetAssetID() string {
	switch c.network {
	case NetworkMainnet:
		return network.Liquid.AssetID
	case NetworkRegtest:
		return network.Regtest.AssetID
	case NetworkTestnet:
		return network.Testnet.AssetID
	default:
		return network.Testnet.AssetID
	}
}

func (c *Conf) Enabled() bool {
	return Validate(
		c.electrumEndpoint,
		c.lwkEndpoint,
		c.network,
		c.signerName,
		c.walletName) == nil && c.liquidSwaps
}

type LwkNetwork string

const (
	NetworkMainnet LwkNetwork = "liquid"
	NetworkTestnet LwkNetwork = "liquid-testnet"
	NetworkRegtest LwkNetwork = "liquid-regtest"
)

func (n LwkNetwork) String() string {
	return string(n)
}

func NewlwkNetwork(lekNetwork string) (LwkNetwork, error) {
	switch lekNetwork {
	case "liquid":
		return NetworkMainnet, nil
	case "liquid-testnet":
		return NetworkTestnet, nil
	case "liquid-regtest":
		return NetworkRegtest, nil
	default:
		return "", fmt.Errorf("expected liquid, liquid-testnet or liquid-regtest, got %s", lekNetwork)
	}
}

func (n LwkNetwork) validate() error {
	switch n {
	case NetworkMainnet, NetworkTestnet, NetworkRegtest:
		return nil
	default:
		return errors.New("invalid network")
	}
}

type confname string

func NewConfName(name string) confname {
	return confname(name)
}

func (n confname) validate() error {
	if n == "" {
		return errors.New("name must be set")
	}
	return nil
}

func (n confname) String() string {
	return string(n)
}

type lwkurl struct {
	*url.URL
}

func NewLWKURL(endpoint string) (*lwkurl, error) {
	u, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return nil, err
	}
	return &lwkurl{u}, nil
}

func (u lwkurl) validate() error {
	if u.URL == nil {
		return errors.New("url must be set")
	}
	if u.URL.String() == "" {
		return errors.New("could not parse url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("expected http or https scheme, got %s", u.Scheme)
	}
	return nil
}

type electsurl struct {
	*url.URL
}

func NewElectrsURL(endpoint string) (*electsurl, error) {
	u, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return nil, err
	}
	return &electsurl{u}, nil
}

func (u electsurl) validate() error {
	if u.URL == nil {
		return errors.New("url must be set")
	}
	if u.URL.String() == "" {
		return errors.New("could not parse url")
	}
	if u.Scheme != "ssl" && u.Scheme != "tcp" {
		return fmt.Errorf("expected ssl or tcp scheme, got %s", u.Scheme)
	}
	return nil
}
