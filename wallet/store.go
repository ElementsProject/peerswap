package wallet

import (
	"github.com/btcsuite/btcd/btcec"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
)

type DummyWalletStore struct {
	PrivKey *btcec.PrivateKey
}

func (d *DummyWalletStore) LoadPrivKey() (*btcec.PrivateKey, error) {
	return d.PrivKey, nil
}

func (d *DummyWalletStore) ListAddresses() ([]string, error) {
	pubkey := d.PrivKey.PubKey()
	p2pkhBob := payment.FromPublicKey(pubkey, &network.Regtest, nil)
	address, err := p2pkhBob.PubKeyHash()
	if err != nil {
		return nil, err
	}

	return []string{address}, nil

}
