package lwk

import (
	"errors"
	"net/url"

	"github.com/vulpemventures/go-elements/network"
)

type Conf struct {
	signerName       confname
	walletName       confname
	lwkEndpoint      confurl
	electrumEndpoint confurl
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
		return "", errors.New("invalid network")
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

type confurl struct {
	*url.URL
}

func NewConfURL(endpoint string) (*confurl, error) {
	u, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return nil, err
	}
	return &confurl{u}, nil
}

func (n confurl) validate() error {
	if n.URL == nil {
		return errors.New("url must be set")
	}
	if n.URL.String() == "" {
		return errors.New("could not parse url")
	}
	return nil
}
