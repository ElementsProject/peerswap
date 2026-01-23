package test

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

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

func await2HopPreimageClaim(
	t *testing.T,
	bitcoind *testframework.BitcoinNode,
	u, m, v *testframework.LndNode,
	uPeerswapd, vPeerswapd *PeerSwapd,
	swapType swap.SwapType,
) {
	t.Helper()

	require := requireNew(t)

	logs := scenario.NewLogBook()

	// Taker waits for the opening tx confirmation and later claims.
	awaitOpening := scenario.LogExpectation{
		Action:  "await-opening-confirmation",
		Message: "Await confirmation for tx",
		Timeout: testframework.TIMEOUT,
	}
	claimLog := scenario.LogExpectation{
		Action:  "claim-success",
		Timeout: testframework.TIMEOUT,
	}

	// Maker logs when the claim invoice is paid.
	invoiceLog := scenario.LogExpectation{
		Action:  "invoice-paid",
		Timeout: testframework.TIMEOUT,
	}

	switch swapType {
	case swap.SWAPTYPE_OUT:
		awaitOpening.Waiter = uPeerswapd.DaemonProcess
		invoiceLog.Waiter = vPeerswapd.DaemonProcess
		invoiceLog.Message = "Event_OnClaimInvoicePaid on State_SwapOutReceiver_AwaitClaimInvoicePayment"
		claimLog.Waiter = uPeerswapd.DaemonProcess
		claimLog.Message = "Event_ActionSucceeded on State_SwapOutSender_ClaimSwap"
	case swap.SWAPTYPE_IN:
		awaitOpening.Waiter = vPeerswapd.DaemonProcess
		invoiceLog.Waiter = uPeerswapd.DaemonProcess
		invoiceLog.Message = "Event_OnClaimInvoicePaid on State_SwapInSender_AwaitClaimPayment"
		claimLog.Waiter = vPeerswapd.DaemonProcess
		claimLog.Message = "Event_ActionSucceeded on State_SwapInReceiver_ClaimSwap"
	default:
		t.Fatalf("unknown swap type %v", swapType)
	}

	logs.Register(awaitOpening)
	logs.Register(invoiceLog)
	logs.Register(claimLog)

	// Wait for opening tx, then confirm it.
	require.NoError(logs.Await("await-opening-confirmation"))
	requireNoError(t, bitcoind.GenerateBlocks(BitcoinConfirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, u, m, v)

	// Wait for claim invoice to be paid, then for claim tx broadcast.
	require.NoError(logs.Await("invoice-paid"))
	_, err := waitForTxInMempool(t, bitcoind.RpcProxy, testframework.TIMEOUT)
	require.NoError(err)

	// Confirm claim tx.
	requireNoError(t, bitcoind.GenerateBlocks(BitcoinConfirms))
	waitForBlockheightSync(t, testframework.TIMEOUT, u, m, v)

	// Swap done.
	require.NoError(logs.Await("claim-success"))
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
		startBalance, err := u.GetChannelBalanceSat(scidUM)
		requireNoError(t, err)

		// Ensure the route graph knows u -> m -> v.
		waitForPinned2HopRoute(t, u, v.Id(), channelIDUM, channelIDMV, m.Id())

		startLnd2HopSwap(t, uPeerswapd, channelIDUM, v.Id(), twoHopSwapAmountSat, swap.SWAPTYPE_OUT)
		await2HopPreimageClaim(t, bitcoind, u, m, v, uPeerswapd, vPeerswapd, swap.SWAPTYPE_OUT)

		var endBalance uint64
		err = testframework.WaitFor(func() bool {
			endBalance, err = u.GetChannelBalanceSat(scidUM)
			requireNoError(t, err)
			return endBalance < startBalance
		}, testframework.TIMEOUT)
		requireNoError(t, err)
		t.Logf("u-m local balance changed: before=%d after=%d (swap-out)", startBalance, endBalance)
	})

	t.Run("swapin", func(t *testing.T) {
		startBalance, err := u.GetChannelBalanceSat(scidUM)
		requireNoError(t, err)

		channelIDVM, err := v.ChanIdFromScid(scidMV)
		requireNoError(t, err)
		channelIDMU, err := m.ChanIdFromScid(scidUM)
		requireNoError(t, err)

		// Ensure the route graph knows v -> m -> u.
		waitForPinned2HopRoute(t, v, u.Id(), channelIDVM, channelIDMU, m.Id())

		startLnd2HopSwap(t, uPeerswapd, channelIDUM, v.Id(), twoHopSwapAmountSat, swap.SWAPTYPE_IN)
		await2HopPreimageClaim(t, bitcoind, u, m, v, uPeerswapd, vPeerswapd, swap.SWAPTYPE_IN)

		var endBalance uint64
		err = testframework.WaitFor(func() bool {
			endBalance, err = u.GetChannelBalanceSat(scidUM)
			requireNoError(t, err)
			return endBalance > startBalance
		}, testframework.TIMEOUT)
		requireNoError(t, err)
		t.Logf("u-m local balance changed: before=%d after=%d (swap-in)", startBalance, endBalance)
	})

	// Give LND some time to settle pending HTLCs before cleanup tears down nodes.
	time.Sleep(1 * time.Second)
}
