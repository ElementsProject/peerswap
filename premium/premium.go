package premium

import (
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

	premiumRatePPMKey = "premium_rate_ppm"
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

// Observer pattern implementation
// -----------------------------

// Observer defines the interface for objects that should be notified of premium rate changes
type Observer interface {
	// OnPremiumUpdate is called when premium rates are updated
	OnPremiumUpdate()
}

type Setting struct {
	store    *BBoltPremiumStore
	observer []Observer // List of observers to be notified of premium rate changes
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

// AddObserver registers a new observer to be notified of premium rate changes
func (p *Setting) AddObserver(observer Observer) {
	p.observer = append(p.observer, observer)
}

// notifyObservers notifies all registered observers when premium rates change
func (p *Setting) notifyObservers() {
	for _, observer := range p.observer {
		observer.OnPremiumUpdate()
	}
}

// Premium rate operations
// ---------------------

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

// SetRate sets the premium rate for a given peer and notifies observers if successful.
func (p *Setting) SetRate(peerID string, rate *PremiumRate) error {
	err := p.store.SetRate(peerID, rate)
	if err == nil {
		// Notify observers only if the update was successful
		p.notifyObservers()
	}
	return err
}

func (p *Setting) DeleteRate(peerID string, asset AssetType, operation OperationType) error {
	err := p.store.DeleteRate(peerID, asset, operation)
	if err == nil {
		// Notify observers only if the update was successful
		p.notifyObservers()
	}
	return err
}

// GetDefaultRate retrieves the default premium rate for a given asset and operation.
func (p *Setting) GetDefaultRate(asset AssetType, operation OperationType) (*PremiumRate, error) {
	rate, err := p.store.GetDefaultRate(asset, operation)
	if err != nil {
		if errors.Is(err, ErrRateNotFound) {
			return NewPremiumRate(asset, operation, NewPPM(DefaultPremiumRate[asset][operation]))
		}
		return nil, err
	}
	return rate, nil
}

// SetDefaultRate sets the default premium rate and notifies observers if successful.
func (p *Setting) SetDefaultRate(rate *PremiumRate) error {
	err := p.store.SetDefaultRate(rate)
	if err == nil {
		// Notify observers only if the update was successful
		p.notifyObservers()
	}
	return err
}

// Compute calculates the premium in satoshis for a given amount in satoshis.
func (p *Setting) Compute(peerID string, asset AssetType, operation OperationType, amtSat uint64) (int64, error) {
	rate, err := p.GetRate(peerID, asset, operation)
	if err != nil {
		return 0, err
	}
	return rate.PremiumRatePPM().Compute(amtSat), nil
}
