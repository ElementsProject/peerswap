package swap

import (
	"context"
	"github.com/sputn1ck/sugarmama/wallet"
	"log"
	"sync"
	"time"
)

type txWatcher struct {
	blockchain wallet.BlockchainService

	txWatchList map[string]string
	timelockWatchlist map[string] int64
	txCallback  func(swapId string) error
	timelockCallback func(swapId string) error
	newBlockChan chan int
	sync.Mutex
	ctx context.Context
}

func newTxWatcher(ctx context.Context, blockchain wallet.BlockchainService, txCallback func(swapId string) error) *txWatcher {
	return &txWatcher{
		blockchain:  blockchain,
		txCallback:  txCallback,
		txWatchList: make(map[string]string),
		ctx:         ctx,
		newBlockChan: make(chan int),
	}
}

func (t *txWatcher) AddTx(swapId string, txId string) {
	t.Lock()
	t.txWatchList[swapId] = txId
	t.Unlock()
}

func (t *txWatcher) AddTimeLockTx(swapId string, blockheight int64) {
	t.Lock()
	t.timelockWatchlist[swapId] = blockheight
	t.Unlock()
}

func (s *txWatcher) StartWatchingTxs(swaps []*Swap) error {
	for _, v := range swaps {
		if v.State == SWAPSTATE_WAITING_FOR_TX {
			s.AddTx(v.Id, v.OpeningTxId)
		}
		if v.State == SWAPSTATE_OPENING_TX_BROADCASTED {
			s.AddTimeLockTx(v.Id, v.Cltv)
		}
	}
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case nb := <-s.newBlockChan:
			err := s.HandleConfirmedTx(nb)
			if err != nil {
				return err
			}
			err = s.HandleTimelockTx(nb)
			if err != nil {
				return err
			}
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func (s *txWatcher) StartBlockWatcher() error{
	currentBlock, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return err
	}
	for {
		select {
		case <-s.ctx.Done():
		default:
			nextBlock, err := s.blockchain.GetBlockHeight()
			if err != nil {
				return err
			}
			if nextBlock > currentBlock {
				currentBlock = nextBlock
				s.newBlockChan <- currentBlock
			}
			time.Sleep(1 * time.Second)
		}
	}
}
func (s *txWatcher) HandleTimelockTx(blockheight int) error {
	s.Lock()
	var toRemove []string
	for k,v := range s.timelockWatchlist {
		if v >= int64(blockheight) {
			continue
		}
		err := s.timelockCallback(k)
		if err != nil {
			return err
		}
		toRemove = append(toRemove, k)
	}
	s.Unlock()
	for _, v := range toRemove {
		delete(s.txWatchList, v)
	}
	return nil
}
func (s *txWatcher) HandleConfirmedTx(blockheight int) error{
		var toRemove []string
		s.Lock()
		for k, v := range s.txWatchList {
			res, err := s.blockchain.FetchTx(v)
			if err != nil {
				log.Printf("watchlist fetchtx err: %v", err)
				continue
			}
			if !res.Status.Confirmed {
				log.Printf("tx is not yet confirmed")
				continue
			}
			if blockheight - res.Status.BlockHeight < 1 {
				log.Printf("tx needs 2 confirmation")
				continue
			}

			err = s.txCallback(k)
			if err != nil {
				log.Printf("tx callback error %v", err)
				continue
			}

			toRemove = append(toRemove, k)
		}
		s.Unlock()
		for _, v := range toRemove {
			delete(s.txWatchList, v)
		}
		return nil
}

func (s *txWatcher) TxClaimed(swapId string) {
	s.Lock()
	delete(s.txWatchList, swapId)
	delete(s.timelockWatchlist, swapId)
	s.Unlock()
}