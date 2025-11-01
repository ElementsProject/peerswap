package peersync

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	// ErrPeerNotFound is returned when a peer is not present in the store.
	ErrPeerNotFound = errors.New("peer not found")

	pollBucketName       = []byte("poll-list")
	errPollBucketMissing = errors.New("poll bucket missing")
)

// Store persists peer state snapshots backed by BoltDB.
type Store struct {
	db *bolt.DB
}

// NewStore opens (or creates) a Bolt database at the provided path.
func NewStore(path string) (*Store, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(pollBucketName)
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init peers bucket: %w", err)
	}

	return &Store{db: db}, nil
}

// Close gracefully closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SavePeerState persists or updates the state of a peer.
func (s *Store) SavePeerState(peer *Peer) error {
	if peer == nil {
		return errors.New("peer is nil")
	}

	record := peerToRecord(peer)

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal peer %s: %w", peer.ID().String(), err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pollBucketName)
		if bucket == nil {
			return errPollBucketMissing
		}
		return bucket.Put([]byte(peer.ID().String()), data)
	})
}

// GetPeerState retrieves the stored state for a peer.
func (s *Store) GetPeerState(id PeerID) (*Peer, error) {
	var record peerRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pollBucketName)
		if bucket == nil {
			return errPollBucketMissing
		}

		data := bucket.Get([]byte(id.String()))
		if data == nil {
			return ErrPeerNotFound
		}

		if err := json.Unmarshal(data, &record); err != nil {
			return fmt.Errorf("unmarshal peer %s: %w", id.String(), err)
		}
		return nil
	})

	if errors.Is(err, ErrPeerNotFound) {
		return nil, ErrPeerNotFound
	}

	if err != nil {
		if errors.Is(err, errPollBucketMissing) {
			return nil, ErrPeerNotFound
		}
		return nil, err
	}

	peer, err := record.toPeer(id.String())
	if err != nil {
		return nil, fmt.Errorf("materialize peer %s: %w", id.String(), err)
	}

	return peer, nil
}

// GetAllPeerStates lists the state for every stored peer.
func (s *Store) GetAllPeerStates() ([]*Peer, error) {
	peers := make([]*Peer, 0)

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pollBucketName)
		if bucket == nil {
			return errPollBucketMissing
		}

		return bucket.ForEach(func(k, v []byte) error {
			var record peerRecord
			if err := json.Unmarshal(v, &record); err != nil {
				return fmt.Errorf("unmarshal peer %s: %w", string(k), err)
			}

			peer, err := record.toPeer(string(k))
			if err != nil {
				return fmt.Errorf("materialize peer %s: %w", string(k), err)
			}

			peers = append(peers, peer)
			return nil
		})
	})

	if errors.Is(err, errPollBucketMissing) {
		return peers, nil
	}

	if err != nil {
		return nil, err
	}

	return peers, nil
}

// RemovePeerState deletes the stored state for a peer.
func (s *Store) RemovePeerState(id PeerID) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pollBucketName)
		if bucket == nil {
			return errPollBucketMissing
		}
		return bucket.Delete([]byte(id.String()))
	})
}

// CleanupExpired removes peers whose last observation exceeds the timeout.
func (s *Store) CleanupExpired(timeout time.Duration) (int, error) {
	if timeout <= 0 {
		return 0, errors.New("timeout must be positive")
	}

	removed := 0
	err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pollBucketName)
		if bucket == nil {
			return errPollBucketMissing
		}

		cursor := bucket.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			removedPeer, err := s.processPeerRecord(bucket, cursor, key, value, timeout)
			if err != nil {
				return err
			}
			if removedPeer {
				removed++
			}
		}
		return nil
	})

	if errors.Is(err, errPollBucketMissing) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	return removed, nil
}

func (s *Store) processPeerRecord(
	bucket *bolt.Bucket,
	cursor *bolt.Cursor,
	key, value []byte,
	timeout time.Duration,
) (bool, error) {
	record, err := unmarshalPeerRecord(key, value)
	if err != nil {
		return false, err
	}

	peer, err := record.toPeer(string(key))
	if err != nil {
		return false, fmt.Errorf("materialize peer %s: %w", string(key), err)
	}

	if peer.IsExpired(timeout) {
		if err := cursor.Delete(); err != nil {
			return false, fmt.Errorf("delete peer %s: %w", string(key), err)
		}
		return true, nil
	}

	peer.CheckAndUpdateStatus(timeout)
	if err := persistPeer(bucket, key, peer); err != nil {
		return false, err
	}
	return false, nil
}

func unmarshalPeerRecord(key, value []byte) (peerRecord, error) {
	var record peerRecord
	if err := json.Unmarshal(value, &record); err != nil {
		return peerRecord{}, fmt.Errorf("unmarshal peer %s: %w", string(key), err)
	}
	return record, nil
}

func persistPeer(bucket *bolt.Bucket, key []byte, peer *Peer) error {
	payload, err := json.Marshal(peerToRecord(peer))
	if err != nil {
		return fmt.Errorf("marshal peer %s: %w", string(key), err)
	}

	if err := bucket.Put(key, payload); err != nil {
		return fmt.Errorf("persist peer %s: %w", string(key), err)
	}
	return nil
}

type peerRecord struct {
	ID                        string     `json:"id,omitempty"`
	Address                   string     `json:"address,omitempty"`
	Status                    PeerStatus `json:"status,omitempty"`
	LastPollAt                time.Time  `json:"last_poll_at,omitempty"`
	LastSeen                  time.Time  `json:"LastSeen,omitempty"`
	Version                   uint64     `json:"version,omitempty"`
	Assets                    []string   `json:"assets,omitempty"`
	PeerAllowed               bool       `json:"PeerAllowed,omitempty"`
	BTCSwapInPremiumRatePPM   int64      `json:"btc_swap_in_premium_rate_ppm,omitempty"`
	BTCSwapOutPremiumRatePPM  int64      `json:"btc_swap_out_premium_rate_ppm,omitempty"`
	LBTCSwapInPremiumRatePPM  int64      `json:"lbtc_swap_in_premium_rate_ppm,omitempty"`
	LBTCSwapOutPremiumRatePPM int64      `json:"lbtc_swap_out_premium_rate_ppm,omitempty"`
}

func peerToRecord(peer *Peer) *peerRecord {
	record := &peerRecord{
		ID:         peer.ID().String(),
		Address:    peer.Address(),
		Status:     peer.Status(),
		LastPollAt: peer.LastPollAt(),
		LastSeen:   peer.LastObservedAt(),
	}

	if capability := peer.Capability(); capability != nil {
		assets := make([]string, len(capability.supportedAssets))
		for idx, asset := range capability.supportedAssets {
			assets[idx] = asset.String()
		}
		record.Version = capability.version.Value()
		record.Assets = assets
		record.PeerAllowed = capability.isPeerAllowed
		record.BTCSwapInPremiumRatePPM = ppmValue(capability.btcSwapInPremiumRate)
		record.BTCSwapOutPremiumRatePPM = ppmValue(capability.btcSwapOutPremiumRate)
		record.LBTCSwapInPremiumRatePPM = ppmValue(capability.lbtcSwapInPremiumRate)
		record.LBTCSwapOutPremiumRatePPM = ppmValue(capability.lbtcSwapOutPremiumRate)
	}

	return record
}

func (r *peerRecord) toPeer(key string) (*Peer, error) {
	id, err := r.resolvePeerID(key)
	if err != nil {
		return nil, err
	}

	peer := NewPeer(id, r.Address)
	r.applyLifecycle(peer)

	capability, err := r.rehydrateCapability()
	if err != nil {
		return nil, err
	}

	if capability != nil {
		peer.capability = capability
	}

	return peer, nil
}

func (r *peerRecord) resolvePeerID(fallback string) (PeerID, error) {
	idValue := r.ID
	if idValue == "" {
		idValue = fallback
	}
	return NewPeerID(idValue)
}

func (r *peerRecord) applyLifecycle(peer *Peer) {
	if r.Status != "" {
		peer.SetStatus(r.Status)
	} else {
		peer.SetStatus(StatusUnknown)
	}

	if !r.LastPollAt.IsZero() {
		peer.SetLastPollAt(r.LastPollAt)
	}
	if !r.LastSeen.IsZero() {
		peer.SetLastObservedAt(r.LastSeen)
	}
}

func (r *peerRecord) rehydrateCapability() (*PeerCapability, error) {
	if !r.hasCapabilityData() {
		return nil, nil
	}

	assets, err := r.decodeAssets()
	if err != nil {
		return nil, err
	}

	btcIn, err := NewPremiumRate(r.BTCSwapInPremiumRatePPM)
	if err != nil {
		return nil, err
	}

	btcOut, err := NewPremiumRate(r.BTCSwapOutPremiumRatePPM)
	if err != nil {
		return nil, err
	}

	lbtcIn, err := NewPremiumRate(r.LBTCSwapInPremiumRatePPM)
	if err != nil {
		return nil, err
	}

	lbtcOut, err := NewPremiumRate(r.LBTCSwapOutPremiumRatePPM)
	if err != nil {
		return nil, err
	}

	return &PeerCapability{
		version:                NewVersion(r.Version),
		supportedAssets:        assets,
		isPeerAllowed:          r.PeerAllowed,
		btcSwapInPremiumRate:   btcIn,
		btcSwapOutPremiumRate:  btcOut,
		lbtcSwapInPremiumRate:  lbtcIn,
		lbtcSwapOutPremiumRate: lbtcOut,
		observedAt:             r.LastSeen,
	}, nil
}

func (r *peerRecord) hasCapabilityData() bool {
	return r.Version != 0 ||
		len(r.Assets) > 0 ||
		r.PeerAllowed ||
		r.BTCSwapInPremiumRatePPM != 0 ||
		r.BTCSwapOutPremiumRatePPM != 0 ||
		r.LBTCSwapInPremiumRatePPM != 0 ||
		r.LBTCSwapOutPremiumRatePPM != 0
}

func (r *peerRecord) decodeAssets() ([]Asset, error) {
	if len(r.Assets) == 0 {
		return nil, nil
	}

	assets := make([]Asset, 0, len(r.Assets))
	for _, symbol := range r.Assets {
		asset, err := NewAsset(symbol)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, nil
}
