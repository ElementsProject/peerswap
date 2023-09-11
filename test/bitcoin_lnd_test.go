package test

import (
	"context"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_OnlyOneActiveSwapPerChannelLnd checks that there is only one active swap per
// channel.
func Test_OnlyOneActiveSwapPerChannelLnd(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()
	require := require.New(t)

	bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(9)))
	defer func() {
		if t.Failed() {
			pprintFail(
				tailableProcess{
					p:     bitcoind.DaemonProcess,
					lines: defaultLines,
				},
				tailableProcess{
					p:     lightningds[0].DaemonProcess,
					lines: defaultLines,
				},
				tailableProcess{
					p:     lightningds[1].DaemonProcess,
					lines: defaultLines,
				},
				tailableProcess{
					p:     peerswapds[0].DaemonProcess,
					lines: defaultLines,
				},
				tailableProcess{
					p:     peerswapds[1].DaemonProcess,
					lines: defaultLines,
				},
			)
		}
	}()

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

	lcid, err := lightningds[0].ChanIdFromScid(scid)
	if err != nil {
		t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
	}

	params := &testParams{
		swapAmt:          channelBalances[0] / 5,
		scid:             scid,
		origTakerWallet:  walletBalances[0],
		origMakerWallet:  walletBalances[1],
		origTakerBalance: channelBalances[0],
		origMakerBalance: channelBalances[1],
		takerNode:        lightningds[0],
		makerNode:        lightningds[1],
		takerPeerswap:    peerswapds[0].DaemonProcess,
		makerPeerswap:    peerswapds[1].DaemonProcess,
		chainRpc:         bitcoind.RpcProxy,
		chaind:           bitcoind,
		confirms:         BitcoinConfirms,
		swapType:         swap.SWAPTYPE_IN,
	}
	asset := "btc"

	// Do swap. Expect N_SWAPS - 1 errors.
	wg := sync.WaitGroup{}
	N_SWAPS := 10
	var nErr int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < N_SWAPS; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			res, err := peerswapds[1].PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
			t.Logf("[%d] Response: %v", n, res)
			if err != nil {
				t.Logf("[%d] Err: %s", n, err.Error())
				atomic.AddInt32(&nErr, 1)
			}
		}(i)
	}
	wg.Wait()

	assert.EqualValues(t, N_SWAPS-1, nErr, "expected nswaps-1=%d errors, got: %d", N_SWAPS-1, nErr)
	err = testframework.WaitForWithErr(func() (bool, error) {
		res, err := peerswapds[1].PeerswapClient.ListActiveSwaps(ctx, &peerswaprpc.ListSwapsRequest{})
		if err != nil {
			return false, err
		}
		for _, r := range res.Swaps {
			if r.State == string(swap.State_SwapInSender_AwaitAgreement) {
				return false, nil
			}
		}
		assert.EqualValues(t, 1, len(res.Swaps), "expected only 1 active swap, got: %d - %v", len(res.Swaps), res)
		return true, nil
	}, 2*time.Second)
	assert.NoError(t, err)
}

func Test_LndLnd_Bitcoin_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(9)))
		defer func() {
			if t.Failed() {
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[1].DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapds[0].DaemonProcess,
			makerPeerswap:    peerswapds[1].DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapds[1].PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(9)))
		defer func() {
			if t.Failed() {
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[1].DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapds[0].DaemonProcess,
			makerPeerswap:    peerswapds[1].DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapds[1].PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(9)))
		defer func() {
			if t.Failed() {
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[1].DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapds[0].DaemonProcess,
			makerPeerswap:    peerswapds[1].DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapds[1].PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		csvClaimTest(t, params)
	})
}

func Test_LndLnd_Bitcoin_SwapOut(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(9)))
		defer func() {
			if t.Failed() {
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[1].DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapds[0].DaemonProcess,
			makerPeerswap:    peerswapds[1].DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}

		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapds[0].PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(9)))
		defer func() {
			if t.Failed() {
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[1].DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapds[0].DaemonProcess,
			makerPeerswap:    peerswapds[1].DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}

		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapds[0].PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(9)))
		defer func() {
			if t.Failed() {
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[0].DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     peerswapds[1].DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapds[0].DaemonProcess,
			makerPeerswap:    peerswapds[1].DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapds[0].PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		csvClaimTest(t, params)
	})
}

func Test_LndCln_Bitcoin_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     peerswapd.DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[1].(*testframework.LndNode).ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapd.PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     peerswapd.DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[1].(*testframework.LndNode).ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapd.PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()

		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     peerswapd.DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[1].(*testframework.LndNode).ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*testframework.CLightningNode).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapd.PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		csvClaimTest(t, params)
	})
}

func Test_LndCln_Bitcoin_SwapOut(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_LND)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     peerswapd.DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].(*testframework.LndNode).ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapd.DaemonProcess,
			makerPeerswap:    lightningds[1].(*testframework.CLightningNode).DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapd.PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_LND)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     peerswapd.DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].(*testframework.LndNode).ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapd.DaemonProcess,
			makerPeerswap:    lightningds[1].(*testframework.CLightningNode).DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapd.PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(9)), FUNDER_LND)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     peerswapd.DaemonProcess,
						lines: defaultLines,
					},
				)
			}
		}()

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

		lcid, err := lightningds[0].(*testframework.LndNode).ChanIdFromScid(scid)
		if err != nil {
			t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
		}

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    peerswapd.DaemonProcess,
			makerPeerswap:    lightningds[1].(*testframework.CLightningNode).DaemonProcess,
			chainRpc:         bitcoind.RpcProxy,
			chaind:           bitcoind,
			confirms:         BitcoinConfirms,
			csv:              BitcoinCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "btc"

		// Do swap.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			peerswapd.PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
				ChannelId:  lcid,
				SwapAmount: params.swapAmt,
				Asset:      asset,
			})
		}()
		csvClaimTest(t, params)
	})
}
