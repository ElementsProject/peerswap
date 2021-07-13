package swap

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go.etcd.io/bbolt"
)

var (
	swapBuckets = []byte("swaps")

	ErrDoesNotExist  = fmt.Errorf("does not exist")
	ErrAlreadyExists = fmt.Errorf("swap already exist")
)

type bboltStore struct {
	db *bbolt.DB
}

func (p *bboltStore) UpdateData(swap *SwapStateMachine) error {
	err := p.Update(swap)
	if err == ErrDoesNotExist {
		err = nil
		err = p.Create(swap)
	}
	if err != nil {
		return err
	}
	return nil

}

func (p *bboltStore) GetData(id string) (*SwapStateMachine, error) {
	swap, err := p.GetById(id)
	if err == ErrDoesNotExist {
		return nil, ErrDataNotAvailable
	}
	return swap, nil
}

func NewBboltStore(db *bbolt.DB) (*bboltStore, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.CreateBucketIfNotExists(swapBuckets)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &bboltStore{db: db}, nil
}

func (p *bboltStore) Create(swap *SwapStateMachine) error {
	exists, err := p.idExists(swap.Id)
	if err != nil {
		return err
	}
	if exists {
		return ErrAlreadyExists
	}

	tx, err := p.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(swapBuckets)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}

	jData, err := json.Marshal(swap)
	if err != nil {
		return err
	}

	if err := b.Put(h2b(swap.Id), jData); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *bboltStore) Update(swap *SwapStateMachine) error {
	exists, err := p.idExists(swap.Id)
	if err != nil {
		return err
	}
	if !exists {
		return ErrDoesNotExist
	}
	tx, err := p.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(swapBuckets)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}
	jData, err := json.Marshal(swap)
	if err != nil {
		return err
	}

	if err := b.Put(h2b(swap.Id), jData); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *bboltStore) DeleteById(s string) error {
	tx, err := p.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(swapBuckets)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}

	if err := b.Delete(h2b(s)); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *bboltStore) GetById(s string) (*SwapStateMachine, error) {
	tx, err := p.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	b := tx.Bucket(swapBuckets)
	if b == nil {
		return nil, fmt.Errorf("bucket nil")
	}

	jData := b.Get(h2b(s))
	if jData == nil {
		return nil, ErrDoesNotExist
	}

	swap := &SwapStateMachine{}
	if err := json.Unmarshal(jData, swap); err != nil {
		return nil, err
	}

	return swap, nil
}

func (p *bboltStore) ListAll() ([]*SwapStateMachine, error) {
	tx, err := p.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	b := tx.Bucket(swapBuckets)
	if b == nil {
		return nil, fmt.Errorf("bucket nil")
	}
	var swaps []*SwapStateMachine
	err = b.ForEach(func(k, v []byte) error {

		swap := &SwapStateMachine{}
		if err := json.Unmarshal(v, swap); err != nil {
			return err
		}
		swaps = append(swaps, swap)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return swaps, nil
}

func (p *bboltStore) idExists(id string) (bool, error) {
	_, err := p.GetById(id)
	if err != nil {
		if err == ErrDoesNotExist {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
