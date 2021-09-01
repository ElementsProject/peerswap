package txwatcher

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type BlockchainRpc interface {
	GetBlockHeight() (uint64, error)
	GetTxOut(txid string, vout uint32) (*TxOutResp, error)
}

type TxOutResp struct {
	BestBlockHash string  `json:"bestblock"`
	Confirmations uint32  `json:"confirmations"`
	Value         float64 `json:"value"`
	Coinbase      bool    `json:"coinbase"`
}

type SwapTxInfo struct {
	TxId   string
	TxVout string
	Cltv   int64
}

// todo zmq notifications

// BlockchainRpcTxWatcher handles notifications of confirmed and cltv-passed events
type BlockchainRpcTxWatcher struct {
	blockchain BlockchainRpc

	txCallback       func(swapId string) error
	timelockCallback func(swapId string) error

	txWatchList       map[string]string
	timelockWatchlist map[string]int64
	newBlockChan      chan uint64
	requiredConfs     uint32
	ctx               context.Context
	sync.Mutex
}

func NewBlockchainRpcTxWatcher(ctx context.Context, blockchain BlockchainRpc, requiredConfs uint32) *BlockchainRpcTxWatcher {
	return &BlockchainRpcTxWatcher{
		ctx:               ctx,
		blockchain:        blockchain,
		txWatchList:       make(map[string]string),
		timelockWatchlist: make(map[string]int64),
		newBlockChan:      make(chan uint64),
		requiredConfs:     requiredConfs,
	}
}

// StartWatchingTxs starts the txwatcher
func (s *BlockchainRpcTxWatcher) StartWatchingTxs() error {
	if s.blockchain == nil {
		return fmt.Errorf("missing blockchain rpc client")
	}

	currentBlock, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return err
	}

	go s.StartBlockWatcher(currentBlock)
	go func() error {
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
	}()
	return nil
}

// StartBlockWatcher starts listening for new blocks
func (s *BlockchainRpcTxWatcher) StartBlockWatcher(currentBlock uint64) error {
	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
		case <-ticker.C:
			nextBlock, err := s.blockchain.GetBlockHeight()
			if err != nil {
				return err
			}
			if nextBlock > currentBlock {
				currentBlock = nextBlock
				s.Lock()
				s.newBlockChan <- currentBlock
				s.Unlock()
			}
		}
	}
}

// HandleTimelockTx looks for transactions that have passed their cltv height
func (s *BlockchainRpcTxWatcher) HandleTimelockTx(blockheight uint64) error {
	s.Lock()
	var toRemove []string
	for k, v := range s.timelockWatchlist {
		if v >= int64(blockheight) {
			continue
		}
		log.Printf("timelock triggered")
		if s.timelockCallback == nil {
			continue
		}
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

// HandleConfirmedTx looks for transactions that are confirmed
func (s *BlockchainRpcTxWatcher) HandleConfirmedTx(blockheight uint64) error {
	var toRemove []string
	s.Lock()
	for k, v := range s.txWatchList {
		// todo does vout matter here?
		res, err := s.blockchain.GetTxOut(v, 0)
		if err != nil {
			log.Printf("watchlist fetchtx err: %v", err)
			continue
		}
		if res == nil {
			continue
		}
		if !(res.Confirmations >= s.requiredConfs) {
			log.Printf("tx does not have enough confirmations")
			continue
		}
		if s.txCallback == nil {
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

func (l *BlockchainRpcTxWatcher) AddConfirmationsTx(swapId, txId string) {
	l.Lock()
	defer l.Unlock()
	l.txWatchList[swapId] = txId
}

func (l *BlockchainRpcTxWatcher) AddCltvTx(swapId string, cltv int64) {
	l.Lock()
	defer l.Unlock()
	l.timelockWatchlist[swapId] = cltv
}

func (l *BlockchainRpcTxWatcher) TxClaimed(swaps []string) {
	l.Lock()
	defer l.Unlock()
	for _, v := range swaps {
		delete(l.txWatchList, v)
		delete(l.timelockWatchlist, v)
	}
}

func (l *BlockchainRpcTxWatcher) AddTxConfirmedHandler(f func(swapId string) error) {
	l.Lock()
	defer l.Unlock()
	l.txCallback = f
}

func (l *BlockchainRpcTxWatcher) AddCltvPassedHandler(f func(swapId string) error) {
	l.Lock()
	defer l.Unlock()
	l.timelockCallback = f
}
