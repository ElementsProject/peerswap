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

	// Stop maker peer so that csv can trigger
	_ = params.makerNode.Stop()

	// Generate one less block than required for csv.
	_ = params.chaind.GenerateBlocks(params.csv - 1)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode)

	// Generate one more block to trigger csv.
	_ = params.chaind.GenerateBlocks(1)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode)

	// Stop taker node to avoid racy claim payment
	_ = params.takerNode.Stop()

	// Restart maker node and wait for recover
	require.NoError(params.makerNode.Run(true, true))
	require.NoError(params.makerPeerswap.WaitForLog("Recovering from", testframework.TIMEOUT))
	require.NoError(params.makerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv", testframework.TIMEOUT))

	// Wait for claim tx being broadcasted.
	// Get claim fee.
	claimFee, err := waitForTxInMempool(t, params.chainRpc, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm claim tx.
	_ = params.chaind.GenerateBlocks(params.confirms)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	// Check channel and wallet balance
	require.True(testframework.AssertWaitForChannelBalance(t, params.makerNode, params.scid, float64(params.origMakerBalance+premium), 1., testframework.TIMEOUT))

	balance, err := params.makerNode.GetBtcBalanceSat()
	require.NoError(err)
	require.InDelta(params.origMakerWallet-commitFee-claimFee, balance, 1., "expected %d, got %d",
		params.origMakerWallet-commitFee-claimFee, balance)
}
