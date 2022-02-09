package test

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/sputn1ck/peerswap/peerswaprpc"
	"github.com/sputn1ck/peerswap/testframework"
	"github.com/stretchr/testify/suite"
)

type LndLndSwapsOnLiquidSuite struct {
	suite.Suite
	assertions *AssertionCounter

	bitcoind    *testframework.BitcoinNode
	liquidd     *testframework.LiquidNode
	lightningds []*testframework.LndNode
	peerswapds  []*PeerSwapd
	scid        string
	lcid        uint64

	channelBalances []uint64
	walletBalances  []uint64
}

// TestLndLndSwapsOnLiquid runs all integration tests concerning
// liquid liquid and lnd-lnd operation.
func TestLndLndSwapsOnLiquid(t *testing.T) {
	t.Parallel()
	// Long running tests only run in integration test mode.
	testEnabled := os.Getenv("RUN_INTEGRATION_TESTS")
	if testEnabled == "" {
		t.Skip("set RUN_INTEGRATION_TESTS to run this test")
	}
	suite.Run(t, new(LndLndSwapsOnLiquidSuite))
}

func (suite *LndLndSwapsOnLiquidSuite) SetupSuite() {
	t := suite.T()
	suite.assertions = &AssertionCounter{}

	// Settings
	// Inital channel capacity
	var fundAmt = uint64(math.Pow(10, 7))

	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "peerswapd")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 1 liquidd, 2 lightningd, 2 peerswapd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	liquidd, err := testframework.NewLiquidNode(testDir, bitcoind, 1)
	if err != nil {
		t.Fatal("error creating liquidd node", err)
	}
	t.Cleanup(liquidd.Kill)

	var lightningds []*testframework.LndNode
	for i := 1; i <= 2; i++ {
		lightningd, err := testframework.NewLndNode(testDir, bitcoind, i)
		if err != nil {
			t.Fatalf("could not create liquidd %v", err)
		}
		t.Cleanup(lightningd.Kill)

		lightningds = append(lightningds, lightningd)
	}

	var peerswapds []*PeerSwapd
	for i, lightningd := range lightningds {
		extraConfig := map[string]string{
			"liquid.rpcuser":   liquidd.RpcUser,
			"liquid.rpcpass":   liquidd.RpcPassword,
			"liquid.rpchost":   "http://127.0.0.1",
			"liquid.rpcport":   fmt.Sprintf("%d", liquidd.RpcPort),
			"liquid.rpcwallet": fmt.Sprintf("swap-test-wallet-%d", i),
		}

		peerswapd, err := NewPeerSwapd(testDir, pathToPlugin, &LndConfig{LndHost: fmt.Sprintf("localhost:%d", lightningd.RpcPort), TlsPath: lightningd.TlsPath, MacaroonPath: lightningd.MacaroonPath}, extraConfig, i+1)
		if err != nil {
			t.Fatalf("could not create peerswapd %v", err)
		}
		t.Cleanup(peerswapd.Kill)

		// Create policy file and accept all peers
		err = os.WriteFile(filepath.Join(peerswapd.DataDir, "..", "policy.conf"), []byte("accept_all_peers=1"), os.ModePerm)
		if err != nil {
			t.Fatal("could not create policy file", err)
		}

		peerswapds = append(peerswapds, peerswapd)
	}

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	err = liquidd.Run(true)
	if err != nil {
		t.Fatalf("Run() got err %v", err)
	}

	for _, lightningd := range lightningds {
		err = lightningd.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err %v", err)
		}
	}

	for _, peerswapd := range peerswapds {
		err = peerswapd.Run(true)
		if err != nil {
			t.Fatalf("peerswapd.Run() got err %v", err)
		}
	}

	// Give liquid funds to nodes to have something to swap.
	for _, peerswapd := range peerswapds {
		r, err := peerswapd.PeerswapClient.LiquidGetAddress(context.Background(), &peerswaprpc.GetAddressRequest{})
		suite.Require().NoError(err)

		_, err = liquidd.Rpc.Call("sendtoaddress", r.Address, 1., "", "", false, false, 1, "UNSET")
		suite.Require().NoError(err)
	}

	// Lock txs.
	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	suite.Require().NoError(err)

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	lcid, err := lightningds[0].ChanIdFromScid(scid)
	if err != nil {
		t.Fatalf("lightingds[0].ChanIdFromScid() %v", err)
	}

	// Give btc to node [1] in order to initiate swap-in.
	_, err = lightningds[1].FundWallet(10*fundAmt, true)
	if err != nil {
		t.Fatalf("lightningds[1].FundWallet() %v", err)
	}

	suite.bitcoind = bitcoind
	suite.lightningds = lightningds
	suite.liquidd = liquidd
	suite.peerswapds = peerswapds
	suite.scid = scid
	suite.lcid = lcid
}

func (suite *LndLndSwapsOnLiquidSuite) BeforeTest(suiteName, testName string) {
	// make shure we dont have pending balances
	var err error
	for _, lightningd := range suite.lightningds {
		err = testframework.WaitForWithErr(func() (bool, error) {
			hasPending, err := lightningd.HasPendingHtlcOnChannel(suite.scid)
			return !hasPending, err
		}, testframework.TIMEOUT)
	}
	suite.Require().NoError(err)

	var channelBalances []uint64
	for _, lightningd := range suite.lightningds {
		cb, err := lightningd.GetChannelBalanceSat(suite.scid)
		suite.Require().NoError(err)
		channelBalances = append(channelBalances, cb)
	}

	var liquidWalletBalances []uint64
	for _, peerswapd := range suite.peerswapds {
		r, err := peerswapd.PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
		suite.Require().NoError(err)
		liquidWalletBalances = append(liquidWalletBalances, r.SatAmount)
	}

	suite.channelBalances = channelBalances
	suite.walletBalances = liquidWalletBalances
}

func (suite *LndLndSwapsOnLiquidSuite) HandleStats(suiteName string, stats *suite.SuiteInformation) {
	if !stats.Passed() {
		filter := os.Getenv("PEERSWAP_TEST_FILTER")
		fmt.Println("============================= FAILURE ==============================")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ bitcoind +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.bitcoind.DaemonProcess.StdOut.String())
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ bitcoind (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.bitcoind.DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ bitcoind +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ liquidd +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.liquidd.DaemonProcess.StdOut.String())
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ liquidd (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.liquidd.DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ liquidd +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ lnd 1 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.lightningds[0].DaemonProcess.StdOut.Filter(filter))
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ lnd 1 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.lightningds[0].DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ lnd 1 +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ lnd 2 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.lightningds[1].DaemonProcess.StdOut.Filter(filter))
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ lnd 2 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.lightningds[1].DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ lnd 2 +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ peerswapd 1 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.peerswapds[0].DaemonProcess.StdOut.Filter(filter))
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ peerswapd 1 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.peerswapds[0].DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ peerswapd 1 +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ peerswapd 2 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.peerswapds[1].DaemonProcess.StdOut.Filter(filter))
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ peerswapd 2 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.peerswapds[1].DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ peerswapd 2 +++++++++++++++++++++++++++++")
	}
}

//
// Swap in tests
// =================

// TestSwapIn_ClaimPreimage execute a swap-in with the claim by preimage
// spending branch.
func (suite *LndLndSwapsOnLiquidSuite) TestSwapIn_ClaimPreimage() {
	var err error

	lightningds := suite.lightningds
	peerswapds := suite.peerswapds
	chaind := suite.liquidd
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap-in.
	go func() {
		peerswapds[1].PeerswapClient.SwapIn(context.Background(), &peerswaprpc.SwapInRequest{
			ChannelId:  lcid,
			SwapAmount: swapAmt,
			Asset:      "l-btc",
		})
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

	// Confirm opening tx. We need 2 confirmations.
	chaind.GenerateBlocks(2)

	//
	//	STEP 2: Pay invoice
	//

	// Wait for invoice being paid.
	err = peerswapds[1].DaemonProcess.WaitForLog("Event_OnClaimInvoicePaid on State_SwapInSender_AwaitClaimPayment", testframework.TIMEOUT)
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

	// Wait for claim tx confirmation.
	err = peerswapds[0].DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_ClaimSwap", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check Wallet balance.
	// Expect:
	// - [0] before - claim_fee + swapamt
	// - [1] before - commitment_fee - swapamt
	expected = float64(beforeWalletBalances[0] - claimFee + swapAmt)
	br, err := peerswapds[0].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(br.SatAmount), 1., "expected %d, got %d", uint64(expected), br.SatAmount)

	expected = float64(beforeWalletBalances[1] - commitmentFee - swapAmt)
	br, err = peerswapds[1].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(br.SatAmount), 1., "expected %d, got %d", uint64(expected), br.SatAmount)
}

// TestSwapIn_ClaimCsv execute a swap-in where the peer does not pay the
// invoice and the maker claims by csv.
func (suite *LndLndSwapsOnLiquidSuite) TestSwapIn_ClaimCsv() {
	var err error

	lightningds := suite.lightningds
	peerswapds := suite.peerswapds
	chaind := suite.liquidd
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap-in.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		peerswapds[1].PeerswapClient.SwapIn(ctx, &peerswaprpc.SwapInRequest{
			ChannelId:  lcid,
			SwapAmount: swapAmt,
			Asset:      "l-btc",
		})
	}()

	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitFee.
	var commitFee uint64
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
				commitFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	//
	// STEP 2: Stop peer, this leads to the maker
	// claiming by csv as the peer does not pay the
	// invoice.
	//

	peerswapds[0].Stop()

	// Generate one less block than required.
	chaind.GenerateBlocks(int(LiquidCsv) - 1)
	testframework.WaitFor(func() bool {
		ok, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return ok
	}, testframework.TIMEOUT)

	// Check that csv is not claimed yet.
	triedToClaim, err := peerswapds[1].DaemonProcess.HasLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCsv")
	suite.Require().NoError(err)
	suite.Require().False(triedToClaim)

	// Generate one more block to trigger claim by csv.
	chaind.GenerateBlocks(1)
	testframework.WaitFor(func() bool {
		isSynced, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return isSynced
	}, testframework.TIMEOUT)

	// Check that csv gets claimed.
	err = peerswapds[1].DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCsv", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check claim tx is broadcasted.
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

	// Generate to claim
	chaind.GenerateBlocks(LiquidConfirms)
	testframework.WaitFor(func() bool {
		isSynced, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return isSynced
	}, testframework.TIMEOUT)

	// Start node again
	err = peerswapds[0].Run(true)
	suite.Require().NoError(err)

	// Wait for node to coop cancel swap
	err = peerswapds[0].WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_SendCoopClose", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check if channel balance is correct.
	suite.Require().True(testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(beforeChannelBalances[0]), 1., testframework.TIMEOUT))
	suite.Require().True(testframework.AssertWaitForChannelBalance(suite.T(), lightningds[1], scid, float64(beforeChannelBalances[1]), 1., testframework.TIMEOUT))

	// Check Wallet balance.
	br, err := peerswapds[0].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().EqualValuesf(beforeWalletBalances[0], br.SatAmount, "expected %d, got %d", beforeWalletBalances[0], br.SatAmount)

	br, err = peerswapds[1].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	// fixme: This should also hold for equal values but diverges with 1. sat.
	// suite.Require().EqualValuesf(beforeWalletBalances[1]-commitFee-claimFee, response.LiquidBalance, "expected %d, got %d", beforeWalletBalances[1]-commitFee-claimFee, response.LiquidBalance)
	suite.Require().InDelta(beforeWalletBalances[1]-commitFee-claimFee, br.SatAmount, 1.)
}

// TestSwapIn_ClaimCoop execute a swap-in where one node cancels and the
//coop spending branch is used.
func (suite *LndLndSwapsOnLiquidSuite) TestSwapIn_ClaimCoop() {
	var err error

	lightningds := suite.lightningds
	peerswapds := suite.peerswapds
	chaind := suite.liquidd
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 2

	// Do swap-in.
	go func() {
		peerswapds[1].PeerswapClient.SwapIn(context.Background(), &peerswaprpc.SwapInRequest{
			ChannelId:  lcid,
			SwapAmount: swapAmt,
			Asset:      "l-btc",
		})
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
		// We have to split the invoices so that they succeed.

		inv, err := lightningds[1].Rpc.AddInvoice(context.Background(), &lnrpc.Invoice{Value: int64(moveAmt / 2), Memo: "shift balance"})
		suite.Require().NoError(err)

		pstream, err := lightningds[0].Rpc.SendPaymentSync(context.Background(), &lnrpc.SendRequest{PaymentRequest: inv.PaymentRequest})
		suite.Require().NoError(err)
		suite.Require().Len(pstream.PaymentError, 0)
	}

	// Make shure we have no pending htlcs.
	for _, lightningd := range suite.lightningds {
		err := testframework.WaitForWithErr(func() (bool, error) {
			hasPending, err := lightningd.HasPendingHtlcOnChannel(suite.scid)
			return !hasPending, err
		}, testframework.TIMEOUT)
		suite.Require().NoError(err)
	}

	// Check channel balance [0] is less than the swapAmt.
	var setupFunds uint64
	setupFunds, err = lightningds[0].GetChannelBalanceSat(scid)
	suite.Require().NoError(err)
	suite.Require().True(setupFunds < swapAmt)

	//
	//	STEP 3: Confirm opening tx
	//

	chaind.GenerateBlocks(2)

	// Check that coop close was sent.
	suite.Require().NoError(peerswapds[0].WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_SendCoopClose", 10*testframework.TIMEOUT))

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
	chaind.GenerateBlocks(2)

	// Check swap is done.
	suite.Require().NoError(peerswapds[1].WaitForLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCoop", testframework.TIMEOUT))

	// Check no invoice was paid.
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(setupFunds), 1., testframework.TIMEOUT) {
		pays, err := lightningds[0].Rpc.ListPayments(context.Background(), &lnrpc.ListPaymentsRequest{})
		suite.Require().NoError(err)
		for i, p := range pays.Payments {
			suite.T().Log("PAYMENT NO ", i, p)
		}
		suite.T().Log("SWAP AMT", swapAmt)
		suite.T().Log("SETUP FUNDS", setupFunds)
		suite.T().Log("BEFORE FUNDS", beforeChannelBalances[0])
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(float64(setupFunds), balance, 1.)
	}

	// Check Wallet balance.
	// Expect:
	// - [0] before
	// - [1] before - commitment_fee - claim_fee
	expected := float64(beforeWalletBalances[0])
	br, err := peerswapds[0].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(br.SatAmount), 1., "expected %d, got %d", uint64(expected), br.SatAmount)

	expected = float64(beforeWalletBalances[1] - commitmentFee - claimFee)
	br, err = peerswapds[1].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(br.SatAmount), 1., "expected %d, got %d", uint64(expected), br.SatAmount)

	//
	// Step 5: Reset channel
	//

	chs, err := lightningds[0].Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	suite.Require().NoError(err)

	var resetBalance int64
	for _, ch := range chs.Channels {
		if ch.ChanId == lcid {
			resetBalance = ch.Capacity / 2
			amt := resetBalance - ch.LocalBalance

			inv, err := lightningds[0].Rpc.AddInvoice(context.Background(), &lnrpc.Invoice{Value: amt, Memo: "shift balance"})
			suite.Require().NoError(err)

			_, err = lightningds[1].RpcV2.SendPaymentV2(context.Background(), &routerrpc.SendPaymentRequest{PaymentRequest: inv.PaymentRequest, TimeoutSeconds: int32(testframework.TIMEOUT.Seconds())})
			suite.Require().NoError(err)
		}
	}

	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(resetBalance), 1., testframework.TIMEOUT) {
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(float64(resetBalance), balance, 1.)
	}
}

// //
// // Swap out tests
// // ==================

// TestSwapOut_ClaimPreimage execute a swap-out with the claim by
// preimage spending branch.
func (suite *LndLndSwapsOnLiquidSuite) TestSwapOut_ClaimPreimage() {
	lightningds := suite.lightningds
	peerswapds := suite.peerswapds
	chaind := suite.liquidd
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap-in.
	go func() {
		peerswapds[0].PeerswapClient.SwapOut(context.Background(), &peerswaprpc.SwapOutRequest{
			ChannelId:  lcid,
			SwapAmount: swapAmt,
			Asset:      "l-btc",
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
	chaind.GenerateBlocks(2)

	// Wait for invoice being paid.
	err = peerswapds[1].DaemonProcess.WaitForLog("Event_OnClaimInvoicePaid on State_SwapOutReceiver_AwaitClaimInvoicePayment", testframework.TIMEOUT)
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
	chaind.GenerateBlocks(2)

	// Wail for claim tx confirmation.
	err = peerswapds[0].DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapOutSender_ClaimSwap", testframework.TIMEOUT)
	suite.Require().NoError(err)

	//
	//	STEP 4: Onchain balance change
	//

	// Check Wallet balance.
	// Expect:
	// - [0] before - claim_fee + swapAmt
	// - [1] before - commitment_fee - swapAmt
	expected = float64(beforeWalletBalances[0] - claimFee + swapAmt)
	br, err := peerswapds[0].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(br.SatAmount), 1., "expected %d, got %d", uint64(expected), br.SatAmount)

	expected = float64(beforeWalletBalances[1] - commitmentFee - swapAmt)
	br, err = peerswapds[1].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(br.SatAmount), 1., "expected %d, got %d", uint64(expected), br.SatAmount)
}

// TestSwapOut_ClaimCsv execute a swap-in where the peer does not pay the
// invoice and the maker claims by csv.
func (suite *LndLndSwapsOnLiquidSuite) TestSwapOut_ClaimCsv() {
	lightningds := suite.lightningds
	peerswapds := suite.peerswapds
	chaind := suite.liquidd
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		peerswapds[0].PeerswapClient.SwapOut(ctx, &peerswaprpc.SwapOutRequest{
			ChannelId:  lcid,
			SwapAmount: swapAmt,
			Asset:      "l-btc",
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
	//	STEP 2: Broadcasting commitment tx
	//

	// Wait for commitment tx being broadcasted.
	// Get commitFee.
	var commitFee uint64
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
				commitFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	//
	// STEP 3: Stop peer, this leads to the maker
	// claiming by csv as the peer does not pay the
	// invoice.
	//

	peerswapds[0].Stop()

	// Generate one less block than required.
	chaind.GenerateBlocks(int(LiquidCsv) - 1)
	testframework.WaitFor(func() bool {
		ok, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return ok
	}, testframework.TIMEOUT)

	// Check that csv is not claimed yet.
	triedToClaim, err := peerswapds[1].DaemonProcess.HasLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv")
	suite.Require().NoError(err)
	suite.Require().False(triedToClaim)

	// Generate one more block to trigger claim by csv.
	chaind.GenerateBlocks(1)
	testframework.WaitFor(func() bool {
		isSynced, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return isSynced
	}, testframework.TIMEOUT)

	// Check that csv gets claimed.
	err = peerswapds[1].DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check claim tx is broadcasted.
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

	// Generate to claim
	chaind.GenerateBlocks(LiquidConfirms)
	testframework.WaitFor(func() bool {
		isSynced, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return isSynced
	}, testframework.TIMEOUT)

	// Start node again
	err = peerswapds[0].Run(true)
	suite.Require().NoError(err)

	// Wait for node to coop cancel swap
	err = peerswapds[0].WaitForLog("Event_ActionSucceeded on State_SwapOutSender_SendCoopClose", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check if channel balance is correct.
	suite.Require().True(testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(beforeChannelBalances[0]-premium), 1., testframework.TIMEOUT))
	suite.Require().True(testframework.AssertWaitForChannelBalance(suite.T(), lightningds[1], scid, float64(beforeChannelBalances[1]+premium), 1., testframework.TIMEOUT))

	// Check Wallet balance.
	br, err := peerswapds[0].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().EqualValuesf(beforeWalletBalances[0], br.SatAmount, "expected %d, got %d", beforeWalletBalances[0], br.SatAmount)

	br, err = peerswapds[1].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	// fixme: This should also hold for equal values but diverges with 1. sat.
	// suite.Require().EqualValuesf(beforeWalletBalances[1]-commitFee-claimFee, response.LiquidBalance, "expected %d, got %d", beforeWalletBalances[1]-commitFee-claimFee, response.LiquidBalance)
	suite.Require().InDelta(beforeWalletBalances[1]-commitFee-claimFee, br.SatAmount, 1.)
}

// TestSwapOut_ClaimCoop execute a swap-in where one node cancels and the
// coop spending branch is used.
func (suite *LndLndSwapsOnLiquidSuite) TestSwapOut_ClaimCoop() {
	var err error

	lightningds := suite.lightningds
	peerswapds := suite.peerswapds
	chaind := suite.liquidd
	scid := suite.scid
	lcid := suite.lcid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 2

	// Do swap-out.
	go func() {
		peerswapds[0].PeerswapClient.SwapOut(context.Background(), &peerswaprpc.SwapOutRequest{
			ChannelId:  lcid,
			SwapAmount: swapAmt,
			Asset:      "l-btc",
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
		// We have to split the invoices so that they succeed.

		inv, err := lightningds[1].Rpc.AddInvoice(context.Background(), &lnrpc.Invoice{Value: int64(moveAmt / 2), Memo: "shift balance"})
		suite.Require().NoError(err)

		pstream, err := lightningds[0].Rpc.SendPaymentSync(context.Background(), &lnrpc.SendRequest{PaymentRequest: inv.PaymentRequest})
		suite.Require().NoError(err)
		suite.Require().Len(pstream.PaymentError, 0)
	}

	// Check channel balance [0] is less than the swapAmt.
	var setupFunds uint64
	// Make shure we have no pending htlcs.
	for _, lightningd := range suite.lightningds {
		err := testframework.WaitForWithErr(func() (bool, error) {
			hasPending, err := lightningd.HasPendingHtlcOnChannel(suite.scid)
			return !hasPending, err
		}, testframework.TIMEOUT)
		suite.Require().NoError(err)
	}
	setupFunds, err = lightningds[0].GetChannelBalanceSat(scid)
	suite.Require().NoError(err)
	suite.Require().True(setupFunds < swapAmt)

	//
	//	STEP 4: Confirm opening tx
	//

	chaind.GenerateBlocks(2)

	// Check that coop close was sent.
	suite.Require().NoError(peerswapds[0].WaitForLog("Event_ActionSucceeded on State_SwapOutSender_SendCoopClose", 10*testframework.TIMEOUT))

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
	chaind.GenerateBlocks(2)

	// Check swap is done.
	suite.Require().NoError(peerswapds[1].WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCoop", testframework.TIMEOUT))

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
	br, err := peerswapds[0].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(br.SatAmount), 1., "expected %d, got %d", uint64(expected), br.SatAmount)

	expected = float64(beforeWalletBalances[1] - commitmentFee - claimFee)
	br, err = peerswapds[1].PeerswapClient.LiquidGetBalance(context.Background(), &peerswaprpc.GetBalanceRequest{})
	suite.Require().NoError(err)
	suite.Require().InDelta(expected, float64(br.SatAmount), 1., "expected %d, got %d", uint64(expected), br.SatAmount)

	//
	// Step 7: Reset channel
	//

	chs, err := lightningds[0].Rpc.ListChannels(context.Background(), &lnrpc.ListChannelsRequest{})
	suite.Require().NoError(err)

	var resetBalance int64
	for _, ch := range chs.Channels {
		if ch.ChanId == lcid {
			resetBalance = ch.Capacity / 2
			amt := resetBalance - ch.LocalBalance

			inv, err := lightningds[0].Rpc.AddInvoice(context.Background(), &lnrpc.Invoice{Value: amt, Memo: "shift balance"})
			suite.Require().NoError(err)

			_, err = lightningds[1].RpcV2.SendPaymentV2(context.Background(), &routerrpc.SendPaymentRequest{PaymentRequest: inv.PaymentRequest, TimeoutSeconds: int32(testframework.TIMEOUT.Seconds())})
			suite.Require().NoError(err)
		}
	}

	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(resetBalance), 1., testframework.TIMEOUT) {
		balance, err := lightningds[0].GetChannelBalanceSat(scid)
		suite.Require().NoError(err)
		suite.Require().InDelta(float64(resetBalance), balance, 1.)
	}
}
