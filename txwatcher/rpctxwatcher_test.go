package txwatcher

import (
	"context"
	"sync"
	"testing"

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
	txWatcher.AddConfirmationCallback(func(swapId string, txHex string) error {
		go func() { txWatcherChan <- swapId }()
		return nil
	})

	db.SetBlockHeight(1)
	db.SetNextTxOutResp(&TxOutResp{
		Confirmations: 2,
	})
	txConfirmedId := <-txWatcherChan
	assert.Equal(t, swapId, txConfirmedId)
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
	return d.nextTxOutResp, nil
}
