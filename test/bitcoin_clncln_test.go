package test

import (
	"crypto/rand"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sputn1ck/peerswap/clightning"
	"github.com/sputn1ck/peerswap/testframework"
	"github.com/stretchr/testify/suite"
)

type ClnClnSwapsOnBitcoinSuite struct {
	suite.Suite
	assertions *AssertionCounter

	bitcoind    *testframework.BitcoinNode
	lightningds []*testframework.CLightningNode
	scid        string

	channelBalances []uint64
	walletBalances  []uint64
}

// TestClnClnSwapsOnBitcoin runs all integration tests concerning
// bitcoin backend and cln-cln operation.
func TestClnClnSwapsOnBitcoin(t *testing.T) {
	t.Parallel()
	// Long running tests only run in integration test mode.
	testEnabled := os.Getenv("RUN_INTEGRATION_TESTS")
	if testEnabled == "" {
		t.Skip("set RUN_INTEGRATION_TESTS to run this test")
	}
	suite.Run(t, new(ClnClnSwapsOnBitcoinSuite))
}

func (suite *ClnClnSwapsOnBitcoinSuite) SetupSuite() {
	t := suite.T()

	suite.assertions = &AssertionCounter{}

	// Settings
	// Inital channel capacity
	var fundAmt = uint64(math.Pow(10, 7))

	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "out", "peerswap")
	testDir := t.TempDir()

	// Setup nodes (1 bitcoind, 2 lightningd)
	bitcoind, err := testframework.NewBitcoinNode(testDir, 1)
	if err != nil {
		t.Fatalf("could not create bitcoind %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	var lightningds []*testframework.CLightningNode
	for i := 1; i <= 2; i++ {
		lightningd, err := testframework.NewCLightningNode(testDir, bitcoind, i)
		if err != nil {
			t.Fatalf("could not create liquidd %v", err)
		}
		t.Cleanup(lightningd.Kill)

		// Create policy file and accept all peers
		err = os.WriteFile(filepath.Join(lightningd.GetDataDir(), "..", "policy.conf"), []byte("accept_all_peers=1"), os.ModePerm)
		if err != nil {
			t.Fatal("could not create policy file", err)
		}

		// Use lightningd with dev flags enabled
		lightningd.WithCmd("lightningd-dev")

		// Add plugin to cmd line options
		lightningd.AppendCmdLine([]string{
			"--dev-bitcoind-poll=1",
			"--dev-fast-gossip",
			fmt.Sprint("--plugin=", pathToPlugin),
			fmt.Sprintf("--peerswap-policy-path=%s", filepath.Join(lightningd.DataDir, "policy.conf")),
		})

		lightningds = append(lightningds, lightningd)
	}

	// Start nodes
	err = bitcoind.Run(true)
	if err != nil {
		t.Fatalf("bitcoind.Run() got err %v", err)
	}

	for _, lightningd := range lightningds {
		err = lightningd.Run(true, true)
		if err != nil {
			t.Fatalf("lightningd.Run() got err %v", err)
		}
		err = lightningd.WaitForLog("peerswap initialized", testframework.TIMEOUT)
		if err != nil {
			t.Fatalf("lightningd.WaitForLog() got err %v", err)
		}
	}

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1])
	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	// Give btc to node [1] in order to initiate swap-in.
	_, err = lightningds[1].FundWallet(10*fundAmt, true)
	suite.Require().NoError(err)

	// Sync peer polling
	for i := 0; i < 2; i++ {
		// Reload policy to trigger sync
		var result interface{}
		err = lightningds[(i+1)%2].Rpc.Request(&clightning.ReloadPolicyFile{}, &result)
		if err != nil {
			t.Fatalf("ListPeers %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		lightningds[i].WaitForLog(fmt.Sprintf("From: %s got msgtype: a463", lightningds[(i+1)%2].Info.Id), testframework.TIMEOUT)
	}

	suite.bitcoind = bitcoind
	suite.lightningds = lightningds
	suite.scid = scid
}

func (suite *ClnClnSwapsOnBitcoinSuite) BeforeTest(suiteName, testName string) {
	var channelBalances []uint64
	var walletBalances []uint64
	for _, lightningd := range suite.lightningds {
		b, err := lightningd.GetBtcBalanceSat()
		suite.Require().NoError(err)
		walletBalances = append(walletBalances, b)

		f, err := lightningd.Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(f.Channels, 1)
		channelBalances = append(channelBalances, f.Channels[0].ChannelSatoshi)
	}

	suite.channelBalances = channelBalances
	suite.walletBalances = walletBalances
}

func (suite *ClnClnSwapsOnBitcoinSuite) HandleStats(suiteName string, stats *suite.SuiteInformation) {
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
		fmt.Println("+++++++++++++++++++++++++++++ clightning 1 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.lightningds[0].DaemonProcess.StdOut.Filter(filter))
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ clightning 1 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.lightningds[0].DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ clightning 1 +++++++++++++++++++++++++++++")

		fmt.Println()
		fmt.Println("+++++++++++++++++++++++++++++ clightning 2 +++++++++++++++++++++++++++++")
		fmt.Printf("%s", suite.lightningds[1].DaemonProcess.StdOut.Filter(filter))
		if suite.bitcoind.DaemonProcess.StdErr.String() != "" {
			fmt.Println("+++++++++++++++++++++++++++++ clightning 2 (ERR) +++++++++++++++++++++++++++++")
			fmt.Printf("%s", suite.lightningds[1].DaemonProcess.StdErr.String())
		}
		fmt.Println("+++++++++++++++++++++++++++++ clightning 2 +++++++++++++++++++++++++++++")
	}
}

//
// Swap in tests
// =================

// TestSwapIn_ClaimPreimage execute a swap-in with the claim by preimage
// spending branch.
func (suite *ClnClnSwapsOnBitcoinSuite) TestSwapIn_ClaimPreimage() {
	var err error

	lightningds := suite.lightningds
	bitcoind := suite.bitcoind
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: swapAmt, ShortChannelId: scid, Asset: "btc"}, &response)
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
		jsonR, err := bitcoind.Rpc.Call("getrawmempool", true)
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
	bitcoind.GenerateBlocks(3)
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
	err = lightningds[1].DaemonProcess.WaitForLog("Event_OnClaimInvoicePaid on State_SwapInSender_AwaitClaimPayment", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Check if swap invoice was payed.
	// Expect: [0] before - swapamt ------ before + swapamt [1]
	expected := float64(beforeChannelBalances[0] - swapAmt)
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[0].Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(funds.Channels, 1)
		suite.Require().InDelta(expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}
	expected = float64(beforeChannelBalances[1] + swapAmt)
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[1], scid, expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[1].Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(funds.Channels, 1)
		suite.Require().InDelta(expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
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
		jsonR, err := bitcoind.Rpc.Call("getrawmempool", true)
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
	bitcoind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Wail for claim tx confirmation.
	err = lightningds[0].DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_ClaimSwap", testframework.TIMEOUT)
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
func (suite *ClnClnSwapsOnBitcoinSuite) TestSwapIn_ClaimCsv() {
	suite.T().SkipNow()
	var err error

	lightningds := suite.lightningds
	bitcoind := suite.bitcoind
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Expectations.
	confirmationsForCsv := 100
	expectedLightningSat := []uint64{beforeChannelBalances[0], beforeChannelBalances[1]}
	expectedOnchainSat := []uint64{beforeWalletBalances[0], beforeWalletBalances[1] - 1914}

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: swapAmt, ShortChannelId: scid, Asset: "btc"}, &response)
	}()

	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	testframework.WaitFor(func() bool {
		var mempool []string
		jsonR, err := bitcoind.Rpc.Call("getrawmempool")
		suite.Require().NoError(err)

		err = jsonR.GetObject(&mempool)
		suite.Require().NoError(err)

		return len(mempool) == 1
	}, testframework.TIMEOUT)

	//
	// STEP 2: Stop peer, this leads to the maker
	// claiming by csv as the peer does not pay the
	// invoice.
	//

	lightningds[0].Shutdown()

	// Generate one less block than required.
	bitcoind.GenerateBlocks(confirmationsForCsv - 1)
	testframework.WaitFor(func() bool {
		isSynced, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return isSynced
	}, testframework.TIMEOUT)

	// Check that csv is not claimed yet.
	triedToClaim, err := lightningds[1].DaemonProcess.HasLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCsv")
	suite.Require().NoError(err)
	suite.Require().False(triedToClaim)

	// Generate one more block to trigger claim by csv.
	bitcoind.GenerateBlocks(1)
	testframework.WaitFor(func() bool {
		isSynced, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return isSynced
	}, testframework.TIMEOUT)

	// Check that csv gets claimed.
	triedToClaim, err = lightningds[1].DaemonProcess.HasLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCsv")
	suite.Require().NoError(err)
	suite.Require().True(triedToClaim)

	// Check claim tx is broadcasted.
	var mempool []string
	jsonR, err := bitcoind.Rpc.Call("getrawmempool")
	suite.Require().NoError(err)

	err = jsonR.GetObject(&mempool)
	suite.Require().NoError(err)
	suite.Require().Len(mempool, 1)

	// Generate to claim
	bitcoind.GenerateBlocks(3)
	testframework.WaitFor(func() bool {
		isSynced, err := lightningds[1].IsBlockHeightSynced()
		suite.Require().NoError(err)
		return isSynced
	}, testframework.TIMEOUT)

	// Start node again
	err = lightningds[0].Run(true, true)
	suite.Require().NoError(err)

	// Check if channel balance is correct.
	suite.Require().True(testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(expectedLightningSat[0]), 5000., testframework.TIMEOUT))
	suite.Require().True(testframework.AssertWaitForChannelBalance(suite.T(), lightningds[1], scid, float64(expectedLightningSat[1]), 5000., testframework.TIMEOUT))

	// Check Wallet balance.
	balance, err := lightningds[0].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().EqualValuesf(expectedOnchainSat[0], balance, "expected %d, got %d", expectedOnchainSat[0], balance)

	balance, err = lightningds[1].GetBtcBalanceSat()
	suite.Require().NoError(err)
	suite.Require().EqualValuesf(expectedOnchainSat[1], balance, "expected %d, got %d", expectedOnchainSat[1], balance)
}

// TestSwapIn_ClaimCoop execute a swap-in where one node cancels and the
//coop spending branch is used.
func (suite *ClnClnSwapsOnBitcoinSuite) TestSwapIn_ClaimCoop() {
	var err error

	lightningds := suite.lightningds
	bitcoind := suite.bitcoind
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 2

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: swapAmt, ShortChannelId: scid, Asset: "btc"}, &response)
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
		jsonR, err := bitcoind.Rpc.Call("getrawmempool", true)
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
	for i := 0; i < 2; i++ {
		// We have to split the invoices so that they succeed.
		amt := ((beforeChannelBalances[0] - swapAmt) / 2) + 1
		inv, err := lightningds[1].Rpc.Invoice(amt*1000, fmt.Sprintf("test-move-balance-%d", i), "move-balance")
		suite.Require().NoError(err)

		_, err = lightningds[0].Rpc.PayBolt(inv.Bolt11)
		suite.Require().NoError(err)
	}

	// Check if channel balance [0] is less than the swapAmt.
	var setupFunds uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		funds, err := lightningds[0].Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(funds.Channels, 1)
		setupFunds = funds.Channels[0].ChannelSatoshi
		return setupFunds < swapAmt
	}, testframework.TIMEOUT))

	//
	//	STEP 3: Confirm opening tx
	//

	bitcoind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Check that coop close was sent.
	suite.Require().NoError(lightningds[0].WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_SendCoopClose", 10*testframework.TIMEOUT))

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
		jsonR, err := bitcoind.Rpc.Call("getrawmempool", true)
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
	bitcoind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Check swap is done.
	suite.Require().NoError(lightningds[1].WaitForLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCoop", testframework.TIMEOUT))

	// Check no invoice was paid.
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, float64(setupFunds), 1., testframework.TIMEOUT) {
		funds, err := lightningds[0].Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(funds.Channels, 1)
		suite.Require().InDelta(float64(setupFunds), funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
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

	suite.Require().NoError(testframework.BalanceChannel5050(lightningds[0], lightningds[1], scid))
}

//
// Swap out tests
// ==================

// TestSwapOut_ClaimPreimage execute a swap-out with the claim by
// preimage spending branch.
func (suite *ClnClnSwapsOnBitcoinSuite) TestSwapOut_ClaimPreimage() {
	var err error

	lightningds := suite.lightningds
	bitcoind := suite.bitcoind
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: swapAmt, ShortChannelId: scid, Asset: "btc"}, &response)
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
	// Get commitmentFee.
	var commitmentFee uint64

	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := bitcoind.Rpc.Call("getrawmempool", true)
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
	bitcoind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Wait for invoice being paid.
	err = lightningds[1].DaemonProcess.WaitForLog("Event_OnClaimInvoicePaid on State_SwapOutReceiver_AwaitClaimInvoicePayment", testframework.TIMEOUT)
	suite.Require().NoError(err)

	// Wait for claim tx being broadcasted.
	var claimFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := bitcoind.Rpc.Call("getrawmempool", true)
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
	// Expect: [0] (before - commitment_fee) - swapamt ------ (before + commitment_fee) + swapamt [1]
	expected := float64(beforeChannelBalances[0] - premium - swapAmt)
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[0], scid, expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[0].Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(funds.Channels, 1)
		suite.Require().InDelta(expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}
	expected = float64(beforeChannelBalances[1] + premium + swapAmt)
	if !testframework.AssertWaitForChannelBalance(suite.T(), lightningds[1], scid, expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[1].Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(funds.Channels, 1)
		suite.Require().InDelta(expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}

	// Confirm claim tx.
	bitcoind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Wail for claim tx confirmation.
	err = lightningds[0].DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapOutSender_ClaimSwap", testframework.TIMEOUT)
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
func (suite *ClnClnSwapsOnBitcoinSuite) TestSwapOut_ClaimCsv() {
	suite.T().SkipNow()
	// Todo: add test!
}

// TestSwapOut_ClaimCoop execute a swap-in where one node cancels and the
//coop spending branch is used.
func (suite *ClnClnSwapsOnBitcoinSuite) TestSwapOut_ClaimCoop() {
	var err error

	lightningds := suite.lightningds
	bitcoind := suite.bitcoind
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.walletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 2

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: swapAmt, ShortChannelId: scid, Asset: "btc"}, &response)
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

	// Wait for commitment tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := bitcoind.Rpc.Call("getrawmempool", true)
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

	// Move local balance from node [0] to [1] so that
	// [0] does not have enough balance to pay the
	// invoice and cancels the swap.
	moveAmtMSat := ((beforeChannelBalances[0]-swapAmt)/2 + 1) * 1000
	for i := 0; i < 2; i++ {
		var labelBytes = make([]byte, 5)
		_, err = rand.Read(labelBytes)
		suite.Require().NoError(err)
		// We have to split the invoices so that they succeed.
		inv, err := lightningds[1].Rpc.Invoice(moveAmtMSat, string(labelBytes), "move-balance")
		suite.Require().NoError(err)

		_, err = lightningds[0].Rpc.PayBolt(inv.Bolt11)
		suite.Require().NoError(err)
	}

	// Check if channel balance [0] is less than the swapAmt.
	// Get channel state for later reference.
	var setupFunds uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		funds, err := lightningds[0].Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(funds.Channels, 1)
		setupFunds = funds.Channels[0].ChannelSatoshi
		return setupFunds < swapAmt
	}, testframework.TIMEOUT))

	//
	//	STEP 3: Confirm opening tx
	//

	bitcoind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Check that coop close was sent.
	suite.Require().NoError(lightningds[0].WaitForLog("Event_ActionSucceeded on State_SwapOutSender_SendCoopClose", testframework.TIMEOUT))

	//
	//	STEP 4: Broadcasting coop claim tx
	//

	// Wait for coop claim tx being broadcasted.
	var claimFee uint64
	suite.Require().NoError(testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := bitcoind.Rpc.Call("getrawmempool", true)
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
	bitcoind.GenerateBlocks(3)
	for _, lightningd := range lightningds {
		testframework.WaitFor(func() bool {
			ok, err := lightningd.IsBlockHeightSynced()
			suite.Require().NoError(err)
			return ok
		}, testframework.TIMEOUT)
	}

	// Check swap is done.
	suite.Require().NoError(lightningds[1].WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCoop", testframework.TIMEOUT))

	//
	//	STEP 4: Balance change
	//

	// Check that channel balance did not change.
	// Expect: setup funds from above
	funds, err := lightningds[0].Rpc.ListFunds()
	suite.Require().NoError(err)
	suite.Require().Len(funds.Channels, 1)
	suite.Require().InDelta(setupFunds, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")

	// Check on-chain wallet balance.
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

	suite.Require().NoError(testframework.BalanceChannel5050(lightningds[0], lightningds[1], scid))
}
