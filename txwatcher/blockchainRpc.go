package txwatcher

import (
	"errors"
	"fmt"

	"github.com/elementsproject/glightning/gbitcoin"
	"github.com/elementsproject/glightning/gelements"
	"github.com/elementsproject/peerswap/log"
)

type ElementsBlockChainRpc struct {
	ecli *gelements.Elements
}

func (e *ElementsBlockChainRpc) GetBlockHeightByHash(blockhash string) (uint32, error) {
	res, err := e.ecli.GetBlockHeader(blockhash)
	if err != nil {
		return 0, err
	}
	return res.Height, nil
}

func NewElementsCli(ecli *gelements.Elements) *ElementsBlockChainRpc {
	return &ElementsBlockChainRpc{ecli: ecli}
}

func (e *ElementsBlockChainRpc) GetBlockHeight() (uint64, error) {
	return e.ecli.GetBlockHeight()
}

func (e *ElementsBlockChainRpc) GetTxOut(txid string, vout uint32) (*TxOutResp, error) {
	txout, err := e.ecli.GetTxOut(txid, vout)
	if err != nil {
		return nil, err
	}
	if txout == nil {
		return nil, nil
	}
	return &TxOutResp{
		Confirmations: txout.Confirmations,
		Value:         txout.Value,
		BestBlockHash: txout.BestBlockHash,
	}, nil
}

func (e *ElementsBlockChainRpc) String() string {
	return "lbtc"
}

func (e *ElementsBlockChainRpc) GetBlockHash(height uint32) (string, error) {
	return e.ecli.GetBlockHash(height)
}

func (e *ElementsBlockChainRpc) GetRawtransactionWithBlockHash(txId string, blockHash string) (string, error) {
	return e.ecli.GetRawtransactionWithBlockHash(txId, blockHash)
}

type BitcoinBlockchainRpc struct {
	bcli *gbitcoin.Bitcoin
}

func NewBitcoinRpc(bcli *gbitcoin.Bitcoin) *BitcoinBlockchainRpc {
	return &BitcoinBlockchainRpc{bcli: bcli}
}

func (b *BitcoinBlockchainRpc) GetBlockHeight() (uint64, error) {
	return b.bcli.GetBlockHeight()
}

func (b *BitcoinBlockchainRpc) GetTxOut(txid string, vout uint32) (*TxOutResp, error) {
	txout, err := b.bcli.GetTxOut(txid, vout)
	if err != nil {
		return nil, err
	}
	if txout == nil {
		return nil, nil
	}
	return &TxOutResp{
		Confirmations: txout.Confirmations,
		Value:         txout.Value,
		BestBlockHash: txout.BestBlockHash,
	}, nil
}

func (b *BitcoinBlockchainRpc) GetBlockHeightByHash(blockhash string) (uint32, error) {
	res, err := b.bcli.GetBlockHeader(blockhash)
	if err != nil {
		return 0, err
	}
	return res.Height, nil
}

func (b *BitcoinBlockchainRpc) String() string {
	return "btc"
}
func (b *BitcoinBlockchainRpc) GetBlockHash(height uint32) (string, error) {
	return b.bcli.GetBlockHash(height)
}

func (b *BitcoinBlockchainRpc) GetRawtransactionWithBlockHash(txId string, blockHash string) (string, error) {
	return b.bcli.GetRawtransactionWithBlockHash(txId, blockHash)
}

type CommonBlockchainObserver struct {
	blockchain BlockchainRpc
}

func NewCommonBlockchainObserver(blockchain BlockchainRpc) *CommonBlockchainObserver {
	return &CommonBlockchainObserver{blockchain: blockchain}
}

var ErrNotFound = errors.New("not found")
var ErrUnconfirmed = errors.New("unconfirmed")
var ErrOutOfSync = errors.New("out of sync")
var ErrBlockHashMismatch = errors.New("block hash mismatch")
var ErrContextCanceled = errors.New("context canceled")

// IsTxInRange returns the transaction with the given txId if it is found in the
// given block range and the blockheight it was found at. If it could not be
// found ErrNotFound is returned.
func (b *CommonBlockchainObserver) IsTxInRange(txId string, startBlock, endBlock uint32) (string, uint32, error) {
	if endBlock < startBlock {
		return "", 0, fmt.Errorf("expected start_block < end_block: %d < %d ", startBlock, endBlock)
	}

	for i := startBlock; i <= endBlock; i++ {
		h, err := b.blockchain.GetBlockHash(i)
		if err != nil {
			return "", 0, err
		}

		// The `getrawtransaction` function by default only works on the
		// mempool. If called with a blockhash, the raw transaction is returned
		// if the block is available (exists and the block is not pruned) and
		// the transaction is found in the block.
		// That is why we do not return on error, but rather log the error. It
		// is to our concern if the returned string is empty or not.
		rtx, _ := b.blockchain.GetRawtransactionWithBlockHash(txId, h)
		if rtx != "" {
			// Did find tx.
			return rtx, i, nil
		}
	}

	// Did NOT find tx.
	return "", 0, ErrNotFound
}

func (b *CommonBlockchainObserver) IsTxInMempoolOrRange(txId string, startHeight, vout uint32) (string, uint32, error) {
	ctmp, err := b.blockchain.GetBlockHeight()
	if err != nil {
		return "", 0, fmt.Errorf("could not get current block height: %v", err)
	}
	current := uint32(ctmp)
	bHash, err := b.blockchain.GetBlockHash(current)
	if err != nil {
		return "", 0, fmt.Errorf("could not get current block hash: %v", err)
	}

	// Check if the tx is in the mempool or if it has been confirmed in the
	// current block.
	txInfo, err := b.blockchain.GetTxOut(txId, vout)
	if err != nil {
		return "", 0, fmt.Errorf(
			"error calling gettxout(%s, %d): %v",
			txId, vout, err)
	}
	if txInfo != nil {
		var txBlockHash string
		if txInfo.BestBlockHash != bHash {
			// The block hashes should match.
			log.Infof(
				"block watcher might be out of sync: current_block=%s, best_block=%s",
				bHash, txInfo.BestBlockHash)
			return "", 0, ErrOutOfSync
		}
		if txInfo.Confirmations == 0 {
			// Tx is in mempool
			return "", 0, ErrUnconfirmed
		} else if txInfo.Confirmations == 1 {
			// Tx was confirmed in the current block, same block hash.
			txBlockHash = bHash
		} else {
			// Tx has confirmations we have to substract them from current to
			// get the height the tx was first seen.
			current = current + 1 - txInfo.Confirmations
			txBlockHash, err = b.blockchain.GetBlockHash(
				current,
			)
			if err != nil {
				return "", 0, fmt.Errorf(
					"could not get current block hash: %v", err,
				)
			}
		}
		rtx, err := b.blockchain.GetRawtransactionWithBlockHash(txId, txBlockHash)
		if err != nil {
			log.Infof(
				"unforeseen block hash mismatch: tx_id=%s, block_hash=%s: %v",
				txId, bHash, err)
			return "", 0, ErrBlockHashMismatch
		}
		return rtx, current, nil
	}

	// The transaction could not be found in the mempool or the transaction
	// output has already been spent. We double check. If the Tx was spent in
	// the past we should still find it in the range of starting the swap until
	// now. If we can not find the tx in this range this means that the tx is
	// not yet in our view of the mempool.
	return b.IsTxInRange(txId, startHeight, current)
}
