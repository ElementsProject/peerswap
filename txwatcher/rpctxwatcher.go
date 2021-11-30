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
	TxVout uint32
	Csv    uint32
}

// todo zmq notifications

// BlockchainRpcTxWatcher handles notifications of confirmed and csv-passed events
type BlockchainRpcTxWatcher struct {
	blockchain BlockchainRpc

	txCallback        func(swapId string) error
	csvPassedCallback func(swapId string) error

	txWatchList    map[string]string
	csvtxWatchList map[string]*SwapTxInfo
	newBlockChan   chan uint64

	requiredConfs uint32
	csv           uint32

	ctx context.Context
	sync.Mutex
}

func (s *BlockchainRpcTxWatcher) GetBlockHeight() (uint32, error) {
	blockheight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return 0, err
	}
	return uint32(blockheight), nil
}

func NewBlockchainRpcTxWatcher(ctx context.Context, blockchain BlockchainRpc, requiredConfs uint32, csv uint32) *BlockchainRpcTxWatcher {
	return &BlockchainRpcTxWatcher{
		ctx:            ctx,
		blockchain:     blockchain,
		txWatchList:    make(map[string]string),
		csvtxWatchList: make(map[string]*SwapTxInfo),
		newBlockChan:   make(chan uint64),
		requiredConfs:  requiredConfs,
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
				err = s.HandleCsvTx(nb)
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

// HandleCsvTx looks for transactions that have enough confirmations to be spend using the csv path
func (s *BlockchainRpcTxWatcher) HandleCsvTx(blockheight uint64) error {
	var toRemove []string
	s.Lock()
	for k, v := range s.csvtxWatchList {
		// todo does vout matter here?
		res, err := s.blockchain.GetTxOut(v.TxId, v.TxVout)
		if err != nil {
			log.Printf("watchlist fetchtx err: %v", err)
			continue
		}
		if res == nil {
			continue
		}
		if v.Csv > res.Confirmations {
			continue
		}
		log.Printf("watchlist want to claim: csv %v confs %v vout %v txid %s", v.Csv, res.Confirmations, v.TxVout, v.TxId)
		if s.csvPassedCallback == nil {
			continue
		}
		err = s.csvPassedCallback(k)
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
func (l *BlockchainRpcTxWatcher) AddWaitForConfirmationTx(swapId, txId string, startingHeight uint32, scriptpubkey []byte) {
	l.Lock()
	defer l.Unlock()
	l.txWatchList[swapId] = txId
}

func (l *BlockchainRpcTxWatcher) AddWaitForCsvTx(swapId, txId string, vout uint32, startingHeight uint32, scriptpubkey []byte) {
	l.Lock()
	defer l.Unlock()
	l.csvtxWatchList[swapId] = &SwapTxInfo{
		TxId:   txId,
		TxVout: vout,
		Csv:    l.csv,
	}
}

func (l *BlockchainRpcTxWatcher) TxClaimed(swaps []string) {
	l.Lock()
	defer l.Unlock()
	for _, v := range swaps {
		delete(l.txWatchList, v)
		delete(l.csvtxWatchList, v)
	}
}

func (l *BlockchainRpcTxWatcher) AddConfirmationCallback(f func(swapId string) error) {
	l.Lock()
	defer l.Unlock()
	l.txCallback = f
}

func (l *BlockchainRpcTxWatcher) AddCsvCallback(f func(swapId string) error) {
	l.Lock()
	defer l.Unlock()
	l.csvPassedCallback = f
}
