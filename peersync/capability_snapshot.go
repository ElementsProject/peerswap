package peersync

import (
	"encoding/json"

	"github.com/elementsproject/peerswap/premium"
)

// PeerCapabilitySnapshot normalizes capability data for transport (custom
// messages) and persistence (store records). It owns the conversion between
// internal structures and JSON-friendly representations.
type PeerCapabilitySnapshot struct {
	Version                   uint64   `json:"version,omitempty"`
	Assets                    []string `json:"assets,omitempty"`
	PeerAllowed               bool     `json:"peer_allowed,omitempty"`
	BTCSwapInPremiumRatePPM   int64    `json:"btc_swap_in_premium_rate_ppm,omitempty"`
	BTCSwapOutPremiumRatePPM  int64    `json:"btc_swap_out_premium_rate_ppm,omitempty"`
	LBTCSwapInPremiumRatePPM  int64    `json:"lbtc_swap_in_premium_rate_ppm,omitempty"`
	LBTCSwapOutPremiumRatePPM int64    `json:"lbtc_swap_out_premium_rate_ppm,omitempty"`
	// ChannelAdjacency is an optional hint used for 2-hop discovery. Nodes that
	// do not understand the field ignore it.
	ChannelAdjacency *ChannelAdjacency `json:"channel_adjacency,omitempty"`
}

// UnmarshalJSON keeps backwards-compatibility for legacy field names.
func (s *PeerCapabilitySnapshot) UnmarshalJSON(data []byte) error {
	type alias PeerCapabilitySnapshot
	var tmp struct {
		alias
		LegacyNeighborsAd *ChannelAdjacency `json:"neighbors_ad,omitempty"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*s = PeerCapabilitySnapshot(tmp.alias)
	if s.ChannelAdjacency == nil && tmp.LegacyNeighborsAd != nil {
		s.ChannelAdjacency = tmp.LegacyNeighborsAd
	}
	return nil
}

// SnapshotFromCapability builds a snapshot from the given capability.
func SnapshotFromCapability(capability *PeerCapability) *PeerCapabilitySnapshot {
	if capability == nil {
		return nil
	}

	assets := capability.SupportedAssets()
	assetSymbols := make([]string, 0, len(assets))
	for _, asset := range assets {
		assetSymbols = append(assetSymbols, asset.String())
	}

	return &PeerCapabilitySnapshot{
		Version:                   capability.Version().Value(),
		Assets:                    assetSymbols,
		PeerAllowed:               capability.IsAllowed(),
		BTCSwapInPremiumRatePPM:   ppmValue(capability.GetPremiumRate(premium.BTC, premium.SwapIn)),
		BTCSwapOutPremiumRatePPM:  ppmValue(capability.GetPremiumRate(premium.BTC, premium.SwapOut)),
		LBTCSwapInPremiumRatePPM:  ppmValue(capability.GetPremiumRate(premium.LBTC, premium.SwapIn)),
		LBTCSwapOutPremiumRatePPM: ppmValue(capability.GetPremiumRate(premium.LBTC, premium.SwapOut)),
	}
}

// ToCapability converts the snapshot back into a PeerCapability.
func (s *PeerCapabilitySnapshot) ToCapability() (*PeerCapability, error) {
	if s == nil {
		return nil, nil
	}

	assets := make([]Asset, 0, len(s.Assets))
	for _, symbol := range s.Assets {
		asset, err := NewAsset(symbol)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}

	btcIn, err := NewPremiumRate(s.BTCSwapInPremiumRatePPM)
	if err != nil {
		return nil, err
	}
	btcOut, err := NewPremiumRate(s.BTCSwapOutPremiumRatePPM)
	if err != nil {
		return nil, err
	}
	lbtcIn, err := NewPremiumRate(s.LBTCSwapInPremiumRatePPM)
	if err != nil {
		return nil, err
	}
	lbtcOut, err := NewPremiumRate(s.LBTCSwapOutPremiumRatePPM)
	if err != nil {
		return nil, err
	}

	return NewPeerCapability(
		NewVersion(s.Version),
		assets,
		s.PeerAllowed,
		btcIn,
		btcOut,
		lbtcIn,
		lbtcOut,
	), nil
}

// PollMessageDTO and RequestPollMessageDTO reuse the snapshot layout.
type PollMessageDTO = PeerCapabilitySnapshot
type RequestPollMessageDTO = PeerCapabilitySnapshot
