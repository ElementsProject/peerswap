package test

import (
	"math"
	"os"
	"testing"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/stretchr/testify/require"
)

// Test_RestoreFromPassedCSV checks the following scenario: A swap is initiated
// and the opening tx is broadcasted. The maker node then goes offline before
// the opening tx is confirmed such that the taker node can not pay the invoice.
// After the csv limit has passed the maker node goes back online and claims the
// refund.
func Test_RestoreFromPassedCSV(t *testing.T) {
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
		swapAmt:          channelBalances[0] / 2,
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
		swapType:         swap.SWAPTYPE_OUT,
	}
	asset := "btc"

	// Do swap.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
	}()

	var premium uint64
	if params.swapType == swap.SWAPTYPE_OUT {
		// Wait for channel balance to change, this means the invoice was payed.
		testframework.AssertWaitForBalanceChange(t, params.takerNode, params.scid, params.origTakerBalance, testframework.TIMEOUT)
		testframework.AssertWaitForBalanceChange(t, params.makerNode, params.scid, params.origMakerBalance, testframework.TIMEOUT)

		// Get premium from difference.
		newBalance, err := params.takerNode.GetChannelBalanceSat(params.scid)
		require.NoError(err)
		premium = params.origTakerBalance - newBalance
	}

	// Wait for opening tx being broadcasted.
	// Get commitFee.
	commitFee, err := waitForTxInMempool(t, params.chainRpc, testframework.TIMEOUT)
	require.NoError(err)

	// Stop taker peer so that csv can trigger
	params.makerNode.Stop()

	// Generate one less block than required for csv.
	params.chaind.GenerateBlocks(params.csv - 1)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode)

	// Generate one more block to trigger csv.
	params.chaind.GenerateBlocks(1)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode)

	// Restart maker node and wait for recover
	require.NoError(params.makerNode.Run(true, true))
	require.NoError(params.makerPeerswap.WaitForLog("Recovering from", testframework.TIMEOUT))
	require.NoError(params.makerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv", testframework.TIMEOUT))

	// Wait for claim tx being broadcasted.
	// Get claim fee.
	claimFee, err := waitForTxInMempool(t, params.chainRpc, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm claim tx.
	params.chaind.GenerateBlocks(params.confirms)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	// Check channel and wallet balance
	require.True(testframework.AssertWaitForChannelBalance(t, params.makerNode, params.scid, float64(params.origMakerBalance+premium), 1., testframework.TIMEOUT))

	balance, err := params.makerNode.GetBtcBalanceSat()
	require.NoError(err)
	require.InDelta(params.origMakerWallet-commitFee-claimFee, balance, 1., "expected %d, got %d",
		params.origMakerWallet-commitFee-claimFee, balance)
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
			swapAmt:          channelBalances[0] / 2,
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
			swapAmt:          channelBalances[0] / 2,
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
			swapAmt:          channelBalances[0] / 2,
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
			swapAmt:          channelBalances[0] / 2,
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
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
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
			swapAmt:          channelBalances[0] / 2,
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
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
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
			swapAmt:          channelBalances[0] / 2,
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
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
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
			swapAmt:          channelBalances[0] / 2,
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
			swapAmt:          channelBalances[0] / 2,
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
			swapAmt:          channelBalances[0] / 2,
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
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
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
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
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
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*testframework.CLightningNode).Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		csvClaimTest(t, params)
	})

}
