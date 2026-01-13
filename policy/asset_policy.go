package policy

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// AssetPolicyRule defines per-asset constraints for Liquid arbitrary-asset swaps.
//
// The configured bounds are interpreted against a swap request's:
// - ln_amount_sat (Lightning side, sats)
// - asset_amount  (on-chain asset base units)
//
// For price constraints we define an implied price as:
//
//	implied_sat_per_unit = ln_amount_sat * price_scale / asset_amount
//
// Constraints are checked without division to avoid rounding issues:
//
//	ln_amount_sat * price_scale >= min_sat_per_unit * asset_amount
//	ln_amount_sat * price_scale <= max_sat_per_unit * asset_amount
type AssetPolicyRule struct {
	// AssetID is the 32-byte hex encoded asset id (big-endian as commonly displayed).
	AssetID string

	// MinAssetAmount/MaxAssetAmount constrain the on-chain asset_amount (base units).
	// A value of 0 means "unset".
	MinAssetAmount uint64
	MaxAssetAmount uint64

	// PriceScale defines what "1 unit" means for the price bounds.
	// Example: if the asset uses 8 decimals, set price_scale=100000000 so
	// min/max_sat_per_unit are expressed per whole token.
	// A value of 0 is treated as 1.
	PriceScale uint64

	// MinSatPerUnit/MaxSatPerUnit bound the implied price.
	// A value of 0 means "unset".
	MinSatPerUnit uint64
	MaxSatPerUnit uint64
}

// UnmarshalFlag parses a policy entry in the following format:
//
//	asset_policy=asset_id=<hex>,min_asset_amount=<n>,max_asset_amount=<n>,price_scale=<n>,min_sat_per_unit=<n>,max_sat_per_unit=<n>
//
// Only asset_id is required; other fields are optional.
func (r *AssetPolicyRule) UnmarshalFlag(value string) error {
	parts := strings.Split(value, ",")
	kv := make(map[string]string, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			return fmt.Errorf("invalid asset_policy segment %q (expected key=value)", part)
		}
		key = strings.TrimSpace(strings.ToLower(key))
		val = strings.TrimSpace(val)
		if key == "" {
			return fmt.Errorf("invalid asset_policy segment %q (empty key)", part)
		}
		kv[key] = val
	}

	assetID, ok := kv["asset_id"]
	if !ok || assetID == "" {
		return fmt.Errorf("asset_policy missing required field asset_id")
	}
	assetID = strings.ToLower(assetID)
	assetBytes, err := hex.DecodeString(assetID)
	if err != nil {
		return fmt.Errorf("asset_policy invalid asset_id: %w", err)
	}
	if len(assetBytes) != 32 {
		return fmt.Errorf("asset_policy invalid asset_id length: %d", len(assetBytes))
	}
	r.AssetID = assetID

	parseUint := func(key string, dst *uint64) error {
		raw, ok := kv[key]
		if !ok || raw == "" {
			return nil
		}
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("asset_policy invalid %s=%q: %w", key, raw, err)
		}
		*dst = n
		return nil
	}

	if err := parseUint("min_asset_amount", &r.MinAssetAmount); err != nil {
		return err
	}
	if err := parseUint("max_asset_amount", &r.MaxAssetAmount); err != nil {
		return err
	}
	if err := parseUint("price_scale", &r.PriceScale); err != nil {
		return err
	}
	if err := parseUint("min_sat_per_unit", &r.MinSatPerUnit); err != nil {
		return err
	}
	if err := parseUint("max_sat_per_unit", &r.MaxSatPerUnit); err != nil {
		return err
	}

	if r.MinAssetAmount > 0 && r.MaxAssetAmount > 0 && r.MinAssetAmount > r.MaxAssetAmount {
		return fmt.Errorf("asset_policy invalid min_asset_amount > max_asset_amount")
	}
	if r.MinSatPerUnit > 0 && r.MaxSatPerUnit > 0 && r.MinSatPerUnit > r.MaxSatPerUnit {
		return fmt.Errorf("asset_policy invalid min_sat_per_unit > max_sat_per_unit")
	}

	return nil
}

func (p *Policy) normalizeAssetPolicies() error {
	if len(p.AssetPolicies) == 0 {
		p.assetPolicyByID = nil
		return nil
	}
	byID := make(map[string]AssetPolicyRule, len(p.AssetPolicies))
	for _, rule := range p.AssetPolicies {
		id := strings.ToLower(rule.AssetID)
		if _, ok := byID[id]; ok {
			return ErrCreatePolicy(fmt.Sprintf("duplicate asset_policy for asset_id=%s", id))
		}
		byID[id] = rule
	}
	p.assetPolicyByID = byID
	return nil
}

// ValidateAssetSwap applies any configured per-asset policy for the given swap
// parameters. If no policy exists for the asset_id, it returns nil.
//
// network is currently unused (asset_id is assumed to be unique per network),
// but is part of the signature for future-proofing.
func (p *Policy) ValidateAssetSwap(network, assetId string, lnAmountSat, assetAmount uint64) error {
	mu.Lock()
	defer mu.Unlock()

	if assetId == "" {
		return nil
	}
	rule, ok := p.assetPolicyByID[strings.ToLower(assetId)]
	if !ok {
		return nil
	}

	if rule.MinAssetAmount > 0 && assetAmount < rule.MinAssetAmount {
		return fmt.Errorf("asset_amount below minimum for asset_id=%s: got %d, min %d", rule.AssetID, assetAmount, rule.MinAssetAmount)
	}
	if rule.MaxAssetAmount > 0 && assetAmount > rule.MaxAssetAmount {
		return fmt.Errorf("asset_amount above maximum for asset_id=%s: got %d, max %d", rule.AssetID, assetAmount, rule.MaxAssetAmount)
	}

	// Price bounds (optional).
	if rule.MinSatPerUnit == 0 && rule.MaxSatPerUnit == 0 {
		return nil
	}
	scale := rule.PriceScale
	if scale == 0 {
		scale = 1
	}

	// Compare using big ints to avoid overflow:
	//   ln_amount_sat * scale ? sat_per_unit * asset_amount
	lhs := new(big.Int).Mul(
		new(big.Int).SetUint64(lnAmountSat),
		new(big.Int).SetUint64(scale),
	)

	if rule.MinSatPerUnit > 0 {
		minRhs := new(big.Int).Mul(
			new(big.Int).SetUint64(rule.MinSatPerUnit),
			new(big.Int).SetUint64(assetAmount),
		)
		if lhs.Cmp(minRhs) < 0 {
			return fmt.Errorf(
				"implied price below minimum for asset_id=%s: ln_amount_sat=%d asset_amount=%d price_scale=%d min_sat_per_unit=%d",
				rule.AssetID, lnAmountSat, assetAmount, scale, rule.MinSatPerUnit,
			)
		}
	}

	if rule.MaxSatPerUnit > 0 {
		maxRhs := new(big.Int).Mul(
			new(big.Int).SetUint64(rule.MaxSatPerUnit),
			new(big.Int).SetUint64(assetAmount),
		)
		if lhs.Cmp(maxRhs) > 0 {
			return fmt.Errorf(
				"implied price above maximum for asset_id=%s: ln_amount_sat=%d asset_amount=%d price_scale=%d max_sat_per_unit=%d",
				rule.AssetID, lnAmountSat, assetAmount, scale, rule.MaxSatPerUnit,
			)
		}
	}

	_ = network // reserved for future use
	return nil
}
