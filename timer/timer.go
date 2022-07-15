package timer

import (
	"context"
	"time"
)

func TimedCallback(ctx context.Context, d time.Duration, callback func()) {
	timer := time.NewTimer(d)

	select {
	case <-timer.C:
		callback()
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
	}
}
