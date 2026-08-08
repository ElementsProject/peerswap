package test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/elementsproject/glightning/glightning"
	"github.com/elementsproject/peerswap/clightning"
	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/elementsproject/peerswap/swap"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/lightningnetwork/lnd/lnrpc"
)

// drainClnToUnconfirmed spends all of the node's confirmed onchain coins
// back to itself without confirming the transaction. The wallet balance
// still counts the unconfirmed change while coin selection for the opening
// transaction (minconf=1) can not spend it.
func drainClnToUnconfirmed(t *testing.T, node *testframework.CLightningNode) {
	t.Helper()

	addr, err := node.Rpc.NewAddr()
	requireNoError(t, err)

	_, err = node.Rpc.Withdraw(addr, glightning.AllSats(), nil, nil)
	requireNoError(t, err)
}

// drainLndToUnconfirmed spends all of the node's confirmed onchain coins
// back to itself without confirming the transaction. The wallet balance
// still counts the unconfirmed change while FundPsbt (min_confs=1) can not
// spend it.
func drainLndToUnconfirmed(t *testing.T, node *testframework.LndNode) {
	t.Helper()

	ctx := context.Background()
	addr, err := node.Rpc.NewAddress(ctx, &lnrpc.NewAddressRequest{})
	requireNoError(t, err)

	_, err = node.Rpc.SendCoins(ctx, &lnrpc.SendCoinsRequest{
		Addr:    addr.Address,
		SendAll: true,
	})
	requireNoError(t, err)
}

// assertNoPrepayment checks that the taker never paid the fee invoice
// (prepayment) for the canceled swap.
func assertNoPrepayment(t *testing.T, takerPeerswap *testframework.DaemonProcess) {
	t.Helper()

	hasLog, err := takerPeerswap.HasLog("Paid Feeinvoice of")
	requireNoError(t, err)
	newAssertions(t).False(hasLog, "taker should not have paid the fee invoice")

	hasLog, err = takerPeerswap.HasLog("Warning: Paid swap-out prepayment")
	requireNoError(t, err)
	newAssertions(t).False(hasLog, "taker should not have lost a prepayment")
}

// Test_ClnCln_SwapOutPrecheck checks that a swap out is canceled before the
// taker pays the fee invoice when the maker can not fund the opening
// transaction, even though its flat wallet balance is sufficient (issue
// #324). The maker's coins are made unspendable by draining them into an
// unconfirmed change output.
func Test_ClnCln_SwapOutPrecheck(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	bitcoind, lightningds, scid := clnclnSetup(t, uint64(math.Pow10(9)))
	DumpOnFailure(t, WithBitcoin(bitcoind), WithCLightnings(lightningds))

	params := clnParams(t, bitcoind, lightningds, scid, swap.SWAPTYPE_OUT)

	drainClnToUnconfirmed(t, lightningds[1])

	var response map[string]interface{}
	err := lightningds[0].Rpc.Request(&clightning.SwapOut{
		SatAmt:              params.swapAmt,
		ShortChannelId:      params.scid,
		Asset:               "btc",
		PremiumLimitRatePPM: params.premiumLimitRatePPM,
	}, &response)
	assertError(t, err)

	requireNoError(t, params.makerPeerswap.WaitForLog(
		"Swap canceled. Reason: opening transaction precheck failed",
		testframework.TIMEOUT))
	requireNoError(t, params.takerPeerswap.WaitForLog(
		"Swap canceled. Reason: opening transaction precheck failed",
		testframework.TIMEOUT))

	assertNoPrepayment(t, params.takerPeerswap)
}

// Test_LndLnd_SwapOutPrecheck is the lnd variant of
// Test_ClnCln_SwapOutPrecheck.
func Test_LndLnd_SwapOutPrecheck(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	bitcoind, lightningds, peerswapds, scid := lndlndSetup(t, lndFundAmount)
	DumpOnFailure(t, WithBitcoin(bitcoind), WithLnds(lightningds), WithPeerSwapds(peerswapds...))

	params, channelID := lndParams(t, bitcoind, lightningds, peerswapds, scid, swap.SWAPTYPE_OUT)

	drainLndToUnconfirmed(t, lightningds[1])

	_, err := peerswapds[0].PeerswapClient.SwapOut(context.Background(), &peerswaprpc.SwapOutRequest{
		ChannelId:           channelID,
		SwapAmount:          params.swapAmt,
		Asset:               "btc",
		PremiumLimitRatePpm: params.premiumLimitRatePPM,
	})
	assertError(t, err)

	requireNoError(t, params.makerPeerswap.WaitForLog(
		"Swap canceled. Reason: opening transaction precheck failed",
		testframework.TIMEOUT))
	requireNoError(t, params.takerPeerswap.WaitForLog(
		"Swap canceled. Reason: opening transaction precheck failed",
		testframework.TIMEOUT))

	assertNoPrepayment(t, params.takerPeerswap)
}

// Test_ClnCln_ElementsSwapOutPrecheckLockedWallet checks that the original
// Elements wallet-lock failure from issue #324 is detected before the taker
// pays the fee invoice.
func Test_ClnCln_ElementsSwapOutPrecheckLockedWallet(t *testing.T) {
	IsIntegrationTest(t)
	t.Parallel()

	bitcoind, liquidd, lightningds, scid := clnclnElementsSetup(t, uint64(math.Pow10(9)))
	DumpOnFailure(t,
		WithBitcoin(bitcoind),
		WithLiquid(liquidd),
		WithCLightningNodes(lightningds, nil),
	)

	params := clnLiquidParams(t, liquidd, lightningds, scid, swap.SWAPTYPE_OUT)

	// The second node is the swap-out maker and uses the Elements wallet
	// named swap2. Encrypting and explicitly locking it reproduces the
	// signing failure from the original report while leaving its balance
	// visible to the preliminary balance check.
	liquidd.UpdateServiceUrl(fmt.Sprintf(
		"http://127.0.0.1:%d/wallet/swap2",
		liquidd.RpcPort,
	))
	_, err := liquidd.Rpc.Call("encryptwallet", "peerswap-test-passphrase")
	requireNoError(t, err)
	_, err = liquidd.Rpc.Call("walletlock")
	requireNoError(t, err)

	var response map[string]interface{}
	err = lightningds[0].Rpc.Request(&clightning.SwapOut{
		SatAmt:              params.swapAmt,
		ShortChannelId:      params.scid,
		Asset:               "lbtc",
		PremiumLimitRatePPM: params.premiumLimitRatePPM,
	}, &response)
	assertError(t, err)

	requireNoError(t, params.makerPeerswap.WaitForLog(
		"Swap canceled. Reason: opening transaction precheck failed",
		testframework.TIMEOUT))
	requireNoError(t, params.takerPeerswap.WaitForLog(
		"Swap canceled. Reason: opening transaction precheck failed",
		testframework.TIMEOUT))

	assertNoPrepayment(t, params.takerPeerswap)
}
