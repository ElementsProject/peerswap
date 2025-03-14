package premium

import (
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
	bolt "go.etcd.io/bbolt"
)

const (
	bucketName    = "premium"
	defaultPeerID = "default"
)

// ErrRateNotFound is returned when a rate is not found in the database.
var ErrRateNotFound = errors.New("Rate not found")

type BBoltPremiumStore struct {
	db *bolt.DB
}

func NewBBoltPremiumStore(db *bbolt.DB) (*BBoltPremiumStore, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.CreateBucketIfNotExists([]byte(bucketName))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &BBoltPremiumStore{db: db}, err
}

// SetRate sets the premium rate for a given peer, asset, and operation.
func (p *BBoltPremiumStore) SetRate(peer string, rate *PremiumRate) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("Bucket not found")
		}
		key := fmt.Sprintf("%s.%d.%d", peer, rate.Asset(), rate.Operation())
		value := []byte(fmt.Sprintf("%d", rate.PremiumRatePPM().Value()))
		return bucket.Put([]byte(key), value)
	})
}

// DeleteRate removes the premium rate for a given peer, asset, and operation.
func (p *BBoltPremiumStore) DeleteRate(peer string, asset AssetType, operation OperationType) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("Bucket not found")
		}
		key := fmt.Sprintf("%s.%d.%d", peer, asset, operation)
		return bucket.Delete([]byte(key))
	})
}

// GetRate retrieves the premium rate for a given peer, asset, and operation.
// If the rate is not found, it tries to retrieve the default rate.
func (p *BBoltPremiumStore) GetRate(peer string, asset AssetType, operation OperationType) (*PremiumRate, error) {
	var rate int64
	err := p.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("Bucket not found")
		}
		key := fmt.Sprintf("%s.%d.%d", peer, asset, operation)
		value := bucket.Get([]byte(key))
		if value == nil {
			return ErrRateNotFound
		}
		_, err := fmt.Sscanf(string(value), "%d", &rate)
		return err
	})
	if err != nil {
		return nil, err
	}
	return NewPremiumRate(asset, operation, NewPPM(rate))
}

// SetDefaultRate sets the default premium rate for a given asset and operation.
func (p *BBoltPremiumStore) SetDefaultRate(rate *PremiumRate) error {
	return p.SetRate(defaultPeerID, rate)
}

// GetDefaultRate retrieves the default premium rate for a given asset and operation.
func (p *BBoltPremiumStore) GetDefaultRate(asset AssetType, operation OperationType) (*PremiumRate, error) {
	return p.GetRate(defaultPeerID, asset, operation)
}
