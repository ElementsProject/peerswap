package elements

import (
	"strings"
	"testing"
)

func TestValidateLiquidMainnetElementsVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		chain      string
		version    int
		subversion string
		wantErr    bool
	}{
		{
			name:       "liquid mainnet rejects 23.2.5",
			chain:      liquidMainnetChain,
			version:    230205,
			subversion: "/Elements Core:23.2.5/",
			wantErr:    true,
		},
		{
			name:       "liquid mainnet accepts 23.3.1",
			chain:      liquidMainnetChain,
			version:    230301,
			subversion: "/Elements Core:23.3.1/",
		},
		{
			name:       "liquid mainnet accepts 23.3.3",
			chain:      liquidMainnetChain,
			version:    230303,
			subversion: "/Elements Core:23.3.3/",
		},
		{
			name:       "liquid testnet does not enforce mainnet minimum",
			chain:      "liquidtestnet",
			version:    230205,
			subversion: "/Elements Core:23.2.5/",
		},
		{
			name:       "liquid regtest does not enforce mainnet minimum",
			chain:      "liquidregtest",
			version:    230205,
			subversion: "/Elements Core:23.2.5/",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateLiquidMainnetElementsVersion(tt.chain, tt.version, tt.subversion)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateLiquidMainnetElementsVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				return
			}

			for _, want := range []string{
				unsupportedLiquidMainnetElementsErrorText,
				minLiquidMainnetElementsVersionString,
				recommendedLiquidMainnetElementsVersion,
				tt.subversion,
			} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("validateLiquidMainnetElementsVersion() error = %q, want substring %q", err, want)
				}
			}
		})
	}
}
