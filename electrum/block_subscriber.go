package electrum

import (
	"context"

	"github.com/elementsproject/peerswap/log"
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
}

func NewLiquidBlockHeaderSubscriber() *liquidBlockHeaderSubscriber {
	return &liquidBlockHeaderSubscriber{}
}

var _ BlockHeaderSubscriber = (*liquidBlockHeaderSubscriber)(nil)

func (h *liquidBlockHeaderSubscriber) Register(tx TXObserver) {
	h.txObservers = append(h.txObservers, tx)
}

func (h *liquidBlockHeaderSubscriber) Deregister(o TXObserver) {
	h.txObservers = remove(h.txObservers, o)
}

func (h *liquidBlockHeaderSubscriber) Update(ctx context.Context, blockHeight BlocKHeight) error {
	for _, observer := range h.txObservers {
		callbacked, err := observer.Callback(ctx, blockHeight)
		if callbacked && err == nil {
			// callbacked and no error, remove observer
			h.Deregister(observer)
		}
		if err != nil {
			log.Infof("Error in callback: %v", err)
		}
	}
	return nil
}

func remove(observerList []TXObserver, observerToRemove TXObserver) []TXObserver {
	newObservers := make([]TXObserver, len(observerList)-1)
	for _, observer := range observerList {
		if observer.GetSwapID() != observerToRemove.GetSwapID() {
			newObservers = append(newObservers, observer)
		}
	}
	return newObservers
}
