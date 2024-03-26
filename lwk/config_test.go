package lwk_test

import (
	"testing"

	"github.com/elementsproject/peerswap/lwk"
	"github.com/stretchr/testify/assert"
)

func TestNewlwkNetwork(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		network string
		want    lwk.LwkNetwork
	}{
		"mainnet": {
			network: "liquid",
			want:    lwk.NetworkMainnet,
		},
		"testnet": {
			network: "liquid-testnet",
			want:    lwk.NetworkTestnet,
		},
		"regtest": {
			network: "liquid-regtest",
			want:    lwk.NetworkRegtest,
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := lwk.NewlwkNetwork(tt.network)
			if err != nil {
				t.Errorf("NewlwkNetwork() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("NewlwkNetwork() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewConfURL(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		endpoint string
		want     string
		wantErr  bool
	}{
		"valid url": {
			endpoint: "http://localhost:32111",
			want:     "http://localhost:32111",
		},
		"without protocol": {
			endpoint: "localhost:32111",
			want:     "localhost:32111",
		},
		"invalid url": {
			endpoint: "invalid url",
			wantErr:  true,
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := lwk.NewConfURL(tt.endpoint)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			if got.String() != tt.want {
				t.Errorf("NewConfURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
