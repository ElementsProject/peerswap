package onchain

import (
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/elementsproject/glightning/gbitcoin"
	"github.com/stretchr/testify/require"
)

func TestGBitcoin_Estimator(t *testing.T) {
	gbitcoinBackend := &GBitcoinBackendMock{}

	// Set the PingReturn to true so that we mimic a connection.
	gbitcoinBackend.PingReturn = true

	feeEstimator, err := NewGBitcoindEstimator(
		gbitcoinBackend,
		"ECONOMICAL",
		LegacyFeeFloorSatPerKw,
		LegacyFeeFloorSatPerKw,
	)
	require.NoError(t, err)

	// Set GetMempoolInfoReturn value for startup. This value is in BTC/kb
	const mempoolMinFeeBTCPerKb = 0.000001
	gbitcoinBackend.GetMempoolInfoReturn = &gbitcoin.MempoolInfo{
		MempoolMinFee: mempoolMinFeeBTCPerKb,
	}

	// Start fee estimator.
	require.NoError(t, feeEstimator.Start())

	// Check that min fee manager is set. mempoolMinFee should be in sat/kw
	mempoolMinFeeSatPerKb, _ := btcutil.NewAmount(mempoolMinFeeBTCPerKb)
	mempoolMinFeeSatPerKw := mempoolMinFeeSatPerKb / 4
	require.Less(t, mempoolMinFeeSatPerKw, LegacyFeeFloorSatPerKw)
	require.Equal(t, LegacyFeeFloorSatPerKw, feeEstimator.minFeeManager.minFeePerKW)

	// Check that fallback fee is returned if the feeEstimator returns an error
	efe := fmt.Errorf("some error")
	gbitcoinBackend.EstimateFeeError = &efe
	fee, err := feeEstimator.EstimateFeePerKW(10)
	require.NoError(t, err)
	require.Equal(t, LegacyFeeFloorSatPerKw, fee)

	// Check that fee is converted correctly
	feeRateBTCPerKb := 0.00023
	feeRateSatPerKb, _ := btcutil.NewAmount(feeRateBTCPerKb)
	feeRateSatPerKw := feeRateSatPerKb / 4
	gbitcoinBackend.EstimateFeeError = nil
	gbitcoinBackend.EstimateFeeReturn = &gbitcoin.FeeResponse{
		FeeRate: feeRateBTCPerKb,
	}

	fee, err = feeEstimator.EstimateFeePerKW(10)
	require.NoError(t, err)
	require.Equal(t, feeRateSatPerKw, fee)
}

// GBitcoinBackendMock is a mock for the GBitcoinBackend interface that is the
// RPC proxy we use to connect to bitcoind.
type GBitcoinBackendMock struct {
	GetMempoolInfoCalled int
	GetMempoolInfoReturn *gbitcoin.MempoolInfo
	GetMempoolInfoError  *error

	EstimateFeeCalled int
	EstimateFeeReturn *gbitcoin.FeeResponse
	EstimateFeeError  *error

	PingCalled int
	PingReturn bool
	PingError  *error
}

func (m *GBitcoinBackendMock) GetMempoolInfo() (*gbitcoin.MempoolInfo, error) {
	m.GetMempoolInfoCalled++
	if m.GetMempoolInfoError != nil {
		return m.GetMempoolInfoReturn, *m.GetMempoolInfoError
	}
	return m.GetMempoolInfoReturn, nil
}

func (m *GBitcoinBackendMock) EstimateFee(blocks uint32, mode string) (*gbitcoin.FeeResponse, error) {
	m.EstimateFeeCalled++
	if m.EstimateFeeError != nil {
		return m.EstimateFeeReturn, *m.EstimateFeeError
	}
	return m.EstimateFeeReturn, nil
}

func (m *GBitcoinBackendMock) Ping() (bool, error) {
	m.PingCalled++
	if m.PingError != nil {
		return m.PingReturn, *m.PingError
	}
	return m.PingReturn, nil
}
