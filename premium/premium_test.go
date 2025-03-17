package premium_test

import (
	"os"
	"path"
	"testing"

	"github.com/elementsproject/peerswap/premium"
	"github.com/stretchr/testify/assert"
	"go.etcd.io/bbolt"

	"github.com/samber/lo"
)

const (
	testPeer = "test-peer"
	testPPM1 = 1000
	testPPM2 = 2000
	testPPM3 = 1500
	testPPM4 = 2500
)

func setupPremium(t *testing.T) *premium.Setting {
	dir := t.TempDir()
	db, err := bbolt.Open(path.Join(dir, "premium-db"), os.ModePerm, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	p, err := premium.NewSetting(db)
	if err != nil {
		t.Fatalf("failed to create premium setting: %v", err)
	}
	return p
}

func Test_Setting_GetRate(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		peerID    string
		asset     premium.AssetType
		operation premium.OperationType
		ppm       int64
	}{
		"BTC SwapIn":   {testPeer, premium.BTC, premium.SwapIn, testPPM1},
		"BTC SwapOut":  {testPeer, premium.BTC, premium.SwapOut, testPPM2},
		"LBTC SwapIn":  {testPeer, premium.LBTC, premium.SwapIn, testPPM3},
		"LBTC SwapOut": {testPeer, premium.LBTC, premium.SwapOut, testPPM4},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			setting := setupPremium(t)
			rate := lo.Must(premium.NewPremiumRate(tt.asset, tt.operation, premium.NewPPM(tt.ppm)))
			// Add test data to the store
			err := setting.SetRate(tt.peerID, rate)
			if err != nil {
				t.Fatalf("failed to set rate: %v", err)
			}
			// Retrieve the rate
			got, err := setting.GetRate(tt.peerID, rate.Asset(), rate.Operation())
			if err != nil {
				t.Fatalf("failed to get rate: %v", err)
			}
			assert.Equal(t, rate, got)
		})
	}
}

func Test_Setting_DeleteRate(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		peerID    string
		asset     premium.AssetType
		operation premium.OperationType
		ppm       int64
		want      int64
	}{
		"BTC SwapIn":   {testPeer, premium.BTC, premium.SwapIn, testPPM1, 0},
		"BTC SwapOut":  {testPeer, premium.BTC, premium.SwapOut, testPPM2, 2000},
		"LBTC SwapIn":  {testPeer, premium.LBTC, premium.SwapIn, testPPM3, 0},
		"LBTC SwapOut": {testPeer, premium.LBTC, premium.SwapOut, testPPM4, 1000},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			setting := setupPremium(t)
			rate := lo.Must(premium.NewPremiumRate(tt.asset, tt.operation, premium.NewPPM(tt.ppm)))
			// Add test data to the store
			err := setting.SetRate(tt.peerID, rate)
			if err != nil {
				t.Fatalf("failed to set rate: %v", err)
			}
			// Delete the rate
			err = setting.DeleteRate(tt.peerID, rate.Asset(), rate.Operation())
			if err != nil {
				t.Fatalf("failed to delete rate: %v", err)
			}
			// Retrieve the rate
			got, err := setting.GetRate(tt.peerID, rate.Asset(), rate.Operation())
			if err != nil {
				t.Fatalf("failed to get rate: %v", err)
			}
			assert.Equal(t, rate.Asset(), got.Asset())
			assert.Equal(t, rate.Operation(), got.Operation())
			assert.Equal(t, tt.want, got.PremiumRatePPM().Value())
		})
	}
}

func Test_Setting_GetRate_Default(t *testing.T) {
	t.Parallel()
	setting := setupPremium(t)
	rate := lo.Must(premium.NewPremiumRate(premium.BTC, premium.SwapIn, premium.NewPPM(testPPM1)))
	err := setting.SetRate("peer", rate)
	if err != nil {
		t.Fatalf("failed to set rate: %v", err)
	}
	// Retrieve the rate
	got, err := setting.GetRate("test", rate.Asset(), rate.Operation())
	if err != nil {
		t.Fatalf("failed to get rate: %v", err)
	}
	assert.Equal(t, rate.Asset(), got.Asset())
	assert.Equal(t, rate.Operation(), got.Operation())
	assert.Equal(t, int64(0), got.PremiumRatePPM().Value())
}

func Test_Setting_Compute(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		peerID    string
		asset     premium.AssetType
		operation premium.OperationType
		ppm       int64
		amtSat    uint64
		expected  int64
	}{
		"BTC SwapIn":   {testPeer, premium.BTC, premium.SwapIn, testPPM1, 1000000, 1000},
		"BTC SwapOut":  {testPeer, premium.BTC, premium.SwapOut, testPPM2, 1000000, 2000},
		"LBTC SwapIn":  {testPeer, premium.LBTC, premium.SwapIn, testPPM3, 1000000, 1500},
		"LBTC SwapOut": {testPeer, premium.LBTC, premium.SwapOut, testPPM4, 1000000, 2500},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			setting := setupPremium(t)
			rate := lo.Must(premium.NewPremiumRate(tt.asset, tt.operation, premium.NewPPM(tt.ppm)))
			// Add test data to the store
			err := setting.SetRate(tt.peerID, rate)
			if err != nil {
				t.Fatalf("failed to set rate: %v", err)
			}
			// Compute the premium
			got, err := setting.Compute(tt.peerID, tt.asset, tt.operation, tt.amtSat)
			if err != nil {
				t.Fatalf("failed to compute premium: %v", err)
			}
			assert.Equal(t, tt.expected, got)
		})
	}
}
