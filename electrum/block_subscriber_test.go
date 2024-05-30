package electrum

import (
	"context"
	"sync"
	"testing"

	"github.com/elementsproject/peerswap/swap"
)

type testtxo struct {
	swapid swap.SwapId
}

func (t *testtxo) GetSwapID() swap.SwapId {
	return t.swapid
}

func (t *testtxo) Callback(context.Context, BlocKHeight) (bool, error) {
	return true, nil
}

func TestConcurrentUpdate(t *testing.T) {
	t.Parallel()
	observers := make([]TXObserver, 10)
	for i := range observers {
		observers[i] = &testtxo{
			swapid: *swap.NewSwapId(),
		}
	}

	h := &liquidBlockHeaderSubscriber{
		txObservers: observers,
	}

	var wg sync.WaitGroup
	const concurrency = 100
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := h.Update(context.Background(), BlocKHeight(0))
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if len(h.txObservers) != 0 {
		t.Errorf("Expected length %d, but got %d", 0, len(h.txObservers))
	}
}
