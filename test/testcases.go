package test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/test/scenario"
	"github.com/elementsproject/peerswap/testframework"
)

type testParams struct {
	swapAmt             uint64
	scid                string
	origTakerWallet     uint64
	origMakerWallet     uint64
	origTakerBalance    uint64
	origMakerBalance    uint64
	takerNode           testframework.LightningNode
	makerNode           testframework.LightningNode
	takerPeerswap       *testframework.DaemonProcess
	makerPeerswap       *testframework.DaemonProcess
	chainRPC            *testframework.RpcProxy
	chaind              ChainNode
	confirms            int
	csv                 int
	swapType            swap.SwapType
	premiumLimitRatePPM int64
	swapInPremiumRate   int64
	swapOutPremiumRate  int64
}

func (p *testParams) premium() int64 {
	switch p.swapType {
	case swap.SWAPTYPE_IN:
		return premium.NewPPM(p.swapInPremiumRate).Compute(p.swapAmt)
	case swap.SWAPTYPE_OUT:
		return premium.NewPPM(p.swapOutPremiumRate).Compute(p.swapAmt)
	default:
		return 0
	}
}

func (p *testParams) expectations() scenario.Expectations {
	return scenario.Expectations{
		SwapAmt:            p.swapAmt,
		SwapType:           p.swapType,
		SwapInPremiumRate:  p.swapInPremiumRate,
		SwapOutPremiumRate: p.swapOutPremiumRate,
		OrigTakerChannel:   p.origTakerBalance,
		OrigMakerChannel:   p.origMakerBalance,
		OrigTakerWallet:    p.origTakerWallet,
		OrigMakerWallet:    p.origMakerWallet,
	}
}

func coopClaimTest(t *testing.T, params *testParams) {
	t.Helper()

	require := requireNew(t)
	expect := params.expectations()
	logs := scenario.NewLogBook()
	closeLog := scenario.LogExpectation{
		Action:  "coop-close-sent",
		Waiter:  params.takerPeerswap,
		Timeout: testframework.TIMEOUT,
	}
	claimLog := scenario.LogExpectation{
		Action:  "coop-claim",
		Waiter:  params.makerPeerswap,
		Timeout: testframework.TIMEOUT,
	}
	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitFee.
	commitFee, err := waitForTxInMempool(t, params.chainRPC, testframework.TIMEOUT)
	require.NoError(err)

	//
	//	STEP 2: Move balance
	//
	// Move local balance from taker to maker so that the taker does not
	// have enough balance to pay the invoice and cancels the swap coop.
	feeInvoiceAmt, err := params.makerNode.GetFeeInvoiceAmtSat()
	require.NoError(err)

	moveAmt := (params.origTakerBalance - feeInvoiceAmt - params.swapAmt) + 1
	var premiumUint uint64
	if params.swapType == swap.SWAPTYPE_OUT {
		premiumValue := params.premium()
		if premiumValue < 0 {
			t.Fatalf("unexpected negative premium: %d", premiumValue)
		}
		premiumUint = uint64(premiumValue)
		if premiumUint > moveAmt {
			t.Fatalf("premium %d exceeds move amount %d", premiumUint, moveAmt)
		}
		moveAmt -= premiumUint
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
			expectTakerFunds += premiumUint
		}
		return setTakerFunds == expectTakerFunds
	}, testframework.TIMEOUT)
	require.NoError(err)

	//
	//	STEP 3: Confirm opening tx
	//

	requireNoError(t, params.chaind.GenerateBlocks(params.confirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode, params.makerNode)

	// Check that coop close was sent.
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		closeLog.Message = "Event_ActionSucceeded on State_SwapInReceiver_SendCoopClose"
		claimLog.Message = "Event_ActionSucceeded on State_SwapInSender_ClaimSwapCoop"
	case swap.SWAPTYPE_OUT:
		closeLog.Message = "Event_ActionSucceeded on State_SwapOutSender_SendCoopClose"
		claimLog.Message = "Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCoop"
	default:
		t.Fatal("unknown role")
	}
	logs.Register(closeLog)
	logs.Register(claimLog)
	require.NoError(logs.Await("coop-close-sent"))

	//
	//	STEP 4: Broadcasting coop claim tx
	//

	// Wait for coop claim tx being broadcasted.
	// Get claim fee.
	claimFee, err := waitForTxInMempool(t, params.chainRPC, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm coop claim tx.
	requireNoError(t, params.chaind.GenerateBlocks(params.confirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode, params.makerNode)

	// Check swap is done.
	require.NoError(logs.Await("coop-claim"))

	// Check no invoice was paid.
	testframework.RequireWaitForChannelBalance(
		t,
		params.takerNode,
		params.scid,
		float64(setTakerFunds),
		1.,
		testframework.TIMEOUT,
	)

	// Check Wallet balance.
	// Expect:
	// - [0] before
	// - [1] before - commitment_fee - claim_fee
	testframework.AssertOnchainBalanceInDelta(t,
		params.takerNode, expect.TakerWalletUnchanged(), 1, time.Second*30)
	testframework.AssertOnchainBalanceInDelta(t,
		params.makerNode, expect.MakerWalletAfterFees(commitFee, claimFee), 1, time.Second*30)
}

func preimageClaimTest(t *testing.T, params *testParams) {
	t.Helper()

	require := requireNew(t)

	expect := params.expectations()
	logs := scenario.NewLogBook()
	logs.Register(scenario.LogExpectation{
		Action:  "await-opening-confirmation",
		Message: "Await confirmation for tx",
		Waiter:  params.takerPeerswap,
		Timeout: testframework.TIMEOUT,
	})

	invoiceLog := scenario.LogExpectation{
		Action:  "invoice-paid",
		Waiter:  params.makerPeerswap,
		Timeout: testframework.TIMEOUT,
	}
	claimLog := scenario.LogExpectation{
		Action:  "claim-success",
		Timeout: testframework.TIMEOUT,
	}

	switch params.swapType {
	case swap.SWAPTYPE_IN:
		invoiceLog.Message = "Event_OnClaimInvoicePaid on State_SwapInSender_AwaitClaimPayment"
		claimLog.Waiter = params.takerPeerswap
		claimLog.Message = "Event_ActionSucceeded on State_SwapInReceiver_ClaimSwap"
	case swap.SWAPTYPE_OUT:
		invoiceLog.Message = "Event_OnClaimInvoicePaid on State_SwapOutReceiver_AwaitClaimInvoicePayment"
		claimLog.Waiter = params.takerPeerswap
		claimLog.Message = "Event_ActionSucceeded on State_SwapOutSender_ClaimSwap"
	default:
		t.Fatal("unknown role")
	}

	logs.Register(invoiceLog)
	logs.Register(claimLog)

	var feeInvoiceAmt uint64
	if params.swapType == swap.SWAPTYPE_OUT {
		// Wait for channel balance to change, this means the invoice was paid.
		testframework.AssertWaitForBalanceChange(
			t,
			params.takerNode,
			params.scid,
			params.origTakerBalance,
			testframework.TIMEOUT,
		)
		testframework.AssertWaitForBalanceChange(
			t,
			params.makerNode,
			params.scid,
			params.origMakerBalance,
			testframework.TIMEOUT,
		)

		// Get fee from difference.
		newBalance, err := params.takerNode.GetChannelBalanceSat(params.scid)
		require.NoError(err)
		feeInvoiceAmt = params.origTakerBalance - newBalance
	}

	// Wait for opening tx being broadcasted.
	// Get commitFee.
	commitFee, err := waitForTxInMempool(t, params.chainRPC, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm opening tx.
	require.NoError(logs.Await("await-opening-confirmation"))
	requireNoError(t, params.chaind.GenerateBlocks(params.confirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode, params.makerNode)

	// Wait for invoice being paid.
	require.NoError(logs.Await("invoice-paid"))

	// Check channel balances match.
	// fee invoice amount is only !=0 when swap type is swap_out.
	require.True(testframework.AssertWaitForChannelBalance(
		t,
		params.takerNode,
		params.scid,
		expect.TakerChannelAfterPreimageClaim(feeInvoiceAmt),
		1.,
		testframework.TIMEOUT,
	))
	require.True(testframework.AssertWaitForChannelBalance(
		t,
		params.makerNode,
		params.scid,
		expect.MakerChannelAfterPreimageClaim(feeInvoiceAmt),
		1.,
		testframework.TIMEOUT,
	))

	// Wait for claim tx being broadcasted.
	// Get claim fee.
	claimFee, err := waitForTxInMempool(t, params.chainRPC, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm claim tx.
	requireNoError(t, params.chaind.GenerateBlocks(params.confirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, params.takerNode, params.makerNode)

	// Wait for claim done
	require.NoError(logs.Await("claim-success"))

	// Check Wallet takerBalance.
	// Expect:
	// - taker -> before - claim_fee + swapamt
	// - maker -> before - commitment_fee - swapamt
	testframework.AssertOnchainBalanceInDelta(t,
		params.takerNode, expect.TakerWalletAfterPreimageClaim(claimFee), 1, time.Second*10)
	testframework.AssertOnchainBalanceInDelta(t,
		params.makerNode, expect.MakerWalletAfterPreimageClaim(commitFee), 1, time.Second*10)

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
	t.Helper()

	require := requireNew(t)
	expect := params.expectations()
	logs := scenario.NewLogBook()
	claimLog := scenario.LogExpectation{
		Action:  "csv-claim",
		Waiter:  params.makerPeerswap,
		Timeout: testframework.TIMEOUT,
	}
	suspiciousLog := scenario.LogExpectation{
		Action:  "suspicious-peer",
		Waiter:  params.makerPeerswap,
		Message: fmt.Sprintf("added peer %s to suspicious peer list", params.takerNode.Id()),
		Timeout: testframework.TIMEOUT,
	}

	var premiumAmt uint64
	if params.swapType == swap.SWAPTYPE_OUT {
		// Wait for channel balance to change, this means the invoice was paid.
		testframework.AssertWaitForBalanceChange(
			t,
			params.takerNode,
			params.scid,
			params.origTakerBalance,
			testframework.TIMEOUT,
		)
		testframework.AssertWaitForBalanceChange(
			t,
			params.makerNode,
			params.scid,
			params.origMakerBalance,
			testframework.TIMEOUT,
		)

		// Get premium from difference.
		newBalance, err := params.takerNode.GetChannelBalanceSat(params.scid)
		require.NoError(err)
		premiumAmt = params.origTakerBalance - newBalance
	}

	// Wait for opening tx being broadcasted.
	// Get commitFee.
	commitFee, err := waitForTxInMempool(t, params.chainRPC, testframework.TIMEOUT)
	require.NoError(err)

	// Stop taker peer so that csv can trigger
	params.takerPeerswap.Kill()

	// if the taker is lnd kill it
	if lnd, ok := params.takerNode.(*testframework.LndNode); ok {
		lnd.Kill()
	}

	// Generate one less block than required for csv.
	requireNoError(t, params.chaind.GenerateBlocks(params.csv-1))
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	// Check that csv is not claimed yet
	var triedToClaim bool
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		triedToClaim, err = params.makerPeerswap.HasLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCsv")
		claimLog.Message = "Event_ActionSucceeded on State_SwapInSender_ClaimSwapCsv"
	case swap.SWAPTYPE_OUT:
		triedToClaim, err = params.makerPeerswap.HasLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv")
		claimLog.Message = "Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv"
	default:
		t.Fatal("unknown swap type")
	}
	require.NoError(err)
	require.False(triedToClaim)
	logs.Register(claimLog)
	logs.Register(suspiciousLog)

	// Generate one more block to trigger csv.
	requireNoError(t, params.chaind.GenerateBlocks(1))
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	require.NoError(logs.Await("csv-claim"))

	// Wait for claim tx being broadcasted.
	// Get claim fee.
	claimFee, err := waitForTxInMempool(t, params.chainRPC, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm claim tx.
	requireNoError(t, params.chaind.GenerateBlocks(params.confirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, params.makerNode)

	// Check channel and wallet balance
	require.True(testframework.AssertWaitForChannelBalance(
		t,
		params.makerNode,
		params.scid,
		expect.MakerChannelAfterCsv(premiumAmt),
		1.,
		testframework.TIMEOUT,
	))

	// Check Wallet balance.
	testframework.AssertOnchainBalanceInDelta(t,
		params.makerNode, expect.MakerWalletAfterFees(commitFee, claimFee), 1, time.Second*10)

	require.NoError(logs.Await("suspicious-peer"))
}
