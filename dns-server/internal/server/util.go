package server

import (
	"context"
	"net/url"
	"strings"
	"time"
)

func debounceUpdateChannel(ctx context.Context, minInterval, maxInterval time.Duration, ch <-chan struct{}) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		var minTimer, maxTimer <-chan time.Time
		sendUpdate := func() {
			minTimer = nil
			maxTimer = nil
			select {
			case <-ctx.Done():
			case out <- struct{}{}:
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ch:
				if !ok {
					return
				}
				minTimer = time.After(minInterval)
				if maxTimer == nil {
					maxTimer = time.After(maxInterval)
				}
			case <-minTimer:
				sendUpdate()
			case <-maxTimer:
				sendUpdate()
			}
		}
	}()
	return out
}

func queryParamSet(q url.Values, name string) bool {
	if q.Has(name) {
		v := q.Get(name)
		return v == "" || v == "1" || strings.ToLower(v) == "true"
	}
	return false
}

func updateURLQuery(u url.URL, values map[string]string) string {
	q := u.Query()
	for k, v := range values {
		if v == "\x00" {
			q.Del(k)
		} else {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
