package util

import (
	"time"
)

type Waiter interface {
	Wait()
}

func NewWaiter() (waiter Waiter, signal func()) {
	sw := stopWaiter{make(chan struct{})}
	return sw, sw.signal
}

func RunPeriodically(stopCh <-chan struct{}, interval time.Duration, fn func()) Waiter {
	waiter, signal := NewWaiter()
	go func() {
		defer signal()
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
	}()
	return waiter
}

type stopWaiter struct {
	ch chan struct{}
}

func (s stopWaiter) signal() {
	close(s.ch)
}

func (s stopWaiter) Wait() {
	<-s.ch
}
