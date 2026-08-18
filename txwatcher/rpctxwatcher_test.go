package txwatcher

import (
	"context"
	"sync"
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
