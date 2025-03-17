package test

import (
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_OnlyOneActiveSwapPerChannelCln checks that there is only one active swap per
// channel.
func Test_OnlyOneActiveSwapPerChannelCln(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	require := require.New(t)

	bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(6)))
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
		swapAmt:             channelBalances[0] / 5,
		scid:                scid,
		origTakerWallet:     walletBalances[0],
		origMakerWallet:     walletBalances[1],
		origTakerBalance:    channelBalances[0],
		origMakerBalance:    channelBalances[1],
		takerNode:           lightningds[0],
		makerNode:           lightningds[1],
		takerPeerswap:       lightningds[0].DaemonProcess,
		makerPeerswap:       lightningds[1].DaemonProcess,
		chainRpc:            bitcoind.RpcProxy,
		chaind:              bitcoind,
		confirms:            BitcoinConfirms,
		csv:                 BitcoinCsv,
		swapType:            swap.SWAPTYPE_OUT,
		premiumLimitRatePPM: 100000,
	}
	asset := "btc"

	// Do swap. Expect N_SWAPS - 1 errors.
	wg := sync.WaitGroup{}
	N_SWAPS := 10
	var nErr int32
	for i := 0; i < N_SWAPS; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			var response map[string]interface{}
			err := lightningds[0].Rpc.Request(&clightning.SwapOut{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
			t.Logf("[%d] Response: %v", n, response)
			if err != nil {
				t.Logf("[%d] Err: %s", n, err.Error())
				atomic.AddInt32(&nErr, 1)
			}
		}(i)
	}
	wg.Wait()

	var response *peerswaprpc.ListSwapsResponse
	lightningds[0].Rpc.Request(&clightning.ListActiveSwaps{}, &response)
	t.Logf("GOT: %v", response)

	assert.EqualValues(t, N_SWAPS-1, nErr, "expected nswaps-1=%d errors, got: %d", N_SWAPS-1, nErr)
	assert.EqualValues(t, 1, len(response.Swaps), "expected only 1 active swap, got: %d", len(response.Swaps))
}

func Test_ClnCln_Bitcoin_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
						lines:  1000,
					},
					tailableProcess{
						p:      lightningds[1].DaemonProcess,
						filter: filter,
						lines:  1000,
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			var response map[string]interface{}
			lightningds[1].Rpc.Request(&clightning.SwapIn{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			var response map[string]interface{}
			err := lightningds[1].Rpc.Request(&clightning.SwapIn{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
			require.NoError(err)

		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			var response map[string]interface{}
			lightningds[1].Rpc.Request(&clightning.SwapIn{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)

		}()
		csvClaimTest(t, params)
	})
}

func Test_ClnCln_Bitcoin_SwapOut(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_OUT,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_OUT,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_OUT,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		csvClaimTest(t, params)
	})
}

func Test_ClnLnd_Bitcoin_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_LND)
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       peerswapd.DaemonProcess,
			makerPeerswap:       lightningds[1].(*testframework.CLightningNode).DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[1].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapIn{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_LND)
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       peerswapd.DaemonProcess,
			makerPeerswap:       lightningds[1].(*testframework.CLightningNode).DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[1].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapIn{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_LND)
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       peerswapd.DaemonProcess,
			makerPeerswap:       lightningds[1].(*testframework.CLightningNode).DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[1].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapIn{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		csvClaimTest(t, params)
	})
}

func Test_ClnLnd_Bitcoin_SwapOut(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:       peerswapd.DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_OUT,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapOut{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:       peerswapd.DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_OUT,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapOut{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:       peerswapd.DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_OUT,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapOut{
				SatAmt:              params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		}()
		csvClaimTest(t, params)
	})

}

func Test_ClnCln_ExcessiveAmount(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("excessive", func(t *testing.T) {
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
						p:     lightningds[1].DaemonProcess,
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
			swapAmt:             2 * channelBalances[0],
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_OUT,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		// Swap out should fail as the swap_amt is to high.
		var response map[string]interface{}
		err := lightningds[0].Rpc.Request(&clightning.SwapOut{
			SatAmt:              params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		assert.Error(t, err)

		// Swap in should fail as the swap_amt is to high.
		err = lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset, PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		assert.Error(t, err)
	})
}

func Test_Cln_HtlcMaximum(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()
	t.Run("swapout", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		_, err := lightningds[0].SetHtlcMaximumMilliSatoshis(scid, channelBalances[0]*1000/2-1)
		assert.NoError(t, err)
		// Swap out should fail as the swap_amt is to high.
		var response map[string]interface{}
		err = lightningds[0].Rpc.Request(&clightning.SwapOut{
			SatAmt:              params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		assert.Error(t, err)
	})
	t.Run("swapin", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
		}
		asset := "btc"

		_, err := lightningds[0].SetHtlcMaximumMilliSatoshis(scid, channelBalances[0]*1000/2-1)
		assert.NoError(t, err)
		// Swap in should fail as the swap_amt is to high.
		var response map[string]interface{}
		err = lightningds[1].Rpc.Request(&clightning.SwapIn{
			SatAmt:              params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		assert.Error(t, err)
	})
}

func Test_Cln_Premium(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("negative_swapin", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetupWithConfig(t, uint64(math.Pow10(9)), 0, []string{
			"--dev-bitcoind-poll=1",
			"--dev-fast-gossip",
			"--large-channels",
		}, true, []byte("accept_all_peers=1\n"),
		)
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

		var premiumRatePPM int64 = -10000

		var premiumRes interface{}
		err := lightningds[0].Rpc.Request(&clightning.UpdatePremiumRate{
			PeerID:         lightningds[1].Id(),
			Asset:          premium.BTC.String(),
			Operation:      premium.SwapIn.String(),
			PremiumRatePPM: premiumRatePPM,
		}, &premiumRes)
		assert.NoError(t, err)

		params := &testParams{
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: 100000,
			swapInPremiumRate:   premiumRatePPM,
		}
		asset := "btc"

		var response map[string]interface{}
		err = lightningds[1].Rpc.Request(&clightning.SwapIn{
			SatAmt:              params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		assert.NoError(t, err)

		preimageClaimTest(t, params)
	})

	t.Run("negative_swapout", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetupWithConfig(t, uint64(math.Pow10(9)), 0, []string{
			"--dev-bitcoind-poll=1",
			"--dev-fast-gossip",
			"--large-channels",
		}, true, []byte("accept_all_peers=1\n"),
		)
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

		var premiumRatePPM int64 = -10000

		var premiumRes interface{}
		err := lightningds[1].Rpc.Request(&clightning.UpdatePremiumRate{
			PeerID:         lightningds[0].Id(),
			Asset:          premium.BTC.String(),
			Operation:      premium.SwapOut.String(),
			PremiumRatePPM: premiumRatePPM,
		}, &premiumRes)
		assert.NoError(t, err)

		params := &testParams{
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_OUT,
			premiumLimitRatePPM: 100000,
			swapOutPremiumRate:  premiumRatePPM,
		}
		asset := "btc"

		// Swap out should fail as the premium is to high.
		var response map[string]interface{}
		err = lightningds[0].Rpc.Request(&clightning.SwapOut{
			SatAmt:              params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		assert.NoError(t, err)

		preimageClaimTest(t, params)
	})

	t.Run("exceed_limit", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
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
			swapAmt:             channelBalances[0] / 2,
			scid:                scid,
			origTakerWallet:     walletBalances[0],
			origMakerWallet:     walletBalances[1],
			origTakerBalance:    channelBalances[0],
			origMakerBalance:    channelBalances[1],
			takerNode:           lightningds[0],
			makerNode:           lightningds[1],
			takerPeerswap:       lightningds[0].DaemonProcess,
			makerPeerswap:       lightningds[1].DaemonProcess,
			chainRpc:            bitcoind.RpcProxy,
			chaind:              bitcoind,
			confirms:            BitcoinConfirms,
			csv:                 BitcoinCsv,
			swapType:            swap.SWAPTYPE_IN,
			premiumLimitRatePPM: -1,
		}
		asset := "btc"

		// Swap in should fail as the premium is to high.
		var response map[string]interface{}
		err := lightningds[1].Rpc.Request(&clightning.SwapIn{
			SatAmt:              params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM}, &response)
		assert.Error(t, err)
	})

}

// Test_ClnCln_StuckChannels tests that the swap fails if the channel is stuck.
// For more information about stuck channel, please check the link.
// https://github.com/lightning/bolts/issues/728
func Test_ClnCln_StuckChannels(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	require := require.New(t)
	// repro by using the push_msat in the open_channel.
	// Assumption that feperkw is 253perkw in reg test.
	bitcoind, lightningds, scid := clnclnSetupWithConfig(t, 37500, 35315, []string{
		"--dev-bitcoind-poll=1",
		"--dev-fast-gossip",
		"--large-channels",
		"--min-capacity-sat=1000",
		"--min-emergency-msat=600000",
	}, false, []byte("accept_all_peers=1\n"))

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
					p:     lightningds[1].DaemonProcess,
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
		swapAmt:          channelBalances[0],
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

	assert.NoError(t, lightningds[0].ForceFeeUpdate(scid, "2530"))
	assert.NoError(t, testframework.WaitForWithErr(func() (bool, error) {
		return lightningds[1].IsChannelActive(scid)
	}, testframework.TIMEOUT))
	// Swap in should fail by probing payment as the channel is stuck.
	var response map[string]interface{}
	err := lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: 100, ShortChannelId: params.scid, Asset: "btc"}, &response)
	assert.Error(t, err)
}

func Test_Cln_shutdown(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()
	require := require.New(t)
	bitcoind, lightningds, _ := clnclnSetup(t, uint64(math.Pow10(9)))
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
					p:     lightningds[1].DaemonProcess,
					lines: defaultLines,
				},
			)
		}
	}()
	lightningds[0].Shutdown()
	require.NoError(lightningds[0].WaitForLog(
		"plugin-peerswap: Killing plugin: exited during normal operation", 30))
}

func Test_ClnCln_Poll(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()
	require := require.New(t)
	bitcoind, lightningds, _ := clnclnSetup(t, uint64(math.Pow10(9)))
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
					p:     lightningds[1].DaemonProcess,
					lines: defaultLines,
				},
			)
		}
	}()
	// Ensure that the poll executed at the start of peerswap succeeds
	require.Error(lightningds[0].WaitForLog("failed to send custom message", 20*time.Second))
	for _, lightningd := range lightningds {
		var result interface{}
		err := lightningd.Rpc.Request(&clightning.ReloadPolicyFile{}, &result)
		if err != nil {
			t.Fatal(err)
		}
	}

}
