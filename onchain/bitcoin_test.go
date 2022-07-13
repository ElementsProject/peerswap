package onchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/stretchr/testify/require"
)

func TestBitcoinOnChain_GetFee_UseFallbackFeeRate(t *testing.T) {
	var txSize int64 = 10

	estimator := &EstimatorMock{}
	fallbackFeeRate := btcutil.Amount(300)
	btcOnChain := NewBitcoinOnChain(
		estimator,
		fallbackFeeRate,
		&chaincfg.Params{},
	)

	// Set estimator returned fee rate above fallback and check that the
	// estimated fee is used. 400 sat/kw correspond to 1.6 sat/vb.
	estimator.EstimateFeePerKWReturn = btcutil.Amount(400)
	fee, err := btcOnChain.GetFee(10)
	require.NoError(t, err)
	require.Equal(
		t,
		int64(1.6*float64(txSize)),
		fee,
	)

	// Set estimator returned fee rate to 0 sat/kw and check that the
	// fallback fee rate is used. 300 sat/kw correspond to 1.2 sat/vb.
	estimator.EstimateFeePerKWReturn = btcutil.Amount(0)
	fee, err = btcOnChain.GetFee(10)
	require.NoError(t, err)
	require.Equal(
		t,
		int64(1.2*float64(txSize)),
		fee,
	)
}

func TestBitcoinOnChain_GetFee_UseFloorFeeRate(t *testing.T) {
	var txSize int64 = 10

	estimator := &EstimatorMock{}
	// Set fallback below floor so that floor is used
	fallbackFeeRate := btcutil.Amount(200)
	btcOnChain := NewBitcoinOnChain(
		estimator,
		fallbackFeeRate,
		&chaincfg.Params{},
	)

	// Set estimator returned fee rate above floor and check that the
	// estimated fee is used. 400 sat/kw corrresponds to 1.6 sat/vb.
	estimator.EstimateFeePerKWReturn = btcutil.Amount(400)
	fee, err := btcOnChain.GetFee(10)
	require.NoError(t, err)
	require.Equal(
		t,
		int64(1.6*float64(txSize)),
		fee,
	)

	// Set estimator returned fee rate below the floor and check that the
	// floor fee rate is used. We assume 1.1 sat/vb here.
	estimator.EstimateFeePerKWReturn = btcutil.Amount(200)
	fee, err = btcOnChain.GetFee(10)
	require.NoError(t, err)
	require.Equal(
		t,
		int64(1.1*float64(txSize)),
		fee,
	)
}

type EstimatorMock struct {
	EstimateFeePerKWCalled int
	EstimateFeePerKWReturn btcutil.Amount
	EstimateFeePerKWError  *error
}

func (e *EstimatorMock) EstimateFeePerKW(targetBlocks uint32) (btcutil.Amount, error) {
	e.EstimateFeePerKWCalled++
	if e.EstimateFeePerKWError != nil {
		return e.EstimateFeePerKWReturn, *e.EstimateFeePerKWError
	}
	return e.EstimateFeePerKWReturn, nil
}

func (e *EstimatorMock) Start() error {
	panic("not implemented") // We dont need this function.
}
