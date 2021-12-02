package liquidtest

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sputn1ck/peerswap/clightning"
	"github.com/sputn1ck/peerswap/test"
	"github.com/sputn1ck/peerswap/testframework"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LiquidTestSuite struct {
	suite.Suite
	assertions *test.AssertionCounter

	bitcoind    *testframework.BitcoinNode
	liquidd     *testframework.LiquidNode
	lightningds []*testframework.CLightningNode
	scid        string

	channelBalances      []uint64
	btcWalletBalances    []uint64
	liquidWalletBalances []uint64

	liquidWalletNames []string
}

func (suite *LiquidTestSuite) SetupSuite() {
	t := suite.T()

	suite.assertions = &test.AssertionCounter{}

	// Settings
	// Inital channel capacity
	var fundAmt = uint64(math.Pow(10, 7))

	// Get PeerSwap plugin path and test dir
	_, filename, _, _ := runtime.Caller(0)
	pathToPlugin := filepath.Join(filename, "..", "..", "..", "peerswap")
	testDir := t.TempDir()

	// Misc setup
	// assertions := &AssertionCounter{}

	// Setup nodes (1 bitcoind, 2 lightningd)
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

	var lightningds []*testframework.CLightningNode
	var liquidWalletNames []string
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

		// Set wallet name
		walletName := fmt.Sprintf("swap%d", i)
		liquidWalletNames = append(liquidWalletNames, walletName)

		// Use lightningd with dev flags enabled
		lightningd.WithCmd("lightningd-dev")

		// Add plugin to cmd line options
		lightningd.AppendCmdLine([]string{
			"--dev-bitcoind-poll=1",
			"--dev-fast-gossip",
			fmt.Sprint("--plugin=", pathToPlugin),
			fmt.Sprintf("--peerswap-policy-path=%s", filepath.Join(lightningd.DataDir, "policy.conf")),
			"--peerswap-liquid-rpchost=http://127.0.0.1",
			fmt.Sprintf("--peerswap-liquid-rpcport=%d", liquidd.RpcPort),
			fmt.Sprintf("--peerswap-liquid-rpcuser=%s", liquidd.RpcUser),
			fmt.Sprintf("--peerswap-liquid-rpcpassword=%s", liquidd.RpcPassword),
			fmt.Sprintf("--peerswap-liquid-network=%s", liquidd.Network),
			fmt.Sprintf("--peerswap-liquid-rpcwallet=%s", walletName),
		})

		lightningds = append(lightningds, lightningd)
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
		err = lightningd.WaitForLog("peerswap initialized", testframework.TIMEOUT)
		if err != nil {
			t.Fatalf("lightningd.WaitForLog() got err %v", err)
		}
	}

	// Give liquid funds to nodes to have something to swap.
	for _, lightningd := range lightningds {
		var result clightning.GetAddressResponse
		lightningd.Rpc.Request(&clightning.GetAddressMethod{}, &result)

		_, err = liquidd.Rpc.Call("sendtoaddress", result.LiquidAddress, 1., "", "", false, false, 1, "UNSET")
		suite.Require().NoError(err)
	}

	// Lock txs.
	_, err = liquidd.Rpc.Call("generatetoaddress", 1, testframework.LBTC_BURN)
	suite.Require().NoError(err)

	// Setup channel ([0] fundAmt(10^7) ---- 0 [1]).
	scid, err := lightningds[0].OpenChannel(lightningds[1], fundAmt, true, true, true)
	if err != nil {
		t.Fatalf("lightingds[0].OpenChannel() %v", err)
	}

	// Sync peer polling
	t.Log("Wait for poll syncing")
	var result interface{}
	err = lightningds[0].Rpc.Request(&clightning.ReloadPolicyFile{}, &result)
	if err != nil {
		t.Fatalf("ListPeers %v", err)
	}
	lightningds[1].WaitForLog(fmt.Sprintf("From: %s got msgtype: a465", lightningds[0].Info.Id), testframework.TIMEOUT)

	err = lightningds[1].Rpc.Request(&clightning.ReloadPolicyFile{}, &result)
	if err != nil {
		t.Fatalf("ListPeers %v", err)
	}
	lightningds[0].WaitForLog(fmt.Sprintf("From: %s got msgtype: a465", lightningds[1].Info.Id), testframework.TIMEOUT)

	suite.bitcoind = bitcoind
	suite.lightningds = lightningds
	suite.liquidd = liquidd
	suite.liquidWalletNames = liquidWalletNames
	suite.scid = scid
}

func (suite *LiquidTestSuite) BeforeTest(_, _ string) {
	var channelBalances []uint64
	var btcWalletBalances []uint64
	var liquidWalletBalances []uint64
	for _, lightningd := range suite.lightningds {
		b, err := testframework.GetBtcWalletBalanceSat(lightningd)
		suite.Require().NoError(err)
		btcWalletBalances = append(btcWalletBalances, b)

		var response clightning.GetBalanceResponse
		err = lightningd.Rpc.Request(&clightning.GetBalanceMethod{}, &response)
		suite.Require().NoError(err)
		liquidWalletBalances = append(liquidWalletBalances, response.LiquidBalance)

		f, err := lightningd.Rpc.ListFunds()
		suite.Require().NoError(err)
		suite.Require().Len(f.Channels, 1)
		channelBalances = append(channelBalances, f.Channels[0].ChannelSatoshi)
	}

	suite.channelBalances = channelBalances
	suite.btcWalletBalances = btcWalletBalances
	suite.liquidWalletBalances = liquidWalletBalances
}

func (suite *LiquidTestSuite) AfterTest(_, testname string) {
	if suite.assertions.HasAssertion() {
		suite.T().Logf("Has assertions on test: %s", testname)
		suite.T().FailNow()
	}
}

func (suite *LiquidTestSuite) HandleStats(_ string, stats *suite.SuiteInformation) {
	suite.T().Log(fmt.Sprintf("Time elapsed: %v", time.Since(stats.Start)))
}

func TestLiquidSwaps(t *testing.T) {
	// Long running tests only run in integration test mode.
	testEnabled := os.Getenv("RUN_INTEGRATION_TESTS")
	if testEnabled == "" {
		t.Skip("set RUN_INTEGRATION_TESTS to run this test")
	}
	suite.Run(t, new(LiquidTestSuite))
}

//
// Swap in tests
// =================

// TestSwapInClaimPreimage execute a swap-in with the claim by preimage
// spending branch.
func (suite *LiquidTestSuite) TestSwapInClaimPreimage() {
	var err error

	t := suite.T()
	assertions := suite.assertions
	lightningds := suite.lightningds
	liquidd := suite.liquidd
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.liquidWalletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: swapAmt, ShortChannelId: scid, Asset: "l-btc"}, &response)
	}()

	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := liquidd.Rpc.Call("getrawmempool", true)
		require.NoError(t, err)

		err = jsonR.GetObject(&mempool)
		require.NoError(t, err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				commitmentFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Confirm opening tx. We need 2 confirmations.
	liquidd.GenerateBlocks(2)

	//
	//	STEP 2: Pay invoice
	//

	// Wait for invoice being paid.
	err = lightningds[1].DaemonProcess.WaitForLog("Event_OnClaimInvoicePaid on State_SwapInSender_AwaitClaimPayment", testframework.TIMEOUT)
	require.NoError(t, err)

	// Check if swap invoice was payed.
	// Expect: [0] before - swapamt ------ before + swapamt [1]
	expected := float64(beforeChannelBalances[0] - swapAmt)
	if !testframework.AssertWaitForChannelBalance(t, lightningds[0], expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[0].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}
	expected = float64(beforeChannelBalances[1] + swapAmt)
	if !testframework.AssertWaitForChannelBalance(t, lightningds[1], expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[1].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}

	//
	//	STEP 3: Broadcasting claim tx
	//

	// Wait for claim tx being broadcasted. We need 3 confirmations.
	// Get claim fee.
	var claimFee uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := liquidd.Rpc.Call("getrawmempool", true)
		require.NoError(t, err)

		err = jsonR.GetObject(&mempool)
		require.NoError(t, err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				claimFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Confirm claim tx.
	liquidd.GenerateBlocks(2)

	// Wail for claim tx confirmation.
	err = lightningds[0].DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_ClaimSwap", testframework.TIMEOUT)
	require.NoError(t, err)

	// Check Wallet balance.
	// Expect:
	// - [0] before - claim_fee + swapamt
	// - [1] before - commitment_fee - swapamt
	var response clightning.GetBalanceResponse
	expected = float64(beforeWalletBalances[0] - claimFee + swapAmt)
	err = lightningds[0].Rpc.Request(&clightning.GetBalanceMethod{}, &response)
	require.NoError(t, err)
	assertions.Count(assert.InDelta(t, expected, float64(response.LiquidBalance), 1., "expected %d, got %d", expected, response.LiquidBalance))

	expected = float64(beforeWalletBalances[1] - commitmentFee - swapAmt)
	err = lightningds[1].Rpc.Request(&clightning.GetBalanceMethod{}, &response)
	require.NoError(t, err)
	assertions.Count(assert.InDelta(t, expected, float64(response.LiquidBalance), 1., "expected %d, got %d", expected, response.LiquidBalance))
}

// TestSwapInClaimCsv execute a swap-in where the peer does not pay the
// invoice and the maker claims by csv.
//
// Todo: Is skipped for now because we can not run it in the suite as it
// gets the channel stuck. See
// https://github.com/sputn1ck/peerswap/issues/69. As soon as this is
// fixed, the skip has to be removed.
func (suite *LiquidTestSuite) TestSwapInClaimCsv() {
	suite.T().SkipNow()
	// todo: implement test
}

// TestSwapInClaimCoop execute a swap-in where one node cancels and the
// coop spending branch is used.
func (suite *LiquidTestSuite) TestSwapInClaimCoop() {
	var err error

	t := suite.T()
	assertions := suite.assertions
	lightningds := suite.lightningds
	liquidd := suite.liquidd
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.liquidWalletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 2

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[1].Rpc.Request(&clightning.SwapIn{SatAmt: swapAmt, ShortChannelId: scid, Asset: "l-btc"}, &response)
	}()

	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := liquidd.Rpc.Call("getrawmempool", true)
		require.NoError(t, err)

		err = jsonR.GetObject(&mempool)
		require.NoError(t, err)

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
		var labelBytes = make([]byte, 5)
		_, err = rand.Read(labelBytes)
		require.NoError(t, err)
		// We have to split the invoices so that they succeed.
		amt := ((beforeChannelBalances[0] - swapAmt) / 2) + 1
		inv, err := lightningds[1].Rpc.Invoice(amt*1000, string(labelBytes), "move-balance")
		require.NoError(t, err)

		_, err = lightningds[0].Rpc.PayBolt(inv.Bolt11)
		require.NoError(t, err)
	}

	// Check if channel balance [0] is less than the swapAmt.
	var setupFunds uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		funds, err := lightningds[0].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		setupFunds = funds.Channels[0].ChannelSatoshi
		return setupFunds < swapAmt
	}, testframework.TIMEOUT))

	//
	//	STEP 3: Confirm opening tx
	//

	liquidd.GenerateBlocks(2)

	// Check that coop close was sent.
	require.NoError(t, lightningds[0].WaitForLog("Event_ActionSucceeded on State_SwapInReceiver_SendCoopClose", testframework.TIMEOUT))

	//
	//	STEP 4: Broadcasting coop claim tx
	//

	// Wait for coop claim tx being broadcasted.
	// Get claim fee.
	var claimFee uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := liquidd.Rpc.Call("getrawmempool", true)
		require.NoError(t, err)

		err = jsonR.GetObject(&mempool)
		require.NoError(t, err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				claimFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Confirm coop claim tx.
	liquidd.GenerateBlocks(2)

	// Check swap is done.
	require.NoError(t, lightningds[1].WaitForLog("Event_ActionSucceeded on State_SwapInSender_ClaimSwapCoop", testframework.TIMEOUT))

	// Check no invoice was paid.
	if !testframework.AssertWaitForChannelBalance(t, lightningds[0], float64(setupFunds), 1., testframework.TIMEOUT) {
		funds, err := lightningds[0].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, float64(setupFunds), funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}

	// Check Wallet balance.
	// Expect:
	// - [0] before
	// - [1] before - commitment_fee - claim_fee
	var response clightning.GetBalanceResponse
	expected := float64(beforeWalletBalances[0])
	err = lightningds[0].Rpc.Request(&clightning.GetBalanceMethod{}, &response)
	require.NoError(t, err)
	assertions.Count(assert.InDelta(t, expected, float64(response.LiquidBalance), 1., "expected %d, got %d", expected, response.LiquidBalance))

	expected = float64(beforeWalletBalances[1] - commitmentFee - claimFee)
	err = lightningds[1].Rpc.Request(&clightning.GetBalanceMethod{}, &response)
	require.NoError(t, err)
	assertions.Count(assert.InDelta(t, expected, float64(response.LiquidBalance), 1., "expected %d, got %d", expected, response.LiquidBalance))

	//
	// Step 5: Reset channel
	//

	require.NoError(t, testframework.BalanceChannel5050(lightningds[0], lightningds[1], scid))
}

//
// Swap out tests
// ==================

// TestSwapOutClaimPreimage execute a swap-out with the claim by
// preimage spending branch.
func (suite *LiquidTestSuite) TestSwapOutClaimPreimage() {
	var err error

	t := suite.T()
	assertions := suite.assertions
	lightningds := suite.lightningds
	liquidd := suite.liquidd
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.liquidWalletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 10

	// Expectations.
	// expectedLightningSat := []uint64{beforeChannelBalances[0] - swapAmt - 1386, beforeChannelBalances[1] + swapAmt + 1386}
	// expectedOnchainSat := []uint64{beforeWalletBalances[0] + swapAmt - 117, beforeWalletBalances[1] - swapAmt - 1386}
	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: swapAmt, ShortChannelId: scid, Asset: "l-btc"}, &response)
	}()

	//
	//	STEP 1: Broadcasting commitment tx
	//

	// Wait for commitment tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := liquidd.Rpc.Call("getrawmempool", true)
		require.NoError(t, err)

		err = jsonR.GetObject(&mempool)
		require.NoError(t, err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				commitmentFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Check if Fee Invoice was payed. (Should have been payed before
	// commitment tx was broadcasted).
	// Expect: [0] before - commitment_fee ------ before + commitment_fee [1]
	expected := float64(beforeChannelBalances[0] - commitmentFee)
	if !testframework.AssertWaitForChannelBalance(t, lightningds[0], expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[0].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}
	expected = float64(beforeChannelBalances[1] + commitmentFee)
	if !testframework.AssertWaitForChannelBalance(t, lightningds[1], expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[1].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}

	//
	//	STEP 2: Pay invoice // Broadcast claim Tx
	//

	// Confirm commitment tx. We need 2 confirmations.
	liquidd.GenerateBlocks(2)

	// Wait for claim invoice being paid.
	err = lightningds[1].DaemonProcess.WaitForLog("Event_OnClaimInvoicePaid on State_SwapOutReceiver_AwaitClaimInvoicePayment", testframework.TIMEOUT)
	require.NoError(t, err)

	// Wait for claim tx being broadcasted.
	var claimFee uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := liquidd.Rpc.Call("getrawmempool", true)
		require.NoError(t, err)

		err = jsonR.GetObject(&mempool)
		require.NoError(t, err)

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
	expected = float64(beforeChannelBalances[0] - commitmentFee - swapAmt)
	if !testframework.AssertWaitForChannelBalance(t, lightningds[0], expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[0].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}
	expected = float64(beforeChannelBalances[1] + commitmentFee + swapAmt)
	if !testframework.AssertWaitForChannelBalance(t, lightningds[1], expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[1].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, expected, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")
	}

	// Confirm claim tx.
	liquidd.GenerateBlocks(2)

	// Wail for claim tx confirmation.
	err = lightningds[0].DaemonProcess.WaitForLog("Event_ActionSucceeded on State_SwapOutSender_ClaimSwap", testframework.TIMEOUT)
	require.NoError(t, err)

	//
	//	STEP 4: Onchain balance change
	//

	// Check Wallet balance.
	// Expect:
	// - [0] before - claim_fee + swapAmt
	// - [1] before - commitment_fee - swapAmt
	var response clightning.GetBalanceResponse
	expected = float64(beforeWalletBalances[0] - claimFee + swapAmt)
	err = lightningds[0].Rpc.Request(&clightning.GetBalanceMethod{}, &response)
	require.NoError(t, err)
	assertions.Count(assert.InDelta(t, expected, float64(response.LiquidBalance), 1., "expected %d, got %d", expected, response.LiquidBalance))

	expected = float64(beforeWalletBalances[1] - commitmentFee - swapAmt)
	err = lightningds[1].Rpc.Request(&clightning.GetBalanceMethod{}, &response)
	require.NoError(t, err)
	assertions.Count(assert.InDelta(t, expected, float64(response.LiquidBalance), 1., "expected %d, got %d", expected, response.LiquidBalance))
}

// TestSwapOutClaimCsv execute a swap-in where the peer does not pay the
// invoice and the maker claims by csv.
//
// Todo: Is skipped for now because we can not run it in the suite as it
// gets the channel stuck. See
// https://github.com/sputn1ck/peerswap/issues/69. As soon as this is
// fixed, the skip has to be removed.
func (suite *LiquidTestSuite) TestSwapOutClaimCsv() {
	suite.T().SkipNow()
	// Todo: add test!
}

// TestSwapOutClaimCoop execute a swap-in where one node cancels and the
//coop spending branch is used.
func (suite *LiquidTestSuite) TestSwapOutClaimCoop() {
	var err error

	t := suite.T()
	assertions := suite.assertions
	lightningds := suite.lightningds
	liquidd := suite.liquidd
	scid := suite.scid

	beforeChannelBalances := suite.channelBalances
	beforeWalletBalances := suite.liquidWalletBalances

	// Changes.
	var swapAmt uint64 = beforeChannelBalances[0] / 2

	// Do swap-in.
	go func() {
		// We need to run this in a go routine as the Request call is blocking and sometimes does not return.
		var response map[string]interface{}
		lightningds[0].Rpc.Request(&clightning.SwapOut{SatAmt: swapAmt, ShortChannelId: scid, Asset: "l-btc"}, &response)
	}()

	//
	//	STEP 1: Broadcasting opening tx
	//

	// Wait for opening tx being broadcasted.
	// Get commitmentFee.
	var commitmentFee uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := liquidd.Rpc.Call("getrawmempool", true)
		require.NoError(t, err)

		err = jsonR.GetObject(&mempool)
		require.NoError(t, err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				commitmentFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Check if Fee Invoice was payed. (Should have been payed before
	// commitment tx was broadcasted).
	// Expect: [0] before - commitment_fee ------ before + commitment_fee [1]
	expected := float64(beforeChannelBalances[0] - commitmentFee)
	if !testframework.AssertWaitForChannelBalance(t, lightningds[0], expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[0].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, expected, funds.Channels[0].ChannelSatoshi, 1.)
	}
	expected = float64(beforeChannelBalances[1] + commitmentFee)
	if !testframework.AssertWaitForChannelBalance(t, lightningds[1], expected, 1., testframework.TIMEOUT) {
		funds, err := lightningds[1].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		require.InDelta(t, expected, funds.Channels[0].ChannelSatoshi, 1.)
	}

	//
	//	STEP 2: Move balance
	//

	// Move local balance from node [0] to [1] so that
	// [0] does not have enough balance to pay the
	// invoice and cancels the swap.
	moveAmtMSat := ((beforeChannelBalances[0]-swapAmt)/2 + 1) * 1000
	for i := 0; i < 2; i++ {
		var labelBytes = make([]byte, 5)
		_, err = rand.Read(labelBytes)
		require.NoError(t, err)
		// We have to split the invoices so that they succeed.
		inv, err := lightningds[1].Rpc.Invoice(moveAmtMSat, string(labelBytes), "move-balance")
		require.NoError(t, err)

		_, err = lightningds[0].Rpc.PayBolt(inv.Bolt11)
		require.NoError(t, err)
	}

	// Check if channel balance [0] is less than the swapAmt.
	// Get channel state for later reference.
	var setupFunds uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		funds, err := lightningds[0].Rpc.ListFunds()
		require.NoError(t, err)
		require.Len(t, funds.Channels, 1)
		setupFunds = funds.Channels[0].ChannelSatoshi
		return setupFunds < swapAmt
	}, testframework.TIMEOUT))

	//
	//	STEP 3: Confirm opening tx
	//

	liquidd.GenerateBlocks(2)

	// Check that coop close was sent.
	require.NoError(t, lightningds[0].WaitForLog("Event_ActionSucceeded on State_SwapOutSender_SendCoopClose", testframework.TIMEOUT))

	//
	//	STEP 4: Broadcasting coop claim tx
	//

	// Wait for coop claim tx being broadcasted.
	var claimFee uint64
	require.NoError(t, testframework.WaitFor(func() bool {
		var mempool map[string]struct {
			Fees struct {
				Base float64 `json:"base"`
			} `json:"fees"`
		}
		jsonR, err := liquidd.Rpc.Call("getrawmempool", true)
		require.NoError(t, err)

		err = jsonR.GetObject(&mempool)
		require.NoError(t, err)

		if len(mempool) == 1 {
			for _, tx := range mempool {
				claimFee = uint64(tx.Fees.Base * 100000000)
				return true
			}
		}
		return false
	}, testframework.TIMEOUT))

	// Confirm coop claim tx.
	liquidd.GenerateBlocks(2)

	// Check swap is done.
	require.NoError(t, lightningds[1].WaitForLog("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCoop", testframework.TIMEOUT))

	//
	//	STEP 4: Balance change
	//

	// Check that channel balance did not change.
	// Expect: setup funds from above
	funds, err := lightningds[0].Rpc.ListFunds()
	require.NoError(t, err)
	require.Len(t, funds.Channels, 1)
	require.InDelta(t, setupFunds, funds.Channels[0].ChannelSatoshi, 1., "expected %d, got %d")

	// Check Wallet balance.
	// Expect:
	// - [0] before
	// - [1] before - commitment_fee - claim_fee
	var response clightning.GetBalanceResponse
	expected = float64(beforeWalletBalances[0])
	err = lightningds[0].Rpc.Request(&clightning.GetBalanceMethod{}, &response)
	require.NoError(t, err)
	assertions.Count(assert.InDelta(t, expected, float64(response.LiquidBalance), 1., "expected %d, got %d", expected, response.LiquidBalance))

	expected = float64(beforeWalletBalances[1] - commitmentFee - claimFee)
	err = lightningds[1].Rpc.Request(&clightning.GetBalanceMethod{}, &response)
	require.NoError(t, err)
	assertions.Count(assert.InDelta(t, expected, float64(response.LiquidBalance), 1., "expected %d, got %d", expected, response.LiquidBalance))

	//
	// Step 5: Reset channel
	//

	require.NoError(t, testframework.BalanceChannel5050(lightningds[0], lightningds[1], scid))
}
