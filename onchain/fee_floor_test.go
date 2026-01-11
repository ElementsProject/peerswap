package onchain

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/google/go-cmp/cmp"
)

func TestDetermineFeeFloor(t *testing.T) {
	tcs := []struct {
		name           string
		input          string
		wantFloor      btcutil.Amount
		wantNormalized string
	}{
		{
			name:           "modern version",
			input:          "/Satoshi:29.2.0/",
			wantFloor:      ModernFeeFloorSatPerKw,
			wantNormalized: "29.2.0",
		},
		{
			name:           "legacy version",
			input:          "/Satoshi:29.1.1/",
			wantFloor:      LegacyFeeFloorSatPerKw,
			wantNormalized: "29.1.1",
		},
		{
			name:           "unknown input",
			input:          "custom",
			wantFloor:      LegacyFeeFloorSatPerKw,
			wantNormalized: "",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotFloor, gotNormalized := DetermineFeeFloor(tc.input)

			if diff := cmp.Diff(tc.wantFloor, gotFloor); diff != "" {
				t.Errorf("fee floor mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantNormalized, gotNormalized); diff != "" {
				t.Errorf("normalized version mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
