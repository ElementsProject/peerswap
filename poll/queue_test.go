package poll

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Queue_Enqueue(t *testing.T) {
	t.Parallel()
	q := queue[int]{}
	q.Enqueue(1)
	assert.Equal(t, 1, len(q.q))
}

func Test_Queue_Dequeue(t *testing.T) {
	t.Parallel()
	q := queue[int]{}
	q.Enqueue(101)
	q.Enqueue(102)
	i, ok := q.Dequeue()
	assert.True(t, ok)
	assert.Equal(t, 101, i)
	assert.Equal(t, 1, len(q.q))
}

func Test_Queue_Peek(t *testing.T) {
	t.Parallel()
	q := queue[int]{}
	q.Enqueue(101)
	q.Enqueue(102)
	i, ok := q.Peek()
	assert.True(t, ok)
	assert.Equal(t, 101, i)
	assert.Equal(t, 2, len(q.q))
}
