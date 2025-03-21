package test

import (
	"math"
	"os"
	"testing"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/premium"
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
		swapInPremiumRate:   premium.DefaultBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultBTCSwapOutPremiumRatePPM,
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
	params.chaind.GenerateBlocks(3)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode)

	// Check channel and wallet balance
	require.True(testframework.AssertWaitForChannelBalance(t, params.makerNode, params.scid, float64(params.origMakerBalance+premium), 1., testframework.TIMEOUT))

	require.NoError(testframework.WaitFor(func() bool {
		balance, err := params.makerNode.GetBtcBalanceSat()
		if err != nil {
			t.Logf("get balance errored: %v", err)
			return false
		}
		if balance == params.origMakerWallet-commitFee-claimFee {
			return true
		}
		return false
	}, testframework.TIMEOUT))
}

// Test_Recover_PassedSwap_BTC that peerswap can recover from a swap that
// has already been claimed by the other node (passed csv).
func Test_Recover_PassedSwap_BTC(t *testing.T) {
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
		swapInPremiumRate:   premium.DefaultBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultBTCSwapOutPremiumRatePPM,
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
	_, err := waitForTxInMempool(t, params.chainRpc, testframework.TIMEOUT)
	require.NoError(err)
	params.chaind.GenerateBlocks(1)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode)

	// Stop taker peer so that csv can trigger
	params.takerNode.Stop()

	// Generate enought blocks to trigger csv
	params.chaind.GenerateBlocks(params.csv + 50)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	// Restart taker node and wait for recover
	require.NoError(params.takerNode.Run(true, true))
	require.NoError(params.takerPeerswap.WaitForLog("Recovering from", testframework.TIMEOUT))
	require.NoError(params.takerPeerswap.WaitForLog("Event_ActionFailed on State_SwapOutSender_AwaitTxConfirmation", testframework.TIMEOUT))

	balance, err := params.takerNode.GetChannelBalanceSat(params.scid)
	require.NoError(err)
	require.InDelta(params.origTakerBalance-premium, balance, 1., "expected %d, got %d",
		params.origTakerBalance-premium, balance)
}

// Test_Recover_PassedSwap_LBTC that peerswap can recover from a swap that
// has already been claimed by the other node (passed csv).
func Test_Recover_PassedSwap_LBTC(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	require := require.New(t)

	bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(6)))
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
		chainRpc:            liquidd.RpcProxy,
		chaind:              liquidd,
		confirms:            LiquidConfirms,
		csv:                 LiquidCsv,
		swapType:            swap.SWAPTYPE_OUT,
		premiumLimitRatePPM: 100000,
		swapInPremiumRate:   premium.DefaultBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultBTCSwapOutPremiumRatePPM,
	}
	asset := "lbtc"

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
	_, err := waitForTxInMempool(t, params.chainRpc, testframework.TIMEOUT)
	require.NoError(err)
	params.chaind.GenerateBlocks(1)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode)

	// Stop taker peer so that csv can trigger
	params.takerNode.Stop()

	// Generate enought blocks to trigger csv
	params.chaind.GenerateBlocks(params.csv + 50)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	// Restart taker node and wait for recover
	require.NoError(params.takerNode.Run(true, true))
	require.NoError(params.takerPeerswap.WaitForLog("Recovering from", testframework.TIMEOUT))
	require.NoError(params.takerPeerswap.WaitForLog("Event_ActionFailed on State_SwapOutSender_AwaitTxConfirmation", testframework.TIMEOUT))

	balance, err := params.takerNode.GetChannelBalanceSat(params.scid)
	require.NoError(err)
	require.InDelta(params.origTakerBalance-premium, balance, 1., "expected %d, got %d",
		params.origTakerBalance-premium, balance)
}
