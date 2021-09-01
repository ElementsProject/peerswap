package txwatcher

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_RpcTxWatcherConfirmations(t *testing.T) {

	swapId := "foo"
	txId := "bar"

	db := &DummyBlockchain{}
	txWatcherChan := make(chan string)

	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db,2)

	err := txWatcher.StartWatchingTxs()
	if err != nil {
		t.Fatal(err)
	}
	db.nextBlockheight = 1
	db.nextTxOutResp = &TxOutResp{
		Confirmations: 2,
	}
	txWatcher.AddConfirmationsTx(swapId, txId)
	txWatcher.AddTxConfirmedHandler(func(swapId string) error {
		go func() { txWatcherChan <- swapId }()
		return nil
	})
	txConfirmedId := <-txWatcherChan
	assert.Equal(t, swapId, txConfirmedId)
}
func Test_RpcTxWatcherCltv(t *testing.T) {

	swapId := "foo"
	cltv := int64(100)

	db := &DummyBlockchain{}
	txWatcherChan := make(chan string)

	txWatcher := NewBlockchainRpcTxWatcher(context.Background(), db, 2)

	err := txWatcher.StartWatchingTxs()
	if err != nil {
		t.Fatal(err)
	}
	db.nextBlockheight = 101
	db.nextTxOutResp = &TxOutResp{
		Confirmations: 2,
	}
	txWatcher.AddCltvTx(swapId, cltv)
	txWatcher.AddCltvPassedHandler(func(swapId string) error {
		go func() { txWatcherChan <- swapId }()
		return nil
	})
	txConfirmedId := <-txWatcherChan
	assert.Equal(t, swapId, txConfirmedId)
}

type DummyBlockchain struct {
	nextBlockheight uint64
	nextTxOutResp   *TxOutResp
}

func (d *DummyBlockchain) GetBlockHeight() (uint64, error) {
	return d.nextBlockheight, nil
}

func (d *DummyBlockchain) GetTxOut(txid string, vout uint32) (*TxOutResp, error) {
	return d.nextTxOutResp, nil
}
