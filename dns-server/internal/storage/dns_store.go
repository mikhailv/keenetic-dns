package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/gzip"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/tsv"
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

func (s *DNSStore) Load(file string) (count int, loadErr error) {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer closeCloser(f, &loadErr)

	var r io.ReadCloser = f
	if strings.HasSuffix(file, ".gz") {
		r, err = gzip.NewReader(f)
		if err != nil {
			return 0, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer closeCloser(r, &loadErr)
	}

	if strings.HasSuffix(file, ".json") || strings.HasSuffix(file, ".json.gz") {
		return s.loadJSON(r)
	}
	if strings.HasSuffix(file, ".tsv") || strings.HasSuffix(file, ".tsv.gz") {
		return s.loadTSV(r)
	}
	return 0, fmt.Errorf("unsupported file format: %s", file)
}

func (s *DNSStore) loadJSON(r io.Reader) (int, error) {
	var records []types.DNSRecord
	if err := json.NewDecoder(r).Decode(&records); err != nil {
		return 0, fmt.Errorf("failed to read JSON data: %w", err)
	}
	s.fill(records)
	return len(records), nil
}

func (s *DNSStore) loadTSV(r io.Reader) (int, error) {
	tsvReader := tsv.NewReader[types.DNSRecord](r)
	loaded := 0
	for rec, err := range tsvReader.Iterator() {
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return loaded, fmt.Errorf("failed to read TSV data: %w", err)
		}
		loaded++
		s.Add(rec)
	}
	return loaded, nil
}

func (s *DNSStore) Save(file string) (records int, saveErr error) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("failed to create dump file: %w", err)
	}
	defer closeCloser(f, &saveErr)

	var w io.WriteCloser = f
	if strings.HasSuffix(file, ".gz") {
		w = gzip.NewWriter(w)
		defer closeCloser(w, &saveErr)
	}

	if strings.HasSuffix(file, ".json") || strings.HasSuffix(file, ".json.gz") {
		return s.saveJSON(w)
	}
	if strings.HasSuffix(file, ".tsv") || strings.HasSuffix(file, ".tsv.gz") {
		return s.saveTSV(w)
	}
	return 0, fmt.Errorf("unsupported file format: %s", file)
}

func (s *DNSStore) saveJSON(w io.Writer) (int, error) {
	records := make([]types.DNSRecord, 0, 100)
	records = slices.AppendSeq(records, s.RecordIterator())
	if err := json.NewEncoder(w).Encode(records); err != nil {
		return 0, fmt.Errorf("failed to save records to dump file: %w", err)
	}
	return len(records), nil
}

func (s *DNSStore) saveTSV(w io.Writer) (count int, err error) {
	tsvWriter := tsv.NewWriter[types.DNSRecord](w)
	defer closeCloser(tsvWriter, &err)
	saved := 0
	for rec := range s.RecordIterator() {
		if err := tsvWriter.Write(rec); err != nil {
			return saved, fmt.Errorf("failed to save record to tsv file: %w", err)
		}
		saved++
	}
	return saved, nil
}

func closeCloser(c io.Closer, err *error) {
	cErr := c.Close()
	if *err == nil {
		*err = cErr
	}
}
