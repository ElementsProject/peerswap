package txwatcher

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_RpcTxWatcherConfirmations(t *testing.T) {
	swapId := "foo"
	txId := "bar"

	db := &DummyBlockchain{}
	txWatcherChan := make(chan string)

	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 2)

	err := txWatcher.StartWatchingTxs()
	if err != nil {
		t.Fatal(err)
	}

	txWatcher.AddWaitForConfirmationTx(swapId, txId, 0, 0, 50, nil)
	txWatcher.AddConfirmationCallback(func(swapId, txHex string, err error) error {
		go func() { txWatcherChan <- swapId }()
		return nil
	})

	db.SetBlockHeight(1)
	db.SetNextTxOutResp(&TxOutResp{
		BestBlockHash: "blockhash",
		Confirmations: 2,
	})
	txConfirmedId := <-txWatcherChan
	assert.Equal(t, swapId, txConfirmedId)
}

func Test_RpcTxWatcherOutOfSyncWaitsForNextBlock(t *testing.T) {
	swapId := "foo"
	txId := "bar"
	txOutCalls := make(chan struct{}, 1)

	db := &DummyBlockchain{
		nextBlockheight: 1,
		nextTxOutResp: &TxOutResp{
			BestBlockHash: "stale-blockhash",
			Confirmations: 1,
		},
		txOutCalls: txOutCalls,
	}
	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 1)

	callbackErr := make(chan error, 1)
	txWatcher.AddConfirmationCallback(func(swapId, txHex string, err error) error {
		callbackErr <- err
		return nil
	})

	newBlock := make(chan uint32)
	go txWatcher.observationLoop(context.Background(), swapId, txId, 0, 0, 100, newBlock)

	newBlock <- 1
	select {
	case <-txOutCalls:
	case <-time.After(time.Second):
		t.Fatal("expected txout lookup")
	}

	select {
	case err := <-callbackErr:
		t.Fatalf("unexpected callback after out-of-sync lookup: %v", err)
	default:
	}

	db.SetBlockHeight(2)
	db.SetNextTxOutResp(&TxOutResp{
		BestBlockHash: "blockhash",
		Confirmations: 1,
	})

	newBlock <- 2
	select {
	case err := <-callbackErr:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("expected confirmation callback")
	}
}

func Test_RpcTxWatcherRejectsAtPaymentDeadline(t *testing.T) {
	db := &DummyBlockchain{nextBlockheight: 160}
	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 1)

	callbackErr := make(chan error, 1)
	txWatcher.AddConfirmationCallback(func(swapID, txHex string, err error) error {
		callbackErr <- err
		return nil
	})

	newBlock := make(chan uint32)
	go txWatcher.observationLoop(
		context.Background(),
		"swap",
		"tx",
		0,
		100,
		60,
		newBlock,
	)

	newBlock <- 160
	select {
	case err := <-callbackErr:
		assert.EqualError(
			t,
			err,
			"exceeded csv limit",
		)
	case <-time.After(time.Second):
		t.Fatal("expected payment deadline callback")
	}
}

func Test_RpcTxWatcherConfirmationPaymentWindow(t *testing.T) {
	tests := []struct {
		name          string
		start         uint32
		current       uint32
		confirmations uint32
		window        uint32
		wantExpired   bool
	}{
		{"before deadline", 100, 159, 59, 60, false},
		{"at deadline", 100, 160, 60, 60, true},
		{"after deadline", 100, 161, 61, 60, true},
		{"custom window", 100, 110, 10, 10, true},
		{"delayed confirmation", 100, 160, 11, 60, true},
		{"confirmation before anchor", 100, 150, 61, 60, true},
		{"confirmation age boundary", 100, 159, 60, 60, true},
		{"height below anchor", 100, 99, 10, 60, true},
		{"zero window", 100, 100, 1, 0, true},
		{"height arithmetic does not wrap", ^uint32(0) - 20, ^uint32(0), 20, 30, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &DummyBlockchain{
				nextBlockheight: uint64(tt.current),
				nextTxOutResp: &TxOutResp{
					BestBlockHash: "blockhash",
					Confirmations: tt.confirmations,
				},
			}
			watcher := NewBlockchainRpcTxWatcher(context.Background(), db, 1)
			callbackErr := make(chan error, 1)
			watcher.AddConfirmationCallback(func(_ string, rawTx string, err error) error {
				if err == nil && rawTx != "txhex" {
					t.Errorf("unexpected transaction: %s", rawTx)
				}
				callbackErr <- err
				return nil
			})
			newBlock := make(chan uint32, 1)
			newBlock <- tt.current
			done := make(chan struct{})
			go func() {
				defer close(done)
				watcher.observationLoop(context.Background(), "swap", "tx", 0, tt.start, tt.window, newBlock)
			}()
			select {
			case err := <-callbackErr:
				if tt.wantExpired {
					assert.EqualError(t, err, "exceeded csv limit")
				} else {
					assert.NoError(t, err)
				}
			case <-time.After(time.Second):
				t.Fatal("watcher did not deliver a callback")
			}
			<-done
		})
	}
}

func Test_RpcTxWatcherCsv(t *testing.T) {
	csv := uint32(100)
	swapId := "foo"
	txid := "bar"
	vout := uint32(0)
	db := &DummyBlockchain{
		nextBlockheight: 12,
		nextTxOutResp: &TxOutResp{
			Confirmations: 0,
		},
	}

	txWatcherChan := make(chan string)

	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 2)

	err := txWatcher.StartWatchingTxs()
	if err != nil {
		t.Fatal(err)
	}

	txWatcher.AddWaitForCsvTx(swapId, txid, vout, 0, csv, nil)
	txWatcher.AddCsvCallback(func(swapId string) error {
		go func() { txWatcherChan <- swapId }()
		return nil
	})

	db.SetBlockHeight(101)
	db.SetNextTxOutResp(&TxOutResp{
		Confirmations: 101,
	})

	txConfirmedId := <-txWatcherChan
	assert.Equal(t, swapId, txConfirmedId)
}

func Test_RpcTxWatcherStoresPerSwapCsv(t *testing.T) {
	db := &DummyBlockchain{nextTxOutResp: &TxOutResp{Confirmations: 0}}
	watcher := NewBlockchainRpcTxWatcher(context.Background(), db, 2)

	watcher.AddWaitForCsvTx("legacy", "legacy-tx", 0, 1, 60, nil)
	watcher.AddWaitForCsvTx("current", "current-tx", 0, 1, 10080, nil)

	assert.Equal(t, uint32(60), watcher.csvtxWatchList["legacy"].Csv)
	assert.Equal(t, uint32(10080), watcher.csvtxWatchList["current"].Csv)
}

func TestRpcTxWatcherMatureCsvCallbackDoesNotBlockRegistration(t *testing.T) {
	db := &DummyBlockchain{nextTxOutResp: &TxOutResp{Confirmations: 1008}}
	watcher := NewBlockchainRpcTxWatcher(context.Background(), db, 3)
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	callbackDone := make(chan struct{})
	registrationDone := make(chan struct{})
	watcher.AddCsvCallback(func(string) error {
		close(callbackStarted)
		// Model a callback waiting for the calling swap's recovery lock.
		<-releaseCallback
		close(callbackDone)
		return nil
	})
	go func() {
		watcher.AddWaitForCsvTx("mature-swap", "opening", 0, 100, 1008, nil)
		close(registrationDone)
	}()
	t.Cleanup(func() {
		close(releaseCallback)
		select {
		case <-callbackDone:
		case <-time.After(time.Second):
			t.Error("CSV callback did not finish after releasing the caller lock")
		}
		select {
		case <-registrationDone:
		case <-time.After(time.Second):
			t.Error("CSV registration did not finish after releasing the callback")
		}
	})

	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("mature CSV callback was not invoked")
	}
	select {
	case <-registrationDone:
	case <-time.After(time.Second):
		t.Fatal("CSV registration blocked on its callback; recovery cannot release the swap lock")
	}
}

func TestRpcTxWatcherMatureCsvCallbackErrorRetainsWatch(t *testing.T) {
	db := &DummyBlockchain{nextTxOutResp: &TxOutResp{Confirmations: 1008}}
	watcher := NewBlockchainRpcTxWatcher(context.Background(), db, 3)
	var calls atomic.Int32
	watcher.AddCsvCallback(func(string) error {
		if calls.Add(1) == 1 {
			return errors.New("temporary claim failure")
		}
		return nil
	})
	watcher.AddWaitForCsvTx("retry-swap", "opening", 0, 100, 1008, nil)

	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		watcher.Lock()
		_, registered := watcher.csvtxWatchList["retry-swap"]
		watcher.Unlock()
		if registered {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("failed mature CSV callback did not retain its watch")
		}
	}
	if err := watcher.HandleCsvTx(1108); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("callback called %d times, want initial attempt and retry", got)
	}
	watcher.Lock()
	_, registered := watcher.csvtxWatchList["retry-swap"]
	watcher.Unlock()
	if registered {
		t.Fatal("successful CSV retry did not remove its watch")
	}
}

type DummyBlockchain struct {
	sync.RWMutex
	nextBlockheight uint64
	nextTxOutResp   *TxOutResp
	txOutCalls      chan struct{}
}

func (d *DummyBlockchain) GetBlockHeightByHash(blockhash string) (uint32, error) {
	return 1, nil
}

func (d *DummyBlockchain) GetBlockHash(height uint32) (string, error) {
	return "blockhash", nil
}

func (d *DummyBlockchain) GetRawtransactionWithBlockHash(txId string, blockHash string) (string, error) {
	return "txhex", nil
}

func (d *DummyBlockchain) SetBlockHeight(height uint64) {
	d.Lock()
	defer d.Unlock()
	d.nextBlockheight = height
}

func (d *DummyBlockchain) SetNextTxOutResp(out *TxOutResp) {
	d.Lock()
	defer d.Unlock()
	d.nextTxOutResp = out
}

func (d *DummyBlockchain) GetBlockHeight() (uint64, error) {
	d.RLock()
	defer d.RUnlock()
	return d.nextBlockheight, nil
}

func (d *DummyBlockchain) GetTxOut(txid string, vout uint32) (*TxOutResp, error) {
	d.RLock()
	defer d.RUnlock()
	if d.txOutCalls != nil {
		select {
		case d.txOutCalls <- struct{}{}:
		default:
		}
	}
	return d.nextTxOutResp, nil
}
