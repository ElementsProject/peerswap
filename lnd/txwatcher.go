package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/onchain"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/chainrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type confirmationEvent struct {
	swapId      string
	rawTx       []byte
	blockHeight uint32
}

type TxWatcher struct {
	sync.Mutex
	wg sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc

	lnrpcClient    lnrpc.LightningClient
	chainrpcClient chainrpc.ChainNotifierClient

	network     *chaincfg.Params
	targetConfs uint32
	targetCsv   uint32

	confirmationCallback func(swapId, txHex string, err error) error
	csvPassedCallback    func(swapId string) error

	confirmationWatchers map[string]bool
	waitForCsvWatchers   map[string]bool
}

func NewTxWatcher(ctx context.Context, cc *grpc.ClientConn, network *chaincfg.Params, targetConfirmation, targetCsv uint32) (*TxWatcher, error) {
	lnrpcClient := lnrpc.NewLightningClient(cc)
	chainrpcClient := chainrpc.NewChainNotifierClient(cc)

	// Check that service is reachable.
	_, err := lnrpcClient.GetInfo(context.TODO(), &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf(
			"unable to reach out to lnd for GetInfo(): %v", err,
		)
	}

	ctx, cancel := context.WithCancel(ctx)

	confirmationWatchers := make(map[string]bool)
	waitForCsvWatchers := make(map[string]bool)

	return &TxWatcher{
		ctx:                  ctx,
		cancel:               cancel,
		lnrpcClient:          lnrpcClient,
		chainrpcClient:       chainrpcClient,
		network:              network,
		targetConfs:          targetConfirmation,
		targetCsv:            targetCsv,
		confirmationWatchers: confirmationWatchers,
		waitForCsvWatchers:   waitForCsvWatchers,
	}, nil
}

func (t *TxWatcher) Start() error {
	return nil
}
func (t *TxWatcher) StartWatchingTxs() error {
	return nil
}

func (t *TxWatcher) Stop() error {
	t.cancel()
	log.Infof("[TxWatcher] Canceled contexts, waiting for subscriptions to close")
	t.wg.Wait()
	return nil
}

func (t *TxWatcher) addTxWatcher(ctx context.Context, swapId string, txId string, numConfs, heightHint uint32, script []byte) (
	chan confirmationEvent, chan error, error) {

	txIdHash, err := chainhash.NewHashFromStr(txId)
	if err != nil {
		return nil, nil, err
	}

	stream, err := t.chainrpcClient.RegisterConfirmationsNtfn(
		ctx,
		&chainrpc.ConfRequest{
			Txid:       txIdHash.CloneBytes(),
			Script:     script,
			NumConfs:   numConfs,
			HeightHint: heightHint,
		},
	)
	if err != nil {
		return nil, nil, err
	}

	confChan := make(chan confirmationEvent, 1)
	errChan := make(chan error, 1)

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()

		for {
			res, err := stream.Recv()
			if err == io.EOF {
				// EOF means the stream was closed and returned.
				return
			} else if err != nil {
				errChan <- err
				return
			}

			switch event := res.Event.(type) {
			case *chainrpc.ConfEvent_Conf:
				confChan <- confirmationEvent{
					swapId:      swapId,
					rawTx:       event.Conf.RawTx,
					blockHeight: event.Conf.BlockHeight,
				}
				return

			case *chainrpc.ConfEvent_Reorg:
				// Not supported yet, we can continue as we return on conf and
				// should never get an reorg event in return.
				log.Debugf("[TxWatcher] Swap: %s: Got an reorg event", swapId)
				continue

			default:
				errChan <- fmt.Errorf("event has unexpected types")
				return
			}
		}
	}()

	return confChan, errChan, nil
}

// AddWaitForConfirmationTx subscribes to the lnd onchain tx watcher and calls
// the callback as soon as the tx is confirmed. The empty uint32 parameter is
// due to the Watcher interface of swap expecting a signature with a vout
// parameter.
func (t *TxWatcher) AddWaitForConfirmationTx(swapId string, txId string, _ uint32, heightHint uint32, script []byte) {
	t.Lock()
	if _, ok := t.confirmationWatchers[swapId]; ok {
		log.Debugf("[TxWatcher] Swap: %s: Tried to resubscribe to tx watcher for tx %s", swapId, txId)
		t.Unlock()
		return
	}
	log.Debugf("[TxWatcher] Swap: %s: Add new confirmation watcher for tx %s, awaiting %d confirmations",
		swapId,
		txId,
		t.targetConfs,
	)
	t.confirmationWatchers[swapId] = true
	t.Unlock()

	ctx, cancel := context.WithCancel(t.ctx)
	confChan, errChan, err := t.addTxWatcher(ctx, swapId, txId, t.targetConfs, heightHint, script)
	if err != nil {
		// TODO: Add error return to somehow handle error in swap. Else this
		// could lead to stale swaps that might not resolve.
		log.Infof("[TxWatcher] Swap: %s: Could not subscribe tx watcher for tx %s, %v", swapId, txId, err)
		cancel()
		return
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer func() {
			t.Lock()
			delete(t.confirmationWatchers, swapId)
			t.Unlock()
		}()
		defer cancel()
		defer close(errChan)
		defer close(confChan)

		for {
			select {
			case conf := <-confChan:
				// This tx watcher can also be used on recovery. This can lead to
				// the situation that the tx is confirmed but also already too close
				// to the CSV limit. Therefore we have to check on what we trigger
				// here:  confirmationCallback for the case that we are in the
				// limits, csvPassedCallback if we are above the csv limit.
				currentHeight, err := t.GetBlockHeight()
				if err != nil {
					// TODO: Add error return to somehow handle error in swap. Else
					// this could lead to stale swaps that might not resolve.
					log.Infof("[TxWatcher] Wait for confirmation on swap: %s, Failed on GetBlockHeight() %v", swapId, err)
					return
				}

				// We add a +1 as the confirmation block height is the height of
				// first confirmation.
				confs := currentHeight - conf.blockHeight + 1
				if confs >= onchain.BitcoinCsvSafetyLimit {
					// We are already above half of the csv limit here, it is
					// unsafe to pay for the invoice now.
					// TODO: Check if this is handled correctly by the swap state
					// machine.
					log.Infof("[TxWatcher] Wait for confirmation on swap %s: Confirmations already above csv limit for tx %s", swapId, txId)
					_ = t.csvPassedCallback(swapId)
					return
				}

				// Why should a callback have a return? Let the receiving part of
				// the software decide what to do!
				// TODO: rework callbacks.
				log.Infof("[TxWatcher] Wait for confirmation on swap %s: Got %d confirmations call callback", swapId, confs)
				if t.confirmationCallback == nil {
					log.Infof("[TxWatcher] Wait for confirmation on swap %s: confirmationCallback is nil", swapId)
					return
				}
				_ = t.confirmationCallback(swapId, hex.EncodeToString(conf.rawTx), nil)
				return
			case err := <-errChan:
				if err == io.EOF {
					log.Infof("[TxWatcher] Wait for confirmation on swap %s: Stream closed by server: %s", swapId)
					return
				}
				if IsContextError(err) {
					s := status.Convert(err)
					log.Infof("[TxWatcher] Wait for confirmation on swap %s: Stream closed by client: %s", swapId, s.Message())
					return
				}
				if err != nil {
					// TODO: Add error return to somehow handle error in swap. Else
					// this could lead to stale swaps that might not resolve.
					log.Infof("[TxWatcher] Wait for confirmation on swap: %s: Stream closed with err: %v", swapId, err)
					return
				}
			}
		}
	}()
}

// AddWaitForCsvTx subscribes to the lnd onchain tx watcher and calls
// the callback as soon as the tx is above the csv limit.
func (t *TxWatcher) AddWaitForCsvTx(swapId string, txId string, vout uint32, heightHint uint32, script []byte) {
	t.Lock()
	if _, ok := t.waitForCsvWatchers[swapId]; ok {
		log.Debugf("[TxWatcher] Swap: %s: Tried to resubscribe to tx watcher for tx %s", swapId, txId)
		t.Unlock()
		return
	}
	log.Debugf("[TxWatcher] Swap: %s: Add new csv watcher for tx %s, with csv limit: %d",
		swapId,
		txId,
		t.targetCsv,
	)
	t.confirmationWatchers[swapId] = true
	t.Unlock()

	ctx, cancel := context.WithCancel(t.ctx)
	// We use a confirmation number of 144 here as this is the maximum supported
	// by lnd. Lnd assumes this to be a good value after which it is unlikely
	// for a tx to be reorganized out of the chain.
	// This means that we have to count the blocks after this by our self.
	// TODO: Ask lnd why we can not listen longer?
	confChan, errChan, err := t.addTxWatcher(ctx, swapId, txId, 144, heightHint, script)
	if err != nil {
		// TODO: Add error return to somehow handle error in swap. Else this
		// could lead to stale swaps that might not resolve.
		log.Infof("[TxWatcher] Swap: %s: Could not subscribe tx watcher to tx %s, %v", swapId, txId, err)
		cancel()
		return
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		defer func() {
			t.Lock()
			delete(t.waitForCsvWatchers, swapId)
			t.Unlock()
		}()
		defer cancel()
		defer close(errChan)
		defer close(confChan)

		for {
			select {
			case conf := <-confChan:
				// We get to this point if the tx has been confirmed 144 times
				// as lnd does not allow to track for more confirmations.
				// Instead we listen for new blocks here and calculate the
				// confirmations by the difference of the block heights.
				stream, err := t.chainrpcClient.RegisterBlockEpochNtfn(
					ctx,
					&chainrpc.BlockEpoch{
						Height: conf.blockHeight,
					},
				)
				if err != nil {
					// TODO: Add error return to somehow handle error in swap. Else this
					// could lead to stale swaps that might not resolve.
					log.Infof("[TxWatcher] Wait for csv limit on swap %s: Could not subscribe to block stream, %v", swapId, err)
					return
				}

				for {
					be, err := stream.Recv()
					if err == io.EOF {
						log.Infof("[TxWatcher] Wait for csv limit on swap %s: Block stream closed by server: %s", swapId)
						return
					}
					if IsContextError(err) {
						s := status.Convert(err)
						log.Infof("[TxWatcher] Wait for csv limit on swap %s: Block stream closed by client: %s", swapId, s.Message())
						return
					}
					if err != nil {
						// TODO: Add error return to somehow handle error in swap. Else
						// this could lead to stale swaps that might not resolve.
						log.Infof("[TxWatcher] Wait for csv limit on swap: %s: Block stream closed with err: %v", swapId, err)
						return
					}

					// We add a +1 as the confirmation block height is the height of
					// first confirmation. If the current confirmations are past the
					// csv limit we call back.
					if be.Height-conf.blockHeight+1 >= onchain.BitcoinCsv {
						log.Infof("[TxWatcher] Wait for csv limit on swap %s: Csv passed limit, call csvPassedCallback", swapId)
						if t.csvPassedCallback == nil {
							log.Infof("[TxWatcher] Wait for csv limit on swap %s: confirmationCallback is nil", swapId)
							return
						}
						_ = t.csvPassedCallback(swapId)
						return
					}
				}
			case err := <-errChan:
				if err == io.EOF {
					log.Infof("[TxWatcher] Wait for csv limit on swap %s: Stream closed by server: %s", swapId)
					return
				}
				if IsContextError(err) {
					s := status.Convert(err)
					log.Infof("[TxWatcher] Wait for csv limit on swap %s: Stream closed by client: %s", swapId, s.Message())
					return
				}
				if err != nil {
					// TODO: Add error return to somehow handle error in swap. Else
					// this could lead to stale swaps that might not resolve.
					log.Infof("[TxWatcher] Wait for csv limit on swap: %s: Stream closed with err: %v", swapId, err)
					return
				}
			}
		}
	}()
}

// AddConfirmationCallback adds a callback to the watcher that will be called in
// the case that an active "wait for confirmation" watcher reached the
// confirmation limit for a swap.
func (t *TxWatcher) AddConfirmationCallback(cb func(swapId string, txHex string, err error) error) {
	t.Lock()
	defer t.Unlock()
	t.confirmationCallback = cb
}

// AddCsvCallback adds a callback to the watcher that will be called in
// the case that an active "wait for csv limit reached" watcher reached the csv
// limit for a swap.
func (t *TxWatcher) AddCsvCallback(cb func(swapId string) error) {
	t.Lock()
	defer t.Unlock()
	t.csvPassedCallback = cb
}

// GetBlockHeight returns the current best block from the GetInfo call. Beware
// that this hight is the best block from the nodes view.
func (t *TxWatcher) GetBlockHeight() (uint32, error) {
	r, err := t.lnrpcClient.GetInfo(context.TODO(), &lnrpc.GetInfoRequest{})
	if err != nil {
		return 0, err
	}
	return r.BlockHeight, nil
}
