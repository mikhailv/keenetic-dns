package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"slices"
	"sync"
	"time"
)

type DNSStore struct {
	mu       sync.Mutex
	byDomain MultiMap[string, IPv4, DNSRecord]
	byIP     MultiMap[IPv4, string, DNSRecord]
}

func NewDNSStore() *DNSStore {
	return &DNSStore{
		byDomain: MultiMap[string, IPv4, DNSRecord]{},
		byIP:     MultiMap[IPv4, string, DNSRecord]{},
	}
}

func (s *DNSStore) fill(records []DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.byDomain)
	clear(s.byIP)
	for _, rec := range records {
		s.add(rec)
	}
}

func (s *DNSStore) LookupIP(ip IPv4) []DNSRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs := s.byIP[ip]
	return slices.AppendSeq(make([]DNSRecord, 0, len(recs)), maps.Values(recs))
}

func (s *DNSStore) Add(rec DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.add(rec)
}

func (s *DNSStore) add(rec DNSRecord) {
	s.byDomain.Set(rec.Domain, rec.IP, rec)
	s.byIP.Set(rec.IP, rec.Domain, rec)
}

func (s *DNSStore) Remove(rec DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(rec)
}

func (s *DNSStore) remove(rec DNSRecord) {
	s.byDomain.Remove(rec.Domain, rec.IP)
	s.byIP.Remove(rec.IP, rec.Domain)
}

func (s *DNSStore) RemoveExpired(extraTTL time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for r := range s.iterate() {
		if r.Expired(extraTTL) {
			s.remove(r)
		}
	}
}

func (s *DNSStore) Records() []DNSRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for range s.iterate() {
		count++
	}
	res := make([]DNSRecord, 0, count)
	for r := range s.iterate() {
		res = append(res, r)
	}
	return res
}

func (s *DNSStore) iterate() iter.Seq[DNSRecord] {
	return func(yield func(DNSRecord) bool) {
		for _, records := range s.byDomain {
			for _, r := range records {
				if !yield(r) {
					return
				}
			}
		}
	}
}

func (s *DNSStore) Load(file string) error {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	defer f.Close()
	var records []DNSRecord
	if err := json.NewDecoder(f).Decode(&records); err != nil {
		return fmt.Errorf("failed to load DNS records: %w", err)
	}
	s.fill(records)
	return nil
}

func (s *DNSStore) Save(file string) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create dump file: %w", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(s.Records()); err != nil {
		return fmt.Errorf("failed to save records to dump file: %w", err)
	}
	return nil
}

type MultiMap[K1, K2 comparable, V any] map[K1]map[K2]V

func (m MultiMap[K1, K2, V]) Set(k1 K1, k2 K2, value V) {
	if m2, ok := m[k1]; ok {
		m2[k2] = value
	} else {
		m[k1] = map[K2]V{k2: value}
	}
}

func (m MultiMap[K1, K2, V]) Remove(k1 K1, k2 K2) {
	if m2, ok := m[k1]; ok {
		if _, ok := m2[k2]; ok {
			if len(m2) == 1 {
				delete(m, k1)
			} else {
				delete(m2, k2)
			}
		}
	}
}
