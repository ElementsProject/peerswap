package lnd

import (
	"context"
	"encoding/hex"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/chainrpc"
)

type LndTxWatcher struct {
	chainNotifier chainrpc.ChainNotifierClient
	lnrpc         lnrpc.LightningClient

	network *chaincfg.Params

	txCallback        func(swapId string, txHex string) error
	csvPassedCallback func(swapId string) error

	ctx context.Context
}

func NewLndTxWatcher(ctx context.Context, chainNotifier chainrpc.ChainNotifierClient, lnrpc lnrpc.LightningClient, network *chaincfg.Params) *LndTxWatcher {
	return &LndTxWatcher{chainNotifier: chainNotifier, lnrpc: lnrpc, network: network, ctx: ctx}
}

func (l *LndTxWatcher) GetBlockHeight() (uint32, error) {
	gi, err := l.lnrpc.GetInfo(l.ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return 0, err
	}
	return gi.BlockHeight, nil
}

func (l *LndTxWatcher) AddWaitForConfirmationTx(swapId, txId string, vout, startingHeight uint32, outputScript []byte) {
	go func() {
		confDetails, err := l.listenConfirmationsNtfn(swapId, txId, startingHeight, onchain.BitcoinMinConfs, outputScript)
		if err != nil {
			log.Infof("error waiting for confirmation of tx %v", err)
			return
		}

		err = l.txCallback(swapId, hex.EncodeToString(confDetails.RawTx))
		if err != nil {
			log.Infof("error on callback %v", err)
			return
		}
	}()
}

func (l *LndTxWatcher) AddWaitForCsvTx(swapId, txId string, vout uint32, startingHeight uint32, outputScript []byte) {
	go func() {
		// get confirmation height of tx

		res, err := l.listenConfirmationsNtfn(swapId, txId, startingHeight, 1, outputScript)
		if err != nil {
			log.Infof("error waiting for confirmation of tx %v", err)
			return
		}

		// get current block height
		blockheight, err := l.GetBlockHeight()
		if err != nil {
			log.Infof("error getting blockheight %v", err)
			return
		}
		if blockheight-res.BlockHeight > onchain.BitcoinCsv-1 {
			err = l.csvPassedCallback(swapId)
			if err != nil {
				log.Infof("error on callback %v", err)
				return
			}
		} else {
			reached, err := l.listenForBlockheight(startingHeight, res.BlockHeight+onchain.BitcoinCsv-1)
			if err != nil {
				log.Infof("error on listening for blockheight %v", err)
				return
			}
			if reached {
				err = l.csvPassedCallback(swapId)
				if err != nil {
					log.Infof("error on callback %v", err)
					return
				}
			}
		}

	}()
}

func (l *LndTxWatcher) listenConfirmationsNtfn(swapId, txId string, startingHeight uint32, confirmations uint32, outputScript []byte) (*chainrpc.ConfDetails, error) {
	client, err := l.chainNotifier.RegisterConfirmationsNtfn(l.ctx, &chainrpc.ConfRequest{
		Txid:       make([]byte, 32),
		HeightHint: startingHeight,
		NumConfs:   confirmations,
		Script:     outputScript,
	})

	if err != nil {
		return nil, err
	}
	for {
		select {
		case <-l.ctx.Done():
			log.Infof("context done")
			return nil, nil
		default:
			res, err := client.Recv()
			if err != nil {
				return nil, err
			}
			log.Infof("confirmed %s", res.GetConf().String())
			return res.GetConf(), nil

		}
	}
}

func (l *LndTxWatcher) listenForBlockheight(startingHeight uint32, targetBlockheight uint32) (bool, error) {
	client, err := l.chainNotifier.RegisterBlockEpochNtfn(l.ctx, &chainrpc.BlockEpoch{
		Height: startingHeight,
	})
	if err != nil {
		return false, err
	}
	for {
		select {
		case <-l.ctx.Done():
			return false, nil
		default:
			res, err := client.Recv()
			if err != nil {
				return false, err
			}
			if res.Height >= targetBlockheight {
				return true, nil
			}
		}
	}
}

func (l *LndTxWatcher) AddConfirmationCallback(f func(swapId string, txHex string) error) {
	l.txCallback = f
}

func (l *LndTxWatcher) AddCsvCallback(f func(swapId string) error) {
	l.csvPassedCallback = f
}
