package txwatcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"
)

type BlockchainRpc interface {
	GetBlockHeight() (uint64, error)
	GetTxOut(txid string, vout uint32) (*TxOutResp, error)
	GetBlockHash(height uint32) (string, error)
	GetRawtransactionWithBlockHash(txId string, blockHash string) (string, error)
	GetBlockHeightByHash(blockhash string) (uint32, error)
}

type TxOutResp struct {
	BestBlockHash string  `json:"bestblock"`
	Confirmations uint32  `json:"confirmations"`
	Value         float64 `json:"value"`
	Coinbase      bool    `json:"coinbase"`
}

type SwapTxInfo struct {
	TxId                string
	TxVout              uint32
	StartingBlockHeight uint32
	Csv                 uint32
}

// todo zmq notifications

// BlockchainRpcTxWatcher handles notifications of confirmed and csv-passed events
type BlockchainRpcTxWatcher struct {
	blockchain BlockchainRpc

	txCallback        func(swapId string, txHex string) error
	csvPassedCallback func(swapId string) error

	txWatchList    map[string]*SwapTxInfo
	csvtxWatchList map[string]*SwapTxInfo
	newBlockChan   chan uint64

	requiredConfs uint32
	csv           uint32

	ctx context.Context
	sync.Mutex
}

func (s *BlockchainRpcTxWatcher) GetBlockHeight() (uint32, error) {
	if s.blockchain == nil {
		return 0, fmt.Errorf("missing blockchain rpc client")
	}

	blockheight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return 0, err
	}
	return uint32(blockheight), nil
}

func NewBlockchainRpcTxWatcher(ctx context.Context, blockchain BlockchainRpc, requiredConfs uint32, csv uint32) *BlockchainRpcTxWatcher {
	return &BlockchainRpcTxWatcher{
		ctx:            ctx,
		csv:            csv,
		blockchain:     blockchain,
		txWatchList:    make(map[string]*SwapTxInfo),
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
				// This is a blocking action so we need to spawn it in a separate go routine if we do not want to take
				// risk of deadlocks.
				// Todo: How to care about errors?
				go func() {
					err := s.HandleConfirmedTx(nb)
					if err != nil {
						log.Debugf("HandleConfirmedTx: %v", err)
					}
				}()
				// Todo: Maybe the same goes for the HandleCsvTx.
				err = s.HandleCsvTx(nb)
				if err != nil {
					return err
				}
			default:
				time.Sleep(100 * time.Millisecond)
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
// fixme: why does this function return an error if no error ever is returned?
func (s *BlockchainRpcTxWatcher) HandleConfirmedTx(blockheight uint64) error {
	var toRemove []string
	s.Lock()
	for k, v := range s.txWatchList {
		// todo does vout matter here?
		res, err := s.blockchain.GetTxOut(v.TxId, v.TxVout)
		if err != nil {
			log.Infof("Watchlist fetchtx err: %v", err)
			continue
		}
		if res == nil {
			continue
		}
		if !(res.Confirmations >= s.requiredConfs) {
			log.Debugf("tx does not have enough confirmations")
			continue
		}
		if s.txCallback == nil {
			continue
		}
		txHex, err := s.TxHexFromId(res, v.TxId)
		if err != nil {
			return err
		}
		err = s.txCallback(k, txHex)
		if err != nil {
			log.Infof("tx callback error %v", err)
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
		res, err := s.blockchain.GetTxOut(v.TxId, v.TxVout)
		if err != nil {
			log.Infof("watchlist fetchtx err: %v", err)
			continue
		}
		if res == nil {
			continue
		}
		if v.Csv > res.Confirmations {
			continue
		}
		if s.csvPassedCallback == nil {
			continue
		}
		err = s.csvPassedCallback(k)
		if err != nil {
			log.Infof("tx callback error %v", err)
			continue
		}
		toRemove = append(toRemove, k)
	}
	s.Unlock()
	s.TxClaimed(toRemove)
	return nil
}

func (l *BlockchainRpcTxWatcher) AddWaitForConfirmationTx(swapId, txId string, vout, startingBlockheight uint32, _ []byte) {
	hex := l.CheckTxConfirmed(swapId, txId, vout)
	if hex != "" {
		go func() {
			err := l.txCallback(swapId, hex)
			if err != nil {
				log.Infof("tx callback error %v", err)
				return
			}
		}()
		return
	}
	l.Lock()
	defer l.Unlock()
	l.txWatchList[swapId] = &SwapTxInfo{
		TxId:                txId,
		TxVout:              vout,
		Csv:                 l.csv,
		StartingBlockHeight: startingBlockheight,
	}
}

func (s *BlockchainRpcTxWatcher) CheckTxConfirmed(swapId string, txId string, vout uint32) string {
	res, err := s.blockchain.GetTxOut(txId, vout)
	if err != nil {
		log.Infof("watchlist fetchtx err: %v", err)
		return ""
	}
	if res == nil {
		return ""
	}
	if !(res.Confirmations >= s.requiredConfs) {
		log.Infof("tx does not have enough confirmations")
		return ""
	}
	if s.txCallback == nil {
		return ""
	}
	txHex, err := s.TxHexFromId(res, txId)
	if err != nil {
		log.Infof("watchlist txfrom hex err: %v", err)
		return ""
	}

	return txHex
}

func (l *BlockchainRpcTxWatcher) checkTxAboveCsvHight(txId string, vout uint32) (bool, error) {
	res, err := l.blockchain.GetTxOut(txId, vout)
	if err != nil {
		return false, err
	}
	if res == nil {
		return false, fmt.Errorf("empty gettxout response")
	}
	return res.Confirmations >= l.csv, nil
}

func (l *BlockchainRpcTxWatcher) AddWaitForCsvTx(swapId, txId string, vout uint32, startingBlockheight uint32, _ []byte) {
	// Before we add the tx to the watcher we check if the tx is already
	// above the csv limit.
	above, err := l.checkTxAboveCsvHight(txId, vout)
	if err != nil {
		log.Infof("[TxWatcher] checkTxAboveCsvHeight returned: %s", err.Error())
	}
	if above {
		err = l.csvPassedCallback(swapId)
		if err == nil {
			log.Infof("Swap %s already past CSV limit", swapId)
			return
		}
		log.Infof("csv passed callback error: %v", err)
	}

	l.Lock()
	defer l.Unlock()
	l.csvtxWatchList[swapId] = &SwapTxInfo{
		TxId:                txId,
		TxVout:              vout,
		Csv:                 l.csv,
		StartingBlockHeight: startingBlockheight,
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

func (l *BlockchainRpcTxWatcher) AddConfirmationCallback(f func(swapId string, txHex string) error) {
	l.Lock()
	defer l.Unlock()
	l.txCallback = f
}

func (l *BlockchainRpcTxWatcher) AddCsvCallback(f func(swapId string) error) {
	l.Lock()
	defer l.Unlock()
	l.csvPassedCallback = f
}

func (l *BlockchainRpcTxWatcher) TxHexFromId(resp *TxOutResp, txId string) (string, error) {
	blockheight, err := l.blockchain.GetBlockHeightByHash(resp.BestBlockHash)
	if err != nil {
		return "", err
	}

	blockhash, err := l.blockchain.GetBlockHash(uint32(blockheight) - resp.Confirmations + 1)
	if err != nil {
		return "", err
	}

	rawTxHex, err := l.blockchain.GetRawtransactionWithBlockHash(txId, blockhash)
	if err != nil {
		return "", err
	}
	return rawTxHex, nil
}
