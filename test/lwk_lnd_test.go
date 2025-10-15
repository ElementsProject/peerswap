package test

import (
	"context"
	"math"
	"testing"

	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
)

// Failure dumps are handled via DumpOnFailure in failuredump.go

func buildLndLwkParams(t *testing.T, liquidd *testframework.LiquidNode, lightningds []*LndNodeWithLiquid, peerswapds []*PeerSwapd, scid string, swapType swap.SwapType) *testParams {
	t.Helper()

	require := requireNew(t)

	require.NotEmpty(lightningds)
	require.GreaterOrEqual(len(lightningds), 2)
	require.Len(peerswapds, len(lightningds))

	var (
		channelBalances []uint64
		walletBalances  []uint64
	)

	for _, lightningd := range lightningds {
		balance, err := lightningd.GetBtcBalanceSat()
		require.NoError(err)
		walletBalances = append(walletBalances, balance)

		balance, err = lightningd.GetChannelBalanceSat(scid)
		require.NoError(err)
		channelBalances = append(channelBalances, balance)
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
		chainRPC:            liquidd.RpcProxy,
		chaind:              liquidd,
		confirms:            LiquidConfirms,
		csv:                 LiquidCsv,
		swapType:            swapType,
		premiumLimitRatePPM: 100000,
		swapInPremiumRate:   premium.DefaultLBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultLBTCSwapOutPremiumRatePPM,
	}

	return params
}

func mustLndNode(t *testing.T, node testframework.LightningNode) *LndNodeWithLiquid {
	t.Helper()

	lnd, ok := node.(*LndNodeWithLiquid)
	if !ok {
		t.Fatalf("unexpected lightning node type %T", node)
	}

	return lnd
}

func lndChanIDFromScid(t *testing.T, node *LndNodeWithLiquid, scid string) uint64 {
	t.Helper()

	lcid, err := node.ChanIdFromScid(scid)
	requireNoError(t, err)

	return lcid
}

func startLndLwkSwap(t *testing.T, requestType swap.SwapType, params *testParams, channelID uint64, requester *PeerSwapd, checkResp bool) {
	t.Helper()

	if requester == nil {
		t.Fatalf("nil peerswapd requester")
	}

	var req *testAssertions
	if checkResp {
		req = requireNew(t)
	}

	asset := "lbtc"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var call func(context.Context) error

	switch requestType {
	case swap.SWAPTYPE_IN:
		call = func(ctx context.Context) error {
			_, err := requester.PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:           channelID,
				SwapAmount:          params.swapAmt,
				Asset:               asset,
				PremiumLimitRatePpm: params.premiumLimitRatePPM,
			})
			return err
		}
	case swap.SWAPTYPE_OUT:
		call = func(ctx context.Context) error {
			_, err := requester.PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:           channelID,
				SwapAmount:          params.swapAmt,
				Asset:               asset,
				PremiumLimitRatePpm: params.premiumLimitRatePPM,
			})
			return err
		}
	default:
		t.Fatalf("unknown swap request type: %v", requestType)
	}

	go func() {
		err := call(ctx)
		if req != nil {
			req.NoError(err)
		}
	}()
}

func Test_LndLnd_LWK_SwapIn(t *testing.T) {
	skipLWKTests(t)
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name      string
		claim     func(t *testing.T, params *testParams)
		checkResp bool
	}{
		{name: "claim_normal", claim: preimageClaimTest, checkResp: true},
		{name: "claim_coop", claim: coopClaimTest},
		{name: "claim_csv", claim: csvClaimTest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bitcoind, liquidd, lightningds, peerswapds, scid, electrsd, lwk := lndlndLWKSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithLndNodesWithLiquid(lightningds),
				WithPeerSwapds(peerswapds...),
				WithElectrs(electrsd),
				WithLWK(lwk),
			)

			if len(peerswapds) < 2 {
				t.Fatalf("expected at least two peerswapds, got %d", len(peerswapds))
			}

			params := buildLndLwkParams(t, liquidd, lightningds, peerswapds, scid, swap.SWAPTYPE_IN)
			takerLnd := mustLndNode(t, params.takerNode)
			channelID := lndChanIDFromScid(t, takerLnd, params.scid)

			startLndLwkSwap(t, swap.SWAPTYPE_IN, params, channelID, peerswapds[1], tc.checkResp)

			tc.claim(t, params)
		})
	}
}

func Test_LndLnd_LWK_SwapOut(t *testing.T) {
	skipLWKTests(t)
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name           string
		claim          func(t *testing.T, params *testParams)
		paramsSwapType swap.SwapType
		requestType    swap.SwapType
		requesterIdx   int
		checkResp      bool
	}{
		{
			name:           "claim_normal",
			claim:          preimageClaimTest,
			paramsSwapType: swap.SWAPTYPE_OUT,
			requestType:    swap.SWAPTYPE_OUT,
			requesterIdx:   0,
			checkResp:      true,
		},
		{
			name:           "claim_coop",
			claim:          coopClaimTest,
			paramsSwapType: swap.SWAPTYPE_OUT,
			requestType:    swap.SWAPTYPE_OUT,
			requesterIdx:   0,
		},
		{
			name:           "claim_csv",
			claim:          csvClaimTest,
			paramsSwapType: swap.SWAPTYPE_IN,
			requestType:    swap.SWAPTYPE_OUT,
			requesterIdx:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bitcoind, liquidd, lightningds, peerswapds, scid, electrsd, lwk := lndlndLWKSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithLndNodesWithLiquid(lightningds),
				WithPeerSwapds(peerswapds...),
				WithElectrs(electrsd),
				WithLWK(lwk),
			)

			if tc.requesterIdx >= len(peerswapds) {
				t.Fatalf("requester index %d out of range", tc.requesterIdx)
			}

			params := buildLndLwkParams(t, liquidd, lightningds, peerswapds, scid, tc.paramsSwapType)
			takerLnd := mustLndNode(t, params.takerNode)
			channelID := lndChanIDFromScid(t, takerLnd, params.scid)

			startLndLwkSwap(t, tc.requestType, params, channelID, peerswapds[tc.requesterIdx], tc.checkResp)

			tc.claim(t, params)
		})
	}
}

func Test_LndCln_LWK_SwapIn(t *testing.T) {
	skipLWKTests(t)
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name      string
		claim     func(t *testing.T, params *testParams)
		checkResp bool
	}{
		{name: "claim_normal", claim: preimageClaimTest, checkResp: true},
		{name: "claim_coop", claim: coopClaimTest},
		{name: "claim_csv", claim: csvClaimTest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bitcoind, liquidd, lightningds, peerswapd, scid, electrs, lwk := mixedLWKSetup(t, uint64(math.Pow10(9)), FunderCLN)
			DumpOnFailure(t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithLightningNodes(lightningds),
				WithPeerSwapd(peerswapd),
				WithElectrs(electrs),
				WithLWK(lwk),
			)

			params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, swap.SWAPTYPE_IN)
			makerLnd := mustLndNode(t, params.makerNode)
			channelID := lndChanIDFromScid(t, makerLnd, params.scid)

			startLndLwkSwap(t, swap.SWAPTYPE_IN, params, channelID, peerswapd, tc.checkResp)

			tc.claim(t, params)
		})
	}
}

func Test_LndCln_LWK_SwapOut(t *testing.T) {
	skipLWKTests(t)
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name      string
		claim     func(t *testing.T, params *testParams)
		checkResp bool
	}{
		{name: "claim_normal", claim: preimageClaimTest, checkResp: true},
		{name: "claim_coop", claim: coopClaimTest},
		{name: "claim_csv", claim: csvClaimTest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bitcoind, liquidd, lightningds, peerswapd, scid, electrs, lwk := mixedLWKSetup(t, uint64(math.Pow10(9)), FunderLND)
			DumpOnFailure(t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithLightningNodes(lightningds),
				WithPeerSwapd(peerswapd),
				WithElectrs(electrs),
				WithLWK(lwk),
			)

			params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, swap.SWAPTYPE_OUT)
			takerLnd := mustLndNode(t, params.takerNode)
			channelID := lndChanIDFromScid(t, takerLnd, params.scid)

			startLndLwkSwap(t, swap.SWAPTYPE_OUT, params, channelID, peerswapd, tc.checkResp)

			tc.claim(t, params)
		})
	}
}
