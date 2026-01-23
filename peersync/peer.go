// Package peersync orchestrates capability synchronization between peers.
package peersync

import (
	"errors"
	"fmt"
	"time"

	"github.com/elementsproject/peerswap/premium"
)

// Asset identifies the supported assets for swaps.
type Asset int

const (
	// AssetUnknown represents an unspecified or unsupported asset.
	AssetUnknown Asset = iota
	// AssetBTC represents Bitcoin on the Bitcoin network.
	AssetBTC
	// AssetLBTC represents Liquid Bitcoin on the Liquid network.
	AssetLBTC
)

var (
	assetToString = map[Asset]string{
		AssetBTC:  "BTC",
		AssetLBTC: "LBTC",
	}
	stringToAsset = map[string]Asset{
		"BTC":  AssetBTC,
		"LBTC": AssetLBTC,
	}
)

// NewAsset converts a ticker into the corresponding enum value.
func NewAsset(value string) (Asset, error) {
	if asset, ok := stringToAsset[value]; ok {
		return asset, nil
	}
	return AssetUnknown, fmt.Errorf("unsupported asset: %s", value)
}

// String returns the canonical ticker representation or "UNKNOWN".
func (a Asset) String() string {
	if value, ok := assetToString[a]; ok {
		return value
	}
	return "UNKNOWN"
}

// Equals reports whether two assets are identical.
func (a Asset) Equals(other Asset) bool {
	return a == other
}

const (
	// MaxPremiumRatePPM defines the upper bound for premiums (100%).
	MaxPremiumRatePPM = 1_000_000
	// MinPremiumRatePPM defines the lower bound for premiums (-100%).
	MinPremiumRatePPM = -1_000_000
)

// NewPremiumRate validates bounds and returns a premium PPM value.
func NewPremiumRate(ppm int64) (*premium.PPM, error) {
	if ppm < MinPremiumRatePPM || ppm > MaxPremiumRatePPM {
		return nil, fmt.Errorf("premium rate out of range: %d", ppm)
	}
	return premium.NewPPM(ppm), nil
}

// Version represents a protocol version for peer communication.
type Version struct {
	value uint64
}

// NewVersion constructs a new Version value.
func NewVersion(v uint64) Version {
	return Version{value: v}
}

// Value returns the underlying numeric representation.
func (v Version) Value() uint64 {
	return v.value
}

// IsCompatibleWith reports whether two versions can interoperate.
func (v Version) IsCompatibleWith(other Version) bool {
	return v.value == other.value
}

// Next returns the next sequential version value.
func (v Version) Next() Version {
	return Version{value: v.value + 1}
}

// PeerID represents a unique identifier for a peer participating in sync.
type PeerID struct {
	value string
}

// NewPeerID validates the raw identifier and returns an immutable PeerID.
func NewPeerID(value string) (PeerID, error) {
	if value == "" {
		return PeerID{}, errors.New("peer ID cannot be empty")
	}
	if len(value) > 128 {
		return PeerID{}, errors.New("peer ID too long")
	}
	return PeerID{value: value}, nil
}

// String returns the string representation of the peer ID.
func (id PeerID) String() string {
	return id.value
}

// Equals reports whether two peer identifiers are identical.
func (id PeerID) Equals(other PeerID) bool {
	return id.value == other.value
}

// PeerCapability captures the advertised capabilities of a remote peer.
type PeerCapability struct {
	version                Version
	supportedAssets        []Asset
	isPeerAllowed          bool
	btcSwapInPremiumRate   *premium.PPM
	btcSwapOutPremiumRate  *premium.PPM
	lbtcSwapInPremiumRate  *premium.PPM
	lbtcSwapOutPremiumRate *premium.PPM
	observedAt             time.Time
}

// NewPeerCapability constructs a capability snapshot.
func NewPeerCapability(
	version Version,
	assets []Asset,
	allowed bool,
	btcInRate, btcOutRate *premium.PPM,
	lbtcInRate, lbtcOutRate *premium.PPM,
) *PeerCapability {
	assetsCopy := make([]Asset, len(assets))
	copy(assetsCopy, assets)

	return &PeerCapability{
		version:                version,
		supportedAssets:        assetsCopy,
		isPeerAllowed:          allowed,
		btcSwapInPremiumRate:   btcInRate,
		btcSwapOutPremiumRate:  btcOutRate,
		lbtcSwapInPremiumRate:  lbtcInRate,
		lbtcSwapOutPremiumRate: lbtcOutRate,
		observedAt:             time.Now(),
	}
}

// Version returns the advertised protocol version.
func (c *PeerCapability) Version() Version {
	return c.version
}

// IsAllowed indicates whether the peer is currently allowed by policy.
func (c *PeerCapability) IsAllowed() bool {
	return c.isPeerAllowed
}

// SupportedAssets returns a copy of the supported asset list.
func (c *PeerCapability) SupportedAssets() []Asset {
	assetsCopy := make([]Asset, len(c.supportedAssets))
	copy(assetsCopy, c.supportedAssets)
	return assetsCopy
}

// ObservedAt returns the time at which the capability was observed.
func (c *PeerCapability) ObservedAt() time.Time {
	return c.observedAt
}

// SupportsAsset reports whether the capability supports the given asset.
func (c *PeerCapability) SupportsAsset(asset Asset) bool {
	for _, a := range c.supportedAssets {
		if a.Equals(asset) {
			return true
		}
	}
	return false
}

// GetPremiumRate returns the premium rate for the requested asset/operation pair.
func (c *PeerCapability) GetPremiumRate(
	assetType premium.AssetType,
	operation premium.OperationType,
) *premium.PPM {
	switch assetType {
	case premium.BTC:
		switch operation {
		case premium.SwapIn:
			return c.btcSwapInPremiumRate
		case premium.SwapOut:
			return c.btcSwapOutPremiumRate
		}
	case premium.LBTC:
		switch operation {
		case premium.SwapIn:
			return c.lbtcSwapInPremiumRate
		case premium.SwapOut:
			return c.lbtcSwapOutPremiumRate
		}
	}

	return nil
}

// SupportedAssetStrings exposes the supported assets as their textual symbols.
func (c *PeerCapability) SupportedAssetStrings() []string {
	if c == nil {
		return nil
	}

	result := make([]string, len(c.supportedAssets))
	for i, asset := range c.supportedAssets {
		result[i] = asset.String()
	}
	return result
}

// PremiumRateValue returns the premium rate in ppm for the given tuple.
func (c *PeerCapability) PremiumRateValue(
	asset premium.AssetType,
	operation premium.OperationType,
) int64 {
	if c == nil {
		return 0
	}
	return ppmValue(c.GetPremiumRate(asset, operation))
}

func ppmValue(rate *premium.PPM) int64 {
	if rate == nil {
		return 0
	}
	return rate.Value()
}

// PeerStatus captures the lifecycle state of a peer.
type PeerStatus string

// Peer status values.
const (
	StatusActive   PeerStatus = "active"
	StatusInactive PeerStatus = "inactive"
	StatusUnknown  PeerStatus = "unknown"
	StatusExpired  PeerStatus = "expired"
)

// Peer represents a remote node tracked by peersync.
type Peer struct {
	id               PeerID
	address          string
	capability       *PeerCapability
	channelAdjacency *ChannelAdjacency
	status           PeerStatus
	lastPollAt       time.Time
	lastObservedAt   time.Time
}

// NewPeer constructs a peer model with an unknown initial status.
func NewPeer(id PeerID, address string) *Peer {
	return &Peer{
		id:               id,
		address:          address,
		capability:       nil,
		channelAdjacency: nil,
		status:           StatusUnknown,
		lastPollAt:       time.Time{},
		lastObservedAt:   time.Time{},
	}
}

// ID returns the immutable identifier of the peer.
func (p *Peer) ID() PeerID {
	return p.id
}

// Address returns the last known address of the peer.
func (p *Peer) Address() string {
	return p.address
}

// Capability returns the last observed capability for the peer.
func (p *Peer) Capability() *PeerCapability {
	return p.capability
}

// ChannelAdjacency returns the last received 2-hop discovery hint (if any).
//
// See ChannelAdjacency for the exact domain definition and trust model.
func (p *Peer) ChannelAdjacency() *ChannelAdjacency {
	if p == nil {
		return nil
	}
	return cloneChannelAdjacency(p.channelAdjacency)
}

// UpdateChannelAdjacency stores the advertised 2-hop discovery hint.
func (p *Peer) UpdateChannelAdjacency(ad *ChannelAdjacency) {
	if p == nil {
		return
	}
	p.channelAdjacency = cloneChannelAdjacency(ad)
}

// UpdateCapability refreshes the peer capability and marks it active.
func (p *Peer) UpdateCapability(capability *PeerCapability) {
	p.capability = capability
	p.lastObservedAt = time.Now()
	p.status = StatusActive
}

// MarkAsPolled records the time at which the peer was polled.
func (p *Peer) MarkAsPolled() {
	p.lastPollAt = time.Now()
}

// LastPollAt returns the last poll timestamp.
func (p *Peer) LastPollAt() time.Time {
	return p.lastPollAt
}

// SetLastPollAt forcibly sets the last poll timestamp (primarily for testing).
func (p *Peer) SetLastPollAt(t time.Time) {
	p.lastPollAt = t
}

// LastObservedAt returns when the capability was last seen.
func (p *Peer) LastObservedAt() time.Time {
	return p.lastObservedAt
}

// SetLastObservedAt overrides the observation timestamp.
func (p *Peer) SetLastObservedAt(t time.Time) {
	p.lastObservedAt = t
}

// Status returns the current peer status.
func (p *Peer) Status() PeerStatus {
	return p.status
}

// SetStatus updates the peer status.
func (p *Peer) SetStatus(status PeerStatus) {
	p.status = status
}

// IsExpired determines whether the peer has expired beyond the supplied timeout.
func (p *Peer) IsExpired(timeout time.Duration) bool {
	if p.lastObservedAt.IsZero() {
		return false
	}
	return time.Since(p.lastObservedAt) > timeout
}

// CheckAndUpdateStatus transitions the peer to expired when appropriate.
func (p *Peer) CheckAndUpdateStatus(timeout time.Duration) {
	if p.IsExpired(timeout) {
		p.status = StatusExpired
	}
}

// IsCompatibleWith reports whether the peer capability matches the provided version.
func (p *Peer) IsCompatibleWith(version Version) bool {
	if p.capability == nil {
		return false
	}
	return p.capability.Version().IsCompatibleWith(version)
}
