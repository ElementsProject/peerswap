package test

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/test/scenario"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/lightningnetwork/lnd/lnrpc"
)

const (
	twoHopChanAmountSat = uint64(10_000_000)
	twoHopSwapAmountSat = uint64(1_000_000)
)

func lnd3HopSetup(
	t *testing.T,
	channelCapacitySat uint64,
	pushAmtSat uint64,
) (*testframework.BitcoinNode, *testframework.LndNode, *testframework.LndNode, *testframework.LndNode, *PeerSwapd, *PeerSwapd, string, string) {
	t.Helper()

	builder := NewHarnessBuilder(t)
	bitcoind := builder.Bitcoind()

	extraConfig := map[string]string{
		"protocol.wumbo-channels": "true",
		// Speed up gossip propagation for multihop routes.
		"trickledelay": "50",
	}

	u := builder.AddLndNode(1, WithLndExtraConfig(extraConfig), WithLndFailurePrinter(printFailedFiltered))
	m := builder.AddLndNode(2, WithLndExtraConfig(extraConfig), WithLndFailurePrinter(printFailedFiltered))
	v := builder.AddLndNode(3, WithLndExtraConfig(extraConfig), WithLndFailurePrinter(printFailedFiltered))

	uPeerswapd := builder.AddPeerSwapd(1, u, nil, WithPeerSwapdFailurePrinter(printFailedFiltered))
	// Note: Do not start peerswapd on m, to prove intermediary does not need PeerSwap.
	vPeerswapd := builder.AddPeerSwapd(2, v, nil, WithPeerSwapdFailurePrinter(printFailedFiltered))

	builder.Start()

	scidUM, err := u.OpenChannel(m, channelCapacitySat, pushAmtSat, true, true, true)
	requireNoError(t, err)

	scidMV, err := m.OpenChannel(v, channelCapacitySat, pushAmtSat, true, true, true)
	requireNoError(t, err)

	// LND may require additional confirmations before a channel is announced to
	// the public graph. Since we open two channels sequentially, the second
	// channel can otherwise remain unannounced long enough for QueryRoutes to
	// not find the required 2-hop route on slow CI machines.
	requireNoError(t, bitcoind.GenerateBlocks(BitcoinConfirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, u, m, v)

	// Fund u/v wallets so both swap directions can run without relying on
	// channel open residual wallet balances.
	_, err = u.FundWallet(10*channelCapacitySat, true)
	requireNoError(t, err)
	_, err = v.FundWallet(10*channelCapacitySat, true)
	requireNoError(t, err)

	// Connect u <-> v as peers (no channel).
	err = u.Connect(v, true)
	requireNoError(t, err)

	// Ensure compatibility snapshot is exchanged between u and v.
	err = syncPoll(
		&peerswapPollableNode{uPeerswapd, u.Id()},
		&peerswapPollableNode{vPeerswapd, v.Id()},
	)
	requireNoError(t, err)

	return bitcoind, u, m, v, uPeerswapd, vPeerswapd, scidUM, scidMV
}

func waitForPinned2HopRoute(
	t *testing.T,
	payer *testframework.LndNode,
	destinationPubkey string,
	outgoingChanID uint64,
	incomingChanID uint64,
	lastHopPubkey string,
) {
	t.Helper()

	lastHopBytes, err := hex.DecodeString(lastHopPubkey)
	requireNoError(t, err)

	err = testframework.WaitFor(func() bool {
		// Note: QueryRoutes returns an error when no route is known. We retry
		// until the route graph catches up.
		res, err := payer.Rpc.QueryRoutes(context.Background(), &lnrpc.QueryRoutesRequest{
			PubKey:         destinationPubkey,
			AmtMsat:        1000,
			FinalCltvDelta: 40,
			OutgoingChanId: outgoingChanID,
			LastHopPubkey:  lastHopBytes,
		})
		if err != nil {
			return false
		}
		for _, r := range res.Routes {
			if len(r.Hops) != 2 {
				continue
			}
			if r.Hops[0].ChanId != outgoingChanID {
				continue
			}
			if r.Hops[1].ChanId != incomingChanID {
				continue
			}
			return true
		}
		return false
	}, testframework.TIMEOUT)
	requireNoError(t, err, "did not find pinned 2-hop route (dest=%s outgoing=%d incoming=%d last_hop=%s)", destinationPubkey, outgoingChanID, incomingChanID, lastHopPubkey)
}

func startLnd2HopSwap(t *testing.T, requester *PeerSwapd, channelID uint64, peerPubkey string, swapAmt uint64, swapType swap.SwapType) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	asset := "btc"

	switch swapType {
	case swap.SWAPTYPE_IN:
		go func() {
			_, _ = requester.PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:           channelID,
				SwapAmount:          swapAmt,
				Asset:               asset,
				PeerPubkey:          peerPubkey,
				PremiumLimitRatePpm: 100000,
			})
		}()
	case swap.SWAPTYPE_OUT:
		go func() {
			_, _ = requester.PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:           channelID,
				SwapAmount:          swapAmt,
				Asset:               asset,
				PeerPubkey:          peerPubkey,
				PremiumLimitRatePpm: 100000,
			})
		}()
	default:
		t.Fatalf("unknown swap type %v", swapType)
	}
}

func waitForLndInvoiceByMemoContains(
	t *testing.T,
	node *testframework.LndNode,
	memoSubstring string,
) *lnrpc.Invoice {
	t.Helper()

	require := requireNew(t)

	var invoice *lnrpc.Invoice
	err := testframework.WaitFor(func() bool {
		resp, err := node.Rpc.ListInvoices(context.Background(), &lnrpc.ListInvoiceRequest{})
		require.NoError(err)

		var newest *lnrpc.Invoice
		for _, candidate := range resp.Invoices {
			if !strings.Contains(candidate.Memo, memoSubstring) {
				continue
			}
			if candidate.PaymentRequest == "" {
				continue
			}
			if newest == nil || candidate.AddIndex > newest.AddIndex {
				newest = candidate
			}
		}
		invoice = newest
		return invoice != nil
	}, testframework.TIMEOUT)
	require.NoError(err)
	return invoice
}

func waitForLndPaymentByPayreq(
	t *testing.T,
	node *testframework.LndNode,
	payreq string,
) *lnrpc.Payment {
	t.Helper()

	require := requireNew(t)

	var payment *lnrpc.Payment
	err := testframework.WaitFor(func() bool {
		resp, err := node.Rpc.ListPayments(context.Background(), &lnrpc.ListPaymentsRequest{})
		require.NoError(err)

		for _, candidate := range resp.Payments {
			if candidate.PaymentRequest != payreq {
				continue
			}
			payment = candidate
			return payment.Status == lnrpc.Payment_SUCCEEDED
		}
		return false
	}, testframework.TIMEOUT)
	require.NoError(err)
	return payment
}

func preimageClaim2HopTest(
	t *testing.T,
	bitcoind *testframework.BitcoinNode,
	u, m, v *testframework.LndNode,
	uPeerswapd, vPeerswapd *PeerSwapd,
	scidUM, scidMV string,
	swapAmt uint64,
	swapType swap.SwapType,
) {
	t.Helper()

	require := requireNew(t)

	const asset = "btc"
	expectedClaimMemoPrefix := fmt.Sprintf("peerswap %s claim %s", asset, scidUM)
	expectedFeeMemoPrefix := fmt.Sprintf("peerswap %s fee %s", asset, scidUM)
	parseSwapID := func(memo string) string {
		t.Helper()
		fields := strings.Fields(memo)
		if len(fields) < 5 {
			t.Fatalf("unexpected peerswap memo format: %q", memo)
		}
		return fields[len(fields)-1]
	}

	// Snapshot balances up-front (preimageClaimTest parity).
	origWalletU, err := u.GetBtcBalanceSat()
	require.NoError(err)
	origWalletV, err := v.GetBtcBalanceSat()
	require.NoError(err)

	origChanUM, err := u.GetChannelBalanceSat(scidUM)
	require.NoError(err)
	origChanMV, err := v.GetChannelBalanceSat(scidMV)
	require.NoError(err)

	logs := scenario.NewLogBook()

	awaitOpening := scenario.LogExpectation{
		Action:  "await-opening-confirmation",
		Message: "Await confirmation for tx",
		Timeout: testframework.TIMEOUT,
	}
	claimLog := scenario.LogExpectation{
		Action:  "claim-success",
		Timeout: testframework.TIMEOUT,
	}
	invoiceLog := scenario.LogExpectation{
		Action:  "invoice-paid",
		Timeout: testframework.TIMEOUT,
	}

	var claimInvoiceCreator *testframework.LndNode
	var claimInvoicePayer *testframework.LndNode
	var feeInvoiceCreator *testframework.LndNode
	var feeInvoicePayer *testframework.LndNode

	switch swapType {
	case swap.SWAPTYPE_OUT:
		// u initiates swap-out with v; v creates opening tx + claim invoice; u claims on-chain.
		awaitOpening.Waiter = uPeerswapd.DaemonProcess
		invoiceLog.Waiter = vPeerswapd.DaemonProcess
		invoiceLog.Message = "Event_OnClaimInvoicePaid on State_SwapOutReceiver_AwaitClaimInvoicePayment"
		claimLog.Waiter = uPeerswapd.DaemonProcess
		claimLog.Message = "Event_ActionSucceeded on State_SwapOutSender_ClaimSwap"

		claimInvoiceCreator = v
		claimInvoicePayer = u

		feeInvoiceCreator = v
		feeInvoicePayer = u
	case swap.SWAPTYPE_IN:
		// u initiates swap-in with v; u creates opening tx + claim invoice; v claims on-chain.
		awaitOpening.Waiter = vPeerswapd.DaemonProcess
		invoiceLog.Waiter = uPeerswapd.DaemonProcess
		invoiceLog.Message = "Event_OnClaimInvoicePaid on State_SwapInSender_AwaitClaimPayment"
		claimLog.Waiter = vPeerswapd.DaemonProcess
		claimLog.Message = "Event_ActionSucceeded on State_SwapInReceiver_ClaimSwap"

		claimInvoiceCreator = u
		claimInvoicePayer = v
	default:
		t.Fatalf("unknown swap type %v", swapType)
	}

	logs.Register(awaitOpening)
	logs.Register(invoiceLog)
	logs.Register(claimLog)

	// swap-out pays an initial fee invoice before the opening tx is broadcast.
	var feePayment *lnrpc.Payment
	if swapType == swap.SWAPTYPE_OUT {
		feeInvoice := waitForLndInvoiceByMemoContains(t, feeInvoiceCreator, expectedFeeMemoPrefix)
		feePayment = waitForLndPaymentByPayreq(t, feeInvoicePayer, feeInvoice.PaymentRequest)
		var invoiceValueSat int64
		if feeInvoice.ValueMsat != 0 {
			invoiceValueSat = feeInvoice.ValueMsat / 1000
		} else {
			invoiceValueSat = feeInvoice.Value
		}
		require.Equal(invoiceValueSat, feePayment.ValueSat)

		awaitOpening.Message = fmt.Sprintf("Await confirmation for tx with id: .* on swap %s", parseSwapID(feeInvoice.Memo))
		logs.Register(awaitOpening)
	}

	// Wait for opening tx to hit the mempool and capture its fee (commitFee).
	commitFee, err := waitForTxInMempool(t, bitcoind.RpcProxy, testframework.TIMEOUT)
	require.NoError(err)

	// For swap-in, we can make the await-opening log expectation swap-specific by
	// parsing the swap id from the claim invoice memo (created with the opening tx).
	if swapType == swap.SWAPTYPE_IN {
		claimInvoice := waitForLndInvoiceByMemoContains(t, claimInvoiceCreator, expectedClaimMemoPrefix)
		awaitOpening.Message = fmt.Sprintf("Await confirmation for tx with id: .* on swap %s", parseSwapID(claimInvoice.Memo))
		logs.Register(awaitOpening)
	}

	// Confirm opening tx.
	require.NoError(logs.Await("await-opening-confirmation"))
	requireNoError(t, bitcoind.GenerateBlocks(BitcoinConfirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, u, m, v)

	// Wait for claim invoice to be paid.
	require.NoError(logs.Await("invoice-paid"))

	claimInvoice := waitForLndInvoiceByMemoContains(t, claimInvoiceCreator, expectedClaimMemoPrefix)
	claimPayment := waitForLndPaymentByPayreq(t, claimInvoicePayer, claimInvoice.PaymentRequest)

	// Channel balances should match once the invoice is paid (preimageClaimTest parity).
	switch swapType {
	case swap.SWAPTYPE_OUT:
		swapOutPremium := premium.NewPPM(premium.DefaultBTCSwapOutPremiumRatePPM).Compute(swapAmt)
		if swapOutPremium < 0 {
			t.Fatalf("unexpected negative swap-out premium: %d", swapOutPremium)
		}
		expectedClaimInvoiceAmt := int64(swapAmt) + swapOutPremium
		require.Equal(expectedClaimInvoiceAmt, claimPayment.ValueSat)

		totalInvoiceValue := claimPayment.ValueSat
		totalRoutingFee := claimPayment.FeeSat
		if feePayment != nil {
			totalInvoiceValue += feePayment.ValueSat
			totalRoutingFee += feePayment.FeeSat
		}
		require.True(totalInvoiceValue > 0)

		require.True(testframework.AssertWaitForChannelBalance(
			t,
			u,
			scidUM,
			float64(int64(origChanUM)-totalInvoiceValue-totalRoutingFee),
			1.,
			testframework.TIMEOUT,
		))
		require.True(testframework.AssertWaitForChannelBalance(
			t,
			v,
			scidMV,
			float64(int64(origChanMV)+totalInvoiceValue),
			1.,
			testframework.TIMEOUT,
		))
	case swap.SWAPTYPE_IN:
		require.Equal(int64(swapAmt), claimPayment.ValueSat)

		require.True(testframework.AssertWaitForChannelBalance(
			t,
			u,
			scidUM,
			float64(int64(origChanUM)+claimPayment.ValueSat),
			1.,
			testframework.TIMEOUT,
		))
		require.True(testframework.AssertWaitForChannelBalance(
			t,
			v,
			scidMV,
			float64(int64(origChanMV)-claimPayment.ValueSat-claimPayment.FeeSat),
			1.,
			testframework.TIMEOUT,
		))
	}

	// Wait for claim tx being broadcasted and capture its fee (claimFee).
	claimFee, err := waitForTxInMempool(t, bitcoind.RpcProxy, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm claim tx.
	requireNoError(t, bitcoind.GenerateBlocks(BitcoinConfirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, u, m, v)

	// Swap done.
	require.NoError(logs.Await("claim-success"))

	// Wallet balance assertions (preimageClaimTest parity).
	switch swapType {
	case swap.SWAPTYPE_OUT:
		// u receives swapAmt on-chain and pays the claim tx fee.
		testframework.AssertOnchainBalanceInDelta(t,
			u, origWalletU+swapAmt-claimFee, 1, time.Second*10)
		// v funds the opening tx output and pays the opening tx fee.
		testframework.AssertOnchainBalanceInDelta(t,
			v, origWalletV-swapAmt-commitFee, 1, time.Second*10)
	case swap.SWAPTYPE_IN:
		swapInPremium := premium.NewPPM(premium.DefaultBTCSwapInPremiumRatePPM).Compute(swapAmt)
		if swapInPremium < 0 {
			t.Fatalf("unexpected negative swap-in premium: %d", swapInPremium)
		}

		onchainAmt := swapAmt + uint64(swapInPremium)
		// u funds the opening tx output and pays the opening tx fee.
		testframework.AssertOnchainBalanceInDelta(t,
			u, origWalletU-onchainAmt-commitFee, 1, time.Second*10)
		// v receives on-chain and pays the claim tx fee.
		testframework.AssertOnchainBalanceInDelta(t,
			v, origWalletV+onchainAmt-claimFee, 1, time.Second*10)
	}

	// Check latest claim invoice memo and payer payreq (preimageClaimTest parity).
	require.True(
		strings.Contains(claimInvoice.Memo, expectedClaimMemoPrefix),
		"expected memo to contain: %s, got: %s",
		expectedClaimMemoPrefix,
		claimInvoice.Memo,
	)
	latestPayreq, err := claimInvoicePayer.GetLatestPayReqOfPayment()
	require.NoError(err)
	require.Equal(claimInvoice.PaymentRequest, latestPayreq)
}

func Test_2Hop_Bitcoin_LND(t *testing.T) {
	IsIntegrationTest(t)

	bitcoind, u, m, v, uPeerswapd, vPeerswapd, scidUM, scidMV := lnd3HopSetup(
		t,
		twoHopChanAmountSat,
		twoHopChanAmountSat/2,
	)
	DumpOnFailure(t, WithBitcoin(bitcoind), WithLnds([]*testframework.LndNode{u, m, v}), WithPeerSwapds(uPeerswapd, vPeerswapd))

	// Ensure u and v are peers but do not share a channel.
	_, err := u.GetScid(v)
	assertError(t, err, "expected no direct channel between u and v")

	channelIDUM, err := u.ChanIdFromScid(scidUM)
	requireNoError(t, err)
	channelIDMV, err := m.ChanIdFromScid(scidMV)
	requireNoError(t, err)

	t.Run("swapout", func(t *testing.T) {
		// Ensure the route graph knows u -> m -> v.
		waitForPinned2HopRoute(t, u, v.Id(), channelIDUM, channelIDMV, m.Id())

		startLnd2HopSwap(t, uPeerswapd, channelIDUM, v.Id(), twoHopSwapAmountSat, swap.SWAPTYPE_OUT)
		preimageClaim2HopTest(t, bitcoind, u, m, v, uPeerswapd, vPeerswapd, scidUM, scidMV, twoHopSwapAmountSat, swap.SWAPTYPE_OUT)
	})

	t.Run("swapin", func(t *testing.T) {
		channelIDVM, err := v.ChanIdFromScid(scidMV)
		requireNoError(t, err)
		channelIDMU, err := m.ChanIdFromScid(scidUM)
		requireNoError(t, err)

		// Ensure the route graph knows v -> m -> u.
		waitForPinned2HopRoute(t, v, u.Id(), channelIDVM, channelIDMU, m.Id())

		startLnd2HopSwap(t, uPeerswapd, channelIDUM, v.Id(), twoHopSwapAmountSat, swap.SWAPTYPE_IN)
		preimageClaim2HopTest(t, bitcoind, u, m, v, uPeerswapd, vPeerswapd, scidUM, scidMV, twoHopSwapAmountSat, swap.SWAPTYPE_IN)
	})

	// Give LND some time to settle pending HTLCs before cleanup tears down nodes.
	time.Sleep(1 * time.Second)
}
