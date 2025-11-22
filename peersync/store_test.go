package peersync

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "peers.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestStoreSaveAndGetPeerState(t *testing.T) {
	store := newTestStore(t)

	peerID, err := NewPeerID("peer-store-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	peer := NewPeer(peerID, "addr-1")
	peer.SetStatus(StatusActive)
	peer.SetLastPollAt(time.Now())
	peer.SetLastObservedAt(time.Now())

	if err := store.SavePeerState(peer); err != nil {
		t.Fatalf("failed to save peer state: %v", err)
	}

	stored, err := store.GetPeerState(peerID)
	if err != nil {
		t.Fatalf("failed to get peer state: %v", err)
	}

	if stored.ID().String() != peerID.String() {
		t.Fatalf("unexpected peer id: got %s want %s", stored.ID(), peerID)
	}

	all, err := store.GetAllPeerStates()
	if err != nil {
		t.Fatalf("failed to get all peer states: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(all))
	}
}

func TestStoreCleanupExpired(t *testing.T) {
	store := newTestStore(t)

	timeout := 10 * time.Minute

	freshID, err := NewPeerID("fresh-peer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	freshPeer := NewPeer(freshID, "addr-fresh")
	freshPeer.SetStatus(StatusActive)
	freshPeer.SetLastObservedAt(time.Now())
	if err := store.SavePeerState(freshPeer); err != nil {
		t.Fatalf("failed to store fresh peer: %v", err)
	}

	expiredID, err := NewPeerID("expired-peer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expiredPeer := NewPeer(expiredID, "addr-expired")
	expiredPeer.SetStatus(StatusActive)
	expiredPeer.SetLastObservedAt(time.Now().Add(-2 * timeout))
	if err := store.SavePeerState(expiredPeer); err != nil {
		t.Fatalf("failed to store expired peer: %v", err)
	}

	count, err := store.CleanupExpired(timeout)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 expired peer removed, got %d", count)
	}

	if _, err := store.GetPeerState(expiredID); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("expected ErrPeerNotFound, got %v", err)
	}

	fresh, err := store.GetPeerState(freshID)
	if err != nil {
		t.Fatalf("failed to load fresh peer: %v", err)
	}
	if fresh.Status() == StatusExpired {
		t.Fatalf("fresh peer should remain active")
	}
}

type legacyPollInfo struct {
	ProtocolVersion           uint64   `json:"version"`
	Assets                    []string `json:"assets"`
	BTCSwapInPremiumRatePPM   int64    `json:"btc_swap_in_premium_rate_ppm"`
	BTCSwapOutPremiumRatePPM  int64    `json:"btc_swap_out_premium_rate_ppm"`
	LBTCSwapInPremiumRatePPM  int64    `json:"lbtc_swap_in_premium_rate_ppm"`
	LBTCSwapOutPremiumRatePPM int64    `json:"lbtc_swap_out_premium_rate_ppm"`
	PeerAllowed               bool
	LastSeen                  time.Time
}

func TestStoreReadsLegacyPollInfo(t *testing.T) {
	store := newTestStore(t)

	info := legacyPollInfo{
		ProtocolVersion:           7,
		Assets:                    []string{"BTC"},
		BTCSwapInPremiumRatePPM:   100,
		BTCSwapOutPremiumRatePPM:  200,
		LBTCSwapInPremiumRatePPM:  300,
		LBTCSwapOutPremiumRatePPM: 400,
		PeerAllowed:               true,
		LastSeen:                  time.Now().Add(-time.Minute),
	}

	legacyID := "legacy-peer"
	writeLegacyRecord(t, store, legacyID, info)

	peerID, err := NewPeerID(legacyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	peer, err := store.GetPeerState(peerID)
	if err != nil {
		t.Fatalf("failed to load legacy peer: %v", err)
	}

	assertLegacyPeer(t, peer, info)
	assertLegacyCapability(t, peer.Capability(), info)
}

func writeLegacyRecord(t *testing.T, store *Store, legacyID string, info legacyPollInfo) {
	t.Helper()

	payload, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal legacy info: %v", err)
	}

	if err := store.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(pollBucketName)
		return bucket.Put([]byte(legacyID), payload)
	}); err != nil {
		t.Fatalf("failed to write legacy record: %v", err)
	}
}

func assertLegacyPeer(t *testing.T, peer *Peer, info legacyPollInfo) {
	t.Helper()

	if peer.Status() != StatusUnknown {
		t.Fatalf("expected unknown status for legacy peer, got %s", peer.Status())
	}

	if got := peer.LastObservedAt(); got.IsZero() || got.Before(info.LastSeen) {
		t.Fatalf("expected last observed to be set from legacy data")
	}
}

func assertLegacyCapability(t *testing.T, capability *PeerCapability, info legacyPollInfo) {
	t.Helper()

	if capability == nil {
		t.Fatalf("expected capability reconstructed from legacy data")
	}

	if capability.Version().Value() != info.ProtocolVersion {
		t.Fatalf("unexpected version, got %d want %d", capability.Version().Value(), info.ProtocolVersion)
	}
	if !capability.IsAllowed() {
		t.Fatalf("expected peer allowed")
	}
}

func TestHasCompatiblePeer(t *testing.T) {
	store := newTestStore(t)
	target := NewVersion(42)
	ps := &PeerSync{store: store, version: target}

	makeCapability := func(version Version) *PeerCapability {
		return NewPeerCapability(version, []Asset{AssetBTC}, true, nil, nil, nil, nil)
	}

	savePeer := func(idStr string, capability *PeerCapability) {
		id, err := NewPeerID(idStr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		peer := NewPeer(id, "addr-"+idStr)
		if capability != nil {
			peer.UpdateCapability(capability)
		}
		if err := store.SavePeerState(peer); err != nil {
			t.Fatalf("failed to save peer %s: %v", idStr, err)
		}
	}

	savePeer("compatible-peer", makeCapability(target))
	savePeer("missing-capability", nil)

	if !ps.HasCompatiblePeer("compatible-peer") {
		t.Fatalf("expected compatible peer to be detected")
	}

	ps.version = target.Next()
	if ps.HasCompatiblePeer("compatible-peer") {
		t.Fatalf("expected mismatch when version differs")
	}

	ps.version = target
	if ps.HasCompatiblePeer("missing-capability") {
		t.Fatalf("expected peers without capability to be excluded")
	}
	if ps.HasCompatiblePeer("unknown-peer") {
		t.Fatalf("expected unknown peers to be excluded")
	}

	var nilSync *PeerSync
	if nilSync.HasCompatiblePeer("compatible-peer") {
		t.Fatalf("expected nil peersync to be treated as incompatible")
	}
	if (&PeerSync{}).HasCompatiblePeer("compatible-peer") {
		t.Fatalf("expected peersync without store to be treated as incompatible")
	}
}

func TestCompatiblePeers(t *testing.T) {
	store := newTestStore(t)
	target := NewVersion(7)
	ps := &PeerSync{store: store, version: target}

	registerPeer := func(idStr string, capabilityVersion *Version) {
		id, err := NewPeerID(idStr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		peer := NewPeer(id, "addr-"+idStr)
		if capabilityVersion != nil {
			capability := NewPeerCapability(*capabilityVersion, []Asset{AssetBTC}, true, nil, nil, nil, nil)
			peer.UpdateCapability(capability)
		}
		if err := store.SavePeerState(peer); err != nil {
			t.Fatalf("failed to store peer %s: %v", idStr, err)
		}
	}

	registerPeer("match-1", &target)
	registerPeer("match-2", &target)

	next := target.Next()
	registerPeer("wrong-version", &next)
	registerPeer("no-capability", nil)

	peers, err := ps.CompatiblePeers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 compatible peers, got %d", len(peers))
	}
	if _, ok := peers["match-1"]; !ok {
		t.Fatalf("expected match-1 to be returned")
	}
	if _, ok := peers["match-2"]; !ok {
		t.Fatalf("expected match-2 to be returned")
	}
	if _, ok := peers["wrong-version"]; ok {
		t.Fatalf("did not expect wrong-version to be returned")
	}
	if _, ok := peers["no-capability"]; ok {
		t.Fatalf("did not expect no-capability to be returned")
	}

	var nilSync *PeerSync
	empty, err := nilSync.CompatiblePeers()
	if err != nil {
		t.Fatalf("expected nil peersync lookup to succeed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no peers from nil peersync, got %d", len(empty))
	}
	if res, err := (&PeerSync{}).CompatiblePeers(); err != nil || len(res) != 0 {
		t.Fatalf("expected peersync without store to succeed, err=%v len=%d", err, len(res))
	}
}
