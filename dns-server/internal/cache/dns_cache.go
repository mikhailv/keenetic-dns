package cache

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"maps"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/klauspost/compress/gzip"
	"github.com/miekg/dns"
)

type DNSCache interface {
	Get(ctx context.Context, query dns.Question) *dns.Msg
	Put(ctx context.Context, msg *dns.Msg)
	Close() error
}

type PersistentDNSCache interface {
	Load(reader io.Reader) (int, error)
	Save(writer io.Writer) (int, error)
}

func NewMemoryDNSCache() DNSCache {
	s := &memDNSCache{
		closeCh: make(chan struct{}),
		entries: map[dns.Question]dnsCacheEntry{},
	}
	go s.startCleaner(time.Minute)
	return s
}

var (
	_ DNSCache           = &memDNSCache{}
	_ PersistentDNSCache = &memDNSCache{}
)

type memDNSCache struct {
	mu        sync.RWMutex
	entries   map[dns.Question]dnsCacheEntry
	closeCh   chan struct{}
	closeOnce sync.Once
}

func (s *memDNSCache) startCleaner(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-s.closeCh:
			return
		case <-t.C:
			s.removeExpired()
		}
	}
}

func (s *memDNSCache) Get(ctx context.Context, query dns.Question) *dns.Msg {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry, ok := s.entries[query]; ok && !entry.Expired() {
		return entry.Msg()
	}
	return nil
}

func (s *memDNSCache) Put(ctx context.Context, msg *dns.Msg) {
	if len(msg.Question) == 1 && msg.Response {
		ttl := min(minRecordsTTL(msg.Answer), minRecordsTTL(msg.Ns), minRecordsTTL(msg.Extra))
		if ttl > 0 && ttl < math.MaxUint32 {
			if b, err := msg.Pack(); err == nil {
				s.mu.Lock()
				now := time.Now()
				s.entries[msg.Question[0]] = dnsCacheEntry{
					bytes:   b,
					added:   uint32(now.Unix()),
					expires: uint32(now.Add(time.Duration(ttl) * time.Second).Unix()),
				}
				s.mu.Unlock()
			}
		}
	}
}

func (s *memDNSCache) Close() error {
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})
	return nil
}

func (s *memDNSCache) Load(reader io.Reader) (count int, err error) {
	bufReader := bufio.NewReader(reader)
	gz, err := gzip.NewReader(bufReader)
	if err != nil {
		return 0, err
	}
	defer handleError(gz.Close, &err)

	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.entries)

	for {
		var n uint16
		if err := binary.Read(gz, binary.LittleEndian, &n); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, err
		}
		r := dnsCacheEntry{
			bytes: make([]byte, n),
		}
		if _, err := io.ReadFull(gz, r.bytes); err != nil {
			return 0, err
		}
		if err := binary.Read(gz, binary.LittleEndian, &r.added); err != nil {
			return 0, err
		}
		if err := binary.Read(gz, binary.LittleEndian, &r.expires); err != nil {
			return 0, err
		}
		if r.Expired() {
			continue
		}
		if msg := r.Msg(); msg != nil {
			s.entries[msg.Question[0]] = r
		}
	}
	return len(s.entries), nil
}

func (s *memDNSCache) Save(writer io.Writer) (count int, err error) {
	s.mu.RLock()
	entries := slices.AppendSeq(make([]dnsCacheEntry, 0, len(s.entries)), maps.Values(s.entries))
	s.mu.RUnlock()

	bufWriter := bufio.NewWriter(writer)
	defer handleError(bufWriter.Flush, &err)
	gz := gzip.NewWriter(bufWriter)
	defer handleError(gz.Close, &err)

	for _, v := range entries {
		if v.Expired() {
			continue
		}
		if err := binary.Write(gz, binary.LittleEndian, uint16(len(v.bytes))); err != nil {
			return 0, err
		}
		if _, err := gz.Write(v.bytes); err != nil {
			return 0, err
		}
		if err := binary.Write(gz, binary.LittleEndian, v.added); err != nil {
			return 0, err
		}
		if err := binary.Write(gz, binary.LittleEndian, v.expires); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func (s *memDNSCache) removeExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.entries {
		if v.Expired() {
			delete(s.entries, k)
		}
	}
}

type dnsCacheEntry struct {
	bytes   []byte
	added   uint32 // timestamp
	expires uint32 // timestamp
}

func (s *dnsCacheEntry) AddedAt() time.Time {
	return time.Unix(int64(s.added), 0)
}

func (s *dnsCacheEntry) ExpiresAt() time.Time {
	return time.Unix(int64(s.expires), 0)
}

func (s *dnsCacheEntry) Expired() bool {
	return time.Now().After(s.ExpiresAt())
}

func (s dnsCacheEntry) Msg() *dns.Msg {
	var res dns.Msg
	if err := res.Unpack(s.bytes); err != nil {
		return nil
	}
	seconds := int(time.Since(s.AddedAt()).Seconds())
	updateRecordsTTL(res.Answer, seconds)
	updateRecordsTTL(res.Ns, seconds)
	updateRecordsTTL(res.Extra, seconds)
	return &res
}

func updateRecordsTTL(records []dns.RR, seconds int) {
	for _, r := range records {
		r.Header().Ttl = uint32(max(1, int(r.Header().Ttl)-seconds))
	}
}

func minRecordsTTL(records []dns.RR) uint32 {
	var res uint32 = math.MaxUint32
	for _, rr := range records {
		res = min(res, rr.Header().Ttl)
	}
	return res
}

func handleError(fn func() error, err *error) {
	fnErr := fn()
	if *err == nil {
		*err = fnErr
	}
}
