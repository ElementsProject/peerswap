package txwatcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"
)

var ErrCookieAuthFailed = errors.New("Authorization failed: Incorrect user or password")

type BlockchainRpc interface {
	GetBlockHeight() (uint64, error)
	GetTxOut(txid string, vout uint32) (*TxOutResp, error)
	GetBlockHash(height uint32) (string, error)
	GetRawtransactionWithBlockHash(txId string, blockHash string) (string, error)
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

type observerInfo struct {
	cancel    context.CancelFunc
	blockChan chan uint32
}

// todo zmq notifications

// BlockchainRpcTxWatcher handles notifications of confirmed and csv-passed events
type BlockchainRpcTxWatcher struct {
	observer   *CommonBlockchainObserver
	blockchain BlockchainRpc

	txCallback        func(swapId string, txHex string, err error) error
	csvPassedCallback func(swapId string) error

	txWatchList    map[string]*SwapTxInfo
	csvtxWatchList map[string]*SwapTxInfo
	newBlockChan   chan uint64

	observerLoopList map[string]observerInfo

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
		ctx:              ctx,
		csv:              csv,
		blockchain:       blockchain,
		txWatchList:      make(map[string]*SwapTxInfo),
		csvtxWatchList:   make(map[string]*SwapTxInfo),
		newBlockChan:     make(chan uint64),
		requiredConfs:    requiredConfs,
		observerLoopList: make(map[string]observerInfo),
		observer:         &CommonBlockchainObserver{blockchain: blockchain},
	}
}

// StartWatchingTxs starts the txwatcher
func (s *BlockchainRpcTxWatcher) StartWatchingTxs() error {
	if s.blockchain == nil {
		return fmt.Errorf("missing blockchain rpc client")
	}

	go s.StartBlockWatcher()
	go func() error {
		for {
			select {
			case <-s.ctx.Done():
				return nil
			case nb := <-s.newBlockChan:
				for _, obs := range s.observerLoopList {
					go func(height uint32) { obs.blockChan <- height }(uint32(nb))
				}
				// Todo: HandleCsvTx could also need a refresh.
				err := s.HandleCsvTx(nb)
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
func (s *BlockchainRpcTxWatcher) StartBlockWatcher() error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastHeight uint64
	var lastHash string
	logged := 0
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-ticker.C:
			nextHeight, err := s.blockchain.GetBlockHeight()
			if err != nil {
				if logged == 0 && err.Error() != ErrCookieAuthFailed.Error() {
					log.Infof("block watcher: %v, %v", s.blockchain, err)
					logged++
				}
				if err.Error() == ErrCookieAuthFailed.Error() {
					log.Infof("block watcher: %v, %v", s.blockchain, err)
					time.Sleep(1 * time.Second)
					os.Exit(1)
				}
			}
			nextHash, err := s.blockchain.GetBlockHash(uint32(nextHeight))
			if err != nil {
				if logged == 0 && err.Error() != ErrCookieAuthFailed.Error() {
					log.Infof("block watcher: %v, %v", s.blockchain, err)
					logged++
				}
				if err.Error() == ErrCookieAuthFailed.Error() {
					log.Infof("block watcher: %v, %v", s.blockchain, err)
					time.Sleep(1 * time.Second)
					os.Exit(1)
				}
			}
			if err == nil && logged != 0 {
				log.Infof("block watcher: reconnected to %v daemon", s.blockchain)
				logged = 0
			}
			if nextHeight > lastHeight || nextHash != lastHash {
				lastHeight = nextHeight
				lastHash = nextHash
				s.newBlockChan <- nextHeight
			}
		}
	}
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
	log.Infof("adding tx watcher for %s", swapId)
	ctx, cancel := context.WithCancel(context.Background())
	newBlock := make(chan uint32)
	info := observerInfo{
		cancel:    cancel,
		blockChan: newBlock,
	}
	go l.observationLoop(ctx, swapId, txId, vout, startingBlockheight, l.csv/2, newBlock)
	l.Lock()
	defer l.Unlock()
	l.observerLoopList[swapId] = info

	// Kick off first run manually, after that is only invoked on new blocks.
	height, _ := l.blockchain.GetBlockHeight()
	newBlock <- uint32(height)
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

func (l *BlockchainRpcTxWatcher) AddConfirmationCallback(f func(swapId, txHex string, err error) error) {
	l.Lock()
	defer l.Unlock()
	l.txCallback = f
}

func (l *BlockchainRpcTxWatcher) AddCsvCallback(f func(swapId string) error) {
	l.Lock()
	defer l.Unlock()
	l.csvPassedCallback = f
}

func (l *BlockchainRpcTxWatcher) observationLoop(
	ctx context.Context,
	swapId,
	txId string,
	vout,
	startingHeight,
	safetyLimit uint32,
	newBlock chan uint32,
) {
	// Deletes itself from the list after completion.
	defer func() {
		l.Lock()
		delete(l.observerLoopList, swapId)
		l.Unlock()
	}()

	log.Debugf("starting chain observer for %s", swapId)
	var lastHeight uint32
	for {
		select {
		case <-ctx.Done():
			// We got told to stop observing the chain.
			l.callbackAndLog(swapId, "", ErrContextCanceled)
		case height := <-newBlock:
			log.Debugf(
				"new block height=%v, starting_height=%d, safety_limit=%d for %s",
				height,
				startingHeight,
				safetyLimit,
				swapId,
			)
			current := height

			if current <= lastHeight {
				// We already got this block, wait for the next.
				continue
			}
			lastHeight = current

			// Check if we are outside of our safety limits. This can happen on
			// restart. If the current block height is above our safety limits,
			// we cancel the swap.
			if current >= startingHeight+safetyLimit {
				l.callbackAndLog(swapId, "", fmt.Errorf("exceeded csv limit"))
				return
			}

			// Check if we can find the tx
			rawTx, firstSeen, err := l.observer.IsTxInMempoolOrRange(
				txId, startingHeight, vout)
			if errors.Is(err, ErrNotFound) {
				// Tx was not found from the "Starting Blockheight" until now.
				// Wait for the next block
				continue
			} else if errors.Is(err, ErrUnconfirmed) {
				// Tx was found in mempool but is unconfirmed. Wait for the next
				// block.
				continue
			} else if err != nil {
				// Something serious happened better cancel the swap.
				l.callbackAndLog(swapId, "", err)
				return
			}

			// Check that the amount of confirmation matches with what we expect
			// First check that we are in a safe range.
			if firstSeen > startingHeight+safetyLimit {
				l.callbackAndLog(swapId, "", fmt.Errorf("exceeded csv limit"))
				return
			}

			// Now check if we got enough confirmations. We use first seen - 1
			// as this is the block the tx was confirmed in the first time.
			if current-(firstSeen-1) >= l.requiredConfs {
				// We finally made it, enough confirmations and below the safety
				// limit!
				l.callbackAndLog(swapId, rawTx, nil)
				return
			}
		}
	}
}

func (l *BlockchainRpcTxWatcher) callbackAndLog(swapId, rawTx string, err error) {
	e := l.txCallback(swapId, rawTx, err)
	if e != nil {
		log.Infof("[swapId=%s] error calling confirmation callback: %v",
			swapId, err)
	}
}
