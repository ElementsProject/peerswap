package test

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/premium"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/vulpemventures/go-elements/network"
)

// skipLWKTests skips all LWK-related tests due to intermittent CI failures
// See: https://github.com/ElementsProject/peerswap/actions/runs/16069080660/job/45350236095?pr=385
func skipLWKTests(t *testing.T) {
	t.Helper()

	t.Skip("Skipping lwk_cln_test due to intermittent CI failures")
}

// Failure log dumping is consolidated via DumpOnFailure in test/failuredump.go

func startClnLwkSwap(t *testing.T, params *testParams, checkResp bool) {
	t.Helper()

	var req *testAssertions
	if checkResp {
		req = requireNew(t)
	}

	asset := "lbtc"
	assetID := network.Regtest.AssetID
	switch params.swapType {
	case swap.SWAPTYPE_IN:
		maker, ok := params.makerNode.(*CLightningNodeWithLiquid)
		if !ok {
			t.Fatalf("maker node is not a CLightningNodeWithLiquid")
		}
		go func(node *CLightningNodeWithLiquid) {
			var response map[string]interface{}
			err := node.Rpc.Request(&clightning.SwapIn{
				LnAmountSat:         params.swapAmt,
				AssetAmount:         params.swapAmt,
				AssetId:             assetID,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM,
			}, &response)
			if req != nil {
				req.NoError(err)
			}
		}(maker)
	case swap.SWAPTYPE_OUT:
		taker, ok := params.takerNode.(*CLightningNodeWithLiquid)
		if !ok {
			t.Fatalf("taker node is not a CLightningNodeWithLiquid")
		}
		go func(node *CLightningNodeWithLiquid) {
			var response map[string]interface{}
			err := node.Rpc.Request(&clightning.SwapOut{
				LnAmountSat:         params.swapAmt,
				AssetAmount:         params.swapAmt,
				AssetId:             assetID,
				ShortChannelId:      params.scid,
				Asset:               asset,
				PremiumLimitRatePPM: params.premiumLimitRatePPM,
			}, &response)
			if req != nil {
				req.NoError(err)
			}
		}(taker)
	default:
		t.Fatalf("unknown swap type: %v", params.swapType)
	}
}

func Test_ClnCln_LWKElements_SwapIn(t *testing.T) {
	skipLWKTests(t)
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name   string
		claim  func(t *testing.T, params *testParams)
		filter [2]string
	}{
		{
			name:  "claim_normal",
			claim: preimageClaimTest,
			filter: [2]string{
				os.Getenv("PEERSWAP_TEST_FILTER"),
				"",
			},
		},
		{
			name:  "claim_coop",
			claim: coopClaimTest,
			filter: [2]string{
				os.Getenv("PEERSWAP_TEST_FILTER"),
				os.Getenv("PEERSWAP_TEST_FILTER"),
			},
		},
		{
			name:  "claim_csv",
			claim: csvClaimTest,
			filter: [2]string{
				os.Getenv("PEERSWAP_TEST_FILTER"),
				os.Getenv("PEERSWAP_TEST_FILTER"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, scid, electrs, lwk := clnclnLWKLiquidSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithCLightningNodes(lightningds, []string{tc.filter[0], tc.filter[1]}),
				WithElectrs(electrs),
				WithLWK(lwk),
			)

			params := clnLiquidParams(t, liquidd, lightningds, scid, swap.SWAPTYPE_IN)

			startClnLwkSwap(t, params, true)

			tc.claim(t, params)
		})
	}
}

func Test_ClnCln_LWKLiquid_SwapOut(t *testing.T) {
	skipLWKTests(t)
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name  string
		claim func(t *testing.T, params *testParams)
		setup func(t *testing.T) (*testframework.BitcoinNode, *testframework.LiquidNode, []*CLightningNodeWithLiquid, string, *testframework.Electrs, *testframework.LWK)
	}{
		{
			name:  "claim_normal",
			claim: preimageClaimTest,
			setup: func(t *testing.T) (*testframework.BitcoinNode, *testframework.LiquidNode, []*CLightningNodeWithLiquid, string, *testframework.Electrs, *testframework.LWK) {
				t.Helper()
				return clnclnLWKLiquidSetup(t, uint64(math.Pow10(9)))
			},
		},
		{
			name:  "claim_coop",
			claim: coopClaimTest,
			setup: func(t *testing.T) (*testframework.BitcoinNode, *testframework.LiquidNode, []*CLightningNodeWithLiquid, string, *testframework.Electrs, *testframework.LWK) {
				t.Helper()
				return clnclnLWKSetup(t, uint64(math.Pow10(9)))
			},
		},
		{
			name:  "claim_csv",
			claim: csvClaimTest,
			setup: func(t *testing.T) (*testframework.BitcoinNode, *testframework.LiquidNode, []*CLightningNodeWithLiquid, string, *testframework.Electrs, *testframework.LWK) {
				t.Helper()
				return clnclnLWKSetup(t, uint64(math.Pow10(9)))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, scid, electrs, lwk := tc.setup(t)
			DumpOnFailure(t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithCLightningNodes(lightningds, nil),
				WithElectrs(electrs),
				WithLWK(lwk),
			)

			params := clnLiquidParams(t, liquidd, lightningds, scid, swap.SWAPTYPE_OUT)

			startClnLwkSwap(t, params, true)

			tc.claim(t, params)
		})
	}
}

func Test_ClnCln_LWKLiquid_BackendDown(t *testing.T) {
	skipLWKTests(t)
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name    string
		kill    func(lwk *testframework.LWK, electrs *testframework.Electrs)
		request func(node *CLightningNodeWithLiquid, params *testParams) error
	}{
		{
			name: "lwkdown",
			kill: func(lwk *testframework.LWK, _ *testframework.Electrs) {
				lwk.Process.Kill()
			},
			request: func(node *CLightningNodeWithLiquid, params *testParams) error {
				var response map[string]interface{}
				return node.Rpc.Request(
					&clightning.SwapOut{
						LnAmountSat:         params.swapAmt,
						AssetAmount:         params.swapAmt,
						AssetId:             network.Regtest.AssetID,
						ShortChannelId:      params.scid,
						Asset:               "lbtc",
						PremiumLimitRatePPM: params.premiumLimitRatePPM,
					},
					&response,
				)
			},
		},
		{
			name: "electrsdown",
			kill: func(_ *testframework.LWK, electrs *testframework.Electrs) {
				electrs.Process.Kill()
			},
			request: func(node *CLightningNodeWithLiquid, params *testParams) error {
				var response map[string]interface{}
				return node.Rpc.Request(
					&clightning.SwapOut{
						LnAmountSat:    params.swapAmt,
						AssetAmount:    params.swapAmt,
						AssetId:        network.Regtest.AssetID,
						ShortChannelId: params.scid,
						Asset:          "lbtc",
					},
					&response,
				)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require := requireNew(t)

			bitcoind, liquidd, lightningds, scid, electrs, lwk := clnclnLWKLiquidSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithCLightningNodes(lightningds, nil),
				WithElectrs(electrs),
				WithLWK(lwk),
			)

			params := clnLiquidParams(t, liquidd, lightningds, scid, swap.SWAPTYPE_OUT)

			tc.kill(lwk, electrs)

			err := tc.request(lightningds[1], params)
			require.Error(err)
		})
	}
}

// Failure dumps are handled via DumpOnFailure in failuredump.go

func buildLndLwkParams(
	t *testing.T,
	liquidd *testframework.LiquidNode,
	lightningds []*LndNodeWithLiquid,
	peerswapds []*PeerSwapd,
	scid string,
	swapType swap.SwapType,
) *testParams {
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

func startLndLwkSwap(
	t *testing.T,
	requestType swap.SwapType,
	params *testParams,
	channelID uint64,
	requester *PeerSwapd,
	checkResp bool,
) {
	t.Helper()

	if requester == nil {
		t.Fatalf("nil peerswapd requester")
	}

	var req *testAssertions
	if checkResp {
		req = requireNew(t)
	}

	asset := "lbtc"
	assetID := network.Regtest.AssetID
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var call func(context.Context) error

	switch requestType {
	case swap.SWAPTYPE_IN:
		call = func(ctx context.Context) error {
			_, err := requester.PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:           channelID,
				LnAmountSat:         params.swapAmt,
				Asset:               asset,
				AssetId:             assetID,
				AssetAmount:         params.swapAmt,
				PremiumLimitRatePpm: params.premiumLimitRatePPM,
			})
			return err
		}
	case swap.SWAPTYPE_OUT:
		call = func(ctx context.Context) error {
			_, err := requester.PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:           channelID,
				LnAmountSat:         params.swapAmt,
				Asset:               asset,
				AssetId:             assetID,
				AssetAmount:         params.swapAmt,
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
