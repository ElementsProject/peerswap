package peersync

import (
	"strings"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/premium"
)

func newTestCapability(t *testing.T) *PeerCapability {
	t.Helper()

	version := NewVersion(1)
	btc, err := NewAsset("BTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lbtc, err := NewAsset("LBTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inRate, err := NewPremiumRate(1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outRate, err := NewPremiumRate(2000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lIn, err := NewPremiumRate(500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lOut, err := NewPremiumRate(700)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return NewPeerCapability(
		version,
		[]Asset{btc, lbtc},
		true,
		inRate,
		outRate,
		lIn,
		lOut,
	)
}

func buildCapability(t *testing.T, version uint64) *PeerCapability {
	t.Helper()
	v := NewVersion(version)
	asset, err := NewAsset("BTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rate, err := NewPremiumRate(1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return NewPeerCapability(
		v,
		[]Asset{asset},
		true,
		rate,
		rate,
		rate,
		rate,
	)
}

func buildPeer(t *testing.T) *Peer {
	t.Helper()
	id, err := NewPeerID("peer-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	peer := NewPeer(id, "addr")

	capability := buildCapability(t, 1)
	peer.UpdateCapability(capability)
	return peer
}

func TestPeerCapabilitySupportsAsset(t *testing.T) {
	capability := newTestCapability(t)

	btc, err := NewAsset("BTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lbtc, err := NewAsset("LBTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !capability.SupportsAsset(btc) {
		t.Fatalf("expected capability to support BTC")
	}
	if !capability.SupportsAsset(lbtc) {
		t.Fatalf("expected capability to support LBTC")
	}

	unsupported := AssetUnknown
	if capability.SupportsAsset(unsupported) {
		t.Fatalf("expected capability to not support unsupported assets")
	}

	if got := ppmValue(capability.GetPremiumRate(premium.BTC, premium.SwapIn)); got != 1000 {
		t.Fatalf("unexpected BTC IN rate %d", got)
	}
	if got := ppmValue(capability.GetPremiumRate(premium.BTC, premium.SwapOut)); got != 2000 {
		t.Fatalf("unexpected BTC OUT rate %d", got)
	}
	if got := ppmValue(capability.GetPremiumRate(premium.LBTC, premium.SwapIn)); got != 500 {
		t.Fatalf("unexpected LBTC IN rate %d", got)
	}
	if got := ppmValue(capability.GetPremiumRate(premium.LBTC, premium.SwapOut)); got != 700 {
		t.Fatalf("unexpected LBTC OUT rate %d", got)
	}
}

func TestPeerCapabilitySupportedAssetStrings(t *testing.T) {
	capability := newTestCapability(t)

	assets := capability.SupportedAssetStrings()
	if joined := strings.Join(assets, ","); joined != "BTC,LBTC" {
		t.Fatalf("unexpected asset ordering %s", joined)
	}

	assets[0] = "MUTATED"
	if joined := strings.Join(capability.SupportedAssetStrings(), ","); joined != "BTC,LBTC" {
		t.Fatalf("expected original assets to remain unchanged, got %s", joined)
	}

	var nilCapability *PeerCapability
	if values := nilCapability.SupportedAssetStrings(); values != nil {
		t.Fatalf("expected nil capability to return nil slice, got %v", values)
	}
}

func TestPeerCapabilityPremiumRateValue(t *testing.T) {
	capability := newTestCapability(t)

	if got := capability.PremiumRateValue(premium.BTC, premium.SwapIn); got != 1000 {
		t.Fatalf("unexpected BTC swap-in rate %d", got)
	}
	if got := capability.PremiumRateValue(premium.LBTC, premium.SwapOut); got != 700 {
		t.Fatalf("unexpected LBTC swap-out rate %d", got)
	}
	if got := capability.PremiumRateValue(premium.AssetType(255), premium.SwapIn); got != 0 {
		t.Fatalf("expected unknown asset to return 0, got %d", got)
	}

	var nilCapability *PeerCapability
	if got := nilCapability.PremiumRateValue(premium.BTC, premium.SwapIn); got != 0 {
		t.Fatalf("expected nil capability to return 0, got %d", got)
	}
}

func TestPeerStatusLifecycle(t *testing.T) {
	id, err := NewPeerID("peer-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	peer := NewPeer(id, "127.0.0.1")
	if peer.Status() != StatusUnknown {
		t.Fatalf("expected status unknown, got %s", peer.Status())
	}

	capability := newTestCapability(t)
	peer.UpdateCapability(capability)

	if peer.Status() != StatusActive {
		t.Fatalf("expected status active")
	}
	if peer.LastObservedAt().IsZero() {
		t.Fatalf("expected last observed at to be set")
	}
	if !peer.IsCompatibleWith(NewVersion(1)) {
		t.Fatalf("expected peer to be compatible with version 1")
	}

	peer.MarkAsPolled()
	if peer.LastPollAt().IsZero() {
		t.Fatalf("expected last poll at to be set")
	}
}

func TestPeerExpiration(t *testing.T) {
	id, err := NewPeerID("peer-expire")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	peer := NewPeer(id, "10.0.0.1")

	capability := newTestCapability(t)
	peer.UpdateCapability(capability)

	peer.SetLastObservedAt(time.Now().Add(-time.Hour))
	peer.CheckAndUpdateStatus(10 * time.Minute)

	if peer.Status() != StatusExpired {
		t.Fatalf("expected status expired, got %s", peer.Status())
	}
}

func TestNewPeerID(t *testing.T) {
	id, err := NewPeerID("peer-123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id.String() != "peer-123" {
		t.Fatalf("expected peer-123, got %s", id.String())
	}

	other, err := NewPeerID("peer-123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !id.Equals(other) {
		t.Fatalf("expected IDs to be equal")
	}

	if _, err = NewPeerID(""); err == nil {
		t.Fatalf("expected error for empty ID")
	}

	if _, err = NewPeerID(strings.Repeat("a", 129)); err == nil {
		t.Fatalf("expected error for long ID")
	}
}

func TestVersionBehavior(t *testing.T) {
	v1 := NewVersion(1)
	v2 := NewVersion(1)
	v3 := NewVersion(2)

	if !v1.IsCompatibleWith(v2) {
		t.Fatalf("expected versions to be compatible")
	}
	if v1.IsCompatibleWith(v3) {
		t.Fatalf("expected versions to be incompatible")
	}
	if v1.Next().Value() != uint64(2) {
		t.Fatalf("expected next value to be 2")
	}
}

func TestAssetValidation(t *testing.T) {
	a, err := NewAsset("BTC")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if a != AssetBTC {
		t.Fatalf("expected asset BTC constant")
	}
	if a.String() != "BTC" {
		t.Fatalf("expected asset BTC string to be BTC, got %s", a.String())
	}

	b, err := NewAsset("LBTC")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if b != AssetLBTC {
		t.Fatalf("expected asset LBTC constant")
	}
	if b.String() != "LBTC" {
		t.Fatalf("expected asset LBTC string to be LBTC, got %s", b.String())
	}

	if _, err = NewAsset("lower"); err == nil {
		t.Fatalf("expected error for lowercase asset")
	}

	if _, err = NewAsset("TOO-LONG-ASSET"); err == nil {
		t.Fatalf("expected error for long asset")
	}

	if _, err = NewAsset("USDT"); err == nil {
		t.Fatalf("expected error for unsupported asset")
	}
}

func TestPremiumRate(t *testing.T) {
	rate, err := NewPremiumRate(1000)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ppmValue(rate) != int64(1000) {
		t.Fatalf("expected ppm 1000, got %d", ppmValue(rate))
	}
	percentage := float64(ppmValue(rate)) / 10_000
	if diff := percentage - 0.1; diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("unexpected percentage %.4f", percentage)
	}
	if ppmValue(rate) <= 0 {
		t.Fatalf("expected positive rate")
	}

	if _, err = NewPremiumRate(MaxPremiumRatePPM + 1); err == nil {
		t.Fatalf("expected error for exceeding max")
	}
	if _, err = NewPremiumRate(MinPremiumRatePPM - 1); err == nil {
		t.Fatalf("expected error for below min")
	}
}

func TestShouldPoll(t *testing.T) {
	logic := NewSyncLogic()
	peer := buildPeer(t)

	if !logic.ShouldPoll(peer) {
		t.Fatalf("expected to poll when never polled")
	}

	peer.MarkAsPolled()
	if logic.ShouldPoll(peer) {
		t.Fatalf("expected not to poll when recently polled")
	}

	peer.MarkAsPolled()
	peer.SetLastPollAt(time.Now().Add(-time.Hour))
	if !logic.ShouldPoll(peer) {
		t.Fatalf("expected to poll when last polled long ago")
	}

	peer.SetStatus(StatusExpired)
	if logic.ShouldPoll(peer) {
		t.Fatalf("expected not to poll expired peer")
	}
}

func TestMergeCapabilities(t *testing.T) {
	logic := NewSyncLogic()

	t.Run("remoteNewerWins", func(t *testing.T) {
		local := buildCapability(t, 1)
		remote := buildCapability(t, 2)

		result := logic.MergeCapabilities(local, remote)
		if result != remote {
			t.Fatalf("expected remote capability to win")
		}
	})

	t.Run("localNewerRetained", func(t *testing.T) {
		local := buildCapability(t, 2)
		remote := buildCapability(t, 1)

		result := logic.MergeCapabilities(local, remote)
		if result != local {
			t.Fatalf("expected newer local capability to win")
		}
	})

	t.Run("equalVersionUpdates", func(t *testing.T) {
		local := buildCapability(t, 1)

		asset, err := NewAsset("BTC")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rate, err := NewPremiumRate(5000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		remote := NewPeerCapability(
			NewVersion(1),
			[]Asset{asset},
			false,
			rate,
			rate,
			rate,
			rate,
		)

		result := logic.MergeCapabilities(local, remote)
		if result != remote {
			t.Fatalf("expected remote capability to win when versions equal")
		}
		if result.IsAllowed() {
			t.Fatalf("expected merged capability to reflect remote changes")
		}
	})
}
