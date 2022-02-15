package test

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/sputn1ck/peerswap/peerswaprpc"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/testframework"
	"github.com/stretchr/testify/require"
)

func Test_LndLnd_Bitcoin_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(6)))
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

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(6)))
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

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(6)))
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

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(6)))
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

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(6)))
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

		bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, uint64(math.Pow10(6)))
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

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(6)), FUNDER_CLN)
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

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(6)), FUNDER_CLN)
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

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(6)), FUNDER_CLN)
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

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(6)), FUNDER_LND)
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

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(6)), FUNDER_LND)
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

		bitcoind, lightningds, peerswapd, scid := mixedSetup(t, uint64(math.Pow10(6)), FUNDER_LND)
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
