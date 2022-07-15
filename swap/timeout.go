package swap

import (
	"context"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/timer"
)

type callbackFactory func(string) func()

type timeOutService struct {
	callbackFactory callbackFactory
}

func newTimeOutService(cbf callbackFactory) *timeOutService {
	return &timeOutService{callbackFactory: cbf}
}

func (s *timeOutService) addNewTimeOut(ctx context.Context, d time.Duration, id string) {
	go timer.TimedCallback(ctx, d, s.callbackFactory(id))
}

type timeOutDummy struct {
	sync.Mutex
	called int
}

func (t *timeOutDummy) addNewTimeOut(ctx context.Context, d time.Duration, id string) {
	t.Lock()
	defer t.Unlock()
	t.called++
}

func (t *timeOutDummy) getCalled() int {
	t.Lock()
	defer t.Unlock()
	return t.called
}
