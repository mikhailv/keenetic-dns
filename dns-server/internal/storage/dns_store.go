package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type DNSStore struct {
	mu       sync.RWMutex
	all      map[types.DNSRecordKey]types.DNSRecord
	byIP     map[types.IPv4][]string
	extraTTL time.Duration
}

func NewDNSStore(extraTTL time.Duration) *DNSStore {
	return &DNSStore{
		all:      map[types.DNSRecordKey]types.DNSRecord{},
		byIP:     map[types.IPv4][]string{},
		extraTTL: extraTTL,
	}
}

func (s *DNSStore) fill(records []types.DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.all)
	clear(s.byIP)
	for _, rec := range records {
		s.add(rec)
	}
}

func (s *DNSStore) LookupIP(ip types.IPv4) []types.DNSRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	domains := s.byIP[ip]
	res := make([]types.DNSRecord, 0, len(domains))
	for _, domain := range domains {
		rec := s.all[types.DNSRecordKey{Domain: domain, IP: ip}]
		if !rec.Expired(s.extraTTL) {
			res = append(res, rec)
		}
	}
	return res
}

func (s *DNSStore) Add(rec types.DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.add(rec)
}

func (s *DNSStore) add(rec types.DNSRecord) {
	if rec.Expired(s.extraTTL) {
		return
	}
	s.all[rec.DNSRecordKey] = rec
	domains := s.byIP[rec.IP]
	domains = append(domains, rec.Domain)
	slices.Sort(domains)
	domains = slices.Compact(domains)
	s.byIP[rec.IP] = domains
}

func (s *DNSStore) Remove(rec types.DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(rec)
}

func (s *DNSStore) remove(rec types.DNSRecord) {
	delete(s.all, rec.DNSRecordKey)
	if domains, ok := s.byIP[rec.IP]; ok {
		s.byIP[rec.IP] = slices.DeleteFunc(domains, func(s string) bool { return s == rec.Domain })
	}
}

func (s *DNSStore) RemoveExpired() []types.DNSRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := make([]types.DNSRecord, 0, 20)
	for _, r := range s.all {
		if r.Expired(s.extraTTL) {
			s.remove(r)
			removed = append(removed, r)
		}
	}
	return removed
}

func (s *DNSStore) RecordIterator() iter.Seq[types.DNSRecord] {
	return func(yield func(types.DNSRecord) bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, r := range s.all {
			if r.Expired(s.extraTTL) {
				continue
			}
			if !yield(r) {
				return
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

	var records []types.DNSRecord
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

	records := make([]types.DNSRecord, 0, 100)
	records = slices.AppendSeq(records, s.RecordIterator())

	if err := json.NewEncoder(f).Encode(records); err != nil {
		return fmt.Errorf("failed to save records to dump file: %w", err)
	}
	return nil
}
