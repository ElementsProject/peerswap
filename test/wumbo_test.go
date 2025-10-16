package test

import (
	"fmt"
	"testing"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/swap"
)

// maxChanSize is the maximum channel size without the `--large-channels` or
// `wumbo` option.
const maxChanSize uint64 = 16777215

// maxPaymentSize is the sat amount of the older max_payment_size of 2^32 msat.
const maxPaymentSize uint64 = 4294967

// Test_Wumbo tests swaps with the cln option `--large-channels`
// enabled and disabled. This option determines if there is a max swap amount.
func Test_Wumbo(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	type test struct {
		description          string
		largeChannelsEnabled bool
		swapAmtSat           uint64
		swapType             swap.SwapType
		expectedError        error
	}

	tests := []test{
		{
			description:          "out_nolc_max",
			largeChannelsEnabled: false,
			swapAmtSat:           maxPaymentSize,
			swapType:             swap.SWAPTYPE_OUT,
			expectedError:        nil,
		},
		{
			description:          "out_nolc_max+",
			largeChannelsEnabled: false,
			swapAmtSat:           maxPaymentSize + 1,
			swapType:             swap.SWAPTYPE_OUT,
			expectedError: fmt.Errorf(
				"-1:swap amount is 4294968000: need to enable option '--large-channels' " +
					"to swap amounts larger than 2^32 msat",
			),
		},
		{
			description:          "out_lc_max",
			largeChannelsEnabled: true,
			swapAmtSat:           maxPaymentSize,
			swapType:             swap.SWAPTYPE_OUT,
			expectedError:        nil,
		},
		{
			description:          "out_lc_max+",
			largeChannelsEnabled: true,
			swapAmtSat:           maxPaymentSize + 1,
			swapType:             swap.SWAPTYPE_OUT,
			expectedError:        nil,
		},
		{
			description:          "in_nolc_max",
			largeChannelsEnabled: false,
			swapAmtSat:           maxPaymentSize,
			swapType:             swap.SWAPTYPE_IN,
			expectedError:        nil,
		},
		{
			description:          "in_nolc_max+",
			largeChannelsEnabled: false,
			swapAmtSat:           maxPaymentSize + 1,
			swapType:             swap.SWAPTYPE_IN,
			expectedError: fmt.Errorf(
				"-1:swap amount is 4294968000: need to enable option '--large-channels' " +
					"to swap amounts larger than 2^32 msat",
			),
		},
		{
			description:          "in_lc_max",
			largeChannelsEnabled: true,
			swapAmtSat:           maxPaymentSize,
			swapType:             swap.SWAPTYPE_IN,
			expectedError:        nil,
		},
		{
			description:          "in_lc_max+",
			largeChannelsEnabled: true,
			swapAmtSat:           maxPaymentSize + 1,
			swapType:             swap.SWAPTYPE_IN,
			expectedError:        nil,
		},
	}

	for _, tt := range tests {
		// Rebind for parallel tests.
		t.Run(tt.description, func(t *testing.T) {
			t.Parallel()
			require := requireNew(t)

			options := []string{
				"--dev-bitcoind-poll=1",
				"--dev-fast-gossip",
			}

			// Add large-channel option if enabled.
			if tt.largeChannelsEnabled {
				options = append(options, "--large-channels")
			}

			// Test Swap-out
			bitcoind, lightningds, scid := clnclnSetupWithConfig(t, maxChanSize, 0, options, true,
				[]byte("accept_all_peers=1\nswap_in_premium_rate_ppm=0\nswap_out_premium_rate_ppm=0\n"))
			DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

			var channelBalances []uint64
			var walletBalances []uint64
			for _, lightningd := range lightningds {
				b, err := lightningd.GetBtcBalanceSat()
				require.NoError(err)
				walletBalances = append(walletBalances, b)

				b, err = lightningd.GetChannelBalanceSat(scid)
				require.NoError(err)
				channelBalances = append(channelBalances, b)
			}

			var params *testParams
			var err error
			if tt.swapType == swap.SWAPTYPE_OUT {
				params = &testParams{
					swapAmt:             tt.swapAmtSat,
					scid:                scid,
					origTakerWallet:     walletBalances[0],
					origMakerWallet:     walletBalances[1],
					origTakerBalance:    channelBalances[0],
					origMakerBalance:    channelBalances[1],
					takerNode:           lightningds[0],
					makerNode:           lightningds[1],
					takerPeerswap:       lightningds[0].DaemonProcess,
					makerPeerswap:       lightningds[1].DaemonProcess,
					chainRPC:            bitcoind.RpcProxy,
					chaind:              bitcoind,
					confirms:            BitcoinConfirms,
					csv:                 BitcoinCsv,
					swapType:            tt.swapType,
					premiumLimitRatePPM: int64(tt.swapAmtSat / 10),
				}

				var response map[string]interface{}
				err = lightningds[0].Rpc.Request(
					&clightning.SwapOut{
						SatAmt:              params.swapAmt,
						ShortChannelId:      scid,
						Asset:               "btc",
						PremiumLimitRatePPM: params.premiumLimitRatePPM,
					},
					&response,
				)
			} else {
				params = &testParams{
					swapAmt:             tt.swapAmtSat,
					scid:                scid,
					origTakerWallet:     walletBalances[0],
					origMakerWallet:     walletBalances[1],
					origTakerBalance:    channelBalances[0],
					origMakerBalance:    channelBalances[1],
					takerNode:           lightningds[0],
					makerNode:           lightningds[1],
					takerPeerswap:       lightningds[0].DaemonProcess,
					makerPeerswap:       lightningds[1].DaemonProcess,
					chainRPC:            bitcoind.RpcProxy,
					chaind:              bitcoind,
					confirms:            BitcoinConfirms,
					csv:                 BitcoinCsv,
					swapType:            tt.swapType,
					premiumLimitRatePPM: int64(tt.swapAmtSat / 10),
				}

				var response map[string]interface{}
				err = lightningds[1].Rpc.Request(
					&clightning.SwapIn{
						SatAmt:              params.swapAmt,
						ShortChannelId:      scid,
						Asset:               "btc",
						PremiumLimitRatePPM: params.premiumLimitRatePPM,
					},
					&response,
				)
			}

			if tt.expectedError == nil {
				require.NoError(err)
				preimageClaimTest(t, params)
			} else {
				require.EqualError(err, tt.expectedError.Error())
			}
		})
	}
}
