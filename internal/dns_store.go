package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

type DNSStore struct {
	mu      sync.Mutex
	records map[DNSRecordKey]DNSRecord
}

func NewDNSStore() *DNSStore {
	return &DNSStore{records: map[DNSRecordKey]DNSRecord{}}
}

func (s *DNSStore) Has(key DNSRecordKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.records[key]
	return ok
}

func (s *DNSStore) Add(rec DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[rec.DNSRecordKey] = rec
}

func (s *DNSStore) Remove(key DNSRecordKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
}

func (s *DNSStore) Records() []DNSRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]DNSRecord, 0, len(s.records))
	for _, r := range s.records {
		res = append(res, r)
	}
	return res
}

func (s *DNSStore) Load(file string) error {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var records []DNSRecord
	if err := json.NewDecoder(f).Decode(&records); err != nil {
		return fmt.Errorf("failed to load DNS records: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range records {
		s.records[r.DNSRecordKey] = r
	}
	return nil
}

func (s *DNSStore) Save(file string) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create dump file: %w", err)
	}
	if err := json.NewEncoder(f).Encode(s.Records()); err != nil {
		return fmt.Errorf("failed to save records to dump file: %w", err)
	}
	return nil
}
