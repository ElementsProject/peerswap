package test

import (
	"math"
	"os"
	"testing"

	"github.com/elementsproject/peerswap/swap"
)

func Test_SwapInMatrix(t *testing.T) {
    t.Helper()
    // Run this top-level matrix in parallel with others to reduce CI wall time.
    t.Parallel()

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{name: "bitcoin_clncln", run: runBitcoinClnClnSwapIn},
		{name: "bitcoin_mixed", run: runBitcoinMixedSwapIn},
		{name: "bitcoin_lndlnd", run: runBitcoinLndLndSwapIn},
		{name: "liquid_clncln", run: runLiquidClnClnSwapIn},
		{name: "liquid_mixed", run: runLiquidMixedSwapIn},
		{name: "liquid_lndlnd", run: runLiquidLndLndSwapIn},
		{name: "lwk_clncln", run: runLwkClnClnSwapIn},
		{name: "lwk_mixed", run: runLwkMixedSwapIn},
		{name: "lwk_lndlnd", run: runLwkLndLndSwapIn},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

func runBitcoinClnClnSwapIn(t *testing.T) {
	t.Helper()

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
			bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))
			params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_IN)
			startClnSwap(t, params)
			tc.claim(t, params)
		})
	}
}

func runBitcoinMixedSwapIn(t *testing.T) {
	t.Helper()

	t.Run("cln_lnd", func(t *testing.T) {
		cases := []clnLndSwapCase{
			{name: "claim_normal", funder: FunderLND, swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
			{name: "claim_coop", funder: FunderLND, swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
			{name: "claim_csv", funder: FunderLND, swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
		}
		runClnLndSwapCases(t, cases)
	})

	t.Run("lnd_cln", func(t *testing.T) {
		cases := []lndClnSwapCase{
			{name: "claim_normal", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
			{name: "claim_coop", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
			{name: "claim_csv", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
		}
		runLndClnSwapCases(t, cases)
	})
}

func runBitcoinLndLndSwapIn(t *testing.T) {
	t.Helper()

	cases := []lndSwapCase{
		{name: "claim_normal", swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
		{name: "claim_coop", swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
		{name: "claim_csv", swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
	}
	runLndLndSwapCases(t, cases)
}

func runLiquidClnClnSwapIn(t *testing.T) {
	t.Helper()

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

func runLiquidMixedSwapIn(t *testing.T) {
	t.Helper()

	t.Run("cln_lnd", func(t *testing.T) {
		cases := []clnLndSwapCase{
			{name: "claim_normal", funder: FunderLND, swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
			{name: "claim_coop", funder: FunderLND, swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
			{name: "claim_csv", funder: FunderLND, swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
		}
		runClnLndSwapCases(t, cases)
	})

	t.Run("lnd_cln", func(t *testing.T) {
		cases := []lndClnSwapCase{
			{name: "claim_normal", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
			{name: "claim_coop", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
			{name: "claim_csv", funder: FunderCLN, swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
		}
		runLndClnSwapCases(t, cases)
	})
}

func runLiquidLndLndSwapIn(t *testing.T) {
	t.Helper()

	cases := []lndSwapCase{
		{name: "claim_normal", swapType: swap.SWAPTYPE_IN, claim: preimageClaimTest},
		{name: "claim_coop", swapType: swap.SWAPTYPE_IN, claim: coopClaimTest},
		{name: "claim_csv", swapType: swap.SWAPTYPE_IN, claim: csvClaimTest},
	}
	runLndLndLiquidSwapCases(t, cases)
}

func runLwkClnClnSwapIn(t *testing.T) {
	t.Helper()

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
				os.Getenv("PEERSWAP_TEST_FILTER"),
			},
		},
		{
			name:   "claim_coop",
			claim:  coopClaimTest,
			filter: [2]string{"", ""},
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
			bitcoind, liquidd, lightningds, scid, electrs, lwk := clnclnLWKSetup(t, uint64(math.Pow10(9)))
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

func runLwkMixedSwapIn(t *testing.T) {
	t.Helper()

	t.Run("cln_lnd", func(t *testing.T) {
		skipLWKTests(t)
		IsIntegrationTest(t)
		t.Parallel()
		cases := []struct {
			name      string
			claim     func(t *testing.T, params *testParams)
			checkResp bool
		}{
			{name: "claim_normal", claim: preimageClaimTest, checkResp: true},
			{name: "claim_coop", claim: coopClaimTest, checkResp: false},
			{name: "claim_csv", claim: csvClaimTest, checkResp: false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				bitcoind, liquidd, lightningds, peerswapd, scid, electrs, lwk := mixedLWKSetup(
					t,
					uint64(math.Pow10(9)),
					FunderLND,
				)
				DumpOnFailure(t,
					WithBitcoin(bitcoind),
					WithLiquid(liquidd),
					WithLightningNodes(lightningds),
					WithPeerSwapd(peerswapd),
					WithElectrs(electrs),
					WithLWK(lwk),
				)

				params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, swap.SWAPTYPE_IN)

				startClnLwkSwap(t, params, tc.checkResp)
				tc.claim(t, params)
			})
		}
	})

	t.Run("lnd_cln", func(t *testing.T) {
		skipLWKTests(t)
		IsIntegrationTest(t)
		t.Parallel()
		cases := []struct {
			name      string
			claim     func(t *testing.T, params *testParams)
			checkResp bool
		}{
			{name: "claim_normal", claim: preimageClaimTest, checkResp: true},
			{name: "claim_coop", claim: coopClaimTest, checkResp: false},
			{name: "claim_csv", claim: csvClaimTest, checkResp: false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				bitcoind, liquidd, lightningds, peerswapd, scid, electrs, lwk := mixedLWKSetup(
					t,
					uint64(math.Pow10(9)),
					FunderCLN,
				)
				DumpOnFailure(t,
					WithBitcoin(bitcoind),
					WithLiquid(liquidd),
					WithLightningNodes(lightningds),
					WithPeerSwapd(peerswapd),
					WithElectrs(electrs),
					WithLWK(lwk),
				)

				params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, swap.SWAPTYPE_IN)
				lndNode := mustFindLndNode(t, lightningds)
				channelID, err := lndNode.ChanIdFromScid(scid)
				requireNoError(t, err)

				startLndLwkSwap(t, swap.SWAPTYPE_IN, params, channelID, peerswapd, tc.checkResp)
				tc.claim(t, params)
			})
		}
	})
}

func runLwkLndLndSwapIn(t *testing.T) {
	t.Helper()

	skipLWKTests(t)
	IsIntegrationTest(t)
	t.Parallel()

	cases := []struct {
		name      string
		claim     func(t *testing.T, params *testParams)
		checkResp bool
	}{
		{name: "claim_normal", claim: preimageClaimTest, checkResp: true},
		{name: "claim_coop", claim: coopClaimTest, checkResp: false},
		{name: "claim_csv", claim: csvClaimTest, checkResp: false},
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

func Test_SwapOutMatrix(t *testing.T) {
    t.Helper()
    // Run this top-level matrix in parallel with others to reduce CI wall time.
    t.Parallel()

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{name: "bitcoin_clncln", run: runBitcoinClnClnSwapOut},
		{name: "bitcoin_mixed", run: runBitcoinMixedSwapOut},
		{name: "bitcoin_lndlnd", run: runBitcoinLndLndSwapOut},
		{name: "liquid_clncln", run: runLiquidClnClnSwapOut},
		{name: "liquid_mixed", run: runLiquidMixedSwapOut},
		{name: "liquid_lndlnd", run: runLiquidLndLndSwapOut},
		{name: "lwk_clncln", run: runLwkClnClnSwapOut},
		{name: "lwk_mixed", run: runLwkMixedSwapOut},
		{name: "lwk_lndlnd", run: runLwkLndLndSwapOut},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

func runBitcoinClnClnSwapOut(t *testing.T) {
	t.Helper()

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
			bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))
			params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_OUT)
			startClnSwap(t, params)
			tc.claim(t, params)
		})
	}
}

func runBitcoinMixedSwapOut(t *testing.T) {
	t.Helper()

	t.Run("cln_lnd", func(t *testing.T) {
		cases := []clnLndSwapCase{
			{name: "claim_normal", funder: FunderCLN, swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
			{name: "claim_coop", funder: FunderCLN, swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
			{name: "claim_csv", funder: FunderCLN, swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
		}
		runClnLndSwapCases(t, cases)
	})

	t.Run("lnd_cln", func(t *testing.T) {
		cases := []lndClnSwapCase{
			{name: "claim_normal", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
			{name: "claim_coop", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
			{name: "claim_csv", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
		}
		runLndClnSwapCases(t, cases)
	})
}

func runBitcoinLndLndSwapOut(t *testing.T) {
	t.Helper()

	cases := []lndSwapCase{
		{name: "claim_normal", swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
		{name: "claim_coop", swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
		{name: "claim_csv", swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
	}
	runLndLndSwapCases(t, cases)
}

func runLiquidClnClnSwapOut(t *testing.T) {
	t.Helper()

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

func runLiquidMixedSwapOut(t *testing.T) {
	t.Helper()

	t.Run("cln_lnd", func(t *testing.T) {
		cases := []clnLndSwapCase{
			{name: "claim_normal", funder: FunderCLN, swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
			{name: "claim_coop", funder: FunderCLN, swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
			{name: "claim_csv", funder: FunderCLN, swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
		}
		runClnLndLiquidSwapCases(t, cases)
	})

	t.Run("lnd_cln", func(t *testing.T) {
		cases := []lndClnSwapCase{
			{name: "claim_normal", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
			{name: "claim_coop", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
			{name: "claim_csv", funder: FunderLND, swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
		}
		runLndClnLiquidSwapCases(t, cases)
	})
}

func runLiquidLndLndSwapOut(t *testing.T) {
	t.Helper()

	cases := []lndSwapCase{
		{name: "claim_normal", swapType: swap.SWAPTYPE_OUT, claim: preimageClaimTest},
		{name: "claim_coop", swapType: swap.SWAPTYPE_OUT, claim: coopClaimTest},
		{name: "claim_csv", swapType: swap.SWAPTYPE_OUT, claim: csvClaimTest},
	}
	runLndLndLiquidSwapCases(t, cases)
}

func runLwkClnClnSwapOut(t *testing.T) {
	t.Helper()

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
				os.Getenv("PEERSWAP_TEST_FILTER"),
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
			bitcoind, liquidd, lightningds, scid, electrs, lwk := clnclnLWKSetup(t, uint64(math.Pow10(9)))
			DumpOnFailure(t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithCLightningNodes(lightningds, []string{tc.filter[0], tc.filter[1]}),
				WithElectrs(electrs),
				WithLWK(lwk),
			)

			params := clnLiquidParams(t, liquidd, lightningds, scid, swap.SWAPTYPE_OUT)

			startClnLwkSwap(t, params, true)
			tc.claim(t, params)
		})
	}
}

func runLwkMixedSwapOut(t *testing.T) {
	t.Helper()

	t.Run("cln_lnd", func(t *testing.T) {
		skipLWKTests(t)
		IsIntegrationTest(t)
		t.Parallel()
		cases := []struct {
			name      string
			claim     func(t *testing.T, params *testParams)
			checkResp bool
		}{
			{name: "claim_normal", claim: preimageClaimTest, checkResp: true},
			{name: "claim_coop", claim: coopClaimTest, checkResp: false},
			{name: "claim_csv", claim: csvClaimTest, checkResp: false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				bitcoind, liquidd, lightningds, peerswapd, scid, electrs, lwk := mixedLWKSetup(
					t,
					uint64(math.Pow10(9)),
					FunderCLN,
				)
				DumpOnFailure(t,
					WithBitcoin(bitcoind),
					WithLiquid(liquidd),
					WithLightningNodes(lightningds),
					WithPeerSwapd(peerswapd),
					WithElectrs(electrs),
					WithLWK(lwk),
				)

				params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, swap.SWAPTYPE_OUT)

				startClnLwkSwap(t, params, tc.checkResp)
				tc.claim(t, params)
			})
		}
	})

	t.Run("lnd_cln", func(t *testing.T) {
		skipLWKTests(t)
		IsIntegrationTest(t)
		t.Parallel()
		cases := []struct {
			name      string
			claim     func(t *testing.T, params *testParams)
			checkResp bool
		}{
			{name: "claim_normal", claim: preimageClaimTest, checkResp: true},
			{name: "claim_coop", claim: coopClaimTest, checkResp: false},
			{name: "claim_csv", claim: csvClaimTest, checkResp: false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				bitcoind, liquidd, lightningds, peerswapd, scid, electrs, lwk := mixedLWKSetup(
					t,
					uint64(math.Pow10(9)),
					FunderLND,
				)
				DumpOnFailure(t,
					WithBitcoin(bitcoind),
					WithLiquid(liquidd),
					WithLightningNodes(lightningds),
					WithPeerSwapd(peerswapd),
					WithElectrs(electrs),
					WithLWK(lwk),
				)

				params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, swap.SWAPTYPE_OUT)
				lndNode := mustFindLndNode(t, lightningds)
				channelID, err := lndNode.ChanIdFromScid(scid)
				requireNoError(t, err)

				startLndLwkSwap(t, swap.SWAPTYPE_OUT, params, channelID, peerswapd, tc.checkResp)
				tc.claim(t, params)
			})
		}
	})
}

func runLwkLndLndSwapOut(t *testing.T) {
	t.Helper()

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
			checkResp:      false,
		},
		{
			name:           "claim_csv",
			claim:          csvClaimTest,
			paramsSwapType: swap.SWAPTYPE_IN,
			requestType:    swap.SWAPTYPE_OUT,
			requesterIdx:   1,
			checkResp:      false,
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

			if len(peerswapds) < 2 {
				t.Fatalf("expected at least two peerswapds, got %d", len(peerswapds))
			}

			params := buildLndLwkParams(t, liquidd, lightningds, peerswapds, scid, tc.paramsSwapType)
			requester := peerswapds[tc.requesterIdx]
			takerLnd := mustLndNode(t, params.takerNode)
			channelID := lndChanIDFromScid(t, takerLnd, params.scid)

			startLndLwkSwap(t, tc.requestType, params, channelID, requester, tc.checkResp)

			tc.claim(t, params)
		})
	}
}

func runClnLndLiquidSwapCases(t *testing.T, cases []clnLndSwapCase) {
	t.Helper()

	IsIntegrationTest(t)
	t.Parallel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(9)), tc.funder)
			DumpOnFailure(
				t,
				WithBitcoin(bitcoind),
				WithLiquid(liquidd),
				WithLightningNodes(lightningds),
				WithPeerSwapd(peerswapd),
			)

			params := buildMixedLiquidParams(t, liquidd, lightningds, peerswapd, scid, tc.swapType)
			startLiquidSwap(t, params)
			tc.claim(t, params)
		})
	}
}
