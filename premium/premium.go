package premium

import (
	"context"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
)

const (
	// premiumRateParts is the total number of parts used to express fee rates.
	premiumRateParts = 1e6
	// DefaultBTCSwapInPremiumRatePPM is the default premium rate in ppm.
	DefaultBTCSwapInPremiumRatePPM int64 = 0
	// DefaultBTCSwapOutPremiumRatePPM is the default premium rate in ppm.
	DefaultBTCSwapOutPremiumRatePPM int64 = 2000
	// DefaultLBTCSwapInPremiumRatePPM is the default premium rate in ppm.
	DefaultLBTCSwapInPremiumRatePPM int64 = 0
	// DefaultLBTCSwapOutPremiumRatePPM is the default premium rate in ppm.
	DefaultLBTCSwapOutPremiumRatePPM int64 = 1000
)

var DefaultPremiumRate = map[AssetType]map[OperationType]int64{
	BTC: {
		SwapIn:  DefaultBTCSwapInPremiumRatePPM,
		SwapOut: DefaultBTCSwapOutPremiumRatePPM,
	},
	LBTC: {
		SwapIn:  DefaultLBTCSwapInPremiumRatePPM,
		SwapOut: DefaultLBTCSwapOutPremiumRatePPM,
	},
}

// Enum for supported asset types.
type AssetType int32

const (
	AsserUnspecified AssetType = iota
	BTC
	LBTC
)

func (a AssetType) String() string {
	return [...]string{"Unspecified", "BTC", "LBTC"}[a]
}

// Enum for supported operation types.
type OperationType int32

const (
	OperationUnspecified OperationType = iota
	SwapIn
	SwapOut
)

func (o OperationType) String() string {
	return [...]string{"Unspecified", "SWAP_IN", "SWAP_OUT"}[o]
}

// PremiumRate defines the premium rate for a specific asset and operation.
type PremiumRate struct {
	asset       AssetType
	operation   OperationType
	premiumRate *PPM
}

// NewPremiumRate creates a new PremiumRate instance with validation.
func NewPremiumRate(asset AssetType, operation OperationType, premiumRate *PPM) (*PremiumRate, error) {
	if asset == AsserUnspecified {
		return nil, fmt.Errorf("invalid asset type")
	}
	if operation == OperationUnspecified {
		return nil, fmt.Errorf("invalid operation type")
	}
	return &PremiumRate{
		asset:       asset,
		operation:   operation,
		premiumRate: premiumRate,
	}, nil
}

// Asset returns the asset type.
func (p *PremiumRate) Asset() AssetType {
	return p.asset
}

// Operation returns the operation type.
func (p *PremiumRate) Operation() OperationType {
	return p.operation
}

// PremiumRatePPM returns the premium rate in ppm.
func (p *PremiumRate) PremiumRatePPM() *PPM {
	return p.premiumRate
}

// PPM represents a parts-per-million structure.
type PPM struct {
	ppmValue int64 // ppm value
}

// NewPPM creates a new ppm instance.
func NewPPM(value int64) *PPM {
	return &PPM{ppmValue: value}
}

// Value returns the ppm value.
func (p *PPM) Value() int64 {
	if p == nil {
		return 0
	}
	return p.ppmValue
}

// Compute calculates the premium in satoshis for a given amount in satoshis.
func (p *PPM) Compute(amtSat uint64) (sat int64) {
	return int64(amtSat) * p.ppmValue / premiumRateParts
}

// Premium rate operations
// ---------------------

type Setting struct {
	store *BBoltPremiumStore
}

func NewSetting(bbolt *bbolt.DB) (*Setting, error) {
	store, err := NewBBoltPremiumStore(bbolt)
	if err != nil {
		return nil, err
	}
	return &Setting{
		store: store,
	}, nil
}

// GetRate retrieves the premium rate for a given peer, asset, and operation.
// If the rate is not found, it retrieves the default rate.
func (p *Setting) GetRate(peerID string, asset AssetType, operation OperationType) (*PremiumRate, error) {
	rate, err := p.store.GetRate(peerID, asset, operation)
	if err != nil {
		if errors.Is(err, ErrRateNotFound) {
			return p.GetDefaultRate(asset, operation)
		}
		return nil, err
	}
	return rate, nil
}

// SetRate sets the premium rate for a given peer.
func (p *Setting) SetRate(ctx context.Context, peerID string, rate *PremiumRate) error {
	return p.store.SetRate(peerID, rate)
}

func (p *Setting) DeleteRate(ctx context.Context, peerID string, asset AssetType, operation OperationType) error {
	return p.store.DeleteRate(peerID, asset, operation)
}

// GetDefaultRate retrieves the default premium rate for a given asset and operation.
func (p *Setting) GetDefaultRate(asset AssetType, operation OperationType) (*PremiumRate, error) {
	rate, err := p.store.GetDefaultRate(asset, operation)
	if err != nil {
		if errors.Is(err, ErrRateNotFound) {
			ratesByAsset, ok := DefaultPremiumRate[asset]
			if !ok {
				return nil, fmt.Errorf("no default premium rate configured for asset %v", asset)
			}
			value, ok := ratesByAsset[operation]
			if !ok {
				return nil, fmt.Errorf("no default premium rate configured for asset %v and operation %v", asset, operation)
			}
			return NewPremiumRate(asset, operation, NewPPM(value))
		}
		return nil, err
	}
	return rate, nil
}

// SetDefaultRate sets the default premium rate.
func (p *Setting) SetDefaultRate(ctx context.Context, rate *PremiumRate) error {
	return p.store.SetDefaultRate(rate)
}

// Compute calculates the premium in satoshis for a given amount in satoshis.
func (p *Setting) Compute(peerID string, asset AssetType, operation OperationType, amtSat uint64) (int64, error) {
	rate, err := p.GetRate(peerID, asset, operation)
	if err != nil {
		return 0, err
	}
	return rate.PremiumRatePPM().Compute(amtSat), nil
}
