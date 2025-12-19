package test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elementsproject/glightning/jrpc2"
	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
)

// helpers for table-driven Liquid CLN tests.
func clnLiquidParams(
	t *testing.T,
	liquidd *testframework.LiquidNode,
	lightningds []*CLightningNodeWithLiquid,
	scid string,
	st swap.SwapType,
) *testParams {
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
			response map[string]any
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

func buildMixedLiquidParams(
	t *testing.T,
	liquidd *testframework.LiquidNode,
	lightningds []testframework.LightningNode,
	peerswapd *PeerSwapd,
	scid string,
	swapType swap.SwapType,
) *testParams {
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

const lndLiquidFundAmount = uint64(1_000_000_000)

// helpers for table-driven Liquid LND tests.
func lndLiquidParams(
	t *testing.T,
	liquidd *testframework.LiquidNode,
	lightningds []*LndNodeWithLiquid,
	peerswapds []*PeerSwapd,
	scid string,
	st swap.SwapType,
) (*testParams, uint64) {
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
			DumpOnFailure(
				t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithLndNodesWithLiquid(lightningds),
				WithPeerSwapds(peerswapds...),
			)

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
			DumpOnFailure(
				t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithLightningNodes(lightningds),
				WithPeerSwapd(peerswapd),
			)

			params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, tc.swapType)
			lndNode := mustFindLndNode(t, lightningds)
			channelID, err := lndNode.ChanIdFromScid(scid)
			requireNoError(t, err)

			startLndLiquidSwap(t, peerswapd, channelID, params)
			tc.claim(t, params)
		})
	}
}
