package util

import (
	"time"
)

func RunPeriodically(stopCh <-chan struct{}, interval time.Duration, fn func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-t.C:
			fn()
		}
	}
}
