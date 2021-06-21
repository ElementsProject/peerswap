package swap

import (
	"context"
	"github.com/sputn1ck/peerswap/blockchain"
	"log"
	"sync"
	"time"
)

const (
	minconfs = 2
)

type SwapWatcher struct {
	blockchain        blockchain.Blockchain
	txWatchList       map[string]string
	timelockWatchlist map[string]int64
	SwapMap           map[string]*Swap
	txCallback        func(swapId string) error
	timelockCallback  func(swapId string) error
	newBlockChan      chan uint64
	sync.Mutex
	ctx context.Context
}

func newTxWatcher(ctx context.Context, blockchain blockchain.Blockchain, txCallback func(swapId string) error, timelockCallback func(swapId string) error) *SwapWatcher {
	return &SwapWatcher{
		blockchain:        blockchain,
		txCallback:        txCallback,
		txWatchList:       make(map[string]string),
		timelockWatchlist: make(map[string]int64),
		timelockCallback:  timelockCallback,
		SwapMap:           make(map[string]*Swap),
		ctx:               ctx,
		newBlockChan:      make(chan uint64),
	}
}

func (t *SwapWatcher) AddSwap(swap *Swap) {
	t.Lock()
	t.SwapMap[swap.Id] = swap
	if swap.Role == SWAPROLE_TAKER {
		log.Printf("adding swap to watchlist")
		t.txWatchList[swap.Id] = swap.OpeningTxId
	} else {
		log.Printf("adding swap to cltv watchlist")
		t.timelockWatchlist[swap.Id] = swap.Cltv
	}
	t.Unlock()
}

func (s *SwapWatcher) StartWatchingTxs(swaps []*Swap) error {
	go s.StartBlockWatcher()
	for _, v := range swaps {
		if v.State == SWAPSTATE_WAITING_FOR_TX || v.State == SWAPSTATE_OPENING_TX_BROADCASTED {
			s.AddSwap(v)
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

func (s *SwapWatcher) StartBlockWatcher() error {
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
			time.Sleep(1000 * time.Millisecond)
		}
	}
}
func (s *SwapWatcher) HandleTimelockTx(blockheight uint64) error {
	s.Lock()
	var toRemove []string
	for k, v := range s.timelockWatchlist {
		if v >= int64(blockheight) {
			continue
		}
		log.Printf("timelock triggered")
		err := s.timelockCallback(k)
		if err != nil {
			return err
		}
		toRemove = append(toRemove, k)
	}
	s.Unlock()
	s.TxClaimed(toRemove)
	return nil
}
func (s *SwapWatcher) HandleConfirmedTx(blockheight uint64) error {
	var toRemove []string
	s.Lock()
	for k, v := range s.txWatchList {
		swap := s.SwapMap[k]
		res, err := s.blockchain.GetTxOut(v, swap.OpeningTxVout)
		if err != nil {
			log.Printf("watchlist fetchtx err: %v", err)
			continue
		}
		log.Printf("txout: %v", res)
		if res == nil {
			continue
		}
		if !(res.Confirmations > 1) {
			log.Printf("tx does not have enough confirmations")
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
	s.TxClaimed(toRemove)
	return nil
}

func (s *SwapWatcher) TxClaimed(swaps []string) {
	s.Lock()
	for _, v := range swaps {
		delete(s.SwapMap, v)
		delete(s.txWatchList, v)
		delete(s.timelockWatchlist, v)
	}
	s.Unlock()
}
