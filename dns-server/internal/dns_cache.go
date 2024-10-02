package internal

import (
	"math"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type DNSCache struct {
	mu      sync.RWMutex
	entries map[string]dnsCacheEntry
}

func NewDNSCache() *DNSCache {
	return &DNSCache{entries: map[string]dnsCacheEntry{}}
}

func (s *DNSCache) Get(domain string) []dns.RR {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry, ok := s.entries[domain]; ok && !entry.Expired() {
		return entry.Answer()
	}
	return nil
}

func (s *DNSCache) Put(domain string, answer []dns.RR) {
	records := make([]dns.A, 0, len(answer))
	ttl := math.MaxInt
	for _, it := range answer {
		if a, ok := it.(*dns.A); ok {
			records = append(records, *a)
			ttl = min(ttl, int(a.Hdr.Ttl))
		}
	}
	if len(records) > 0 {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.entries[domain] = dnsCacheEntry{records, time.Now().Add(time.Duration(ttl) * time.Second)}
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
	answer  []dns.A
	expires time.Time
}

func (s dnsCacheEntry) Expired() bool {
	return time.Now().After(s.expires)
}

func (s dnsCacheEntry) Answer() []dns.RR {
	res := make([]dns.RR, len(s.answer))
	ttl := max(1, uint32(time.Until(s.expires).Seconds()))
	for i, it := range s.answer {
		it.Hdr.Ttl = ttl
		res[i] = &it
	}
	return res
}
