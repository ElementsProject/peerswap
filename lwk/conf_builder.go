package lwk

type confBuilder struct {
	Conf
}

func NewConfBuilder(network LwkNetwork) *confBuilder {
	return &confBuilder{
		Conf: Conf{
			network: network,
		},
	}
}

func (b *confBuilder) DefaultConf() (*confBuilder, error) {
	var (
		lwkEndpoint, electrumEndpoint string
	)
	switch b.network {
	case NetworkTestnet:
		lwkEndpoint = "http://localhost:32111"
		electrumEndpoint = "tcp://blockstream.info:465"
	case NetworkRegtest:
		lwkEndpoint = "http://localhost:32112"
		electrumEndpoint = "tcp://localhost:60401"
	default:
		// mainnet is the default port
		lwkEndpoint = "http://localhost:32110"
		electrumEndpoint = "tcp://blockstream.info:995"
	}
	lwkURL, err := NewConfURL(lwkEndpoint)
	if err != nil {
		return nil, err
	}
	elementsURL, err := NewConfURL(electrumEndpoint)
	if err != nil {
		return nil, err
	}
	b.signerName = "defaultPeerswapSigner"
	b.walletName = "defaultPeerswapWallet"
	b.lwkEndpoint = *lwkURL
	b.electrumEndpoint = *elementsURL
	return b, nil
}

func (b *confBuilder) SetSignerName(name confname) *confBuilder {
	b.signerName = name
	return b
}

func (b *confBuilder) SetWalletName(name confname) *confBuilder {
	b.walletName = name
	return b
}

func (b *confBuilder) SetLWKEndpoint(endpoint confurl) *confBuilder {
	b.lwkEndpoint = endpoint
	return b
}

func (b *confBuilder) SetElectrumEndpoint(endpoint confurl) *confBuilder {
	b.electrumEndpoint = endpoint
	return b
}

func (b *confBuilder) SetLiquidSwaps(swaps bool) *confBuilder {
	b.liquidSwaps = swaps
	return b
}

func (b *confBuilder) Build() (*Conf, error) {
	if err := Validate(
		b.electrumEndpoint,
		b.lwkEndpoint,
		b.network,
		b.signerName,
		b.walletName); err != nil {
		return nil, err
	}
	return &Conf{
		signerName:       b.signerName,
		walletName:       b.walletName,
		lwkEndpoint:      b.lwkEndpoint,
		electrumEndpoint: b.electrumEndpoint,
		network:          b.network,
		liquidSwaps:      b.liquidSwaps,
	}, nil
}
