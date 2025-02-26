package cache

import (
	"math"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type DNSCache struct {
	mu      sync.RWMutex
	entries map[dns.Question]dnsCacheEntry
}

func NewDNSCache() *DNSCache {
	return &DNSCache{entries: map[dns.Question]dnsCacheEntry{}}
}

func (s *DNSCache) Get(query dns.Question) *dns.Msg {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry, ok := s.entries[query]; ok && !entry.Expired() {
		return entry.Result()
	}
	return nil
}

func (s *DNSCache) Put(query dns.Question, result *dns.Msg) {
	if len(result.Answer) == 0 {
		return
	}
	minTTL := math.MaxInt
	for _, rec := range result.Answer {
		if ttl := int(rec.Header().Ttl); ttl > 0 && ttl < minTTL {
			minTTL = ttl
		}
	}
	if minTTL < math.MaxInt {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.entries[query] = dnsCacheEntry{*result.Copy(), time.Now().Add(time.Duration(minTTL) * time.Second)}
	}
}

func (s *DNSCache) RemoveExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.entries {
		if v.Expired() {
			delete(s.entries, k)
		}
	}
}

type dnsCacheEntry struct {
	result  dns.Msg
	expires time.Time
}

func (s dnsCacheEntry) Expired() bool {
	return time.Now().After(s.expires)
}

func (s dnsCacheEntry) Result() *dns.Msg {
	res := s.result.Copy()
	ttl := max(1, uint32(time.Until(s.expires).Seconds()))
	for i := range res.Answer {
		res.Answer[i].Header().Ttl = ttl
	}
	return res
}
