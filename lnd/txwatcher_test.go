package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/elementsproject/peerswap/cmd/peerswaplnd"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/elementsproject/peerswap/test"
	"github.com/elementsproject/peerswap/testframework"
	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
)

const testTargetConf uint32 = 3
const testCsvLimit uint32 = 1008

func TestTxWatcher_GetBlockHeight(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	bitcoind, _, cc, err := txwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()
	ln := lnrpc.NewLightningClient(cc)
	network, err := GetBitcoinChain(ctx, ln)
	if err != nil {
		t.Fatalf("Failed GetBitcoinChain(): %v", err)
	}
	// Create new tx watcher
	txwatcher, err := NewTxWatcher(ctx, cc, network, testTargetConf, testCsvLimit)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	// We expect a block height of 101 as this is how we setup a bitcoind and
	// lnd setup on startup.
	err = testframework.WaitFor(func() bool {
		bh, err := txwatcher.GetBlockHeight()
		if err != nil {
			t.Fatalf("Failed GetBlockHeight(): %v", err)
		}
		return bh == 101
	}, 20*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for block height of 101: %v", err)
	}

	// Mine one more block
	bitcoind.GenerateBlocks(1)

	// We now expect a block height of 102.
	err = testframework.WaitFor(func() bool {
		bh, err := txwatcher.GetBlockHeight()
		if err != nil {
			t.Fatalf("Failed GetBlockHeight(): %v", err)
		}
		return bh == 102
	}, 20*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for block height of 102: %v", err)
	}
}

// TestTxWatcher_AddWaitForConfirmationTx tests the basic functionality of the
// "await confirmation tx watcher". We add a callback to the watcher and watch
// for a tx. We expect the callback to be called after the blocks necessary for
// confirmation have been generated.
func TestTxWatcher_AddWaitForConfirmationTx(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	bitcoind, _, cc, err := txwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()
	ln := lnrpc.NewLightningClient(cc)
	network, err := GetBitcoinChain(ctx, ln)
	if err != nil {
		t.Fatalf("Failed GetBitcoinChain(): %v", err)
	}

	// Create new tx watcher
	txwatcher, err := NewTxWatcher(ctx, cc, network, testTargetConf, testCsvLimit)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	res, err := bitcoind.Rpc.Call("sendtoaddress", testframework.BTC_BURN, 0.001)
	if err != nil {
		t.Fatalf("Failed sendtoaddress(): %v", err)
	}

	txid, err := res.GetString()
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	res, err = bitcoind.Call("getrawtransaction", txid, true)
	if err != nil {
		t.Fatalf("Failed getrawtransaction(): %v", err)
	}

	var rawtx = struct {
		VOut []struct {
			ScriptPubkey struct {
				Hex string `json:"hex"`
			} `json:"scriptPubkey"`
		} `json:"vout"`
	}{}

	err = res.GetObject(&rawtx)
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	script, err := hex.DecodeString(rawtx.VOut[0].ScriptPubkey.Hex)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}

	// Add a confirmation callback.
	var gotCallback bool
	txwatcher.AddConfirmationCallback(func(swapId, txHex string) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForConfirmationTx("myswap", txid, 0, 101, script)

	// Mine confirmation blocks.
	bitcoind.GenerateBlocks(3)

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 20*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

// TestTxWatcher_AddWaitForConfirmationTx_Reconnect tests that the watcher
// continues on the correct height after the node that the watcher subscribed to
// was killed and restarted. In the time that the node is shutdown we generate
// the blocks necessary for confirmation. We expect the watcher to call the
// callback after the node that the watcher is subscribed to is restarted.
func TestTxWatcher_AddWaitForConfirmationTx_Reconnect(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	bitcoind, lnd, cc, err := txwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()
	ln := lnrpc.NewLightningClient(cc)
	network, err := GetBitcoinChain(ctx, ln)
	if err != nil {
		t.Fatalf("Failed GetBitcoinChain(): %v", err)
	}

	// Create new tx watcher
	txwatcher, err := NewTxWatcher(ctx, cc, network, testTargetConf, testCsvLimit)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	res, err := bitcoind.Rpc.Call("sendtoaddress", testframework.BTC_BURN, 0.001)
	if err != nil {
		t.Fatalf("Failed sendtoaddress(): %v", err)
	}

	txid, err := res.GetString()
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	res, err = bitcoind.Call("getrawtransaction", txid, true)
	if err != nil {
		t.Fatalf("Failed getrawtransaction(): %v", err)
	}

	var rawtx = struct {
		VOut []struct {
			ScriptPubkey struct {
				Hex string `json:"hex"`
			} `json:"scriptPubkey"`
		} `json:"vout"`
	}{}

	err = res.GetObject(&rawtx)
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	script, err := hex.DecodeString(rawtx.VOut[0].ScriptPubkey.Hex)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}

	// Add a confirmation callback.
	var gotCallback bool
	txwatcher.AddConfirmationCallback(func(swapId, txHex string) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForConfirmationTx("myswap", txid, 0, 101, script)

	// We now kill the lnd node and mine the confirmation blocks. We wait a
	// random time between 1 and 6 seconds and restart the node. We expect the
	// txwatcher to retry and return with no error.
	lnd.Kill()
	bitcoind.GenerateBlocks(3)

	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	lnd.Run(true, true)

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 20*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

// TestTxWatcher_AddWaitForConfirmationTx_Reconnect_CSVPassed tests that the
// watcher calls the correct callback if the csv safety limit was reached while
// the node that the watcher is subscribed to was offline. We expect the "csv
// limit callback" to be called after the node is restarted.
func TestTxWatcher_AddWaitForConfirmationTx_Reconnect_CSVPassed(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	bitcoind, lnd, cc, err := txwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()
	ln := lnrpc.NewLightningClient(cc)
	network, err := GetBitcoinChain(ctx, ln)
	if err != nil {
		t.Fatalf("Failed GetBitcoinChain(): %v", err)
	}

	// Create new tx watcher
	txwatcher, err := NewTxWatcher(ctx, cc, network, testTargetConf, testCsvLimit)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	res, err := bitcoind.Rpc.Call("sendtoaddress", testframework.BTC_BURN, 0.001)
	if err != nil {
		t.Fatalf("Failed sendtoaddress(): %v", err)
	}

	txid, err := res.GetString()
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	res, err = bitcoind.Call("getrawtransaction", txid, true)
	if err != nil {
		t.Fatalf("Failed getrawtransaction(): %v", err)
	}

	var rawtx = struct {
		VOut []struct {
			ScriptPubkey struct {
				Hex string `json:"hex"`
			} `json:"scriptPubkey"`
		} `json:"vout"`
	}{}

	err = res.GetObject(&rawtx)
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	script, err := hex.DecodeString(rawtx.VOut[0].ScriptPubkey.Hex)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}

	// Add a csv callback.
	// We want to check if the csv callback is called in the case that the csv
	// limit was reached while lnd was down and in the case of a recover where
	// the csv limit was reached.
	var gotCallback bool
	txwatcher.AddCsvCallback(func(swapId string) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForConfirmationTx("myswap", txid, 0, 101, script)

	// We now kill the lnd node and mine the confirmation blocks. We wait a
	// random time between 1 and 6 seconds and restart the node. We expect the
	// txwatcher to retry and return with no error.
	lnd.Kill()
	bitcoind.GenerateBlocks(onchain.BitcoinCsvSafetyLimit + 1)

	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	lnd.Run(true, true)

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 20*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

// TestTxWatcher_AddWaitForConfirmationTx_Reconnect_OnGracefulStop tests that
// the watcher continues on the correct height after the node that the watcher
// subscribed to was gracefully shutdown and restarted. In the time that the
// node is shutdown we generate the blocks necessary for confirmation. We expect
// the watcher to call the callback after the node that the watcher is
// subscribed to is restarted.
func TestTxWatcher_AddWaitForConfirmationTx_Reconnect_OnGracefulStop(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	bitcoind, lnd, cc, err := txwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()
	ln := lnrpc.NewLightningClient(cc)
	network, err := GetBitcoinChain(ctx, ln)
	if err != nil {
		t.Fatalf("Failed GetBitcoinChain(): %v", err)
	}

	// Create new tx watcher
	txwatcher, err := NewTxWatcher(ctx, cc, network, testTargetConf, testCsvLimit)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	res, err := bitcoind.Rpc.Call("sendtoaddress", testframework.BTC_BURN, 0.001)
	if err != nil {
		t.Fatalf("Failed sendtoaddress(): %v", err)
	}

	txid, err := res.GetString()
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	res, err = bitcoind.Call("getrawtransaction", txid, true)
	if err != nil {
		t.Fatalf("Failed getrawtransaction(): %v", err)
	}

	var rawtx = struct {
		VOut []struct {
			ScriptPubkey struct {
				Hex string `json:"hex"`
			} `json:"scriptPubkey"`
		} `json:"vout"`
	}{}

	err = res.GetObject(&rawtx)
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	script, err := hex.DecodeString(rawtx.VOut[0].ScriptPubkey.Hex)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}

	// Add a confirmation callback.
	var gotCallback bool
	txwatcher.AddConfirmationCallback(func(swapId, txHex string) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForConfirmationTx("myswap", txid, 0, 101, script)

	_, err = lnd.Rpc.StopDaemon(context.Background(), &lnrpc.StopRequest{})
	if err != nil {
		t.Fatalf("Failed StopDaemon(): %v", err)
	}
	bitcoind.GenerateBlocks(int(testTargetConf))

	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	lnd.Run(true, true)

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 20*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

// TestTxWatcher_AddWaitForCsvTx tests the basic functionality of the "await csv
// is reached tx watcher". We add a callback to the watcher and watch for a tx.
// We expect the callback to be called after the blocks necessary for csv have
// been generated.
func TestTxWatcher_AddWaitForCsvTx(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	bitcoind, _, cc, err := txwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()
	ln := lnrpc.NewLightningClient(cc)
	network, err := GetBitcoinChain(ctx, ln)
	if err != nil {
		t.Fatalf("Failed GetBitcoinChain(): %v", err)
	}

	// Create new tx watcher
	txwatcher, err := NewTxWatcher(ctx, cc, network, testTargetConf, testCsvLimit)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	res, err := bitcoind.Rpc.Call("sendtoaddress", testframework.BTC_BURN, 0.001)
	if err != nil {
		t.Fatalf("Failed sendtoaddress(): %v", err)
	}

	txid, err := res.GetString()
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	res, err = bitcoind.Call("getrawtransaction", txid, true)
	if err != nil {
		t.Fatalf("Failed getrawtransaction(): %v", err)
	}

	var rawtx = struct {
		VOut []struct {
			ScriptPubkey struct {
				Hex string `json:"hex"`
			} `json:"scriptPubkey"`
		} `json:"vout"`
	}{}

	err = res.GetObject(&rawtx)
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	script, err := hex.DecodeString(rawtx.VOut[0].ScriptPubkey.Hex)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}

	// Add a csv callback.
	var gotCallback bool
	txwatcher.AddCsvCallback(func(_ string) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForCsvTx("addwaitforcsvtx", txid, 0, 101, script)

	// Mine confirmation blocks, one less than csv limit.
	bitcoind.GenerateBlocks(onchain.BitcoinCsv - 1)

	// We are one block before csv limit, we do not expect the callback to be
	// called.
	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 5*time.Second)
	if err == nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}

	// Mine one more block, now we have reached the csv limit.
	bitcoind.GenerateBlocks(1)

	// We expect the callback to be called as we reached the csv limit.
	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

// TestTxWatcher_AddWaitForCsvTx_Reconnect tests that the watcher continues on
// the correct height after the node that the watcher subscribed to was killed
// and restarted. In the time that the node is shutdown we generate the blocks
// necessary for the csv limit. We expect the watcher to call the callback after
// the node that the watcher is subscribed to is restarted.
func TestTxWatcher_AddWaitForCsvTx_Reconnect(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	bitcoind, lnd, cc, err := txwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()
	ln := lnrpc.NewLightningClient(cc)
	network, err := GetBitcoinChain(ctx, ln)
	if err != nil {
		t.Fatalf("Failed GetBitcoinChain(): %v", err)
	}

	// Create new tx watcher
	txwatcher, err := NewTxWatcher(ctx, cc, network, testTargetConf, testCsvLimit)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	res, err := bitcoind.Rpc.Call("sendtoaddress", testframework.BTC_BURN, 0.001)
	if err != nil {
		t.Fatalf("Failed sendtoaddress(): %v", err)
	}

	txid, err := res.GetString()
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	res, err = bitcoind.Call("getrawtransaction", txid, true)
	if err != nil {
		t.Fatalf("Failed getrawtransaction(): %v", err)
	}

	var rawtx = struct {
		VOut []struct {
			ScriptPubkey struct {
				Hex string `json:"hex"`
			} `json:"scriptPubkey"`
		} `json:"vout"`
	}{}

	err = res.GetObject(&rawtx)
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	script, err := hex.DecodeString(rawtx.VOut[0].ScriptPubkey.Hex)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}

	// Add a confirmation callback.
	var gotCallback bool
	txwatcher.AddCsvCallback(func(_ string) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForCsvTx("addwaitforcsvtx-reconnect", txid, 0, 101, script)

	// We now kill the lnd node and mine the confirmation blocks. We wait a
	// random time between 1 and 6 seconds and restart the node. We expect the
	// txwatcher to retry and return with no error.
	lnd.Kill()
	bitcoind.GenerateBlocks(onchain.BitcoinCsv)

	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	lnd.Run(true, true)

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 20*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

func TestTxWatcher_Stop(t *testing.T) {
	test.IsIntegrationTest(t)
	t.Parallel()

	// Setup bitcoind and lnd.
	tmpDir := t.TempDir()
	bitcoind, _, cc, err := txwatcherNodeSetup(t, tmpDir)
	if err != nil {
		t.Fatalf("Could not create lnd client connection: %v", err)
	}

	ctx := context.Background()
	ln := lnrpc.NewLightningClient(cc)
	network, err := GetBitcoinChain(ctx, ln)
	if err != nil {
		t.Fatalf("Failed GetBitcoinChain(): %v", err)
	}

	// Create new tx watcher
	txwatcher, err := NewTxWatcher(ctx, cc, network, testTargetConf, testCsvLimit)
	if err != nil {
		t.Fatalf("Could not create tx watcher: %v", err)
	}

	res, err := bitcoind.Rpc.Call("sendtoaddress", testframework.BTC_BURN, 0.001)
	if err != nil {
		t.Fatalf("Failed sendtoaddress(): %v", err)
	}

	txid, err := res.GetString()
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	res, err = bitcoind.Call("getrawtransaction", txid, true)
	if err != nil {
		t.Fatalf("Failed getrawtransaction(): %v", err)
	}

	var rawtx = struct {
		VOut []struct {
			ScriptPubkey struct {
				Hex string `json:"hex"`
			} `json:"scriptPubkey"`
		} `json:"vout"`
	}{}

	err = res.GetObject(&rawtx)
	if err != nil {
		t.Fatalf("Failed GetString(): %v", err)
	}

	script, err := hex.DecodeString(rawtx.VOut[0].ScriptPubkey.Hex)
	if err != nil {
		t.Fatalf("Failed DecodeString(): %v", err)
	}

	txwatcher.AddWaitForCsvTx("addwaitforcsvtx-reconnect", txid, 0, 101, script)
	txwatcher.Stop()
}

func txwatcherNodeSetup(t *testing.T, dir string) (bitcoind *testframework.BitcoinNode, lnd *testframework.LndNode, cc *grpc.ClientConn, err error) {
	bitcoind, err = testframework.NewBitcoinNode(dir, 1)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Can not create bitcoin node: %v", err)
	}

	lnd, err = testframework.NewLndNode(dir, bitcoind, 1, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Can not create lnd node: %v", err)
	}

	if err := bitcoind.Run(true); err != nil {
		return nil, nil, nil, fmt.Errorf("Can not start bitcoind: %v", err)
	}
	t.Cleanup(bitcoind.Kill)

	if err := lnd.Run(true, true); err != nil {
		return nil, nil, nil, fmt.Errorf("Can not start lnd: %v", err)
	}
	t.Cleanup(lnd.Kill)

	// Create a client connection to the lnd node. And a new lnd client.
	cc, err = getClientConnectionForTests(
		context.Background(),
		&peerswaplnd.LndConfig{
			LndHost:      fmt.Sprintf("localhost:%d", lnd.RpcPort),
			TlsCertPath:  lnd.TlsPath,
			MacaroonPath: lnd.MacaroonPath,
		},
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Could not create lnd client connection: %v", err)
	}

	return bitcoind, lnd, cc, nil
}
