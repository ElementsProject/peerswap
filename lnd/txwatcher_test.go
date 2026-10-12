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
	"github.com/lightningnetwork/lnd/lnrpc/chainrpc"
	"google.golang.org/grpc"
)

const testTargetConf uint32 = 3
const testCsvLimit uint32 = 1008

// Exercise recovery with a transaction that is already confirmed when the
// watcher subscribes, without running a Lightning or Bitcoin daemon.
func TestTxWatcherConfirmationPaymentWindow(t *testing.T) {
	tests := []struct {
		name        string
		start       uint32
		confirmed   uint32
		current     uint32
		window      uint32
		wantExpired bool
	}{
		{"before deadline", 100, 101, 159, 60, false},
		{"at deadline", 100, 101, 160, 60, true},
		{"after deadline", 100, 101, 161, 60, true},
		{"custom window", 100, 101, 110, 10, true},
		{"delayed confirmation", 100, 150, 160, 60, true},
		{"confirmation before anchor", 100, 90, 150, 60, true},
		{"confirmation age boundary", 100, 100, 159, 60, true},
		{"height below anchor", 100, 90, 99, 60, true},
		{"height below confirmation", 100, 110, 109, 60, true},
		{"zero window", 100, 100, 100, 0, true},
		{"height arithmetic does not wrap", ^uint32(0) - 20, ^uint32(0) - 19, ^uint32(0), 30, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			watcher := &TxWatcher{
				ctx: ctx, cancel: cancel,
				lnrpcClient:          &paymentWindowLightningClient{height: tt.current},
				chainrpcClient:       &paymentWindowChainClient{height: tt.confirmed},
				targetConfs:          1,
				confirmationWatchers: make(map[string]bool),
			}
			t.Cleanup(func() { _ = watcher.Stop() })
			expired := make(chan bool, 1)
			watcher.AddCsvCallback(func(string) error {
				t.Error("confirmation watcher must report errors through its confirmation callback")
				expired <- true
				return nil
			})
			watcher.AddConfirmationCallback(func(_ string, rawTx string, err error) error {
				if err != nil {
					if rawTx != "" || err.Error() != "exceeded safe payment window" {
						t.Errorf("unexpected rejection: tx=%s, err=%v", rawTx, err)
					}
				} else if rawTx != "01" {
					t.Errorf("unexpected confirmation transaction: %s", rawTx)
				}
				expired <- err != nil
				return nil
			})
			watcher.AddWaitForConfirmationTx("swap", fmt.Sprintf("%064x", 1), 0, tt.start, tt.window, nil)
			select {
			case got := <-expired:
				if got != tt.wantExpired {
					t.Fatalf("expired=%v, want %v", got, tt.wantExpired)
				}
			case <-time.After(time.Second):
				t.Fatal("watcher did not deliver a callback")
			}
		})
	}
}

type paymentWindowLightningClient struct {
	lnrpc.LightningClient
	height uint32
}

func (c *paymentWindowLightningClient) GetInfo(
	context.Context,
	*lnrpc.GetInfoRequest,
	...grpc.CallOption,
) (*lnrpc.GetInfoResponse, error) {
	return &lnrpc.GetInfoResponse{BlockHeight: c.height}, nil
}

type paymentWindowChainClient struct {
	chainrpc.ChainNotifierClient
	height uint32
}

func (c *paymentWindowChainClient) RegisterConfirmationsNtfn(
	context.Context,
	*chainrpc.ConfRequest,
	...grpc.CallOption,
) (chainrpc.ChainNotifier_RegisterConfirmationsNtfnClient, error) {
	return &paymentWindowConfirmationStream{height: c.height}, nil
}

type paymentWindowConfirmationStream struct {
	chainrpc.ChainNotifier_RegisterConfirmationsNtfnClient
	height uint32
}

func (s *paymentWindowConfirmationStream) Recv() (*chainrpc.ConfEvent, error) {
	return &chainrpc.ConfEvent{Event: &chainrpc.ConfEvent_Conf{
		Conf: &chainrpc.ConfDetails{BlockHeight: s.height, RawTx: []byte{1}},
	}}, nil
}

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

	initial, err := txwatcher.GetBlockHeight()
	if err != nil {
		t.Fatalf("Failed GetBlockHeight(): %v", err)
	}

	// Mine one more block
	bitcoind.GenerateBlocks(1)

	expected := initial + 1
	err = testframework.WaitFor(func() bool {
		bh, err := txwatcher.GetBlockHeight()
		if err != nil {
			t.Fatalf("Failed GetBlockHeight(): %v", err)
		}
		return bh == expected
	}, 50*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for block height of %d: %v", expected, err)
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
	txwatcher.AddConfirmationCallback(func(swapId, txHex string, err error) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForConfirmationTx("myswap", txid, 0, 101, testCsvLimit/2, script)

	// Mine confirmation blocks.
	bitcoind.GenerateBlocks(3)

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 50*time.Second)
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
	txwatcher.AddConfirmationCallback(func(swapId, txHex string, err error) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForConfirmationTx("myswap", txid, 0, 101, testCsvLimit/2, script)

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
	}, 50*time.Second)
	if err != nil {
		t.Fatalf("Failed waiting for confirmation callback being called: %v", err)
	}
}

// TestTxWatcher_AddWaitForConfirmationTx_Reconnect_CSVPassed checks
// that recovery reports a confirmation error when the payment window expired
// while the node was offline.
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

	// A deadline error must reach the confirmation callback so the waiting
	// swap state can handle it as an action failure.
	callbackErr := make(chan error, 1)
	txwatcher.AddConfirmationCallback(func(_ string, _ string, err error) error {
		callbackErr <- err
		return nil
	})

	txwatcher.AddWaitForConfirmationTx("myswap", txid, 0, 101, testCsvLimit/2, script)

	// Expire the payment window while LND is offline, then ensure recovery
	// rejects the payment instead of reporting a successful confirmation.
	lnd.Kill()
	bitcoind.GenerateBlocks(onchain.BitcoinCsvSafetyLimit + 1)

	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	lnd.Run(true, true)

	select {
	case err := <-callbackErr:
		if err == nil {
			t.Fatal("expected an expired payment window error")
		}
	case <-time.After(50 * time.Second):
		t.Fatal("confirmation callback was not called")
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
	txwatcher.AddConfirmationCallback(func(swapId, txHex string, err error) error {
		gotCallback = true
		return nil
	})

	txwatcher.AddWaitForConfirmationTx("myswap", txid, 0, 101, testCsvLimit/2, script)

	_, err = lnd.Rpc.StopDaemon(context.Background(), &lnrpc.StopRequest{})
	if err != nil {
		t.Logf("Failed StopDaemon(): %v", err)
	}
	bitcoind.GenerateBlocks(int(testTargetConf))

	// Restart lnd
	n := rand.Intn(5) + 1
	time.Sleep(time.Duration(n) * time.Second)
	lnd.Run(true, true)

	err = testframework.WaitFor(func() bool {
		return gotCallback
	}, 50*time.Second)
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

	txwatcher.AddWaitForCsvTx("addwaitforcsvtx", txid, 0, 101, testCsvLimit, script)

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

	txwatcher.AddWaitForCsvTx("addwaitforcsvtx-reconnect", txid, 0, 101, testCsvLimit, script)

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
	}, 50*time.Second)
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

	txwatcher.AddWaitForCsvTx("addwaitforcsvtx-reconnect", txid, 0, 101, testCsvLimit, script)
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
