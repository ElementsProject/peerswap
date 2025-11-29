package format

import (
	"github.com/elementsproject/peerswap/peersync"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
)

// PeerView aggregates capability and swap statistics for presentation layers.
type PeerView struct {
	NodeID          string
	SwapsAllowed    bool
	SupportedAssets []string
	PaidFee         uint64
	Sender          SwapStats
	Receiver        SwapStats
	Premiums        []PremiumRate
}

// SwapStats captures directional swap counters and volumes.
type SwapStats struct {
	SwapsOut uint64
	SwapsIn  uint64
	SatsOut  uint64
	SatsIn   uint64
}

// PremiumRate represents a premium value for a given asset/operation pair.
type PremiumRate struct {
	Asset     premium.AssetType
	Operation premium.OperationType
	RatePPM   int64
}

// BuildPeerView constructs a PeerView from a capability and the peer's swap history.
func BuildPeerView(nodeID string, capability *peersync.PeerCapability, swaps []*swap.SwapStateMachine) PeerView {
	view := PeerView{NodeID: nodeID}

	if capability != nil {
		view.SwapsAllowed = capability.IsAllowed()
		view.SupportedAssets = capability.SupportedAssetStrings()
		view.Premiums = buildPremiumRates(capability)
	}

	view.Sender, view.Receiver, view.PaidFee = summarizeSwaps(swaps)

	return view
}

func buildPremiumRates(capability *peersync.PeerCapability) []PremiumRate {
	if capability == nil {
		return nil
	}

	combos := []struct {
		asset     premium.AssetType
		operation premium.OperationType
	}{
		{premium.BTC, premium.SwapIn},
		{premium.BTC, premium.SwapOut},
		{premium.LBTC, premium.SwapIn},
		{premium.LBTC, premium.SwapOut},
	}

	rates := make([]PremiumRate, 0, len(combos))
	for _, combo := range combos {
		rates = append(rates, PremiumRate{
			Asset:     combo.asset,
			Operation: combo.operation,
			RatePPM:   capability.PremiumRateValue(combo.asset, combo.operation),
		})
	}
	return rates
}

func summarizeSwaps(swaps []*swap.SwapStateMachine) (SwapStats, SwapStats, uint64) {
	var sender SwapStats
	var receiver SwapStats
	var paidFee uint64

	for _, s := range swaps {
		if s == nil || s.Data == nil {
			continue
		}
		if s.Current != swap.State_ClaimedPreimage {
			continue
		}

		amount := s.Data.GetAmount()

		if s.Role == swap.SWAPROLE_SENDER {
			paidFee += s.Data.OpeningTxFee
			if s.Type == swap.SWAPTYPE_OUT {
				sender.SwapsOut++
				sender.SatsOut += amount
			} else {
				sender.SwapsIn++
				sender.SatsIn += amount
			}
			continue
		}

		if s.Type == swap.SWAPTYPE_OUT {
			receiver.SwapsOut++
			receiver.SatsOut += amount
		} else {
			receiver.SwapsIn++
			receiver.SatsIn += amount
		}
	}

	return sender, receiver, paidFee
}
