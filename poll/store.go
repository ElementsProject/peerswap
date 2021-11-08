package poll

import (
	"encoding/json"
	"time"

	"go.etcd.io/bbolt"
)

var POLL_BUCKET = []byte("poll-list")

type pollStore struct {
	db *bbolt.DB
}

func NewPollStore(db *bbolt.DB) (*pollStore, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.CreateBucketIfNotExists(POLL_BUCKET)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &pollStore{db: db}, nil
}

func (s *pollStore) Update(peerId string, info PollInfo) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		infoBytes, err := json.Marshal(info)
		if err != nil {
			return err
		}

		b := tx.Bucket(POLL_BUCKET)
		return b.Put([]byte(peerId), infoBytes)
	})
}

func (s *pollStore) GetAll() (map[string]PollInfo, error) {
	pollinfos := map[string]PollInfo{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(POLL_BUCKET)
		return b.ForEach(func(k, v []byte) error {
			peerId := string(k)
			var info PollInfo
			err := json.Unmarshal(v, &info)
			if err != nil {
				return err
			}
			pollinfos[peerId] = info
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	return pollinfos, nil
}

func (s *pollStore) RemoveUnseen(olderThan time.Duration) error {
	now := time.Now()
	return s.db.Update(func(t *bbolt.Tx) error {
		b := t.Bucket(POLL_BUCKET)
		return b.ForEach(func(k, v []byte) error {
			var info PollInfo
			err := json.Unmarshal(v, &info)
			if err != nil {
				return err
			}
			if now.Sub(info.LastSeen) > olderThan {
				b.Delete(k)
			}
			return nil
		})
	})
}
