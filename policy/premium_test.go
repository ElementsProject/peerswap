package policy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_newPremiumConfig(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		iniFile string
		want    *premium
		wantErr bool
	}{

		"ValidConfig": {
			iniFile: `
[base_premium_rate]
btc_swap_in_premium_rate_ppm=100
[peers_premium_rate]
peer1.btc_swap_in_premium_rate_ppm=1000
peer2.btc_swap_out_premium_rate_ppm=2000`,
			want: &premium{
				baseRates: &premiumRates{
					btcSwapInPremiumRatePPM:   NewPPM(100),
					btcSwapOutPremiumRatePPM:  NewPPM(defaultSwapInPremiumRatePPM),
					lbtcSwapInPremiumRatePPM:  NewPPM(defaultSwapInPremiumRatePPM),
					lbtcSwapOutPremiumRatePPM: NewPPM(defaultSwapInPremiumRatePPM),
				},
				premiumByPeerIds: map[string]*premiumRates{
					"peer1": {
						btcSwapInPremiumRatePPM: NewPPM(1000),
					},
					"peer2": {
						btcSwapOutPremiumRatePPM: NewPPM(2000),
					},
				},
			},
			wantErr: false,
		},
		"InvalidFieldName": {
			iniFile: `
[peers_premium_rate]
peer1.InvalidField=1000
`,
			want:    nil,
			wantErr: true,
		},
		"InvalidKeyFormat": {
			iniFile: `
[peers_premium_rate]
peer1BTCSwapInPremiumRatePPM=1000`,

			want:    nil,
			wantErr: true,
		},
		"EmptyConfig": {
			iniFile: ``,
			want: &premium{
				baseRates: &premiumRates{
					btcSwapInPremiumRatePPM:   NewPPM(defaultSwapInPremiumRatePPM),
					btcSwapOutPremiumRatePPM:  NewPPM(defaultSwapInPremiumRatePPM),
					lbtcSwapInPremiumRatePPM:  NewPPM(defaultSwapInPremiumRatePPM),
					lbtcSwapOutPremiumRatePPM: NewPPM(defaultSwapInPremiumRatePPM),
				},
				premiumByPeerIds: map[string]*premiumRates{},
			},
			wantErr: false,
		},
	}

	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := strings.NewReader(tt.iniFile)
			got, err := newPremiumConfig(r)
			if (err != nil) != tt.wantErr {
				t.Errorf("newPremiumConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
func Test_premium_GetRate(t *testing.T) {
	t.Parallel()
	type args struct {
		peerID string
		k      PremiumRateKind
	}
	tests := map[string]struct {
		p    *premium
		args args
		want *ppm
	}{
		"GetRateForExistingPeer": {
			p: &premium{
				baseRates: &premiumRates{
					btcSwapInPremiumRatePPM:   NewPPM(100),
					btcSwapOutPremiumRatePPM:  NewPPM(200),
					lbtcSwapInPremiumRatePPM:  NewPPM(300),
					lbtcSwapOutPremiumRatePPM: NewPPM(400),
				},
				premiumByPeerIds: map[string]*premiumRates{
					"peer1": {
						btcSwapInPremiumRatePPM: NewPPM(1000),
					},
				},
			},
			args: args{
				peerID: "peer1",
				k:      BtcSwapIn,
			},
			want: NewPPM(1000),
		},
		"GetRateForNonExistingPeer": {
			p: &premium{
				baseRates: &premiumRates{
					btcSwapInPremiumRatePPM:   NewPPM(100),
					btcSwapOutPremiumRatePPM:  NewPPM(200),
					lbtcSwapInPremiumRatePPM:  NewPPM(300),
					lbtcSwapOutPremiumRatePPM: NewPPM(400),
				},
				premiumByPeerIds: map[string]*premiumRates{},
			},
			args: args{
				peerID: "peer2",
				k:      BtcSwapIn,
			},
			want: NewPPM(100),
		},
		"GetRateForExistingPeerWithDifferentKind": {
			p: &premium{
				baseRates: &premiumRates{
					btcSwapInPremiumRatePPM:   NewPPM(100),
					btcSwapOutPremiumRatePPM:  NewPPM(200),
					lbtcSwapInPremiumRatePPM:  NewPPM(300),
					lbtcSwapOutPremiumRatePPM: NewPPM(400),
				},
				premiumByPeerIds: map[string]*premiumRates{
					"peer1": {
						btcSwapInPremiumRatePPM: NewPPM(1000),
					},
				},
			},
			args: args{
				peerID: "peer1",
				k:      BtcSwapOut,
			},
			want: NewPPM(200),
		},
		"GetRateForNonExistingPeerWithDifferentKind": {
			p: &premium{
				baseRates: &premiumRates{
					btcSwapInPremiumRatePPM:   NewPPM(100),
					btcSwapOutPremiumRatePPM:  NewPPM(200),
					lbtcSwapInPremiumRatePPM:  NewPPM(300),
					lbtcSwapOutPremiumRatePPM: NewPPM(400),
				},
				premiumByPeerIds: map[string]*premiumRates{},
			},
			args: args{
				peerID: "peer2",
				k:      LbtcSwapIn,
			},
			want: NewPPM(300),
		},
	}

	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tt.p.GetRate(tt.args.peerID, tt.args.k); !assert.Equal(t, tt.want, got) {
				t.Errorf("premium.GetRate() = %v, want %v", got, tt.want)
			}
		})
	}
}
