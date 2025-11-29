package peersync

import (
	"log"

	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/premium"
)

// PeerGuard centralizes policy and premium lookups so call sites can avoid
// repetitive nil checks and guard clauses.
type PeerGuard interface {
	Allow(peer PeerID) bool
	Suspicious(peer PeerID) bool
	PremiumRate(peer PeerID, asset premium.AssetType, operation premium.OperationType) *premium.PPM
}

// NewPeerGuard returns a guard backed by the provided policy and premium
// setting. Both parameters are optional; the guard falls back to permissive
// defaults when they are nil.
func NewPeerGuard(policyCfg *policy.Policy, premiumSetting *premium.Setting) PeerGuard {
	return &peerGuard{
		policy:  policyCfg,
		premium: premiumSetting,
	}
}

type peerGuard struct {
	policy  *policy.Policy
	premium *premium.Setting
}

func (g *peerGuard) Allow(peer PeerID) bool {
	if g.policy == nil {
		return true
	}
	return g.policy.IsPeerAllowed(peer.String())
}

func (g *peerGuard) Suspicious(peer PeerID) bool {
	if g.policy == nil {
		return false
	}
	return g.policy.IsPeerSuspicious(peer.String())
}

func (g *peerGuard) PremiumRate(peer PeerID, asset premium.AssetType, operation premium.OperationType) *premium.PPM {
	defaultValue := defaultPremiumRate(asset, operation)

	if g.premium == nil {
		return premium.NewPPM(defaultValue)
	}

	rate, err := g.premium.GetRate(peer.String(), asset, operation)
	if err != nil {
		log.Printf("failed to get premium rate for %s (%s/%s): %v", peer.String(), asset, operation, err)
		return premium.NewPPM(defaultValue)
	}

	if rate == nil || rate.PremiumRatePPM() == nil {
		return premium.NewPPM(defaultValue)
	}

	return rate.PremiumRatePPM()
}

func defaultPremiumRate(asset premium.AssetType, operation premium.OperationType) int64 {
	ratesByAsset, ok := premium.DefaultPremiumRate[asset]
	if !ok {
		log.Printf("peersync: no default premium rate configured for asset %v", asset)
		return 0
	}

	value, ok := ratesByAsset[operation]
	if !ok {
		log.Printf("peersync: no default premium rate configured for asset %v and operation %v", asset, operation)
		return 0
	}

	return value
}
