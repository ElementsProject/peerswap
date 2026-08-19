package swap

import "fmt"

const (
	legacyProtocolVersion = 6

	bitcoinSwapCSV          uint32 = 1008
	bitcoinPaymentWindow    uint32 = bitcoinSwapCSV / 2
	bitcoinInvoiceFinalCLTV uint64 = uint64(bitcoinPaymentWindow - 1)

	legacyLiquidSwapCSV     uint32 = 60
	legacyLiquidFinalCLTV   uint64 = uint64(legacyLiquidSwapCSV/2 - 1)
	liquidSwapCSV           uint32 = 7 * 24 * 60
	liquidPaymentWindow     uint32 = 60
	liquidInvoiceFinalCLTV  uint64 = 29
	liquidMaxTotalCLTVDelta uint32 = 32
)

// timelockPolicy contains the chain-specific limits derived from the swap's
// protocol version. Values are kept internal so they cannot become a second
// negotiation surface.
type timelockPolicy struct {
	CSV                  uint32
	PaymentWindow        uint32
	InvoiceFinalCLTV     uint64
	MaxTotalCLTVDelta    uint32
	AllowNewClaimPayment bool
}

// ValidateTotalCLTVDelta enforces an inclusive sender-side route limit. A
// limit of zero preserves the legacy behavior for Bitcoin swaps.
func ValidateTotalCLTVDelta(required, limit uint32) error {
	if limit != 0 && required > limit {
		return fmt.Errorf(
			"invoice requires CLTV delta %d, maximum is %d",
			required,
			limit,
		)
	}
	return nil
}

func (s *SwapData) getTimelockPolicy() (timelockPolicy, error) {
	version := s.GetProtocolVersion()

	switch s.GetChain() {
	case btc_chain:
		if version != legacyProtocolVersion && version != PEERSWAP_PROTOCOL_VERSION {
			return timelockPolicy{}, fmt.Errorf("unsupported protocol version: %d", version)
		}
		return timelockPolicy{
			CSV:                  bitcoinSwapCSV,
			PaymentWindow:        bitcoinPaymentWindow,
			InvoiceFinalCLTV:     bitcoinInvoiceFinalCLTV,
			AllowNewClaimPayment: true,
		}, nil

	case l_btc_chain:
		switch version {
		case legacyProtocolVersion:
			return timelockPolicy{
				CSV:                  legacyLiquidSwapCSV,
				PaymentWindow:        legacyLiquidSwapCSV / 2,
				InvoiceFinalCLTV:     legacyLiquidFinalCLTV,
				AllowNewClaimPayment: false,
			}, nil
		case PEERSWAP_PROTOCOL_VERSION:
			return timelockPolicy{
				CSV:                  liquidSwapCSV,
				PaymentWindow:        liquidPaymentWindow,
				InvoiceFinalCLTV:     liquidInvoiceFinalCLTV,
				MaxTotalCLTVDelta:    liquidMaxTotalCLTVDelta,
				AllowNewClaimPayment: true,
			}, nil
		default:
			return timelockPolicy{}, fmt.Errorf("unsupported protocol version: %d", version)
		}
	default:
		return timelockPolicy{}, fmt.Errorf("unsupported chain: %s", s.GetChain())
	}
}
