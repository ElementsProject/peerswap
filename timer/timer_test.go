package timer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTimer(t *testing.T) {
	t.Run("callback", func(t *testing.T) {
		t.Parallel()
		cbChan := make(chan struct{})
		ctx := context.Background()
		d := 50 * time.Millisecond
		callback := func() {
			cbChan <- struct{}{}
		}
		start := time.Now()
		go TimedCallback(ctx, d, callback)
		<-cbChan
		assert.GreaterOrEqual(t, time.Since(start).Milliseconds(), d.Milliseconds())
	})
	t.Run("cancel", func(t *testing.T) {
		t.Parallel()
		cbChan := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		d := 50 * time.Millisecond
		callback := func() {
			cbChan <- struct{}{}
		}

		go TimedCallback(ctx, d, callback)
		cancel()

		// Check that callback was not called after timer was canceled.
		tm := time.NewTimer(d)
		select {
		case <-cbChan:
			t.Error("expected callback not to be called")
			if !tm.Stop() {
				<-tm.C
			}
		case <-tm.C:
		}
	})
}
