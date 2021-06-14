package wallet

import (
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"

	"go.etcd.io/bbolt"
)

var (
	keyStoreBucketName = []byte("keys")
	privkeyId = []byte("privkey")
	ErrDoesNotExist  = fmt.Errorf("does not exist")
)

type bboltStore struct {
	db *bbolt.DB

	privKey *btcec.PrivateKey
	pubkey  *btcec.PublicKey
}

func (p *bboltStore) Initialize() error {
	key, err := p.LoadPrivKey()
	if err != ErrDoesNotExist && err != nil{
		return err
	}
	if err == ErrDoesNotExist {
		key, err = btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			return err
		}
		err = p.savePrivkey(key)
		if err != nil {
			return err
		}
	}
	p.privKey = key
	p.pubkey = p.privKey.PubKey()
	return nil
}

func (p *bboltStore) savePrivkey(key *btcec.PrivateKey) error{
	tx, err := p.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(keyStoreBucketName)
	if b == nil {
		return errors.New("bucket is nil")
	}

	err = b.Put([]byte("privkey"), key.Serialize())
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (p *bboltStore) LoadPrivKey() (*btcec.PrivateKey, error) {
	tx, err := p.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	b := tx.Bucket(keyStoreBucketName)
	if b == nil {
		return nil, fmt.Errorf("bucket nil")
	}

	jData := b.Get(privkeyId)
	if jData == nil {
		return nil, ErrDoesNotExist
	}

	privkey, _ := btcec.PrivKeyFromBytes(btcec.S256(), jData)

	return privkey, nil
}

func (p *bboltStore) ListAddresses() ([]string, error) {
	p2pkhBob := payment.FromPublicKey(p.pubkey, &network.Liquid, nil)
	address, err := p2pkhBob.PubKeyHash()
	if err != nil {
		return nil, err
	}
	return []string{address}, nil
}

func NewBboltStore(db *bbolt.DB) (*bboltStore, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.CreateBucketIfNotExists(keyStoreBucketName)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &bboltStore{db: db}, nil
}
