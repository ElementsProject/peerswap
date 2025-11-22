package peersync

import (
	"context"
	"sync"
)

type messageBus struct {
	mu          sync.Mutex
	subscribers map[chan CustomMessage]struct{}
}

func newMessageBus() *messageBus {
	return &messageBus{
		subscribers: make(map[chan CustomMessage]struct{}),
	}
}

func (b *messageBus) subscribe(ctx context.Context) (<-chan CustomMessage, error) {
	ch := make(chan CustomMessage, 8)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.remove(ch)
	}()

	return ch, nil
}

func (b *messageBus) publish(msg CustomMessage) {
	b.mu.Lock()
	subs := make([]chan CustomMessage, 0, len(b.subscribers))
	for ch := range b.subscribers {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- msg:
		default:
			// Drop if subscriber is slow.
		}
	}
}

func (b *messageBus) remove(ch chan CustomMessage) {
	b.mu.Lock()
	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		close(ch)
	}
	b.mu.Unlock()
}
