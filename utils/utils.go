package utils

import (
	"fmt"
	"time"
)

type WaitFunc func() bool

func WaitFor(f WaitFunc, retryEvery, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("WaitFor reached timeout with %v", f)
		default:
			if f() {
				return nil
			}
			time.Sleep(retryEvery)
		}
	}
}
