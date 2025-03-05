package test

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/swap"
	"github.com/stretchr/testify/require"
)

func isBlindedTx(res interface{}) bool {
	m, ok := res.(map[string]interface{})
	if !ok {
		return false
	}
	result, ok := m["decoded"].(map[string]interface{})
	if !ok {
		return false
	}
	vouts, ok := result["vout"].([]interface{})
	if !ok {
		return false
	}
	for _, v := range vouts {
		voMap, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if _, hasVC := voMap["valuecommitment"]; hasVC {
			return true
		}
	}
	return false
}
func Test_ClnCln_Liquid_SwapIn_pegin(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	t.Run("claim_normal", func(t *testing.T) {
		t.Parallel()
		require := require.New(t)

		bitcoind, liquidd, lightningds, scid := clnclnElementsSetupPegin(t, uint64(math.Pow10(9)), 10)
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
		asset := "lbtc"

		// Do swap.
		go func() {
			var response map[string]interface{}
			lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: params.swapAmt, ShortChannelId: params.scid, Asset: asset}, &response)

		}()
		preimageClaimTest(t, params)

		swaps := peerswaprpc.ListSwapsResponse{}
		err := lightningds[0].Rpc.Request(&clightning.ListSwaps{}, &swaps)
		require.NoError(err)
		require.Len(swaps.Swaps, 1)
		res, err := liquidd.Rpc.Call("loadwallet", "swap2")
		_ = res
		require.NoError(err)
		openingTxID := swaps.Swaps[0].GetOpeningTxId()
		liquidd.RpcProxy.UpdateServiceUrl(fmt.Sprintf("http://127.0.0.1:%d/wallet/%s", liquidd.RpcPort, "swap2"))
		openingTxRes, err := liquidd.Rpc.Call("gettransaction", openingTxID, true, true)
		require.NoError(err, "failed to get opening tx rawtransaction")
		require.True(isBlindedTx(openingTxRes.Result), "opening tx must be blinded")
		otx, ok := openingTxRes.Result.(map[string]interface{})
		require.True(ok, "failed to parse")
		t.Logf("opening tx: %s", otx["hex"])

		liquidd.RpcProxy.UpdateServiceUrl(fmt.Sprintf("http://127.0.0.1:%d/wallet/%s", liquidd.RpcPort, "swap1"))
		claimTxID := swaps.Swaps[0].GetClaimTxId()
		claimTxRes, err := liquidd.Rpc.Call("gettransaction", claimTxID, true, true)
		require.NoError(err, "failed to get claim tx rawtransaction")
		require.True(isBlindedTx(claimTxRes.Result), "claim tx must be blinded")
		ctx, ok := claimTxRes.Result.(map[string]interface{})
		require.True(ok, "failed to parse")
		t.Logf("claim tx: %s", ctx["hex"])
	})
}
