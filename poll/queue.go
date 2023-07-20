package poll

import (
	"sync"
	"time"
)

type pollQueue = queue[nextPeer]

type nextPeer struct {
	ts   time.Time
	peer string
}

type queue[C any] struct {
	sync.RWMutex
	q []C
}

func (q *queue[C]) Enqueue(elem ...C) {
	q.Lock()
	defer q.Unlock()
	q.q = append(q.q, elem...)
}

func (q *queue[C]) Peek() (C, bool) {
	q.RLock()
	defer q.RUnlock()
	if len(q.q) > 0 {
		return q.q[0], true
	}
	return *new(C), false
}

func (q *queue[C]) Dequeue() (C, bool) {
	q.Lock()
	defer q.Unlock()
	if len(q.q) > 0 {
		elem := q.q[0]
		q.q = q.q[1:]
		return elem, true
	}
	return *new(C), false
}
