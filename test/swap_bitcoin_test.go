package test

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
)

// helpers for table-driven CLN BTC tests.
func clnParams(
	t *testing.T,
	bitcoind *testframework.BitcoinNode,
	lightningds []*testframework.CLightningNode,
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
		chainRPC:            bitcoind.RpcProxy,
		chaind:              bitcoind,
		confirms:            BitcoinConfirms,
		csv:                 BitcoinCsv,
		swapType:            st,
		premiumLimitRatePPM: 100000,
		swapInPremiumRate:   premium.DefaultBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultBTCSwapOutPremiumRatePPM,
	}
}

// Failure dumps are handled via DumpOnFailure in failuredump.go

func startClnSwap(t *testing.T, params *testParams) {
	t.Helper()

	asset := "btc"
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		maker, ok := params.makerNode.(*testframework.CLightningNode)
		if !ok {
			t.Fatalf("maker node is not a CLightningNode")
		}
		go func(node *testframework.CLightningNode) {
			var response map[string]interface{}
			_ = node.Rpc.Request(&clightning.SwapIn{
				LnAmountSat:         params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM,
			}, &response)
		}(maker)
	case swap.SWAPTYPE_OUT:
		taker, ok := params.takerNode.(*testframework.CLightningNode)
		if !ok {
			t.Fatalf("taker node is not a CLightningNode")
		}
		go func(node *testframework.CLightningNode) {
			var response map[string]interface{}
			_ = node.Rpc.Request(&clightning.SwapOut{
				LnAmountSat:         params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM,
			}, &response)
		}(taker)
	default:
		t.Fatalf("unknown swap type: %v", params.swapType)
	}
}

type clnLndSwapCase struct {
	name     string
	funder   fundingNode
	swapType swap.SwapType
	claim    func(t *testing.T, params *testParams)
}

const mixedFundAmount = uint64(1_000_000_000)

func runClnLndSwapCases(t *testing.T, cases []clnLndSwapCase) {
	t.Helper()
	IsIntegrationTest(t)
	t.Parallel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bitcoind, lightningds, peerswapd, scid := mixedSetup(t, mixedFundAmount, tc.funder)
			DumpOnFailure(t, WithBitcoin(bitcoind), WithLightningNodes(lightningds), WithPeerSwapd(peerswapd))

			params := buildClnLndParams(t, bitcoind, lightningds, peerswapd, scid, tc.swapType)
			startClnSwap(t, params)
			tc.claim(t, params)
		})
	}
}

func buildClnLndParams(
	t *testing.T,
	bitcoind *testframework.BitcoinNode,
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
		takerPeerswap:       nil,
		makerPeerswap:       nil,
		chainRPC:            bitcoind.RpcProxy,
		chaind:              bitcoind,
		confirms:            BitcoinConfirms,
		csv:                 BitcoinCsv,
		swapType:            swapType,
		premiumLimitRatePPM: 100000,
		swapInPremiumRate:   premium.DefaultBTCSwapInPremiumRatePPM,
		swapOutPremiumRate:  premium.DefaultBTCSwapOutPremiumRatePPM,
	}

	switch taker := lightningds[0].(type) {
	case *testframework.CLightningNode:
		params.takerPeerswap = taker.DaemonProcess
	case *testframework.LndNode:
		params.takerPeerswap = peerswapd.DaemonProcess
	default:
		t.Fatalf("unexpected taker node type %T", lightningds[0])
	}

	switch maker := lightningds[1].(type) {
	case *testframework.CLightningNode:
		params.makerPeerswap = maker.DaemonProcess
	case *testframework.LndNode:
		params.makerPeerswap = peerswapd.DaemonProcess
	default:
		t.Fatalf("unexpected maker node type %T", lightningds[1])
	}

	return params
}

// registerClnLndFailureDump removed in favor of DumpOnFailure

// Test_OnlyOneActiveSwapPerChannelCln checks that there is only one active swap per
// channel.
func Test_OnlyOneActiveSwapPerChannelCln(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(6)))
	DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

	params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_OUT)
	params.swapAmt = params.origTakerBalance / 5
	asset := "btc"

	// Do swap. Expect N_SWAPS - 1 errors.
	wg := sync.WaitGroup{}
	const nSwaps = 10
	var nErr int32
	for i := range nSwaps {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			var response map[string]interface{}
			err := lightningds[0].Rpc.Request(&clightning.SwapOut{
				LnAmountSat:         params.swapAmt,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM,
			}, &response)
			t.Logf("[%d] Response: %v", n, response)
			if err != nil {
				t.Logf("[%d] Err: %s", n, err.Error())
				atomic.AddInt32(&nErr, 1)
			}
		}(i)
	}
	wg.Wait()

	var response *peerswaprpc.ListSwapsResponse
	err := lightningds[0].Rpc.Request(&clightning.ListActiveSwaps{}, &response)
	requireNoError(t, err)
	t.Logf("GOT: %v", response)

	assertEqualNumericValues(t, nSwaps-1, nErr, "expected nswaps-1=%d errors, got: %d", nSwaps-1, nErr)
	assertEqualValues(t, 1, len(response.Swaps), "expected only 1 active swap, got: %d", len(response.Swaps))
}

func Test_ClnCln_ExcessiveAmount(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("excessive", func(t *testing.T) {
		t.Parallel()

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
		DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

		params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_OUT)
		params.swapAmt = params.origTakerBalance * 2
		asset := "btc"

		// Swap out should fail as the swap_amt is to high.
		var response map[string]interface{}
		err := lightningds[0].Rpc.Request(&clightning.SwapOut{
			LnAmountSat:         params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		}, &response)
		assertError(t, err)

		// Swap in should fail as the swap_amt is to high.
		err = lightningds[1].Rpc.Request(&clightning.SwapIn{
			LnAmountSat:         params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		}, &response)
		assertError(t, err)
	})
}

func Test_Cln_HtlcMaximum(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()
	t.Run("swapout", func(t *testing.T) {
		t.Parallel()

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
		DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

		params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_IN)
		asset := "btc"

		_, err := lightningds[0].SetHtlcMaximumMilliSatoshis(scid, params.origTakerBalance*1000/2-1)
		assertNoError(t, err)

		var response map[string]interface{}
		err = lightningds[0].Rpc.Request(&clightning.SwapOut{
			LnAmountSat:         params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		}, &response)
		assertError(t, err)
	})
	t.Run("swapin", func(t *testing.T) {
		t.Parallel()

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
		DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

		params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_IN)
		asset := "btc"

		_, err := lightningds[0].SetHtlcMaximumMilliSatoshis(scid, params.origTakerBalance*1000/2-1)
		assertNoError(t, err)

		var response map[string]interface{}
		err = lightningds[1].Rpc.Request(&clightning.SwapIn{
			LnAmountSat:         params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		}, &response)
		assertError(t, err)
	})
}

func Test_Cln_Premium(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("negative_swapin", func(t *testing.T) {
		t.Parallel()

		bitcoind, lightningds, scid := clnclnSetupWithConfig(t, uint64(math.Pow10(9)), 0, []string{
			"--dev-bitcoind-poll=1",
			"--dev-fast-gossip",
			"--large-channels",
		}, true, []byte("accept_all_peers=1\n"),
		)
		DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

		var premiumRatePPM int64 = -10000
		var premiumRes interface{}
		err := lightningds[0].Rpc.Request(&clightning.UpdatePremiumRate{
			PeerID:         lightningds[1].Id(),
			Asset:          premium.BTC.String(),
			Operation:      premium.SwapIn.String(),
			PremiumRatePPM: premiumRatePPM,
		}, &premiumRes)
		assertNoError(t, err)

		params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_IN)
		params.swapInPremiumRate = premiumRatePPM
		asset := "btc"

		var response map[string]interface{}
		err = lightningds[1].Rpc.Request(&clightning.SwapIn{
			LnAmountSat:         params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		}, &response)
		assertNoError(t, err)

		preimageClaimTest(t, params)
	})

	t.Run("negative_swapout", func(t *testing.T) {
		t.Parallel()

		bitcoind, lightningds, scid := clnclnSetupWithConfig(t, uint64(math.Pow10(9)), 0, []string{
			"--dev-bitcoind-poll=1",
			"--dev-fast-gossip",
			"--large-channels",
		}, true, []byte("accept_all_peers=1\n"),
		)
		DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

		var premiumRatePPM int64 = -10000
		var premiumRes interface{}
		err := lightningds[1].Rpc.Request(&clightning.UpdatePremiumRate{
			PeerID:         lightningds[0].Id(),
			Asset:          premium.BTC.String(),
			Operation:      premium.SwapOut.String(),
			PremiumRatePPM: premiumRatePPM,
		}, &premiumRes)
		assertNoError(t, err)

		params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_OUT)
		params.swapOutPremiumRate = premiumRatePPM
		asset := "btc"

		var response map[string]interface{}
		err = lightningds[0].Rpc.Request(&clightning.SwapOut{
			LnAmountSat:         params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		}, &response)
		assertNoError(t, err)

		preimageClaimTest(t, params)
	})

	t.Run("exceed_limit", func(t *testing.T) {
		t.Parallel()

		bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
		DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

		params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_IN)
		params.premiumLimitRatePPM = -1
		asset := "btc"

		var response map[string]interface{}
		err := lightningds[1].Rpc.Request(&clightning.SwapIn{
			LnAmountSat:         params.swapAmt,
			ShortChannelId:      params.scid,
			Asset:               asset,
			PremiumLimitRatePPM: params.premiumLimitRatePPM,
		}, &response)
		assertError(t, err)
	})
}

// Test_ClnCln_StuckChannels tests that the swap fails if the channel is stuck.
// For more information about stuck channel, please check the link.
// https://github.com/lightning/bolts/issues/728
func Test_ClnCln_StuckChannels(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	bitcoind, lightningds, scid := clnclnSetupWithConfig(t, 37500, 35315, []string{
		"--dev-bitcoind-poll=1",
		"--dev-fast-gossip",
		"--large-channels",
		"--min-capacity-sat=1000",
		"--min-emergency-msat=600000",
	}, false, []byte("accept_all_peers=1\n"))
	DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

	params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_IN)
	params.swapAmt = params.origTakerBalance

	assertNoError(t, lightningds[0].ForceFeeUpdate(scid, "2530"))
	assertNoError(t, testframework.WaitForWithErr(func() (bool, error) {
		return lightningds[1].IsChannelActive(scid)
	}, testframework.TIMEOUT))

	var response map[string]interface{}
	err := lightningds[1].Rpc.Request(
		&clightning.SwapIn{
			LnAmountSat:    100,
			ShortChannelId: params.scid,
			Asset:          "btc",
		},
		&response,
	)
	assertError(t, err)
}

func Test_Cln_shutdown(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	require := requireNew(t)
	bitcoind, lightningds, _ := clnclnSetup(t, uint64(math.Pow10(9)))
	DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

	require.NoError(lightningds[0].Shutdown())
	require.NoError(lightningds[0].WaitForLog(
		"plugin-peerswap: Killing plugin: exited during normal operation", 30))
}

func Test_ClnCln_Poll(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	require := requireNew(t)
	bitcoind, lightningds, _ := clnclnSetup(t, uint64(math.Pow10(9)))
	DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

	// Ensure that the poll executed at the start of peerswap succeeds
	require.Error(lightningds[0].WaitForLog("failed to send custom message", 20*time.Second))
	for _, lightningd := range lightningds {
		var result interface{}
		err := lightningd.Rpc.Request(&clightning.ReloadPolicyFile{}, &result)
		if err != nil {
			t.Fatal(err)
		}
	}
}

const lndFundAmount = uint64(1_000_000_000)

// helpers for table-driven LND BTC tests.
func lndParams(
	t *testing.T,
	bitcoind *testframework.BitcoinNode,
	lightningds []*testframework.LndNode,
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
				LnAmountSat:         params.swapAmt,
				Asset:               asset,
				PremiumLimitRatePpm: params.premiumLimitRatePPM,
			})
		}()
	case swap.SWAPTYPE_OUT:
		go func() {
			_, _ = peerswapd.PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:           channelID,
				LnAmountSat:         params.swapAmt,
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
				LnAmountSat:         params.swapAmt,
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
			LnAmountSat:         params.swapAmt,
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
			LnAmountSat:         params.swapAmt,
			Asset:               asset,
			PremiumLimitRatePpm: params.premiumLimitRatePPM,
		})
		assertError(t, err)
	})
}
