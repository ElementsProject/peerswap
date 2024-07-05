package electrum

import (
	"context"
	"errors"
	"sync"

	"github.com/elementsproject/peerswap/log"
	"github.com/elementsproject/peerswap/swap"
)

type BlocKHeight uint32

func (b BlocKHeight) Confirmed() bool {
	return b > 0
}

func (b BlocKHeight) Height() uint32 {
	return uint32(b)
}

type BlockHeaderSubscriber interface {
	Register(tx TXObserver)
	Deregister(o TXObserver)
	Update(ctx context.Context, blockHeight BlocKHeight) error
}

type liquidBlockHeaderSubscriber struct {
	txObservers []TXObserver
	mu          sync.Mutex
}

func NewLiquidBlockHeaderSubscriber() *liquidBlockHeaderSubscriber {
	return &liquidBlockHeaderSubscriber{}
}

var _ BlockHeaderSubscriber = (*liquidBlockHeaderSubscriber)(nil)

func (h *liquidBlockHeaderSubscriber) Register(tx TXObserver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.txObservers = append(h.txObservers, tx)
}

func (h *liquidBlockHeaderSubscriber) Deregister(o TXObserver) {
	newObservers := make([]TXObserver, 0, len(h.txObservers))
	for _, observer := range h.txObservers {
		if observer.GetSwapID() != o.GetSwapID() {
			newObservers = append(newObservers, observer)
		}
	}
	h.txObservers = newObservers
}

func (h *liquidBlockHeaderSubscriber) Update(ctx context.Context, blockHeight BlocKHeight) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, observer := range h.txObservers {
		callbacked, err := observer.Callback(ctx, blockHeight)
		if callbacked {
			if err == nil || errors.Is(err, swap.ErrSwapDoesNotExist) {
				// callbacked and no error, remove observer
				h.Deregister(observer)
			}
		}
		if err != nil && !errors.Is(err, swap.ErrSwapDoesNotExist) {
			log.Infof("Error in callback: %v", err)
		}
	}
	return nil
}

func (h *liquidBlockHeaderSubscriber) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.txObservers)
}
