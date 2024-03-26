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
	network          lwkNetwork
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
	return c.electrumEndpoint.String()
}

func (c *Conf) GetNetwork() string {
	return c.network.String()
}

func (c *Conf) GetLiquidSwaps() bool {
	return c.liquidSwaps
}

func (c *Conf) GetChain() *network.Network {
	switch c.network {
	case networkMainnet:
		return &network.Liquid
	case networkRegtest:
		return &network.Regtest
	case networkTestnet:
		return &network.Testnet
	default:
		return &network.Testnet
	}
}

type lwkNetwork string

const (
	networkMainnet lwkNetwork = "liquid"
	networkTestnet lwkNetwork = "liquid-testnet"
	networkRegtest lwkNetwork = "liquid-regtest"
)

func (n lwkNetwork) String() string {
	return string(n)
}

func NewlwkNetwork(lekNetwork string) (lwkNetwork, error) {
	switch lekNetwork {
	case "liquid":
		return networkMainnet, nil
	case "liquid-testnet":
		return networkTestnet, nil
	case "liquid-regtest":
		return networkRegtest, nil
	default:
		return "", errors.New("invalid network")
	}
}

func (n lwkNetwork) validate() error {
	switch n {
	case networkMainnet, networkTestnet, networkRegtest:
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
	u, err := url.Parse(endpoint)
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
