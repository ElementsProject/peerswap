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

	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 2, 100)

	err := txWatcher.StartWatchingTxs()
	if err != nil {
		t.Fatal(err)
	}

	txWatcher.AddWaitForConfirmationTx(swapId, txId, 0, 0, nil)
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
	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 1, 100)

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

	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 2, 100)

	err := txWatcher.StartWatchingTxs()
	if err != nil {
		t.Fatal(err)
	}

	txWatcher.AddWaitForCsvTx(swapId, txid, vout, csv, nil)
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
