package test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/elementsproject/glightning/jrpc2"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
)

// helpers for table-driven Liquid CLN tests.
func clnLiquidParams(t *testing.T, liquidd *testframework.LiquidNode, lightningds []*CLightningNodeWithLiquid, scid string, st swap.SwapType) *testParams {
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

	return &testParams{
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
		chainRPC:            liquidd.RpcProxy,
		chaind:              liquidd,
		confirms:            LiquidConfirms,
		csv:                 LiquidCsv,
		swapType:            st,
		premiumLimitRatePPM: 100000,
		swapInPremiumRate:   premium.DefaultLBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultLBTCSwapOutPremiumRatePPM,
	}
}

// Failure dumps are handled via DumpOnFailure in failuredump.go

func startLiquidSwap(t *testing.T, params *testParams) {
	t.Helper()

	asset := "lbtc"
	callWithRetry := func(node *CLightningNodeWithLiquid, req jrpc2.Method) {
		const (
			maxAttempts = 5
			backoff     = 250 * time.Millisecond
		)

		var (
			response map[string]interface{}
			err      error
		)

		for range maxAttempts {
			err = node.Rpc.Request(req, &response)
			if err == nil || !isTransientClnPipeError(err) {
				break
			}
			time.Sleep(backoff)
		}

		requireNoError(t, err)
	}

	switch params.swapType {
	case swap.SWAPTYPE_IN:
		maker, ok := params.makerNode.(*CLightningNodeWithLiquid)
		if !ok {
			t.Fatalf("maker node is not a CLightningNodeWithLiquid")
		}
		callWithRetry(maker, &clightning.SwapIn{
			SatAmt:              params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		})
	case swap.SWAPTYPE_OUT:
		taker, ok := params.takerNode.(*CLightningNodeWithLiquid)
		if !ok {
			t.Fatalf("taker node is not a CLightningNodeWithLiquid")
		}
		callWithRetry(taker, &clightning.SwapOut{
			SatAmt:              params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		})
	default:
		t.Fatalf("unknown swap type: %v", params.swapType)
	}
}

func isTransientClnPipeError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "Pipe closed unexpectedly") ||
		strings.Contains(msg, "Connection reset by peer")
}

func buildMixedLiquidParams(t *testing.T, liquidd *testframework.LiquidNode, lightningds []testframework.LightningNode, peerswapd *PeerSwapd, scid string, swapType swap.SwapType) *testParams {
	t.Helper()

	require := requireNew(t)

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
		chainRPC:            liquidd.RpcProxy,
		chaind:              liquidd,
		confirms:            LiquidConfirms,
		csv:                 LiquidCsv,
		swapType:            swapType,
		premiumLimitRatePPM: 100000,
		swapInPremiumRate:   premium.DefaultLBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultLBTCSwapOutPremiumRatePPM,
	}

	switch taker := lightningds[0].(type) {
	case *CLightningNodeWithLiquid:
		params.takerPeerswap = taker.DaemonProcess
	case *LndNodeWithLiquid:
		params.takerPeerswap = peerswapd.DaemonProcess
	default:
		t.Fatalf("unexpected taker node type %T", lightningds[0])
	}

	switch maker := lightningds[1].(type) {
	case *CLightningNodeWithLiquid:
		params.makerPeerswap = maker.DaemonProcess
	case *LndNodeWithLiquid:
		params.makerPeerswap = peerswapd.DaemonProcess
	default:
		t.Fatalf("unexpected maker node type %T", lightningds[1])
	}

	return params
}

func Test_ClnCln_Liquid_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name  string
		claim func(t *testing.T, params *testParams)
	}{
		{name: "claim_normal", claim: preimageClaimTest},
		{name: "claim_coop", claim: coopClaimTest},
		{name: "claim_csv", claim: csvClaimTest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLiquid(liquidd), WithCLightningNodes(lightningds, nil))

			params := clnLiquidParams(t, liquidd, lightningds, scid, swap.SWAPTYPE_IN)
			startLiquidSwap(t, params)
			tc.claim(t, params)
		})
	}
}

func Test_ClnCln_Liquid_SwapOut(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name  string
		claim func(t *testing.T, params *testParams)
	}{
		{name: "claim_normal", claim: preimageClaimTest},
		{name: "claim_coop", claim: coopClaimTest},
		{name: "claim_csv", claim: csvClaimTest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLiquid(liquidd), WithCLightningNodes(lightningds, nil))

			params := clnLiquidParams(t, liquidd, lightningds, scid, swap.SWAPTYPE_OUT)
			startLiquidSwap(t, params)
			tc.claim(t, params)
		})
	}
}

func Test_ClnLnd_Liquid_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name  string
		claim func(t *testing.T, params *testParams)
	}{
		{name: "claim_normal", claim: preimageClaimTest},
		{name: "claim_coop", claim: coopClaimTest},
		{name: "claim_csv", claim: csvClaimTest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(9)), FunderLND)
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLiquid(liquidd), WithLightningNodes(lightningds), WithPeerSwapd(peerswapd))

			params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, swap.SWAPTYPE_IN)
			startLiquidSwap(t, params)
			tc.claim(t, params)
		})
	}
}

func Test_ClnLnd_Liquid_SwapOut(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name  string
		claim func(t *testing.T, params *testParams)
	}{
		{name: "claim_normal", claim: preimageClaimTest},
		{name: "claim_coop", claim: coopClaimTest},
		{name: "claim_csv", claim: csvClaimTest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(9)), FunderCLN)
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLiquid(liquidd), WithLightningNodes(lightningds), WithPeerSwapd(peerswapd))

			params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, swap.SWAPTYPE_OUT)
			startLiquidSwap(t, params)
			tc.claim(t, params)
		})
	}
}

func Test_CLNLiquidSetup(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("accept-discount-ct is not enabled", func(t *testing.T) {
		t.Parallel()
		_, _, ln := clnSingleElementsSetup(t, map[string]string{
			"listen":           "1",
			"debug":            "1",
			"rpcuser":          "rpcuser",
			"rpcpassword":      "rpcpass",
			"fallbackfee":      "0.00001",
			"initialfreecoins": "2100000000000000",
			"validatepegin":    "0",
			"chain":            "liquidregtest",
			"minrelaytxfee":    "0.00000001",
			"mintxfee":         "0.00000001",
			"blockmintxfee":    "0.00000001",
		})
		DumpOnFailure(t, WithCLightnings([]*testframework.CLightningNode{ln}))

		err := ln.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err: %v", err)
		}

		err = ln.WaitForLog("accept-discount-ct is not enabled", testframework.TIMEOUT)
		if err != nil {
			t.Fatalf("lightningd.WaitForLog() got err: %v", err)
		}
	})
	t.Run("accept-discount-ct is enabled", func(t *testing.T) {
		t.Parallel()
		_, _, ln := clnSingleElementsSetup(t, map[string]string{
			"listen":           "1",
			"debug":            "1",
			"rpcuser":          "rpcuser",
			"rpcpassword":      "rpcpass",
			"fallbackfee":      "0.00001",
			"initialfreecoins": "2100000000000000",
			"validatepegin":    "0",
			"chain":            "liquidregtest",
			// if `creatediscountct` is enabled, `acceptdiscountct` is also enabled
			"creatediscountct": "1",
			"minrelaytxfee":    "0.00000001",
			"mintxfee":         "0.00000001",
			"blockmintxfee":    "0.00000001",
		})
		DumpOnFailure(t, WithCLightnings([]*testframework.CLightningNode{ln}))

		err := ln.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err: %v", err)
		}

		err = ln.WaitForLog("peerswap initialized", testframework.TIMEOUT)
		if err != nil {
			t.Fatalf("lightningd.WaitForLog() got err: %v", err)
		}
	})
}
