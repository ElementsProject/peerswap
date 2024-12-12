package policy

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/ini.v1"
)

const (
	// premiumRateParts is the total number of parts used to express fee rates.
	premiumRateParts = 1e6
	// defaultSwapInPremiumRatePPM is the default of the swap in premium rate in ppm.
	defaultSwapInPremiumRatePPM int64 = 0
	// defaultSwapOutPremiumRatePPM is the default of the swap out premium rate in ppm.
	defaultSwapOutPremiumRatePPM int64 = 0
	basePremiumSectionKey              = "base_premium_rate"
	peersPremiumSectionKey             = "peers_premium_rate"
	premiumRatePPMKey                  = "premium_rate_ppm"
	swapInPremiumRatePPMKey            = "swap_in_" + premiumRatePPMKey
	SwapOutPremiumRatePPMKey           = "swap_out_" + premiumRatePPMKey
	btcSwapInPremiumRatePPMKey         = "btc_" + swapInPremiumRatePPMKey
	btcSwapOutPremiumRatePPMKey        = "btc_" + SwapOutPremiumRatePPMKey
	lbtcSwapInPremiumRatePPMKey        = "lbtc_" + swapInPremiumRatePPMKey
	lbtcSwapOutPremiumRatePPMKey       = "lbtc_" + SwapOutPremiumRatePPMKey
)

type PremiumRateKind string

const (
	BtcSwapIn   PremiumRateKind = "btcSwapInPremiumRatePPM"
	BtcSwapOut  PremiumRateKind = "btcSwapOutPremiumRatePPM"
	LbtcSwapIn  PremiumRateKind = "lbtcSwapInPremiumRatePPM"
	LbtcSwapOut PremiumRateKind = "lbtcSwapOutPremiumRatePPM"
)

// ppm represents a parts-per-million structure.
type ppm struct {
	ppmValue int64 // ppm value
}

// NewPPM creates a new ppm instance.
func NewPPM(value int64) *ppm {
	return &ppm{ppmValue: value}
}

// Value returns the ppm value.
func (p *ppm) Value() int64 {
	if p == nil {
		return 0
	}
	return p.ppmValue
}

func (p *ppm) Compute(amtSat uint64) (sat int64) {
	return int64(amtSat) * p.ppmValue / premiumRateParts
}

type premiumRates struct {
	// btcSwapInPremiumRatePPM represents the premium rate for BTC swap-in.
	btcSwapInPremiumRatePPM *ppm
	// btcSwapOutPremiumRatePPM represents the premium rate for BTC swap-out.
	btcSwapOutPremiumRatePPM *ppm
	// lbtcSwapInPremiumRatePPM represents the premium rate for LBTC swap-in.
	lbtcSwapInPremiumRatePPM *ppm
	// lbtcSwapOutPremiumRatePPM represents the premium rate for LBTC swap-out.
	lbtcSwapOutPremiumRatePPM *ppm
}

func (p *premiumRates) GetPremiumRate(k PremiumRateKind) *ppm {
	switch k {
	case BtcSwapIn:
		return p.btcSwapInPremiumRatePPM
	case BtcSwapOut:
		return p.btcSwapOutPremiumRatePPM
	case LbtcSwapIn:
		return p.lbtcSwapInPremiumRatePPM
	case LbtcSwapOut:
		return p.lbtcSwapOutPremiumRatePPM
	default:
		return nil
	}
}

type premium struct {
	baseRates        *premiumRates
	premiumByPeerIds map[string]*premiumRates
}

func newPremiumConfig(r io.Reader) (*premium, error) {
	f, err := ini.Load(r)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	premiumByPeerIds := map[string]*premiumRates{}

	for _, key := range f.Section(peersPremiumSectionKey).Keys() {
		parts := strings.Split(key.Name(), ".")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid value to set premium of peer: %s", key.Name())
		}
		peerID := parts[0]
		fieldName := parts[1]
		if _, ok := premiumByPeerIds[peerID]; !ok {
			premiumByPeerIds[peerID] = &premiumRates{}
		}
		switch fieldName {
		case btcSwapInPremiumRatePPMKey:
			premiumByPeerIds[peerID].btcSwapInPremiumRatePPM = NewPPM(key.MustInt64())
		case btcSwapOutPremiumRatePPMKey:
			premiumByPeerIds[peerID].btcSwapOutPremiumRatePPM = NewPPM(key.MustInt64())
		case lbtcSwapInPremiumRatePPMKey:
			premiumByPeerIds[peerID].lbtcSwapInPremiumRatePPM = NewPPM(key.MustInt64())
		case lbtcSwapOutPremiumRatePPMKey:
			premiumByPeerIds[peerID].lbtcSwapOutPremiumRatePPM = NewPPM(key.MustInt64())
		default:
			return nil, fmt.Errorf("invalid field name to set premium of peer: %s", fieldName)
		}
	}
	return &premium{
		baseRates: &premiumRates{
			btcSwapInPremiumRatePPM: NewPPM(
				f.Section(basePremiumSectionKey).Key(btcSwapInPremiumRatePPMKey).MustInt64(defaultSwapInPremiumRatePPM),
			),
			btcSwapOutPremiumRatePPM: NewPPM(
				f.Section(basePremiumSectionKey).Key(btcSwapOutPremiumRatePPMKey).MustInt64(defaultSwapOutPremiumRatePPM),
			),
			lbtcSwapInPremiumRatePPM: NewPPM(
				f.Section(basePremiumSectionKey).Key(lbtcSwapInPremiumRatePPMKey).MustInt64(defaultSwapInPremiumRatePPM),
			),
			lbtcSwapOutPremiumRatePPM: NewPPM(
				f.Section(basePremiumSectionKey).Key(lbtcSwapOutPremiumRatePPMKey).MustInt64(defaultSwapOutPremiumRatePPM),
			),
		},
		premiumByPeerIds: premiumByPeerIds,
	}, nil
}

func (p *premium) GetRate(peerID string, k PremiumRateKind) *ppm {
	if rates, ok := p.premiumByPeerIds[peerID]; ok {
		if ppm := rates.GetPremiumRate(k); ppm != nil {
			return ppm
		}
	}
	return p.baseRates.GetPremiumRate(k)
}

func (p *premium) compute(peerID string, k PremiumRateKind, amtSat uint64) int64 {
	return p.GetRate(peerID, k).Compute(amtSat)
}

func defaultPreium() *premium {
	return &premium{
		baseRates: &premiumRates{
			btcSwapInPremiumRatePPM:   NewPPM(defaultSwapInPremiumRatePPM),
			btcSwapOutPremiumRatePPM:  NewPPM(defaultSwapOutPremiumRatePPM),
			lbtcSwapInPremiumRatePPM:  NewPPM(defaultSwapInPremiumRatePPM),
			lbtcSwapOutPremiumRatePPM: NewPPM(defaultSwapOutPremiumRatePPM),
		},
		premiumByPeerIds: map[string]*premiumRates{},
	}
}
