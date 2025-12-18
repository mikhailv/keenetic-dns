package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type DNSStore struct {
	mu   sync.Mutex
	all  map[types.DNSRecordKey]types.DNSRecord
	byIP map[types.IPv4][]string
}

func NewDNSStore() *DNSStore {
	return &DNSStore{
		all:  map[types.DNSRecordKey]types.DNSRecord{},
		byIP: map[types.IPv4][]string{},
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
	s.mu.Lock()
	defer s.mu.Unlock()
	domains := s.byIP[ip]
	res := make([]types.DNSRecord, len(domains))
	for i, domain := range domains {
		res[i] = s.all[types.DNSRecordKey{Domain: domain, IP: ip}]
	}
	return res
}

func (s *DNSStore) Add(rec types.DNSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.add(rec)
}

func (s *DNSStore) add(rec types.DNSRecord) {
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

func (s *DNSStore) RemoveExpired(extraTTL time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.all {
		if r.Expired(extraTTL) {
			s.remove(r)
		}
	}
}

func (s *DNSStore) Records() []types.DNSRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]types.DNSRecord, 0, len(s.all))
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
	if err := json.NewEncoder(f).Encode(s.Records()); err != nil {
		return fmt.Errorf("failed to save records to dump file: %w", err)
	}
	return nil
}
