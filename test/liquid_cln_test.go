package test

import (
	"math"
	"os"
	"testing"

	"github.com/sputn1ck/peerswap/clightning"
	"github.com/sputn1ck/peerswap/swap"
	"github.com/sputn1ck/peerswap/testframework"
	"github.com/stretchr/testify/require"
)

func Test_ClnCln_Liquid_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(6)))
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].DaemonProcess,
			makerPeerswap:    lightningds[1].DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			var response map[string]interface{}
			lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)

		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(6)))
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].DaemonProcess,
			makerPeerswap:    lightningds[1].DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			var response map[string]interface{}
			lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)

		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(6)))
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].DaemonProcess,
			makerPeerswap:    lightningds[1].DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			var response map[string]interface{}
			lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)

		}()
		csvClaimTest(t, params)
	})
}

func Test_ClnCln_Liquid_SwapOut(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(6)))
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].DaemonProcess,
			makerPeerswap:    lightningds[1].DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(6)))
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].DaemonProcess,
			makerPeerswap:    lightningds[1].DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(6)))
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].DaemonProcess,
			makerPeerswap:    lightningds[1].DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		csvClaimTest(t, params)
	})
}

func Test_ClnLnd_Liquid_SwapIn(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(6)), FUNDER_LND)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].(*LndNodeWithLiquid).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].(*CLightningNodeWithLiquid).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      peerswapd.DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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
			makerPeerswap:    lightningds[1].(*CLightningNodeWithLiquid).DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[1].(*CLightningNodeWithLiquid).Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(6)), FUNDER_LND)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].(*LndNodeWithLiquid).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].(*CLightningNodeWithLiquid).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      peerswapd.DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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
			makerPeerswap:    lightningds[1].(*CLightningNodeWithLiquid).DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[1].(*CLightningNodeWithLiquid).Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(6)), FUNDER_LND)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     lightningds[0].(*LndNodeWithLiquid).DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[1].(*CLightningNodeWithLiquid).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:      peerswapd.DaemonProcess,
						filter: filter,
						lines:  defaultLines,
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
			makerPeerswap:    lightningds[1].(*CLightningNodeWithLiquid).DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_IN,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[1].(*CLightningNodeWithLiquid).Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		csvClaimTest(t, params)
	})

}

func Test_ClnLnd_Liquid_SwapOut(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(6)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*testframework.CLightningNode).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*CLightningNodeWithLiquid).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*CLightningNodeWithLiquid).Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		preimageClaimTest(t, params)
	})
	t.Run("claim_coop", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(6)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*CLightningNodeWithLiquid).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*CLightningNodeWithLiquid).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*CLightningNodeWithLiquid).Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		coopClaimTest(t, params)
	})
	t.Run("claim_csv", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, peerswapd, scid := mixedElementsSetup(t, uint64(math.Pow10(6)), FUNDER_CLN)
		defer func() {
			if t.Failed() {
				filter := os.Getenv("PEERSWAP_TEST_FILTER")
				pprintFail(
					tailableProcess{
						p:     bitcoind.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:     liquidd.DaemonProcess,
						lines: defaultLines,
					},
					tailableProcess{
						p:      lightningds[0].(*CLightningNodeWithLiquid).DaemonProcess,
						filter: filter,
						lines:  defaultLines,
					},
					tailableProcess{
						p:     lightningds[1].(*testframework.LndNode).DaemonProcess,
						lines: defaultLines,
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

		params := &testParams{
			swapAmt:          channelBalances[0] / 2,
			scid:             scid,
			origTakerWallet:  walletBalances[0],
			origMakerWallet:  walletBalances[1],
			origTakerBalance: channelBalances[0],
			origMakerBalance: channelBalances[1],
			takerNode:        lightningds[0],
			makerNode:        lightningds[1],
			takerPeerswap:    lightningds[0].(*CLightningNodeWithLiquid).DaemonProcess,
			makerPeerswap:    peerswapd.DaemonProcess,
			chainRpc:         liquidd.RpcProxy,
			chaind:           liquidd,
			confirms:         LiquidConfirms,
			csv:              LiquidCsv,
			swapType:         swap.SWAPTYPE_OUT,
		}
		asset := "l-btc"

		// Do swap.
		go func() {
			// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
			var response map[string]interface{}
			lightningds[0].(*CLightningNodeWithLiquid).Rpc.Request(&clightning.SwapOut{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)
		}()
		csvClaimTest(t, params)
	})
}
