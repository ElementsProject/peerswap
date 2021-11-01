package swap

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"go.etcd.io/bbolt"
)

var (
	swapBuckets          = []byte("swaps")
	requestedSwapsBucket = []byte("requested-swaps")

	ErrDoesNotExist  = fmt.Errorf("does not exist")
	ErrAlreadyExists = fmt.Errorf("swap already exist")
)

type bboltStore struct {
	db *bbolt.DB
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
	if err != nil {
		return nil, err
	}
	return swap, nil
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

func (p *bboltStore) ListAllByPeer(peer string) ([]*SwapStateMachine, error) {
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
		if swap.Data.PeerNodeId == peer {
			swaps = append(swaps, swap)
		}
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

type RequestedSwapsStore interface {
	Add(id string, reqswap RequestedSwap) error
	Get(id string) ([]RequestedSwap, error)
	GetAll() (map[string][]RequestedSwap, error)
}

type RequestedSwap struct {
	Asset           string   `json:"asset"`
	AmountMsat      uint64   `json:"amount_msat"`
	Type            SwapType `json:"swap_type"`
	RejectionReason string   `json:"rejection_reason"`
}
type requestedSwapsStore struct {
	db *bbolt.DB
}

func NewRequestedSwapsStore(db *bbolt.DB) (*requestedSwapsStore, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.CreateBucketIfNotExists(requestedSwapsBucket)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &requestedSwapsStore{db: db}, nil
}

func (s *requestedSwapsStore) Add(id string, reqswap RequestedSwap) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(requestedSwapsBucket)
		k := b.Get([]byte(id))

		var reqswaps []RequestedSwap
		err := json.Unmarshal(k, &reqswaps)
		if err != nil {
			return err
		}

		if reqswaps == nil {
			reqswaps = make([]RequestedSwap, 1)
			reqswaps[0] = reqswap
		} else {
			reqswaps = append(reqswaps, reqswap)
		}

		buf, err := json.Marshal(reqswaps)
		if err != nil {
			return err
		}

		return b.Put([]byte(id), buf)
	})
}

func (s *requestedSwapsStore) GetAll() (map[string][]RequestedSwap, error) {
	reqswaps := map[string][]RequestedSwap{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(requestedSwapsBucket)
		return b.ForEach(func(k, v []byte) error {
			id := string(k)
			var reqswap []RequestedSwap
			json.Unmarshal(v, &reqswap)
			reqswaps[id] = reqswap
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	return reqswaps, nil
}

func (s *requestedSwapsStore) Get(id string) ([]RequestedSwap, error) {
	var reqswaps []RequestedSwap
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(requestedSwapsBucket)
		k := b.Get([]byte(id))
		return json.Unmarshal(k, &reqswaps)
	})
	if err != nil {
		return nil, err
	}

	return reqswaps, nil
}
