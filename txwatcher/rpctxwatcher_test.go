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

	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 2)

	err := txWatcher.StartWatchingTxs()
	if err != nil {
		t.Fatal(err)
	}

	txWatcher.AddConfirmationsTx(swapId, txId)
	txWatcher.AddTxConfirmedHandler(func(swapId string) error {
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

func Test_RpcTxWatcherCltv(t *testing.T) {
	cltv := int64(100)
	swapId := "foo"

	db := &DummyBlockchain{
		nextBlockheight: 12,
		nextTxOutResp: &TxOutResp{
			Confirmations: 2,
		},
	}

	txWatcherChan := make(chan string)

	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 2)

	err := txWatcher.StartWatchingTxs()
	if err != nil {
		t.Fatal(err)
	}

	txWatcher.AddCltvTx(swapId, cltv)
	txWatcher.AddCltvPassedHandler(func(swapId string) error {
		go func() { txWatcherChan <- swapId }()
		return nil
	})

	db.SetBlockHeight(101)
	db.SetNextTxOutResp(&TxOutResp{
		Confirmations: 2,
	})

	txConfirmedId := <-txWatcherChan
	assert.Equal(t, swapId, txConfirmedId)
}

type DummyBlockchain struct {
	sync.RWMutex
	nextBlockheight uint64
	nextTxOutResp   *TxOutResp
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
