package premium

import "fmt"

// ValidateRate checks that the given rate does not work against the node
// operator's interest, using stored rates for the peer (falling back to
// defaults) for cross-rate checks.
func (p *Setting) ValidateRate(peerID string, rate *PremiumRate) error {
	lookup := func(asset AssetType, op OperationType) int64 {
		r, err := p.GetRate(peerID, asset, op)
		if err != nil {
			return 0
		}
		return r.PremiumRatePPM().Value()
	}
	return validateRate(rate, lookup)
}

// ValidateDefaultRate is like ValidateRate but uses only default rates
// for cross-rate checks.
func (p *Setting) ValidateDefaultRate(rate *PremiumRate) error {
	lookup := func(asset AssetType, op OperationType) int64 {
		r, err := p.GetDefaultRate(asset, op)
		if err != nil {
			return 0
		}
		return r.PremiumRatePPM().Value()
	}
	return validateRate(rate, lookup)
}

// validateRate runs checks in priority order and returns the first violation.
func validateRate(
	rate *PremiumRate,
	lookupRate func(AssetType, OperationType) int64,
) error {
	ppm := rate.PremiumRatePPM().Value()
	asset := rate.Asset()
	op := rate.Operation()

	if err := checkSignConvention(asset, op, ppm); err != nil {
		return err
	}
	if err := checkSameAssetSum(asset, op, ppm, lookupRate); err != nil {
		return err
	}

	// TODO: check BTC_SWAP_IN + LBTC_SWAP_OUT against external Boltz
	// peg-out rate when available (see #394 check 3)

	return checkCrossAssetSum(asset, op, ppm, lookupRate)
}

// checkSignConvention rejects SWAP_IN > 0 and SWAP_OUT < 0.
func checkSignConvention(
	asset AssetType, op OperationType, ppm int64,
) error {
	if op == SwapIn && ppm > 0 {
		return fmt.Errorf(
			"cannot set %s %s rate to %d ppm: positive rate means "+
				"you are paying peers to swap in -- expected negative or zero",
			asset, op, ppm)
	}
	if op == SwapOut && ppm < 0 {
		return fmt.Errorf(
			"cannot set %s %s rate to %d ppm: negative rate means "+
				"you are paying peers to swap out -- expected positive or zero",
			asset, op, ppm)
	}
	return nil
}

// checkSameAssetSum rejects SWAP_IN + SWAP_OUT <= 0 for the same asset.
func checkSameAssetSum(
	asset AssetType, op OperationType, ppm int64,
	lookupRate func(AssetType, OperationType) int64,
) error {
	complementOp := SwapOut
	if op == SwapOut {
		complementOp = SwapIn
	}
	sameAssetSum := ppm + lookupRate(asset, complementOp)
	if sameAssetSum <= 0 {
		return fmt.Errorf(
			"cannot set %s %s rate to %d ppm: %s_SWAP_IN + %s_SWAP_OUT = %d "+
				"(must be > 0) -- peers can profit by doing SWAP_OUT then SWAP_IN; "+
				"hint: if adjusting both rates, increase the SWAP_OUT rate first, "+
				"then decrease the SWAP_IN rate, to avoid temporarily "+
				"triggering this check",
			asset, op, ppm, asset, asset, sameAssetSum)
	}
	return nil
}

// checkCrossAssetSum rejects LBTC_SWAP_IN + BTC_SWAP_OUT <= 0.
func checkCrossAssetSum(
	asset AssetType, op OperationType, ppm int64,
	lookupRate func(AssetType, OperationType) int64,
) error {
	if asset == LBTC && op == SwapIn {
		crossSum := ppm + lookupRate(BTC, SwapOut)
		if crossSum <= 0 {
			return fmt.Errorf(
				"cannot set %s %s rate to %d ppm: "+
					"LBTC_SWAP_IN + BTC_SWAP_OUT = %d (must be > 0) -- "+
					"peers can profit by doing BTC swap-out, "+
					"free peg-in, then LBTC swap-in",
				asset, op, ppm, crossSum)
		}
	}
	if asset == BTC && op == SwapOut {
		crossSum := lookupRate(LBTC, SwapIn) + ppm
		if crossSum <= 0 {
			return fmt.Errorf(
				"cannot set %s %s rate to %d ppm: "+
					"LBTC_SWAP_IN + BTC_SWAP_OUT = %d (must be > 0) -- "+
					"peers can profit by doing BTC swap-out, "+
					"free peg-in, then LBTC swap-in",
				asset, op, ppm, crossSum)
		}
	}
	return nil
}
