package electrum

import (
	"context"
	"sync"
	"testing"

	"github.com/elementsproject/peerswap/swap"
)

type testtxo struct {
	swapid    swap.SwapId
	getSwapID func() swap.SwapId
	callback  func(context.Context, BlocKHeight) (bool, error)
}

func (t *testtxo) GetSwapID() swap.SwapId {
	if t.getSwapID != nil {
		return t.getSwapID()
	}
	return t.swapid
}

func (t *testtxo) Callback(ctx context.Context, b BlocKHeight) (bool, error) {
	if t.callback != nil {
		return t.callback(ctx, b)
	}
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

func Test_liquidBlockHeaderSubscriber_Update(t *testing.T) {
	t.Parallel()
	var blockHeight BlocKHeight = 10
	tests := map[string]struct {
		txObservers []TXObserver
		count       int
		wantErr     bool
	}{
		"no observers": {
			txObservers: nil,
		},
		"observers": {
			txObservers: []TXObserver{
				&testtxo{},
				&testtxo{},
			},
		},
		"swap does not exists": {
			txObservers: []TXObserver{
				&testtxo{
					callback: func(context.Context, BlocKHeight) (bool, error) {
						return true, swap.ErrSwapDoesNotExist
					},
				},
			},
		},
		"error in callback": {
			txObservers: []TXObserver{
				&testtxo{
					callback: func(context.Context, BlocKHeight) (bool, error) {
						return true, swap.ErrEventRejected
					},
				},
			},
			count: 1,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := &liquidBlockHeaderSubscriber{
				txObservers: tt.txObservers,
			}
			if err := h.Update(context.Background(), blockHeight); (err != nil) != tt.wantErr {
				t.Errorf("liquidBlockHeaderSubscriber.Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(h.txObservers) != tt.count {
				t.Errorf("Expected length %d, but got %d", tt.count, len(h.txObservers))
			}
		})
	}
}
