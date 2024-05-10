package lwk

import (
	"context"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/elementsproject/peerswap/electrum"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/swap"
)

// initialBlockHeaderSubscriptionTimeout is
// the initial block header subscription timeout.
const initialBlockHeaderSubscriptionTimeout = 1000 * time.Second

type electrumTxWatcher struct {
	electrumClient       electrum.RPC
	blockHeight          electrum.BlocKHeight
	subscriber           electrum.BlockHeaderSubscriber
	confirmationCallback func(swapId string, txHex string, err error) error
	csvCallback          func(swapId string) error
}

func NewElectrumTxWatcher(electrumClient electrum.RPC) (*electrumTxWatcher, error) {
	r := &electrumTxWatcher{
		electrumClient: electrumClient,
		subscriber:     electrum.NewLiquidBlockHeaderSubscriber(),
	}
	return r, nil
}

func (r *electrumTxWatcher) StartWatchingTxs() error {
	ctx := context.Background()
	headerSubscription, err := r.electrumClient.SubscribeHeaders(ctx)
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case blockHeader, ok := <-headerSubscription:
				if !ok {
					log.Infof("Header subscription closed, stopping watching txs.")
					return
				}
				r.blockHeight = electrum.BlocKHeight(blockHeader.Height)
				log.Infof("New block received. block height:%d", r.blockHeight)
				err := r.subscriber.Update(ctx, r.blockHeight)
				if err != nil {
					log.Infof("Error notifying tx observers: %v", err)
					continue
				}

			case <-ctx.Done():
				log.Infof("Context canceled, stopping watching txs.")
				return
			}
		}
	}()
	return r.waitForInitialBlockHeaderSubscription(ctx)
}

// waitForInitialBlockHeaderSubscription waits for the initial block header subscription to be confirmed.
func (r *electrumTxWatcher) waitForInitialBlockHeaderSubscription(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, initialBlockHeaderSubscriptionTimeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			log.Infof("Initial block header subscription timeout.")
			return ctx.Err()
		default:
			if r.blockHeight.Confirmed() {
				return nil
			}
		}
	}
}

func (r *electrumTxWatcher) AddWaitForConfirmationTx(swapIDStr, txIDStr string, vout, startingHeight uint32, scriptpubkeyByte []byte) {
	swapID := swap.NewSwapId()
	err := swapID.FromString(swapIDStr)
	if err != nil {
		log.Infof("Error parsing swapID: %v", err)
		return
	}
	txID, err := chainhash.NewHashFromStr(txIDStr)
	if err != nil {
		log.Infof("Error parsing txID: %v", err)
		return
	}
	scrypt, err := electrum.NewScriptPubKey(scriptpubkeyByte)
	if err != nil {
		log.Infof("Error parsing scriptpubkey: %v", err)
		return
	}
	tx := electrum.NewObserveOpeningTX(*swapID, txID, scrypt, r.electrumClient, r.confirmationCallback)
	r.subscriber.Register(&tx)
}

func (r *electrumTxWatcher) AddConfirmationCallback(f func(swapId string, txHex string, err error) error) {
	r.confirmationCallback = f
}
func (r *electrumTxWatcher) AddCsvCallback(f func(swapId string) error) {
	r.csvCallback = f
}

func (r *electrumTxWatcher) GetBlockHeight() (uint32, error) {
	if !r.blockHeight.Confirmed() {
		return 0, fmt.Errorf("block height not confirmed")
	}
	return r.blockHeight.Height(), nil
}

func (r *electrumTxWatcher) AddWaitForCsvTx(swapIDStr, txIDStr string, vout, startingHeight uint32, scriptpubkeyByte []byte) {
	swapID := swap.NewSwapId()
	err := swapID.FromString(swapIDStr)
	if err != nil {
		log.Infof("Error parsing swapID: %v", err)
		return
	}
	txID, err := chainhash.NewHashFromStr(txIDStr)
	if err != nil {
		log.Infof("Error parsing txID: %v", err)
		return
	}
	scrypt, err := electrum.NewScriptPubKey(scriptpubkeyByte)
	if err != nil {
		log.Infof("Error parsing scriptpubkey: %v", err)
		return
	}
	tx := electrum.NewobserveCSVTX(*swapID, txID, scrypt, r.electrumClient, r.csvCallback)
	r.subscriber.Register(&tx)
}
