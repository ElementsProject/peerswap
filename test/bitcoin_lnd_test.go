package test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
)

const lndFundAmount = uint64(1_000_000_000)

// helpers for table-driven LND BTC tests.
func lndParams(t *testing.T, bitcoind *testframework.BitcoinNode, lightningds []*testframework.LndNode, peerswapds []*PeerSwapd, scid string, st swap.SwapType) (*testParams, uint64) {
	t.Helper()

	var channelBalances []uint64
	var walletBalances []uint64
	for _, lightningd := range lightningds {
		b, err := lightningd.GetBtcBalanceSat()
		requireNoError(t, err)
		walletBalances = append(walletBalances, b)

		b, err = lightningd.GetChannelBalanceSat(scid)
		requireNoError(t, err)
		channelBalances = append(channelBalances, b)
	}

	lcid, err := lightningds[0].ChanIdFromScid(scid)
	if err != nil {
		t.Fatalf("ChanIdFromScid() %v", err)
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
		takerPeerswap:       peerswapds[0].DaemonProcess,
		makerPeerswap:       peerswapds[1].DaemonProcess,
		chainRPC:            bitcoind.RpcProxy,
		chaind:              bitcoind,
		confirms:            BitcoinConfirms,
		csv:                 BitcoinCsv,
		swapType:            st,
		premiumLimitRatePPM: 100000,
		swapInPremiumRate:   premium.DefaultBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultBTCSwapOutPremiumRatePPM,
	}

	return params, lcid
}

// Failure dumps are handled via DumpOnFailure in failuredump.go

func startLndSwap(t *testing.T, peerswapd *PeerSwapd, channelID uint64, params *testParams) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	asset := "btc"

	switch params.swapType {
	case swap.SWAPTYPE_IN:
		go func() {
			_, _ = peerswapd.PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:           channelID,
				SwapAmount:          params.swapAmt,
				Asset:               asset,
				PremiumLimitRatePpm: params.premiumLimitRatePPM,
			})
		}()
	case swap.SWAPTYPE_OUT:
		go func() {
			_, _ = peerswapd.PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:           channelID,
				SwapAmount:          params.swapAmt,
				Asset:               asset,
				PremiumLimitRatePpm: params.premiumLimitRatePPM,
			})
		}()
	default:
		t.Fatalf("unknown swap type: %v", params.swapType)
	}
}

type lndSwapCase struct {
	name     string
	swapType swap.SwapType
	claim    func(t *testing.T, params *testParams)
}

type lndClnSwapCase struct {
	name     string
	funder   fundingNode
	swapType swap.SwapType
	claim    func(t *testing.T, params *testParams)
}

func runLndLndSwapCases(t *testing.T, cases []lndSwapCase) {
	t.Helper()
	IsIntegrationTest(t)
	t.Parallel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, lndFundAmount)
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLnds(lightningds), WithPeerSwapds(peerswapds...))

			params, channelID := lndParams(t, bitcoind, lightningds, peerswapds, scid, tc.swapType)
			starter := peerswapds[0]
			if tc.swapType == swap.SWAPTYPE_IN {
				starter = peerswapds[1]
			}

			startLndSwap(t, starter, channelID, params)
			tc.claim(t, params)
		})
	}
}

func runLndClnSwapCases(t *testing.T, cases []lndClnSwapCase) {
	t.Helper()
	IsIntegrationTest(t)
	t.Parallel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, lightningds, peerswapd, scid := mixedSetup(t, lndFundAmount, tc.funder)
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLightningNodes(lightningds), WithPeerSwapd(peerswapd))

			params := buildClnLndParams(t, bitcoind, lightningds, peerswapd, scid, tc.swapType)
			lndNode := mustFindLndNode(t, lightningds)
			channelID, err := lndNode.ChanIdFromScid(scid)
			requireNoError(t, err)

			startLndSwap(t, peerswapd, channelID, params)
			tc.claim(t, params)
		})
	}
}

func mustFindLndNode(t *testing.T, nodes []testframework.LightningNode) *testframework.LndNode {
	t.Helper()

	for _, node := range nodes {
		if lnd, ok := node.(*testframework.LndNode); ok {
			return lnd
		}
		if lnd, ok := node.(*LndNodeWithLiquid); ok {
			return lnd.LndNode
		}
	}

	t.Fatalf("did not find LND node in lightningds")
	return nil
}

// Test_OnlyOneActiveSwapPerChannelLnd checks that there is only one active swap per
// channel.
func Test_OnlyOneActiveSwapPerChannelLnd(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()
	bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, lndFundAmount)
	DumpOnFailure(t, WithBitcoin(bitcoind), WithLnds(lightningds), WithPeerSwapds(peerswapds...))

	params, channelID := lndParams(t, bitcoind, lightningds, peerswapds, scid, swap.SWAPTYPE_IN)
	params.swapAmt = params.origTakerBalance / 5
	asset := "btc"

	// Do swap. Expect N_SWAPS - 1 errors.
	wg := sync.WaitGroup{}
	const nSwaps = 10
	var nErr int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := range nSwaps {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			res, err := peerswapds[1].PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:           channelID,
				SwapAmount:          params.swapAmt,
				Asset:               asset,
				PremiumLimitRatePpm: params.premiumLimitRatePPM,
			})
			t.Logf("[%d] Response: %v", n, res)
			if err != nil {
				t.Logf("[%d] Err: %s", n, err.Error())
				atomic.AddInt32(&nErr, 1)
			}
		}(i)
	}
	wg.Wait()

	assertEqualNumericValues(t, nSwaps-1, nErr, "expected nswaps-1=%d errors, got: %d", nSwaps-1, nErr)
	err := testframework.WaitForWithErr(func() (bool, error) {
		res, err := peerswapds[1].PeerswapClient.ListActiveSwaps(ctx, &peerswaprpc.ListSwapsRequest{})
		if err != nil {
			return false, err
		}
		for _, r := range res.Swaps {
			if r.State == string(swap.State_SwapInSender_AwaitAgreement) {
				return false, nil
			}
		}
		assertEqualValues(t, 1, len(res.Swaps), "expected only 1 active swap, got: %d - %v", len(res.Swaps), res)
		return true, nil
	}, 2*time.Second)
	assertNoError(t, err)
}

func Test_LndLnd_Bitcoin_SwapIn(t *testing.T) {
	cases := []lndSwapCase{
		{name: "claim_normal", swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
		{name: "claim_coop", swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
		{name: "claim_csv", swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
	}
	runLndLndSwapCases(t, cases)
}

func Test_LndLnd_Bitcoin_SwapOut(t *testing.T) {
	cases := []lndSwapCase{
		{name: "claim_normal", swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
		{name: "claim_coop", swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
		{name: "claim_csv", swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
	}
	runLndLndSwapCases(t, cases)
}

func Test_LndCln_Bitcoin_SwapIn(t *testing.T) {
	cases := []lndClnSwapCase{
		{name: "claim_normal", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
		{name: "claim_coop", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
		{name: "claim_csv", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
	}
	runLndClnSwapCases(t, cases)
}

func Test_LndCln_Bitcoin_SwapOut(t *testing.T) {
	cases := []lndClnSwapCase{
		{name: "claim_normal", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
		{name: "claim_coop", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
		{name: "claim_csv", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
	}
	runLndClnSwapCases(t, cases)
}

func Test_LndLnd_ExcessiveAmount(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()
	t.Run("swapout", func(t *testing.T) {
		t.Parallel()
		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, lndFundAmount)
		DumpOnFailure(t, WithBitcoin(bitcoind), WithLnds(lightningds), WithPeerSwapds(peerswapds...))

		params, channelID := lndParams(t, bitcoind, lightningds, peerswapds, scid, swap.SWAPTYPE_OUT)
		asset := "btc"

		_, err := lightningds[0].SetHtlcMaximumMilliSatoshis(scid, params.origTakerBalance*1000/2-1)
		assertNoError(t, err)
		// Swap out should fail as the swap amount is too high.
		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, err = peerswapds[0].PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
			ChannelId:           channelID,
			SwapAmount:          params.swapAmt,
			Asset:               asset,
			PremiumLimitRatePpm: params.premiumLimitRatePPM,
		})
		assertError(t, err)
	})
	t.Run("swapin", func(t *testing.T) {
		t.Parallel()
		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, lndFundAmount)
		DumpOnFailure(t, WithBitcoin(bitcoind), WithLnds(lightningds), WithPeerSwapds(peerswapds...))

		params, channelID := lndParams(t, bitcoind, lightningds, peerswapds, scid, swap.SWAPTYPE_IN)
		asset := "btc"

		_, err := lightningds[0].SetHtlcMaximumMilliSatoshis(scid, params.origTakerBalance*1000/2-1)
		assertNoError(t, err)
		// Swap in should fail as the swap amount is too high.
		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, err = peerswapds[1].PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
			ChannelId:           channelID,
			SwapAmount:          params.swapAmt,
			Asset:               asset,
			PremiumLimitRatePpm: params.premiumLimitRatePPM,
		})
		assertError(t, err)
	})
}
