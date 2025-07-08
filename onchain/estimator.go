package onchain

import (
	"context"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/elementsproject/glightning/gbitcoin"
	"github.com/elementsproject/peerswap/log"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
)

const (
	// witnessScaleFactor is the discount for witness data.
	witnessScaleFactor = 4

	// FeePerKwFloor is the lowest fee rate in sat/kw that we should use for
	// estimating transaction fees before signing.
	FeePerKwFloor btcutil.Amount = 253

	// defaultUpdateInterval is the interval in which the minFeeManager collects
	// new data from the bitcoin backend.
	defaultUpdateInterval = 10 * time.Minute

	// DefaultBitcoinStaticFeePerKW is the default fee rate of 50 sat/vb
	// expressed in sat/kw
	DefaultBitcoinStaticFeePerKW = btcutil.Amount(12500)
)

// Estimator is used to estimate on-chain fees for transactions.
type Estimator interface {
	// EstimateFeePerKW returns the estimated fee in sat/kw for a transaction
	// that should be confirmed in targetBlocks.
	EstimateFeePerKW(targetBlocks uint32) (btcutil.Amount, error)

	Start() error
}

// GBitcoindEstimator uses the Bitcoin client from glightning to estimate the
// transaction fee.
type GBitcoindEstimator struct {
	// estimateMode is the field estimate_mode in the estimatesmartfee call. It
	// must be one of "UNSET", "ECONOMICAL" or "CONSERVATIVE".
	estimateMode string

	// fallbackFeeRate is returned in case that the estimator has an error or not
	// enough information to calculate a fee. This value is in sat/kw.
	fallbackFeeRate btcutil.Amount

	// minFeeManager is used to keep track of the minimum fee, in sat/kw,
	// that we should enforce. This will be used as the default fee rate
	// for a transaction when the estimated fee rate is too low to allow
	// the transaction to propagate through the network.
	minFeeManager *minFeeManager

	// bitcoindRpc is an interface that is fulfilled by gbitcoin.
	bitcoindRpc GBitcoindBackend
}

// NewGBitcoindEstimator creates a new BitcoindEstimator given a fully populated
// rpc config that is able to successfully connect and authenticate with the
// bitcoind node, and also a fall back fee rate. The fallback fee rate is used
// in the occasion that the estimator has insufficient data, or returns zero
// for a fee estimate.
func NewGBitcoindEstimator(bitcoindRpc GBitcoindBackend, estimateMode string,
	fallBackFeeRate btcutil.Amount) (*GBitcoindEstimator, error) {

	if ok, err := bitcoindRpc.Ping(); !ok {
		return nil, err
	}

	return &GBitcoindEstimator{
		estimateMode:    estimateMode,
		fallbackFeeRate: fallBackFeeRate,
		bitcoindRpc:     bitcoindRpc,
	}, nil
}

// Start signals the Estimator to start any processes or goroutines
// it needs to perform its duty.
//
// NOTE: This method is part of the Estimator interface.
func (g *GBitcoindEstimator) Start() error {
	// Once the connection to the backend node has been established, we'll
	// initialise the minimum relay fee manager which will query
	// the backend node for its minimum mempool fee.
	relayFeeManager, err := newMinFeeManager(
		defaultUpdateInterval,
		g.fetchMinMempoolFee,
	)
	if err != nil {
		return err
	}
	g.minFeeManager = relayFeeManager

	return nil
}

// fetchMinMempoolFee is used to fetch the minimum fee that the backend node
// requires for a tx to enter its mempool. The returned fee will be the
// maximum of the minimum relay fee and the minimum mempool fee. In sat/kw.
func (g *GBitcoindEstimator) fetchMinMempoolFee() (btcutil.Amount, error) {
	// Fetch and parse the min mempool fee in BTC/kB. mempoolminfee is the max
	// of minrelaytxfee and min mempool fee
	mempoolInfo, err := g.bitcoindRpc.GetMempoolInfo()
	if err != nil {
		return 0, err
	}

	// Convert BTC/kB to sat/kb.
	minMempoolFee, err := btcutil.NewAmount(mempoolInfo.MempoolMinFee)
	if err != nil {
		return 0, err
	}

	// The fee rate is expressed in sat/kB, so we'll manually convert it to
	// our desired sat/kw rate.
	return minMempoolFee / witnessScaleFactor, nil
}

func (g *GBitcoindEstimator) EstimateFeePerKW(targetBlocks uint32) (btcutil.Amount, error) {
	// The EstimateFee function is a wrapper for the rpc call
	// "estimatesmartfee".
	feeEstimate, err := g.estimateFee(targetBlocks)
	switch {
	case err != nil:
		log.Infof("Could not calculate on-chain fee: %v", err)
		fallthrough
	case feeEstimate == 0:
		log.Debugf("Estimated fee is 0, using fallback fee %d", g.fallbackFeeRate)
		return g.fallbackFeeRate, nil

	}

	return feeEstimate, nil
}

func (g *GBitcoindEstimator) estimateFee(targetBlocks uint32) (btcutil.Amount, error) {
	res, err := g.bitcoindRpc.EstimateFee(targetBlocks, g.estimateMode)
	if err != nil {
		return 0, err
	}

	// We need to convert the returned BTC/kB into sat/kB
	satPerKB, err := btcutil.NewAmount(res.FeeRate)
	if err != nil {
		return 0, err
	}

	// Now we want to convert into sat/kw. Therefore we need to divide by the
	// witnessScaleFactor.
	satPerKw := satPerKB / witnessScaleFactor

	// Finally compare the fee to our minimum floor
	minRelayFee := g.minFeeManager.fetchMinFee()
	if satPerKw < minRelayFee {
		log.Debugf("Estimated fee rate %v sat/kw is too low, using floor %v sat/kw", satPerKw, minRelayFee)
		satPerKw = minRelayFee
	}

	return satPerKw, nil
}

// LndEstimator uses the WalletKitClient to estimate the fee.
type LndEstimator struct {
	walletkit walletrpc.WalletKitClient

	// timeout is used as a context timeout on the walletrpc call for
	// EstimateFee.
	timeout time.Duration

	// fallbackFeeRate is returned in case that the estimator has an error or not
	// enough information to calculate a fee. This value is in sat/kw.
	fallbackFeeRate btcutil.Amount
}

func NewLndEstimator(
	walletkit walletrpc.WalletKitClient,
	fallbackFeeRate btcutil.Amount,
	timeout time.Duration,
) (*LndEstimator, error) {
	return &LndEstimator{
		walletkit:       walletkit,
		fallbackFeeRate: fallbackFeeRate,
		timeout:         timeout,
	}, nil
}

// EstimateFeePerKW returns the estimated fee in sat/kw for a transaction
// that should be confirmed in targetBlocks. It uses lnds internal fee
// estimation.
func (l *LndEstimator) EstimateFeePerKW(targetBlocks uint32) (btcutil.Amount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	res, err := l.walletkit.EstimateFee(
		ctx,
		&walletrpc.EstimateFeeRequest{ConfTarget: int32(targetBlocks)},
	)

	switch {
	case err != nil:
		log.Infof("Could not fetch on-chain fee from lnd: %v", err)
		fallthrough
	case res == nil || res.SatPerKw == 0:
		log.Debugf("Estimated fee is 0, using fallback fee %d", l.fallbackFeeRate)
		return l.fallbackFeeRate, nil
	}

	return btcutil.Amount(res.SatPerKw), nil
}

// Start is necessary to implement the Estimator interface but is noop for the
// LndEstimator.
func (l *LndEstimator) Start() error {
	return nil
}

// RegtestFeeEstimator is used as the Estimator when the bitcoin network is set
// to "regtest". We need this fee estimator for cln as lnd uses a static fee
// estimator on regtest that uses a constant fee rate of 12500 sat/kw. See
// "DefaultBitcoinStaticFeePerKW" on chainregistry:
// https://github.com/lightningnetwork/lnd/blob/5c36d96c9cbe8b27c29f9682dcbdab7928ae870f/chainreg/chainregistry.go
type RegtestFeeEstimator struct{}

func NewRegtestFeeEstimator() (*RegtestFeeEstimator, error) {
	return &RegtestFeeEstimator{}, nil
}

// EstimateFeePerKW returns the estimated fee in sat/kw for a transaction
// that should be confirmed in targetBlocks. RegtestFeeEstimator uses the
// DefaultBitcoindStaticFeePerKw.
func (r *RegtestFeeEstimator) EstimateFeePerKW(targetBlocks uint32) (btcutil.Amount, error) {
	return DefaultBitcoinStaticFeePerKW, nil
}

// Start returns nil as we only need it to implement Estimator interface.
func (r *RegtestFeeEstimator) Start() error {
	return nil
}

// minFeeManager is used to store and update the minimum fee that is required
// by a transaction to be accepted to the mempool. The minFeeManager ensures
// that the backend used to fetch the fee is not queried too regularly.
type minFeeManager struct {
	mu                sync.Mutex
	minFeePerKW       btcutil.Amount
	lastUpdatedTime   time.Time
	minUpdateInterval time.Duration
	fetchFeeFunc      fetchFee
}

// fetchFee represents a function that can be used to fetch a fee that is returned
// in sat/kw.
type fetchFee func() (btcutil.Amount, error)

// newMinFeeManager creates a new minFeeManager and uses the
// given fetchMinFee function to set the minFeePerKW of the minFeeManager.
// This function requires the fetchMinFee function to succeed.
func newMinFeeManager(minUpdateInterval time.Duration,
	fetchMinFee fetchFee) (*minFeeManager, error) {

	minFee, err := fetchMinFee()
	if err != nil {
		return nil, err
	}

	return &minFeeManager{
		minFeePerKW:       minFee,
		lastUpdatedTime:   time.Now(),
		minUpdateInterval: minUpdateInterval,
		fetchFeeFunc:      fetchMinFee,
	}, nil
}

// fetchMinFee returns the stored minFeePerKW if it has been updated recently
// or if the call to the chain backend fails. Otherwise, it sets the stored
// minFeePerKW to the fee returned from the backend and floors it based on
// our fee floor.
func (m *minFeeManager) fetchMinFee() btcutil.Amount {
	m.mu.Lock()
	defer m.mu.Unlock()

	if time.Since(m.lastUpdatedTime) < m.minUpdateInterval {
		return m.minFeePerKW
	}

	newMinFee, err := m.fetchFeeFunc()
	if err != nil {
		log.Infof("Unable to fetch updated min fee. "+
			"Using last known min fee instead: %v", err)

		return m.minFeePerKW
	}

	// By default, we'll use the backend node's minimum fee as the
	// minimum fee rate we'll propose for transactions. However, if this
	// happens to be lower than our fee floor, we'll enforce that instead.
	m.minFeePerKW = max(newMinFee, FeePerKwFloor)
	m.lastUpdatedTime = time.Now()

	log.Debugf("Using minimum fee rate of %v sat/kw",
		m.minFeePerKW)

	return m.minFeePerKW
}

type GBitcoindBackend interface {
	GetMempoolInfo() (*gbitcoin.MempoolInfo, error)
	EstimateFee(blocks uint32, mode string) (*gbitcoin.FeeResponse, error)
	Ping() (bool, error)
}
