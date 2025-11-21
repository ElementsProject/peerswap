package peerswaprpc

import (
	"github.com/elementsproject/peerswap/peersync/format"
)

// NewPeerSwapPeerFromView converts a format.PeerView into the RPC representation.
func NewPeerSwapPeerFromView(view format.PeerView) *PeerSwapPeer {
	peer := &PeerSwapPeer{
		NodeId:          view.NodeID,
		SwapsAllowed:    view.SwapsAllowed,
		SupportedAssets: view.SupportedAssets,
		AsSender:        convertStats(view.Sender),
		AsReceiver:      convertStats(view.Receiver),
		PaidFee:         view.PaidFee,
	}

	rates := convertPremiums(view.Premiums)
	if len(rates) > 0 {
		peer.PeerPremium = &PeerPremium{
			NodeId: view.NodeID,
			Rates:  rates,
		}
	}

	return peer
}

func convertStats(stats format.SwapStats) *SwapStats {
	return &SwapStats{
		SwapsOut: stats.SwapsOut,
		SwapsIn:  stats.SwapsIn,
		SatsOut:  stats.SatsOut,
		SatsIn:   stats.SatsIn,
	}
}

func convertPremiums(rates []format.PremiumRate) []*PremiumRate {
	if len(rates) == 0 {
		return nil
	}
	converted := make([]*PremiumRate, 0, len(rates))
	for _, rate := range rates {
		converted = append(converted, &PremiumRate{
			Asset:          ToAssetType(rate.Asset),
			Operation:      ToOperationType(rate.Operation),
			PremiumRatePpm: rate.RatePPM,
		})
	}
	return converted
}
