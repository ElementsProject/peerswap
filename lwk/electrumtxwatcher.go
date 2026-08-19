package lwk

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	goelectrum "github.com/checksum0/go-electrum/electrum"
	"github.com/elementsproject/peerswap/electrum"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/swap"
)

const (
	// initialBlockHeaderSubscriptionTimeout is
	// the initial block header subscription timeout.
	initialBlockHeaderSubscriptionTimeout = 1000 * time.Second
	// Set prime seconds.
	// This way, it prevents many automated clients from attacking the server at the same time.
	// For example, 37, 41, 43, 47, 53, 59, 61, 67, 71, 73, 79, 83, 89 and 97 are good numbers.
	blockHeaderSubscriptionTicker = 37 * time.Second
)

type electrumTxWatcher struct {
	electrumClient       electrum.RPC
	blockHeight          electrum.BlockHeight
	terminalErr          error
	subscriber           electrum.BlockHeaderSubscriber
	confirmationCallback func(swapId string, txHex string, err error) error
	csvCallback          func(swapId string) error
	// resubscribeTicker periodically resubscribes to the block header subscription.
	// Because the connection with the electrum client is
	// disconnected after a certain period of time.
	resubscribeTicker *time.Ticker
	mu                sync.Mutex
}

func NewElectrumTxWatcher(electrumClient electrum.RPC) (*electrumTxWatcher, error) {
	r := &electrumTxWatcher{
		electrumClient:    electrumClient,
		subscriber:        electrum.NewLiquidBlockHeaderSubscriber(),
		resubscribeTicker: time.NewTicker(blockHeaderSubscriptionTicker),
	}
	return r, nil
}

func (r *electrumTxWatcher) StartWatchingTxs() error {
	started := false
	defer func() {
		if !started {
			r.resubscribeTicker.Stop()
		}
	}()
	ctx := context.Background()
	headerSubscription, err := r.electrumClient.SubscribeHeaders(ctx)
	if err != nil {
		return err
	}

	initialTimer := time.NewTimer(initialBlockHeaderSubscriptionTimeout)
	defer initialTimer.Stop()
	select {
	case <-initialTimer.C:
		return errors.New("initial block header subscription timeout")
	case blockHeader, ok := <-headerSubscription:
		if !ok {
			return errors.New("header subscription closed before initial header")
		}
		height, changed, err := r.acceptBlockHeight(blockHeader)
		if err != nil {
			return err
		}
		if changed {
			log.Debugf("New block received. block height:%d", height)
			if err := r.subscriber.Update(ctx, height); err != nil {
				return fmt.Errorf("failed to notify tx observers: %w", err)
			}
		}
	}

	go func() {
		defer r.resubscribeTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Infof("Context canceled, stopping watching txs.")
				return
			case blockHeader, ok := <-headerSubscription:
				if !ok {
					log.Infof("Header subscription closed, stopping watching txs.")
					return
				}
				height, changed, headerErr := r.acceptBlockHeight(blockHeader)
				if headerErr != nil {
					r.fail(headerErr)
					return
				}
				if !changed {
					continue
				}
				log.Debugf("New block received. block height:%d", height)
				err = r.subscriber.Update(ctx, height)
				if err != nil {
					log.Infof("Error notifying tx observers: %v", err)
					continue
				}
			case <-r.resubscribeTicker.C:
				// The old subscription topic will remain in the memory
				// and needs to be cleared by rebooting.
				err := r.electrumClient.Reboot(ctx)
				if err != nil {
					log.Infof("Error rebooting electrum client: %v", err)
					continue
				}
				headerSubscription, err = r.electrumClient.SubscribeHeaders(ctx)
				if err != nil {
					log.Infof("Error subscribe headers: %v", err)
					continue
				}
			}
		}
	}()
	started = true
	return nil
}

func parseBlockHeight(blockHeader *goelectrum.SubscribeHeadersResult) (electrum.BlockHeight, error) {
	if blockHeader == nil {
		return 0, errors.New("electrum returned a nil block header")
	}
	if blockHeader.Height <= 0 {
		return 0, fmt.Errorf(
			"electrum returned invalid block header height: %d",
			blockHeader.Height,
		)
	}
	return electrum.BlockHeight(blockHeader.Height), nil
}

func (r *electrumTxWatcher) acceptBlockHeight(
	blockHeader *goelectrum.SubscribeHeadersResult,
) (electrum.BlockHeight, bool, error) {
	height, err := parseBlockHeight(blockHeader)
	if err != nil {
		return 0, false, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminalErr != nil {
		return 0, false, r.terminalErr
	}
	if r.blockHeight > 0 && height <= r.blockHeight {
		return height, false, nil
	}

	r.blockHeight = height
	return height, true, nil
}

func (r *electrumTxWatcher) fail(err error) {
	r.mu.Lock()
	if r.terminalErr == nil {
		r.terminalErr = err
	}
	r.mu.Unlock()
	log.Infof("Electrum transaction watcher stopped safely: %v", err)
}

func (r *electrumTxWatcher) AddWaitForConfirmationTx(
	swapIDStr,
	txIDStr string,
	vout,
	startingHeight,
	paymentWindow uint32,
	scriptpubkeyByte []byte,
) {
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
	tx := electrum.NewObserveOpeningTX(
		*swapID,
		txID,
		scrypt,
		r.electrumClient,
		r.confirmationCallback,
		startingHeight,
		paymentWindow,
	)
	r.subscriber.Register(&tx)
}

func (r *electrumTxWatcher) AddConfirmationCallback(f func(swapId string, txHex string, err error) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.confirmationCallback = f
}
func (r *electrumTxWatcher) AddCsvCallback(f func(swapId string) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.csvCallback = f
}

func (r *electrumTxWatcher) GetBlockHeight() (uint32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminalErr != nil {
		return 0, fmt.Errorf("electrum transaction watcher stopped: %w", r.terminalErr)
	}
	if r.blockHeight <= 0 {
		return 0, fmt.Errorf("block height not confirmed")
	}
	if r.blockHeight > math.MaxUint32 {
		return 0, fmt.Errorf("block height exceeds uint32: %d", r.blockHeight)
	}
	// #nosec G115 -- the value is checked against both uint32 bounds above.
	return uint32(r.blockHeight), nil
}

func (r *electrumTxWatcher) AddWaitForCsvTx(
	swapIDStr,
	txIDStr string,
	vout,
	startingHeight,
	csv uint32,
	scriptpubkeyByte []byte,
) {
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
	tx := electrum.NewobserveCSVTX(*swapID, txID, scrypt, r.electrumClient, r.csvCallback, csv)
	r.subscriber.Register(&tx)
}
