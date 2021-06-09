package wallet

import (
	"errors"
	"github.com/btcsuite/btcd/btcec"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
)

type DummyWalletStore struct {
	privKey *btcec.PrivateKey
	pubkey *btcec.PublicKey

	initialized bool
}

func (d *DummyWalletStore) Initialize() error {
	if d.initialized {
		return errors.New("already initialized")
	}

	privkey, err := btcec.NewPrivateKey(btcec.S256())
	if err != nil {
		return err
	}
	d.privKey = privkey
	d.pubkey = privkey.PubKey()
	return nil
}

func (d *DummyWalletStore) LoadPrivKey() (*btcec.PrivateKey, error) {
	return d.privKey, nil
}

func (d *DummyWalletStore) ListAddresses() ([]string, error) {
	p2pkhBob := payment.FromPublicKey(d.pubkey, &network.Regtest, nil)
	address, err := p2pkhBob.PubKeyHash()
	if err != nil {
		return nil, err
	}
	return []string{address}, nil

}

type FileWalletStore struct {

}
