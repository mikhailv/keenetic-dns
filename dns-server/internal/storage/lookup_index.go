package storage

import (
	"bytes"
	"cmp"
	"iter"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type lookupKey string

type LookupIndex struct {
	mu       sync.RWMutex
	all      map[lookupKey]*types.DomainLookup
	byIP     map[types.IPv4][]lookupKey
	extraTTL time.Duration
}

func NewLookupIndex(extraTTL time.Duration) *LookupIndex {
	return &LookupIndex{
		all:      map[lookupKey]*types.DomainLookup{},
		byIP:     map[types.IPv4][]lookupKey{},
		extraTTL: extraTTL,
	}
}

func (s *LookupIndex) Clear() {
	s.mu.Lock()
	clear(s.all)
	clear(s.byIP)
	s.mu.Unlock()
}

func (s *LookupIndex) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.all)
}

func (s *LookupIndex) LookupByIP(ip types.IPv4) []*types.DomainLookup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := s.byIP[ip]
	res := make([]*types.DomainLookup, 0, len(keys))
	for _, key := range keys {
		rec := s.all[key]
		if !rec.Expired(s.extraTTL) {
			res = append(res, rec)
		}
	}
	return res
}

func (s *LookupIndex) Add(rec *types.DomainLookup) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.Expired(s.extraTTL) {
		return false
	}
	key := makeLookupKey(rec)
	s.all[key] = rec
	for _, it := range rec.IPs {
		s.byIP[it.IP] = appendUnique(s.byIP[it.IP], key)
	}
	return true
}

func (s *LookupIndex) Remove(rec *types.DomainLookup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(rec)
}

func (s *LookupIndex) remove(rec *types.DomainLookup) {
	key := makeLookupKey(rec)
	delete(s.all, key)
	for _, it := range rec.IPs {
		if keys, ok := s.byIP[it.IP]; ok {
			keys = slices.DeleteFunc(keys, func(it lookupKey) bool { return it == key })
			if len(keys) == 0 {
				delete(s.byIP, it.IP)
			} else {
				s.byIP[it.IP] = keys
			}
		}
	}
}

func (s *LookupIndex) RemoveExpired() []*types.DomainLookup {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := make([]*types.DomainLookup, 0, 10)
	for _, r := range s.all {
		if r.Expired(s.extraTTL) {
			s.remove(r)
			removed = append(removed, r)
		}
	}
	return removed
}

func (s *LookupIndex) Values() []*types.DomainLookup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return util.SeqToSlice(len(s.all), s.iterator(false))
}

func (s *LookupIndex) Iterator() iter.Seq[*types.DomainLookup] {
	return s.iterator(true)
}

func (s *LookupIndex) iterator(withLock bool) iter.Seq[*types.DomainLookup] {
	return func(yield func(*types.DomainLookup) bool) {
		if withLock {
			s.mu.RLock()
			defer s.mu.RUnlock()
		}
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

func makeLookupKey(dl *types.DomainLookup) lookupKey {
	ips := make([][4]byte, len(dl.IPs))
	for i := range dl.IPs {
		ips[i] = [4]byte(dl.IPs[i].IP[:4])
	}
	slices.SortFunc(ips, func(a, b [4]byte) int {
		return bytes.Compare(a[:], b[:])
	})
	var sb strings.Builder
	sb.Grow(len(dl.Domain) + 5*len(ips))
	sb.WriteString(dl.Domain)
	for _, ip := range ips {
		sb.WriteByte(0)
		sb.Write(ip[:])
	}
	return lookupKey(sb.String())
}

func appendUnique[T cmp.Ordered, S ~[]T](values S, value T) S {
	values = append(values, value)
	slices.Sort(values)
	return slices.Compact(values)
}
