package test

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/sputn1ck/peerswap/clightning"
	"github.com/sputn1ck/peerswap/peerswaprpc"
	"github.com/sputn1ck/peerswap/testframework"
	"github.com/stretchr/testify/suite"
)

type LndClnSwapsOnBitcoinSuite struct {
	suite.Suite
	assertions *AssertionCounter

	bitcoind    *testframework.BitcoinNode
	peerswapd   *PeerSwapd
	lnd         *testframework.LndNode
	cln         *testframework.CLightningNode
	lightningds []testframework.LightningNode
	scid        string
	lcid        uint64

	channelBalances []uint64
	walletBalances  []uint64
}

// TestLndClnSwapsOnBitcoin runs all integration tests concerning
// bitcoin backend and lnd-cln operation.
func TestLndClnSwapsOnBitcoin(t *testing.T) {
	t.Parallel()
	// Long running tests only run in integration test mode.
	testEnabled := os.Getenv("RUN_INTEGRATION_TESTS")
	if testEnabled == "" {
		t.Skip("set RUN_INTEGRATION_TESTS to run this test")
	}
	suite.Run(t, new(LndClnSwapsOnBitcoinSuite))
}

func (suite *LndClnSwapsOnBitcoinSuite) SetupSuite() {
	t := suite.T()
	suite.assertions = &AssertionCounter{}

	// Settings
	// Inital channel capacity
	var fundAmt = uint64(math.Pow(10, 7))

	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	peerswapdPath := filepath.Join(filename, "..", "..", "out", "peerswapd")
	peerswapPluginPath := filepath.Join(filename, "..", "..", "out", "peerswap")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 1 cln, 1 lnd, 1 peerswapd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	// cln
	cln, err := testframework.NewCLightningNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatalf("could not create cln %v", err)
	}
	t.Cleanup(cln.Kill)

	// Create policy file and accept all peers
	err = os.WriteFile(filepath.Join(cln.GetDataDir(), "..", "policy.conf"), []byte("accept_all_peers=1"), os.ModePerm)
	if err != nil {
		t.Fatal("could not create policy file", err)
	}

	// Use lightningd with dev flags enabled
	cln.WithCmd("lightningd-dev")

	// Add plugin to cmd line options
	cln.AppendCmdLine([]string{
		"--dev-bitcoind-poll=1",
		"--dev-fast-gossip",
		fmt.Sprint("--plugin=", peerswapPluginPath),
		fmt.Sprintf("--peerswap-policy-path=%s", filepath.Join(cln.DataDir, "policy.conf")),
	})

	// lnd
	lnd, err := testframework.NewLndNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatalf("could not create lnd %v", err)
	}
	t.Cleanup(lnd.Kill)

	// peerswapd
	peerswapd, err := NewPeerSwapd(testDir, peerswapdPath, &LndConfig{LndHost: fmt.Sprintf("localhost:%d", lnd.RpcPort), TlsPath: lnd.TlsPath, MacaroonPath: lnd.MacaroonPath}, nil, 1)
	if err != nil {
		t.Fatalf("could not create peerswapd %v", err)
	}
	t.Cleanup(peerswapd.Kill)

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	err = cln.Run(true, true)
	if err != nil {
		t.Fatalf("cln.Run() got err %v", err)
	}

	err = lnd.Run(true, true)
	if err != nil {
		t.Fatalf("lnd.Run() got err %v", err)
	}

	err = peerswapd.Run(true)
	if err != nil {
		t.Fatalf("peerswapd.Run() got err %v", err)
	}

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	scid, err := lnd.OpenChannel(cln, fundAmt, true, true, true)
	if err != nil {
		t.Fatalf("lnd.OpenChannel() %v", err)
	}

	lcid, err := lnd.ChanIdFromScid(scid)
	if err != nil {
		t.Fatalf("lnd.ChanIdFromScid() %v", err)
	}

	// Give btc to node [1] in order to initiate swap-in.
	_, err = cln.FundWallet(10*fundAmt, true)
	if err != nil {
		t.Fatalf("cln.FundWallet() %v", err)
	}

	// Sync peer polling
	_, err = peerswapd.PeerswapClient.ReloadPolicyFile(context.Background(), &peerswaprpc.ReloadPolicyFileRequest{})
	if err != nil {
		t.Fatalf("ReloadPolicyFile %v", err)
	}
	cln.WaitForLog(fmt.Sprintf("From: %s got msgtype: a463", lnd.Info.IdentityPubkey), testframework.TIMEOUT)

	suite.bitcoind = bitcoind
	suite.lnd = lnd
	suite.cln = cln
	suite.lightningds = []testframework.LightningNode{lnd, cln}
	suite.peerswapd = peerswapd
	suite.scid = scid
	suite.lcid = lcid
}

func (suite *LndClnSwapsOnBitcoinSuite) BeforeTest(suiteName, testName string) {
	// make shure we dont have pending balances on lnd.
	err := testframework.WaitForWithErr(func() (bool, error) {
		hasPending, err := suite.lnd.HasPendingHtlcOnChannel(suite.scid)
		return !hasPending, err
	}, testframework.TIMEOUT)
	suite.Require().NoError(err)

	var channelBalances []uint64
	var walletBalances []uint64
	for _, lightningd := range suite.lightningds {
		b, err := lightningd.GetBtcBalanceSat()
		suite.Require().NoError(err)
		walletBalances = append(walletBalances, b)

		cb, err := lightningd.GetChannelBalanceSat(suite.scid)
		suite.Require().NoError(err)
		channelBalances = append(channelBalances, cb)
	}

	suite.channelBalances = channelBalances
	suite.walletBalances = walletBalances
}

func (suite *LndClnSwapsOnBitcoinSuite) HandleStats(suiteName string, stats *suite.SuiteInformation) {
	if !stats.Passed() {
		filter := os.Getenv("PEERSWAP_TEST_FILTER")
		fmt.Println("============================= FAILURE ==============================")
		fmt.Println()

		fmt.Println("+++++++++++++++++++++++++++++ elementsd +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.bitcoind.DaemonProcess.StdOut.String())
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ bitcoind (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.bitcoind.DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ elementsd +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ clightning 1 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.cln.DaemonProcess.StdOut.Filter(filter))
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ clightning 1 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.cln.DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ clightning 1 +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ lnd 1 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.lnd.DaemonProcess.StdOut.String())
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ lnd 1 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.lnd.DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ lnd 1 +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ peerswapd 1 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.peerswapd.DaemonProcess.StdOut.String())
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ peerswap 1 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.peerswapd.DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ peerswapd 1 +++++++++++++++++++++++++++++")
	}
}

//
// Swap in tests
// =================

// TestSwapIn_ClaimPreimage execute a swap-in with the claim by preimage
// spending branch.
func (suite *LndClnSwapsOnBitcoinSuite) TestSwapIn_ClaimPreimage() {
	var err error

	cln := suite.cln
	lightningds := suite.lightningds
	peerswapd := suite.peerswapd
	chaind := suite.bitcoind
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		cln.Rpc.Request(&clightning.SwapIn{SatAmt: swapAmt, ShortChannelId: scid, Asset: "btc"}, &response)
	}()

	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chaind.Rpc.Call("getrawmempool", true)
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				commitmentFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Confirm opening tx. We need 3 confirmations.
	chaind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	//
	//	STEP 2: Pay invoice
	//

	// Wait for invoice being paid.
	err = cln.DaemonProcess.WaitForLog("Event_OnClaimInvoicePaid on State_SwapInSender_AwaitClaimPayment", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check if swap invoice was payed.
	// Expect: [0] before - swapamt ------ before + swapamt [1]
	expected := float64(beforeChannelBalances[0] - swapAmt)
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, expected, 1., testframework.TIMEOUT) {
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(expected, balance, 1.)
	}
	expected = float64(beforeChannelBalances[1] + swapAmt)
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[1], scid, expected, 1., testframework.TIMEOUT) {
		balance, err := lightningds[1].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(expected, balance, 1.)
	}

	//
	//	STEP 3: Broadcasting claim tx
	//

	// Wait for claim tx being broadcasted. We need 3 confirmations.
	// Get claim fee.
	var claimFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chaind.Rpc.Call("getrawmempool", true)
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				claimFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Confirm claim tx.
	chaind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Wail for claim tx confirmation.
	err = peerswapd.DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_ClaimSwap", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check Wallet balance.
	// Expect:
	// - [0] before - claim_fee + swapamt
	// - [1] before - commitment_fee - swapamt
	expected = float64(beforeWalletBalances[0] - claimFee + swapAmt)
	balance, err := lightningds[0].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(balance), 1., "expected %d, got %d", uint64(expected), balance)

	expected = float64(beforeWalletBalances[1] - commitmentFee - swapAmt)
	balance, err = lightningds[1].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(balance), 1., "expected %d, got %d", uint64(expected), balance)
}

// TestSwapIn_ClaimCsv execute a swap-in where the peer does not pay the
// invoice and the maker claims by csv.
//
// Todo: Is skipped for now because we can not run it in the suite as it
// gets the channel stuck. See
// https://github.com/sputn1ck/peerswap/issues/69. As soon as this is
// fixed, the skip has to be removed.
func (suite *LndClnSwapsOnBitcoinSuite) TestSwapIn_ClaimCsv() {
	suite.T().SkipNow()
}

// TestSwapIn_ClaimCoop execute a swap-in where one node cancels and the
//coop spending branch is used.
func (suite *LndClnSwapsOnBitcoinSuite) TestSwapIn_ClaimCoop() {
	var err error

	cln := suite.cln
	lnd := suite.lnd
	lightningds := suite.lightningds
	peerswapd := suite.peerswapd
	chaind := suite.bitcoind
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 2

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		cln.Rpc.Request(&clightning.SwapIn{SatAmt: swapAmt, ShortChannelId: scid, Asset: "btc"}, &response)
	}()

	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chaind.Rpc.Call("getrawmempool", true)
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				commitmentFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	//
	//	STEP 2: Move balance
	//
	// Move local balance from node [0] to [1] so that
	// [0] does not have enough balance to pay the
	// invoice and cancels the swap.
	moveAmt := (beforeChannelBalances[0] - swapAmt) + 2
	for i := 0; i < 2; i++ {
		var labelBytes = make([]byte, 5)
		_, err = rand.Read(labelBytes)
		suite.Require().NoError(err)
		// We have to split the invoices so that they succeed.
		inv, err := cln.Rpc.Invoice((moveAmt/2)*1000, string(labelBytes), "move-balance")
		suite.Require().NoError(err)

		pstream, err := lnd.Rpc.SendPaymentSync(context.Background(), &lnrpc.SendRequest{PaymentRequest: inv.Bolt11})
		suite.Require().NoError(err)
		suite.Require().Len(pstream.PaymentError, 0)
	}

	// Make shure we have no pending htlcs.
	err = testframework.WaitForWithErr(func() (bool, error) {
		hasPending, err := lnd.HasPendingHtlcOnChannel(suite.scid)
		return !hasPending, err
	}, testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check channel balance [0] is less than the swapAmt.
	var setupFunds uint64
	setupFunds, err = lnd.GetChannelBalanceSat(scid)
	suite.Require().NoError(err)
	suite.Require().True(setupFunds < swapAmt)

	//
	//	STEP 3: Confirm opening tx
	//

	chaind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Check that coop close was sent.
	suite.Require().NoError(peerswapd.WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_SendCoopClose", 10*testframework.TIMEOUT))

	//
	//	STEP 4: Broadcasting coop claim tx
	//

	// Wait for coop claim tx being broadcasted.
	// Get claim fee.
	var claimFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chaind.Rpc.Call("getrawmempool", true)
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				claimFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Confirm coop claim tx.
	chaind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Check swap is done.
	suite.Require().NoError(cln.WaitForLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCoop", testframework.TIMEOUT))

	// Check no invoice was paid.
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(setupFunds), 1., testframework.TIMEOUT) {
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(float64(setupFunds), balance, 1.)
	}

	// Check Wallet balance.
	// Expect:
	// - [0] before
	// - [1] before - commitment_fee - claim_fee
	expected := float64(beforeWalletBalances[0])
	balance, err := lightningds[0].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(balance), 1., "expected %d, got %d", uint64(expected), balance)

	expected = float64(beforeWalletBalances[1] - commitmentFee - claimFee)
	balance, err = lightningds[1].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(balance), 1., "expected %d, got %d", uint64(expected), balance)

	//
	// Step 5: Reset channel
	//

	chs, err := lnd.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	suite.Require().NoError(err)

	var resetBalance int64
	for _, ch := range chs.Channels {
		if ch.ChanId == lcid {
			resetBalance = ch.Capacity / 2
			amt := resetBalance - ch.LocalBalance

			inv, err := lnd.Rpc.AddInvoice(context.Background(), &lnrpc.Invoice{Value: amt, Memo: "shift balance"})
			suite.Require().NoError(err)

			_, err = cln.Rpc.PayBolt(inv.PaymentRequest)
			suite.Require().NoError(err)
		}
	}

	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(resetBalance), 1., testframework.TIMEOUT) {
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(float64(resetBalance), balance, 1.)
	}
}

//
// Swap out tests
// ==================

// TestSwapOut_ClaimPreimage execute a swap-out with the claim by
// preimage spending branch.
func (suite *LndClnSwapsOnBitcoinSuite) TestSwapOut_ClaimPreimage() {
	var err error

	lightningds := suite.lightningds
	peerswapd := suite.peerswapd
	cln := suite.cln
	chaind := suite.bitcoind
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap-in.
	go func() {
		peerswapd.PeerswapClient.SwapOut(context.Background(), &peerswaprpc.SwapOutRequest{
			ChannelId:  lcid,
			SwapAmount: swapAmt,
			Asset:      "btc",
		})
	}()

	//
	// STEP 1: Await fee invoice payment
	//

	// Wait for channel balance to change, this means the invoice was payed.
	for i, d := range lightningds {
		testframework.AssertWaitForBalanceChange(suite.T(), d, scid, beforeChannelBalances[i], testframework.TIMEOUT)
	}

	// Get premium from difference.
	newBalance, err := lightningds[0].GetChannelBalanceSat(scid)
	suite.Require().NoError(err)
	premium := beforeChannelBalances[0] - newBalance

	//
	//	STEP 2: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chaind.Rpc.Call("getrawmempool", true)
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				commitmentFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	//
	//	STEP 3: Pay invoice // Broadcast claim Tx
	//

	// Confirm commitment tx. We need 3 confirmations.
	chaind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Wait for invoice being paid.
	err = cln.DaemonProcess.WaitForLog("Event_OnClaimInvoicePaid on State_SwapOutReceiver_AwaitClaimInvoicePayment", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Wait for claim tx being broadcasted.
	var claimFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chaind.Rpc.Call("getrawmempool", true)
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				claimFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Check if swap Invoice had correct amts.
	// Expect: [0] (before - premium) - swapamt ------ (before + premium) + swapamt [1]
	expected := float64(beforeChannelBalances[0] - premium - swapAmt)
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, expected, 1., testframework.TIMEOUT) {
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(expected, balance, 1., "expected %d, got %d")
	}
	expected = float64(beforeChannelBalances[1] + premium + swapAmt)
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[1], scid, expected, 1., testframework.TIMEOUT) {
		balance, err := lightningds[1].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(expected, balance, 1., "expected %d, got %d")
	}

	// Confirm claim tx.
	chaind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Wail for claim tx confirmation.
	err = peerswapd.DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapOutSender_ClaimSwap", testframework.TIMEOUT)
	suite.Require().NoError(err)

	//
	//	STEP 4: Onchain balance change
	//

	// Check Wallet balance.
	// Expect:
	// - [0] before - claim_fee + swapAmt
	// - [1] before - commitment_fee - swapAmt
	expected = float64(beforeWalletBalances[0] - claimFee + swapAmt)
	balance, err := lightningds[0].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(balance), 1., "expected %d, got %d", uint64(expected), balance)

	expected = float64(beforeWalletBalances[1] - commitmentFee - swapAmt)
	balance, err = lightningds[1].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(balance), 1., "expected %d, got %d", uint64(expected), balance)
}

// TestSwapOut_ClaimCsv execute a swap-in where the peer does not pay the
// invoice and the maker claims by csv.
//
// Todo: Is skipped for now because we can not run it in the suite as it
// gets the channel stuck. See
// https://github.com/sputn1ck/peerswap/issues/69. As soon as this is
// fixed, the skip has to be removed.
func (suite *LndClnSwapsOnBitcoinSuite) TestSwapOut_ClaimCsv() {
	suite.T().SkipNow()
	// Todo: add test!
}

// TestSwapOut_ClaimCoop execute a swap-in where one node cancels and the
// coop spending branch is used.
func (suite *LndClnSwapsOnBitcoinSuite) TestSwapOut_ClaimCoop() {
	var err error

	cln := suite.cln
	lnd := suite.lnd
	lightningds := suite.lightningds
	peerswapd := suite.peerswapd
	chaind := suite.bitcoind
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 2

	// Do swap-in.
	go func() {
		peerswapd.PeerswapClient.SwapOut(context.Background(), &peerswaprpc.SwapOutRequest{
			ChannelId:  lcid,
			SwapAmount: swapAmt,
			Asset:      "btc",
		})
	}()

	//
	// STEP 1: Await fee invoice payment
	//

	// Wait for channel balance to change, this means the invoice was payed.
	for i, d := range lightningds {
		testframework.AssertWaitForBalanceChange(suite.T(), d, scid, beforeChannelBalances[i], testframework.TIMEOUT)
	}

	//
	//	STEP 2: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chaind.Rpc.Call("getrawmempool", true)
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				commitmentFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	//
	//	STEP 3: Move balance
	//
	// Move local balance from node [0] to [1] so that
	// [0] does not have enough balance to pay the
	// invoice and cancels the swap.
	moveAmt := (beforeChannelBalances[0] - swapAmt) + 2
	for i := 0; i < 2; i++ {
		var labelBytes = make([]byte, 5)
		_, err = rand.Read(labelBytes)
		suite.Require().NoError(err)
		// We have to split the invoices so that they succeed.
		inv, err := cln.Rpc.Invoice((moveAmt/2)*1000, string(labelBytes), "move-balance")
		suite.Require().NoError(err)

		pstream, err := lnd.Rpc.SendPaymentSync(context.Background(), &lnrpc.SendRequest{PaymentRequest: inv.Bolt11})
		suite.Require().NoError(err)
		suite.Require().Len(pstream.PaymentError, 0)
	}

	// Make shure we have no pending htlcs.
	err = testframework.WaitForWithErr(func() (bool, error) {
		hasPending, err := lnd.HasPendingHtlcOnChannel(suite.scid)
		return !hasPending, err
	}, testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check channel balance [0] is less than the swapAmt.
	var setupFunds uint64
	setupFunds, err = lnd.GetChannelBalanceSat(scid)
	suite.Require().NoError(err)
	suite.Require().True(setupFunds < swapAmt)

	//
	//	STEP 4: Confirm opening tx
	//

	chaind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Check that coop close was sent.
	suite.Require().NoError(peerswapd.WaitForLog("Event_ActionSucceeded on State_SwapOutSender_SendCoopClose", 10*testframework.TIMEOUT))

	//
	//	STEP 5: Broadcasting coop claim tx
	//

	// Wait for coop claim tx being broadcasted.
	var claimFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := chaind.Rpc.Call("getrawmempool", true)
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				claimFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Confirm coop claim tx.
	chaind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Check swap is done.
	suite.Require().NoError(cln.WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCoop", testframework.TIMEOUT))

	//
	//	STEP 6: Balance change
	//

	// Check that channel balance did not change.
	// Expect: setup funds from above
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(setupFunds), 1., testframework.TIMEOUT) {
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(float64(setupFunds), balance, 1.)
	}

	// Check Wallet balance.
	// Expect:
	// - [0] before
	// - [1] before - commitment_fee - claim_fee
	expected := float64(beforeWalletBalances[0])
	balance, err := lightningds[0].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(balance), 1., "expected %d, got %d", uint64(expected), balance)

	expected = float64(beforeWalletBalances[1] - commitmentFee - claimFee)
	balance, err = lightningds[1].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(balance), 1., "expected %d, got %d", uint64(expected), balance)

	//
	// Step 7: Reset channel
	//

	chs, err := lnd.Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	suite.Require().NoError(err)

	var resetBalance int64
	for _, ch := range chs.Channels {
		if ch.ChanId == lcid {
			resetBalance = ch.Capacity / 2
			amt := resetBalance - ch.LocalBalance

			inv, err := lnd.Rpc.AddInvoice(context.Background(), &lnrpc.Invoice{Value: amt, Memo: "shift balance"})
			suite.Require().NoError(err)

			_, err = cln.Rpc.PayBolt(inv.PaymentRequest)
			suite.Require().NoError(err)
		}
	}

	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(resetBalance), 1., testframework.TIMEOUT) {
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(float64(resetBalance), balance, 1.)
	}
}
