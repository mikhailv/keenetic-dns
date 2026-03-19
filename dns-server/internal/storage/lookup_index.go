package storage

import (
	"iter"
	"slices"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

type lookupKey struct {
	IP     types.IPv4
	Domain string
}

type LookupIndex struct {
	mu       sync.RWMutex
	all      map[lookupKey]*types.DomainLookup
	byIP     map[types.IPv4][]string
	extraTTL time.Duration
}

func NewLookupIndex(extraTTL time.Duration) *LookupIndex {
	return &LookupIndex{
		all:      map[lookupKey]*types.DomainLookup{},
		byIP:     map[types.IPv4][]string{},
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
	// TODO: very ineffective, improve it
	return len(s.Snapshot())
}

func (s *LookupIndex) LookupIP(ip types.IPv4) []*types.DomainLookup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	domains := s.byIP[ip]
	res := make([]*types.DomainLookup, 0, len(domains))
	for _, domain := range domains {
		rec := s.all[lookupKey{ip, domain}]
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
	for _, it := range rec.IPs {
		s.all[lookupKey{it.IP, rec.Domain}] = rec
		domains := s.byIP[it.IP]
		domains = append(domains, rec.Domain)
		slices.Sort(domains)
		domains = slices.Compact(domains)
		s.byIP[it.IP] = domains
	}
	return true
}

func (s *LookupIndex) Remove(rec *types.DomainLookup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(rec)
}

func (s *LookupIndex) remove(rec *types.DomainLookup) {
	for _, it := range rec.IPs {
		delete(s.all, lookupKey{it.IP, rec.Domain})
		if domains, ok := s.byIP[it.IP]; ok {
			domains = slices.DeleteFunc(domains, func(s string) bool { return s == rec.Domain })
			if len(domains) == 0 {
				delete(s.byIP, it.IP)
			} else {
				s.byIP[it.IP] = domains
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

func (s *LookupIndex) Snapshot() []*types.DomainLookup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var res util.Set[*types.DomainLookup]
	for _, it := range s.all {
		res.Add(it)
	}
	return res.Values()
}

func (s *LookupIndex) Iterator() iter.Seq[*types.DomainLookup] {
	return func(yield func(*types.DomainLookup) bool) {
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
