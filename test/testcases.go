package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/stretchr/testify/require"
)

type testParams struct {
	swapAmt          uint64
	scid             string
	origTakerWallet  uint64
	origMakerWallet  uint64
	origTakerBalance uint64
	origMakerBalance uint64
	takerNode        testframework.LightningNode
	makerNode        testframework.LightningNode
	takerPeerswap    *testframework.DaemonProcess
	makerPeerswap    *testframework.DaemonProcess
	chainRpc         *testframework.RpcProxy
	chaind           ChainNode
	confirms         int
	csv              int
	swapType         swap.SwapType
}

func coopClaimTest(t *testing.T, params *testParams) {
	require := require.New(t)
	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitFee.
	commitFee, err := waitForTxInMempool(t, params.chainRpc, testframework.TIMEOUT)
	require.NoError(err)

	//
	//	STEP 2: Move balance
	//
	// Move local balance from taker to maker so that the taker does not
	// have enough balance to pay the invoice and cancels the swap coop.
	feeInvoiceAmt, err := params.makerNode.GetFeeInvoiceAmtSat()
	require.NoError(err)

	moveAmt := (params.origTakerBalance - feeInvoiceAmt - params.swapAmt) + 1
	inv, err := params.makerNode.AddInvoice(moveAmt, "shift balance", "")
	require.NoError(err)

	err = params.takerNode.SendPay(inv, params.scid)
	require.NoError(err)

	// Check channel taker balance is less than the swapAmt.
	var setTakerFunds uint64
	err = testframework.WaitFor(func() bool {
		setTakerFunds, err = params.takerNode.GetChannelBalanceSat(params.scid)
		require.NoError(err)
		return setTakerFunds == params.swapAmt-1
	}, testframework.TIMEOUT)
	require.NoError(err)

	//
	//	STEP 3: Confirm opening tx
	//

	params.chaind.GenerateBlocks(params.confirms)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode, params.makerNode)

	// Check that coop close was sent.
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		require.NoError(params.takerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_SendCoopClose", testframework.TIMEOUT))
	case swap.SWAPTYPE_OUT:
		require.NoError(params.takerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapOutSender_SendCoopClose", testframework.TIMEOUT))
	default:
		t.Fatal("unknown role")
	}

	//
	//	STEP 4: Broadcasting coop claim tx
	//

	// Wait for coop claim tx being broadcasted.
	// Get claim fee.
	claimFee, err := waitForTxInMempool(t, params.chainRpc, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm coop claim tx.
	params.chaind.GenerateBlocks(params.confirms)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode, params.makerNode)

	// Check swap is done.
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		require.NoError(params.makerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCoop", testframework.TIMEOUT))
	case swap.SWAPTYPE_OUT:
		require.NoError(params.makerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCoop", testframework.TIMEOUT))
	default:
		t.Fatal("unknown role")
	}

	// Check no invoice was paid.
	testframework.RequireWaitForChannelBalance(t, params.takerNode, params.scid, float64(setTakerFunds), 1., testframework.TIMEOUT)

	// Check Wallet balance.
	// Expect:
	// - [0] before
	// - [1] before - commitment_fee - claim_fee
	balance, err := params.takerNode.GetBtcBalanceSat()
	require.NoError(err)
	require.EqualValues(params.origTakerWallet, float64(balance), "expected %d, got %d", params.origTakerWallet, balance)

	balance, err = params.makerNode.GetBtcBalanceSat()
	require.NoError(err)
	require.InDelta((params.origMakerWallet - commitFee - claimFee), float64(balance), 1., "expected %d, got %d",
		(params.origMakerWallet - commitFee - claimFee), balance)
}

func preimageClaimTest(t *testing.T, params *testParams) {
	require := require.New(t)

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

	// Confirm opening tx.
	require.NoError(params.takerPeerswap.WaitForLog("Await confirmation for tx", testframework.TIMEOUT))
	params.chaind.GenerateBlocks(params.confirms)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode, params.makerNode)

	// Wait for invoice being paid.
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		require.NoError(params.makerPeerswap.WaitForLog("Event_OnClaimInvoicePaid on State_SwapInSender_AwaitClaimPayment", testframework.TIMEOUT))
	case swap.SWAPTYPE_OUT:
		require.NoError(params.makerPeerswap.WaitForLog("Event_OnClaimInvoicePaid on State_SwapOutReceiver_AwaitClaimInvoicePayment", testframework.TIMEOUT))
	default:
		t.Fatal("unknown role")
	}

	// Check channel balances match.
	// premium is only !=0 when swap type is swap_out.
	expected := float64(params.origTakerBalance - params.swapAmt - premium)
	require.True(testframework.AssertWaitForChannelBalance(t, params.takerNode, params.scid, expected, 1., testframework.TIMEOUT))

	expected = float64(params.origMakerBalance + params.swapAmt + premium)
	require.True(testframework.AssertWaitForChannelBalance(t, params.makerNode, params.scid, expected, 1., testframework.TIMEOUT))

	// Wait for claim tx being broadcasted.
	// Get claim fee.
	claimFee, err := waitForTxInMempool(t, params.chainRpc, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm claim tx.
	params.chaind.GenerateBlocks(params.confirms)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode, params.makerNode)

	// Wait for claim done
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		require.NoError(params.takerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_ClaimSwap", testframework.TIMEOUT))
	case swap.SWAPTYPE_OUT:
		require.NoError(params.takerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapOutSender_ClaimSwap", testframework.TIMEOUT))
	default:
		t.Fatal("unknown role")
	}

	// Check Wallet balance.
	// Expect: (WITHOUT PREMIUM)
	// - taker -> before - claim_fee + swapamt
	// - maker -> before - commitment_fee - swapamt
	balance, err := params.takerNode.GetBtcBalanceSat()
	require.NoError(err)
	require.InDelta(params.origTakerWallet-claimFee+params.swapAmt, float64(balance), 1., "expected %d, got %d",
		params.origTakerWallet-claimFee+params.swapAmt, balance)

	balance, err = params.makerNode.GetBtcBalanceSat()
	require.NoError(err)
	require.InDelta((params.origMakerWallet - commitFee - params.swapAmt), float64(balance), 1., "expected %d, got %d",
		(params.origMakerWallet - commitFee - params.swapAmt), balance)

	// Check latest invoice memo should be of the form "swap-in btc claim <swap_id>"
	bolt11, err := params.makerNode.GetLatestInvoice()
	require.NoError(err)

	memo, err := params.makerNode.GetMemoFromPayreq(bolt11)
	require.NoError(err)
	expectedMemo := fmt.Sprintf("peerswap %s claim %s", params.chaind.ReturnAsset(), params.scid)
	require.True(
		strings.Contains(memo, expectedMemo),
		"Expected memo to contain: %s, got: %s",
		expectedMemo,
		memo,
	)
}

func csvClaimTest(t *testing.T, params *testParams) {
	require := require.New(t)

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
	params.takerPeerswap.Kill()

	// if the taker is lnd kill it
	switch params.takerNode.(type) {
	case *testframework.LndNode:
		params.takerNode.(*testframework.LndNode).Kill()

	}

	// Generate one less block than required for csv.
	params.chaind.GenerateBlocks(params.csv - 1)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	// Check that csv is not claimed yet
	var triedToClaim bool
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		triedToClaim, err = params.makerPeerswap.HasLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCsv")
	case swap.SWAPTYPE_OUT:
		triedToClaim, err = params.makerPeerswap.HasLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv")
	default:
		t.Fatal("unknown swap type")
	}
	require.NoError(err)
	require.False(triedToClaim)

	// Generate one more block to trigger csv.
	params.chaind.GenerateBlocks(1)
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	switch params.swapType {
	case swap.SWAPTYPE_IN:
		require.NoError(params.makerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCsv", testframework.TIMEOUT))
	case swap.SWAPTYPE_OUT:
		require.NoError(params.makerPeerswap.WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv", testframework.TIMEOUT))
	default:
		t.Fatal("unknown swap type")
	}

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
