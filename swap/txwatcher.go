package swap

import (
	"context"
	"github.com/sputn1ck/sugarmama/wallet"
	"github.com/vulpemventures/go-elements/transaction"
	"log"
	"sync"
	"time"
)

type txWatcher struct {

	blockchain wallet.BlockchainService

	txWatchList map[string]string
	txCallback func(swapId string, tx *transaction.Transaction) error
	sync.Mutex
	ctx context.Context
}

func newTxWatcher(ctx context.Context, blockchain wallet.BlockchainService, txCallback func(swapId string, tx *transaction.Transaction) error) *txWatcher {
	return &txWatcher{
		blockchain: blockchain,
		txCallback: txCallback,
		txWatchList: make(map[string]string),
		ctx: ctx,
	}
}

func (t *txWatcher) AddTx(swapId string, txId string) {
	t.Lock()
	t.txWatchList[swapId] = txId
	t.Unlock()
}


func (s *txWatcher) StartWatchingTxs(swaps []*Swap) error {
	for _, v := range swaps {
		if v.State == SWAPSTATE_WAITING_FOR_TX {
			s.AddTx(v.Id,v.OpeningTxId)
		}
	}
	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
			var toRemove []string
			s.Lock()
			for k, v := range s.txWatchList {
				res, err := s.blockchain.FetchTxHex(v)
				if err != nil {
					log.Printf("watchlist err: %v", err)
					continue
				}
				if res != res {
					log.Printf("\n tx is not equal to sent tx")
				}
				tx, err := transaction.NewTxFromHex(res)
				if err != nil {
					log.Printf("tx err %v", err)
					continue
				}
				err = s.txCallback(k, tx)
				if err != nil {
					log.Printf("tx callback error %v", err)
					continue
				}

				toRemove = append(toRemove, k)
			}
			for _, v := range toRemove {
				delete(s.txWatchList, v)
			}
			s.Unlock()
			time.Sleep(1 * time.Second)
		}
	}
}
