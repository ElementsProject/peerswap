package test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/stretchr/testify/require"
)

type testParams struct {
	swapAmt            uint64
	scid               string
	origTakerWallet    uint64
	origMakerWallet    uint64
	origTakerBalance   uint64
	origMakerBalance   uint64
	takerNode          testframework.LightningNode
	makerNode          testframework.LightningNode
	takerPeerswap      *testframework.DaemonProcess
	makerPeerswap      *testframework.DaemonProcess
	chainRpc           *testframework.RpcProxy
	chaind             ChainNode
	confirms           int
	csv                int
	swapType           swap.SwapType
	premiumLimit       int64
	swapInPremiumRate  int64
	swapOutPremiumRate int64
}

func (p *testParams) premium() int64 {
	switch p.swapType {
	case swap.SWAPTYPE_IN:
		return swap.ComputePremium(p.swapAmt, p.swapInPremiumRate)
	case swap.SWAPTYPE_OUT:
		return swap.ComputePremium(p.swapAmt, p.swapOutPremiumRate)
	default:
		return 0
	}
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
	if params.swapType == swap.SWAPTYPE_OUT {
		moveAmt = uint64(int64(moveAmt) - params.premium())
	}
	inv, err := params.makerNode.AddInvoice(moveAmt, "shift balance", "")
	require.NoError(err)

	err = params.takerNode.SendPay(inv, params.scid)
	require.NoError(err)

	// Check channel taker balance is less than the swapAmt.
	var setTakerFunds uint64
	err = testframework.WaitFor(func() bool {
		setTakerFunds, err = params.takerNode.GetChannelBalanceSat(params.scid)
		require.NoError(err)
		expectTakerFunds := params.swapAmt - 1
		if params.swapType == swap.SWAPTYPE_OUT {
			expectTakerFunds = uint64(int64(expectTakerFunds) + params.premium())
		}
		return setTakerFunds == expectTakerFunds
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
	testframework.AssertOnchainBalanceInDelta(t,
		params.takerNode, params.origTakerWallet, 1, time.Second*30)
	testframework.AssertOnchainBalanceInDelta(t,
		params.makerNode, params.origMakerWallet-commitFee-claimFee, 1, time.Second*30)
}

func preimageClaimTest(t *testing.T, params *testParams) {
	require := require.New(t)

	var feeInvoiceAmt uint64
	if params.swapType == swap.SWAPTYPE_OUT {
		// Wait for channel balance to change, this means the invoice was payed.
		testframework.AssertWaitForBalanceChange(t, params.takerNode, params.scid, params.origTakerBalance, testframework.TIMEOUT)
		testframework.AssertWaitForBalanceChange(t, params.makerNode, params.scid, params.origMakerBalance, testframework.TIMEOUT)

		// Get fee from difference.
		newBalance, err := params.takerNode.GetChannelBalanceSat(params.scid)
		require.NoError(err)
		feeInvoiceAmt = params.origTakerBalance - newBalance
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
	// fee invoice amount is only !=0 when swap type is swap_out.
	expectedTakerChannelBalance := float64(int64(params.origTakerBalance - params.swapAmt - feeInvoiceAmt))
	if params.swapType == swap.SWAPTYPE_OUT {
		// taker pay premium by invoice being paid.
		expectedTakerChannelBalance -= float64(params.premium())
	}
	require.True(testframework.AssertWaitForChannelBalance(t, params.takerNode, params.scid, expectedTakerChannelBalance, 1., testframework.TIMEOUT))
	expectedMakerChannelBalance := float64(params.origMakerBalance + params.swapAmt + feeInvoiceAmt)
	if params.swapType == swap.SWAPTYPE_OUT {
		// maker receive premium by invoice being paid.
		expectedMakerChannelBalance += float64(params.premium())
	}
	require.True(testframework.AssertWaitForChannelBalance(t, params.makerNode, params.scid, expectedMakerChannelBalance, 1., testframework.TIMEOUT))

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

	// Check Wallet takerBalance.
	// Expect:
	// - taker -> before - claim_fee + swapamt
	// - maker -> before - commitment_fee - swapamt
	expectTakerBalance := int64(params.origTakerWallet - claimFee + params.swapAmt)
	if swap.SWAPTYPE_IN == params.swapType {
		expectTakerBalance += params.premium()
	}
	testframework.AssertOnchainBalanceInDelta(t,
		params.takerNode, uint64(expectTakerBalance), 1, time.Second*10)
	expectMakerBalance := int64(params.origMakerWallet - commitFee - params.swapAmt)
	if swap.SWAPTYPE_IN == params.swapType {
		expectMakerBalance -= params.premium()
	}
	testframework.AssertOnchainBalanceInDelta(t,
		params.makerNode, uint64(expectMakerBalance), 1, time.Second*10)

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
	payreq, err := params.takerNode.GetLatestPayReqOfPayment()
	require.NoError(err)
	require.Equal(bolt11, payreq)
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

	// Check Wallet balance.
	testframework.AssertOnchainBalanceInDelta(t,
		params.makerNode, params.origMakerWallet-commitFee-claimFee, 1, time.Second*10)

	require.NoError(params.makerPeerswap.WaitForLog(
		fmt.Sprintf("added peer %s to suspicious peer list", params.takerNode.Id()),
		testframework.TIMEOUT))
}
