package version

import (
	"fmt"

	"go.etcd.io/bbolt"
)

var (
	versionBucket = []byte("version")

	ErrDoesNotExist = fmt.Errorf("does not exist")
)

type versionStore struct {
	db *bbolt.DB
}

func NewVersionStore(db *bbolt.DB) (*versionStore, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.CreateBucketIfNotExists(versionBucket)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &versionStore{db: db}, nil
}

func (vs *versionStore) GetVersion() (string, error) {
	tx, err := vs.db.Begin(false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	b := tx.Bucket(versionBucket)
	if b == nil {
		return "", fmt.Errorf("bucket nil")
	}

	jData := b.Get([]byte("version"))
	if jData == nil {
		return "", ErrDoesNotExist
	}

	return string(jData), nil
}

func (vs *versionStore) SetVersion(version string) error {
	tx, err := vs.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(versionBucket)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}

	if err := b.Put([]byte("version"), []byte(version)); err != nil {
		return err
	}

	return tx.Commit()
}
