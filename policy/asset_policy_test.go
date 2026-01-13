package policy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func randomAssetIDHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}

func Test_AssetPolicy_ParseAndLookup(t *testing.T) {
	assetID := randomAssetIDHex(t)
	conf := fmt.Sprintf(
		"asset_policy=asset_id=%s,min_asset_amount=10,max_asset_amount=100,price_scale=100000000,min_sat_per_unit=1,max_sat_per_unit=1000\n",
		assetID,
	)

	p, err := create(strings.NewReader(conf))
	require.NoError(t, err)
	require.Len(t, p.AssetPolicies, 1)
	require.NotNil(t, p.assetPolicyByID)
	_, ok := p.assetPolicyByID[assetID]
	require.True(t, ok)
}

func Test_AssetPolicy_DuplicateRejected(t *testing.T) {
	assetID := randomAssetIDHex(t)
	conf := fmt.Sprintf(
		"asset_policy=asset_id=%s,min_asset_amount=1\nasset_policy=asset_id=%s,max_asset_amount=2\n",
		assetID,
		assetID,
	)

	_, err := create(strings.NewReader(conf))
	require.Error(t, err)
}

func Test_AssetPolicy_ValidateAssetAmount(t *testing.T) {
	assetID := randomAssetIDHex(t)
	conf := fmt.Sprintf(
		"asset_policy=asset_id=%s,min_asset_amount=100,max_asset_amount=200\n",
		assetID,
	)
	p, err := create(strings.NewReader(conf))
	require.NoError(t, err)

	require.Error(t, p.ValidateAssetSwap("liquid-regtest", assetID, 1, 99))
	require.NoError(t, p.ValidateAssetSwap("liquid-regtest", assetID, 1, 100))
	require.NoError(t, p.ValidateAssetSwap("liquid-regtest", assetID, 1, 200))
	require.Error(t, p.ValidateAssetSwap("liquid-regtest", assetID, 1, 201))
}

func Test_AssetPolicy_ValidateImpliedPrice(t *testing.T) {
	assetID := randomAssetIDHex(t)
	conf := fmt.Sprintf(
		"asset_policy=asset_id=%s,price_scale=1,min_sat_per_unit=2,max_sat_per_unit=4\n",
		assetID,
	)
	p, err := create(strings.NewReader(conf))
	require.NoError(t, err)

	// ln=10, asset=4 -> 2.5 sats/unit (ok)
	require.NoError(t, p.ValidateAssetSwap("liquid", assetID, 10, 4))

	// ln=1, asset=1 -> 1 sats/unit (below min)
	require.Error(t, p.ValidateAssetSwap("liquid", assetID, 1, 1))

	// ln=10, asset=1 -> 10 sats/unit (above max)
	require.Error(t, p.ValidateAssetSwap("liquid", assetID, 10, 1))
}

func Test_AssetPolicy_ValidateImpliedPrice_WithScale(t *testing.T) {
	assetID := randomAssetIDHex(t)
	conf := fmt.Sprintf(
		"asset_policy=asset_id=%s,price_scale=100,min_sat_per_unit=50,max_sat_per_unit=150\n",
		assetID,
	)
	p, err := create(strings.NewReader(conf))
	require.NoError(t, err)

	// implied = ln*100/asset
	// ln=1, asset=2 => 50 (ok)
	require.NoError(t, p.ValidateAssetSwap("liquid", assetID, 1, 2))

	// ln=1, asset=3 => 33.33... (below min)
	require.Error(t, p.ValidateAssetSwap("liquid", assetID, 1, 3))

	// ln=2, asset=1 => 200 (above max)
	require.Error(t, p.ValidateAssetSwap("liquid", assetID, 2, 1))
}
