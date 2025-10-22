package scenario

import (
	"testing"

	"github.com/elementsproject/peerswap/swap"
)

func TestExpectationsPremium(t *testing.T) {
	tests := []struct {
		name string
		exp  Expectations
		want int64
	}{
		{
			name: "swap in premium",
			exp: Expectations{
				SwapAmt:           1_000_000,
				SwapType:          swap.SWAPTYPE_IN,
				SwapInPremiumRate: 1500,
			},
			want: 1500,
		},
		{
			name: "swap out premium",
			exp: Expectations{
				SwapAmt:            1_000_000,
				SwapType:           swap.SWAPTYPE_OUT,
				SwapOutPremiumRate: 2000,
			},
			want: 2000,
		},
		{
			name: "unknown type premium",
			exp: Expectations{
				SwapAmt:           1_000_000,
				SwapType:          swap.SwapType(255),
				SwapInPremiumRate: 1500,
			},
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.exp.Premium(); got != tc.want {
				t.Fatalf("Premium() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestExpectationsPreimageChannelBalances(t *testing.T) {
	tests := []struct {
		name       string
		exp        Expectations
		invoiceFee uint64
		wantTaker  float64
		wantMaker  float64
	}{
		{
			name: "swap out channel balances",
			exp: Expectations{
				SwapAmt:            1_000_000,
				SwapType:           swap.SWAPTYPE_OUT,
				SwapOutPremiumRate: 2000,
				OrigTakerChannel:   2_000_000,
				OrigMakerChannel:   500_000,
			},
			invoiceFee: 500,
			wantTaker:  997_500,
			wantMaker:  1_502_500,
		},
		{
			name: "swap in channel balances",
			exp: Expectations{
				SwapAmt:           1_000_000,
				SwapType:          swap.SWAPTYPE_IN,
				SwapInPremiumRate: 1500,
				OrigTakerChannel:  2_500_000,
				OrigMakerChannel:  750_000,
			},
			invoiceFee: 0,
			wantTaker:  1_500_000,
			wantMaker:  1_750_000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.exp.TakerChannelAfterPreimageClaim(tc.invoiceFee); got != tc.wantTaker {
				t.Fatalf("TakerChannelAfterPreimageClaim() = %v, want %v", got, tc.wantTaker)
			}
			if got := tc.exp.MakerChannelAfterPreimageClaim(tc.invoiceFee); got != tc.wantMaker {
				t.Fatalf("MakerChannelAfterPreimageClaim() = %v, want %v", got, tc.wantMaker)
			}
		})
	}
}

func TestExpectationsPreimageWalletBalances(t *testing.T) {
	tests := []struct {
		name      string
		exp       Expectations
		claimFee  uint64
		commitFee uint64
		wantTaker uint64
		wantMaker uint64
	}{
		{
			name: "swap in wallet balances",
			exp: Expectations{
				SwapAmt:           1_000_000,
				SwapType:          swap.SWAPTYPE_IN,
				SwapInPremiumRate: 1500,
				OrigTakerWallet:   200_000,
				OrigMakerWallet:   2_000_000,
			},
			claimFee:  300,
			commitFee: 600,
			wantTaker: 1_201_200,
			wantMaker: 997_900,
		},
		{
			name: "swap out wallet balances",
			exp: Expectations{
				SwapAmt:            1_000_000,
				SwapType:           swap.SWAPTYPE_OUT,
				SwapOutPremiumRate: 2000,
				OrigTakerWallet:    750_000,
				OrigMakerWallet:    2_000_000,
			},
			claimFee:  500,
			commitFee: 700,
			wantTaker: 1_749_500,
			wantMaker: 999_300,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.exp.TakerWalletAfterPreimageClaim(tc.claimFee); got != tc.wantTaker {
				t.Fatalf("TakerWalletAfterPreimageClaim() = %d, want %d", got, tc.wantTaker)
			}
			if got := tc.exp.MakerWalletAfterPreimageClaim(tc.commitFee); got != tc.wantMaker {
				t.Fatalf("MakerWalletAfterPreimageClaim() = %d, want %d", got, tc.wantMaker)
			}
		})
	}
}
