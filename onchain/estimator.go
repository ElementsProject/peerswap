package onchain

// Estimator is used to estimate on-chain fees for transactions.
type Estimator interface {
	// EstimateFeePerKw returns the estimated fee in sat/kw for a transaction
	// that should be confirmed in targetBlocks.
	EstimateFeePerKW(targetBlocks uint32) (uint32, error)
}
