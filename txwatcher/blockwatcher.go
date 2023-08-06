package txwatcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/elementsproject/peerswap/log"
)

type ChainRPC interface {
	GetBlockHeight() (uint64, error)
}

type BlockWatcher struct {
	sync.Mutex

	subscriber map[string]func(next uint64)
	chainRRC   ChainRPC
}

func NewBlockWatcher(chainRPC ChainRPC) *BlockWatcher {
	return &BlockWatcher{
		subscriber: map[string]func(next uint64){},
		chainRRC:   chainRPC,
	}
}

func (b *BlockWatcher) Watch(ctx context.Context, interval time.Duration) error {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	current, err := b.chainRRC.GetBlockHeight()
	if err != nil {
		return fmt.Errorf("could not get initial block height: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ErrContextCanceled
		case <-tick.C:
			next, err := b.chainRRC.GetBlockHeight()
			if err != nil {
				log.Debugf("could not get block height: %v", err)
			}
			if next < current {
				// Todo: think about shutting down here since something really bad happened.
				log.Debugf(
					"did not expect block height to decrease: current=%d, next=%d",
					current,
					next,
				)
				continue
			}
			if next == current+1 {
				// Got next block, announce it!
				go b.announce(next)
				continue
			}
			if next > current {
				// Skipped a block, that is not bad but we may want to log it
				// anyways.
				log.Debugf(
					"skipped some blocks: current=%d, next=%d",
					current,
					next,
				)
				go b.announce(next)
				continue
			}
		}
	}
}

func (b *BlockWatcher) announce(nextHeight uint64) {
	b.Lock()
	defer b.Unlock()
	for _, handler := range b.subscriber {
		go handler(nextHeight)
	}
}

func (b *BlockWatcher) Subscribe(id string, handler func(uint64)) {
	b.Lock()
	defer b.Unlock()
	// We don't care about double subscriptions for now.
	b.subscriber[id] = handler
}

func (b *BlockWatcher) Unbscribe(id string) {
	b.Lock()
	defer b.Unlock()
	// This is a noop if id is not in the map.
	delete(b.subscriber, id)
}
