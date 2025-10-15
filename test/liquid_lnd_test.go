package test

import (
	"context"
	"testing"

	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
)

const lndLiquidFundAmount = uint64(1_000_000_000)

// helpers for table-driven Liquid LND tests.
func lndLiquidParams(t *testing.T, liquidd *testframework.LiquidNode, lightningds []*LndNodeWithLiquid, peerswapds []*PeerSwapd, scid string, st swap.SwapType) (*testParams, uint64) {
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
		chainRPC:            liquidd.RpcProxy,
		chaind:              liquidd,
		confirms:            LiquidConfirms,
		csv:                 LiquidCsv,
		swapType:            st,
		premiumLimitRatePPM: 100000,
		swapInPremiumRate:   premium.DefaultLBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultLBTCSwapOutPremiumRatePPM,
	}

	return params, lcid
}

// Failure dumps are handled via DumpOnFailure in failuredump.go

func startLndLiquidSwap(t *testing.T, peerswapd *PeerSwapd, channelID uint64, params *testParams) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	asset := "lbtc"

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

func runLndLndLiquidSwapCases(t *testing.T, cases []lndSwapCase) {
	t.Helper()
	IsIntegrationTest(t)
	t.Parallel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, peerswapds, scid := lndlndElementsSetup(t, lndLiquidFundAmount)
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLiquid(liquidd), WithLndNodesWithLiquid(lightningds), WithPeerSwapds(peerswapds...))

			params, channelID := lndLiquidParams(t, liquidd, lightningds, peerswapds, scid, tc.swapType)
			starter := peerswapds[0]
			if tc.swapType == swap.SWAPTYPE_IN {
				starter = peerswapds[1]
			}

			startLndLiquidSwap(t, starter, channelID, params)
			tc.claim(t, params)
		})
	}
}

func runLndClnLiquidSwapCases(t *testing.T, cases []lndClnSwapCase) {
	t.Helper()
	IsIntegrationTest(t)
	t.Parallel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, lndLiquidFundAmount, tc.funder)
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLiquid(liquidd), WithLightningNodes(lightningds), WithPeerSwapd(peerswapd))

			params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, tc.swapType)
			lndNode := mustFindLndNode(t, lightningds)
			channelID, err := lndNode.ChanIdFromScid(scid)
			requireNoError(t, err)

			startLndLiquidSwap(t, peerswapd, channelID, params)
			tc.claim(t, params)
		})
	}
}

func Test_LndLnd_Liquid_SwapIn(t *testing.T) {
	cases := []lndSwapCase{
		{name: "claim_normal", swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
		{name: "claim_coop", swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
		{name: "claim_csv", swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
	}
	runLndLndLiquidSwapCases(t, cases)
}

func Test_LndLnd_Liquid_SwapOut(t *testing.T) {
	cases := []lndSwapCase{
		{name: "claim_normal", swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
		{name: "claim_coop", swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
		{name: "claim_csv", swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
	}
	runLndLndLiquidSwapCases(t, cases)
}

func Test_LndCln_Liquid_SwapIn(t *testing.T) {
	cases := []lndClnSwapCase{
		{name: "claim_normal", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
		{name: "claim_coop", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
		{name: "claim_csv", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
	}
	runLndClnLiquidSwapCases(t, cases)
}

func Test_LndCln_Liquid_SwapOut(t *testing.T) {
	cases := []lndClnSwapCase{
		{name: "claim_normal", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
		{name: "claim_coop", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
		{name: "claim_csv", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
	}
	runLndClnLiquidSwapCases(t, cases)
}
