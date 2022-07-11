package test

import (
	"os"
	"testing"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/stretchr/testify/require"
)

const maxChanSize uint64 = 16777215

// Max payment size is 2^32 msat
const maxPaymentSize uint64 = 4294967

// Test_ClnCln_MPP tests the correctness of MPP if the size of a payment exceed
// the maximum size of a single payment between two cln nodes. It is sufficcient
// to only test on Bitcoin Regtest and only test for swap in cases, as all other
// ways share the same MPP splitter.
func Test_ClnCln_MPP(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	tests := []struct {
		description   string
		swapAmountSat uint64
	}{
		{description: "in max size no mpp", swapAmountSat: maxPaymentSize},
		{description: "in min size mpp", swapAmountSat: maxPaymentSize + 1},
		// Default assumption is that 1% of the channel size is kept as a
		// reserve on both sides (see bolt #2).
		{description: "in max size mpp", swapAmountSat: maxChanSize - 2*((maxChanSize/100)+1)},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			t.Parallel()
			require := require.New(t)

			// We use a higher amount to check if MPP splitting is done
			// correctly. This is the max capacity of a standart channel
			// (not large).
			bitcoind, lightningds, scid := clnclnSetup(t, maxChanSize)
			defer func() {
				if t.Failed() {
					filter := os.Getenv("PEERSWAP_TEST_FILTER")
					pprintFail(
						tailableProcess{
							p:     bitcoind.DaemonProcess,
							lines: defaultLines,
						},
						tailableProcess{
							p:      lightningds[0].DaemonProcess,
							filter: filter,
							lines:  defaultLines,
						},
						tailableProcess{
							p:      lightningds[1].DaemonProcess,
							filter: filter,
							lines:  defaultLines,
						},
					)
				}
			}()

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

			params := &testParams{
				swapAmt:          tt.swapAmountSat,
				scid:             scid,
				origTakerWallet:  walletBalances[0],
				origMakerWallet:  walletBalances[1],
				origTakerBalance: channelBalances[0],
				origMakerBalance: channelBalances[1],
				takerNode:        lightningds[0],
				makerNode:        lightningds[1],
				takerPeerswap:    lightningds[0].DaemonProcess,
				makerPeerswap:    lightningds[1].DaemonProcess,
				chainRpc:         bitcoind.RpcProxy,
				chaind:           bitcoind,
				confirms:         BitcoinConfirms,
				csv:              BitcoinCsv,
				swapType:         swap.SWAPTYPE_IN,
			}
			asset := "btc"

			// Do swap.
			go func() {
				var response map[string]interface{}
				lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
			}()
			preimageClaimTest(t, params)
		})
	}
}

// Test_ClnLnd_MPP tests the correctness of MPP if the size of a payment exceed
// the maximum size of a single payment from a requesting cln node. It is
// sufficcient to only test on Bitcoin Regtest and only test for swap in cases,
// as all other ways share the same MPP splitter.
func Test_ClnLnd_MPP(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	tests := []struct {
		description   string
		swapAmountSat uint64
	}{
		{description: "in max size no mpp", swapAmountSat: maxPaymentSize},
		{description: "in min size mpp", swapAmountSat: maxPaymentSize + 1},
		// Default assumption is that 1% of the channel size is kept as a
		// reserve on both sides (see bolt #2).
		{description: "in max size mpp", swapAmountSat: maxChanSize - 2*((maxChanSize/100)+1)},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			t.Parallel()
			require := require.New(t)

			bitcoind, lightningds, peerswapd, scid := mixedSetup(t, maxChanSize, FUNDER_LND)
			defer func() {
				if t.Failed() {
					filter := os.Getenv("PEERSWAP_TEST_FILTER")
					pprintFail(
						tailableProcess{
							p:     bitcoind.DaemonProcess,
							lines: defaultLines,
						},
						tailableProcess{
							p:     lightningds[0].(*testframework.LndNode).DaemonProcess,
							lines: defaultLines,
						},
						tailableProcess{
							p:      lightningds[1].(*testframework.CLightningNode).DaemonProcess,
							filter: filter,
							lines:  defaultLines,
						},
						tailableProcess{
							p:     peerswapd.DaemonProcess,
							lines: defaultLines,
						},
					)
				}
			}()

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

			params := &testParams{
				swapAmt:          tt.swapAmountSat,
				scid:             scid,
				origTakerWallet:  walletBalances[0],
				origMakerWallet:  walletBalances[1],
				origTakerBalance: channelBalances[0],
				origMakerBalance: channelBalances[1],
				takerNode:        lightningds[0],
				makerNode:        lightningds[1],
				takerPeerswap:    peerswapd.DaemonProcess,
				makerPeerswap:    lightningds[1].(*testframework.CLightningNode).DaemonProcess,
				chainRpc:         bitcoind.RpcProxy,
				chaind:           bitcoind,
				confirms:         BitcoinConfirms,
				csv:              BitcoinCsv,
				swapType:         swap.SWAPTYPE_IN,
			}
			asset := "btc"

			// Do swap.
			go func() {
				// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
				var response map[string]interface{}
				lightningds[1].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
			}()
			preimageClaimTest(t, params)
		})
	}
}
