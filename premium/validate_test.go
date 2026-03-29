package premium_test

import (
	"context"
	"strings"
	"testing"

	"github.com/elementsproject/peerswap/premium"
	"github.com/samber/lo"
)

func Test_ValidateRate_SignConvention(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		asset       premium.AssetType
		operation   premium.OperationType
		ppm         int64
		complement  int64
		wantErr     bool
		wantContain string
	}{
		"BTC SwapIn negative ok": {
			premium.BTC, premium.SwapIn, -500, 1000, false, "",
		},
		"BTC SwapIn zero ok": {
			premium.BTC, premium.SwapIn, 0, 1000, false, "",
		},
		"BTC SwapIn positive bad": {
			premium.BTC, premium.SwapIn, 500, 1000, true,
			"cannot set BTC SWAP_IN rate to 500 ppm",
		},
		"BTC SwapIn boundary +1 bad": {
			premium.BTC, premium.SwapIn, 1, 1000, true,
			"cannot set BTC SWAP_IN rate to 1 ppm",
		},
		"BTC SwapOut positive ok": {
			premium.BTC, premium.SwapOut, 500, 0, false, "",
		},
		"BTC SwapOut negative bad": {
			premium.BTC, premium.SwapOut, -500, 0, true,
			"cannot set BTC SWAP_OUT rate to -500 ppm",
		},
		"BTC SwapOut boundary -1 bad": {
			premium.BTC, premium.SwapOut, -1, 0, true,
			"cannot set BTC SWAP_OUT rate to -1 ppm",
		},
		"LBTC SwapIn positive bad": {
			premium.LBTC, premium.SwapIn, 100, 1000, true,
			"cannot set LBTC SWAP_IN rate to 100 ppm",
		},
		"LBTC SwapOut negative bad": {
			premium.LBTC, premium.SwapOut, -100, 0, true,
			"cannot set LBTC SWAP_OUT rate to -100 ppm",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			setting := setupPremium(t)
			if tt.operation == premium.SwapIn {
				complement := lo.Must(premium.NewPremiumRate(
					tt.asset, premium.SwapOut, premium.NewPPM(tt.complement)))
				_ = setting.SetDefaultRate(context.Background(), complement)
			} else {
				complement := lo.Must(premium.NewPremiumRate(
					tt.asset, premium.SwapIn, premium.NewPPM(tt.complement)))
				_ = setting.SetDefaultRate(context.Background(), complement)
			}
			rate := lo.Must(premium.NewPremiumRate(
				tt.asset, tt.operation, premium.NewPPM(tt.ppm)))
			err := setting.ValidateDefaultRate(rate)
			if tt.wantErr && err == nil {
				t.Fatal("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error but got: %v", err)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.wantContain) {
				t.Fatalf("expected error containing %q, got: %v",
					tt.wantContain, err)
			}
		})
	}
}

func Test_ValidateRate_SameAssetSum(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		asset        premium.AssetType
		existingIn   int64
		existingOut  int64
		setOp        premium.OperationType
		setPPM       int64
		wantErr      bool
		wantContains string
	}{
		"BTC sum positive ok": {
			premium.BTC, 0, 0, premium.SwapOut, 2000, false, "",
		},
		"BTC sum zero bad": {
			premium.BTC, 0, 0, premium.SwapOut, 0, true,
			"SWAP_OUT then SWAP_IN",
		},
		"BTC sum negative bad": {
			premium.BTC, 0, 0, premium.SwapIn, -3000, true,
			"SWAP_OUT then SWAP_IN",
		},
		"BTC boundary sum=1 ok": {
			premium.BTC, 0, 0, premium.SwapOut, 1, false, "",
		},
		"BTC boundary -999999+999999=0 bad": {
			premium.BTC, -999999, 0, premium.SwapOut, 999999, true,
			"must be > 0",
		},
		"LBTC sum positive ok": {
			premium.LBTC, -500, 0, premium.SwapOut, 1000, false, "",
		},
		"hint about order of operations": {
			premium.BTC, 0, 0, premium.SwapIn, -100, true,
			"increase the SWAP_OUT rate first",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			setting := setupPremium(t)
			rateIn := lo.Must(premium.NewPremiumRate(
				tt.asset, premium.SwapIn, premium.NewPPM(tt.existingIn)))
			_ = setting.SetDefaultRate(context.Background(), rateIn)
			rateOut := lo.Must(premium.NewPremiumRate(
				tt.asset, premium.SwapOut, premium.NewPPM(tt.existingOut)))
			_ = setting.SetDefaultRate(context.Background(), rateOut)

			rate := lo.Must(premium.NewPremiumRate(
				tt.asset, tt.setOp, premium.NewPPM(tt.setPPM)))
			err := setting.ValidateDefaultRate(rate)
			if tt.wantErr && err == nil {
				t.Fatal("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error but got: %v", err)
			}
			if tt.wantErr && tt.wantContains != "" &&
				!strings.Contains(err.Error(), tt.wantContains) {
				t.Fatalf("expected error containing %q, got: %v",
					tt.wantContains, err)
			}
		})
	}
}

func Test_ValidateRate_CrossAssetSum(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		lbtcSwapIn int64
		btcSwapIn  int64
		btcSwapOut int64
		setAsset   premium.AssetType
		setOp      premium.OperationType
		setPPM     int64
		wantErr    bool
	}{
		"cross sum positive via BTC SwapOut": {
			-500, 0, 0, premium.BTC, premium.SwapOut, 1000, false,
		},
		"cross sum zero via BTC SwapOut": {
			-200, -100, 0, premium.BTC, premium.SwapOut, 200, true,
		},
		"cross sum negative via LBTC SwapIn": {
			0, 0, 500, premium.LBTC, premium.SwapIn, -1000, true,
		},
		"cross sum positive via LBTC SwapIn": {
			0, 0, 2000, premium.LBTC, premium.SwapIn, -500, false,
		},
		"BTC SwapIn does not trigger cross check": {
			0, 0, 5000, premium.BTC, premium.SwapIn, -100, false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			setting := setupPremium(t)
			// Set up all four rates so same-asset sum checks pass.
			lbtcIn := lo.Must(premium.NewPremiumRate(
				premium.LBTC, premium.SwapIn, premium.NewPPM(tt.lbtcSwapIn)))
			_ = setting.SetDefaultRate(context.Background(), lbtcIn)
			lbtcOut := lo.Must(premium.NewPremiumRate(
				premium.LBTC, premium.SwapOut, premium.NewPPM(5000)))
			_ = setting.SetDefaultRate(context.Background(), lbtcOut)
			btcOut := lo.Must(premium.NewPremiumRate(
				premium.BTC, premium.SwapOut, premium.NewPPM(tt.btcSwapOut)))
			_ = setting.SetDefaultRate(context.Background(), btcOut)
			btcIn := lo.Must(premium.NewPremiumRate(
				premium.BTC, premium.SwapIn, premium.NewPPM(tt.btcSwapIn)))
			_ = setting.SetDefaultRate(context.Background(), btcIn)

			rate := lo.Must(premium.NewPremiumRate(
				tt.setAsset, tt.setOp, premium.NewPPM(tt.setPPM)))
			err := setting.ValidateDefaultRate(rate)
			if tt.wantErr && err == nil {
				t.Fatal("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error but got: %v", err)
			}
			if tt.wantErr &&
				!strings.Contains(err.Error(), "LBTC_SWAP_IN + BTC_SWAP_OUT") {
				t.Fatalf("expected error about cross-asset sum, got: %v", err)
			}
		})
	}
}

func Test_ValidateRate_PeerSpecific(t *testing.T) {
	t.Parallel()
	setting := setupPremium(t)
	peerID := "test-peer-validate"

	outRate := lo.Must(premium.NewPremiumRate(
		premium.BTC, premium.SwapOut, premium.NewPPM(3000)))
	_ = setting.SetRate(context.Background(), peerID, outRate)

	// -2000 + 3000 = 1000 > 0, should pass
	inRate := lo.Must(premium.NewPremiumRate(
		premium.BTC, premium.SwapIn, premium.NewPPM(-2000)))
	if err := setting.ValidateRate(peerID, inRate); err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}

	// -4000 + 3000 = -1000 <= 0, should fail
	badIn := lo.Must(premium.NewPremiumRate(
		premium.BTC, premium.SwapIn, premium.NewPPM(-4000)))
	if err := setting.ValidateRate(peerID, badIn); err == nil {
		t.Fatal("expected error but got nil")
	}
}

func Test_ValidateRate_OrderOfOperations(t *testing.T) {
	t.Parallel()
	setting := setupPremium(t)

	// Start with valid rates: SWAP_IN=-1000, SWAP_OUT=1500 (sum=500 > 0).
	setDefault := func(asset premium.AssetType, op premium.OperationType, ppm int64) {
		r := lo.Must(premium.NewPremiumRate(asset, op, premium.NewPPM(ppm)))
		_ = setting.SetDefaultRate(context.Background(), r)
	}
	setDefault(premium.BTC, premium.SwapIn, -1000)
	setDefault(premium.BTC, premium.SwapOut, 1500)

	// Target: SWAP_IN=-2000, SWAP_OUT=2500 (sum=500 > 0, also valid).

	// Wrong order: decrease SWAP_IN first.
	// -2000 + 1500 = -500 <= 0 -> should fail.
	wrongFirst := lo.Must(premium.NewPremiumRate(
		premium.BTC, premium.SwapIn, premium.NewPPM(-2000)))
	if err := setting.ValidateDefaultRate(wrongFirst); err == nil {
		t.Fatal("wrong order: expected error when decreasing SWAP_IN first")
	}

	// Correct order: increase SWAP_OUT first.
	// -1000 + 2500 = 1500 > 0 -> should pass.
	correctFirst := lo.Must(premium.NewPremiumRate(
		premium.BTC, premium.SwapOut, premium.NewPPM(2500)))
	if err := setting.ValidateDefaultRate(correctFirst); err != nil {
		t.Fatalf("correct order step 1: expected no error but got: %v", err)
	}
	setDefault(premium.BTC, premium.SwapOut, 2500)

	// Now decrease SWAP_IN.
	// -2000 + 2500 = 500 > 0 -> should pass.
	correctSecond := lo.Must(premium.NewPremiumRate(
		premium.BTC, premium.SwapIn, premium.NewPPM(-2000)))
	if err := setting.ValidateDefaultRate(correctSecond); err != nil {
		t.Fatalf("correct order step 2: expected no error but got: %v", err)
	}
}
