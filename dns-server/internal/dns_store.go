package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"
)

type DNSStore struct {
	mu   sync.Mutex
	all  map[DNSRecordKey]DNSRecord
	byIP map[IPv4][]DNSRecord
}

func NewDNSStore() *DNSStore {
	return &DNSStore{
		all:  map[DNSRecordKey]DNSRecord{},
		byIP: map[IPv4][]DNSRecord{},
	}
}

func (s *DNSStore) fill(records []DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.all)
	clear(s.byIP)
	for _, rec := range records {
		s.add(rec)
	}
}

func (s *DNSStore) LookupIP(ip IPv4) []DNSRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.byIP[ip])
}

func (s *DNSStore) Add(rec DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.add(rec)
}

func (s *DNSStore) add(rec DNSRecord) {
	s.all[rec.DNSRecordKey] = rec
	records := s.byIP[rec.IP]
	for i, it := range records {
		if it.Domain == rec.Domain {
			records[i] = rec
			return
		}
	}
	s.byIP[rec.IP] = append(records, rec)
}

func (s *DNSStore) Remove(rec DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(rec)
}

func (s *DNSStore) remove(rec DNSRecord) {
	delete(s.all, rec.DNSRecordKey)
	if len(s.byIP[rec.IP]) == 1 {
		delete(s.byIP, rec.IP)
	} else {
		s.byIP[rec.IP] = slices.DeleteFunc(s.byIP[rec.IP], func(it DNSRecord) bool {
			return it.Domain == rec.Domain
		})
	}
}

func (s *DNSStore) RemoveExpired(extraTTL time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.all {
		if r.Expired(extraTTL) {
			s.remove(r)
		}
	}
}

func (s *DNSStore) Records() []DNSRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]DNSRecord, 0, len(s.all))
	for _, r := range s.all {
		res = append(res, r)
	}
	return res
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

func removeExpiredRecords(records []DNSRecord, extraTTL time.Duration) []DNSRecord {
	return slices.DeleteFunc(records, func(rec DNSRecord) bool {
		return rec.Expired(extraTTL)
	})
}
