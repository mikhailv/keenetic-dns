package types

import "time"

type DomainLookup struct {
	Time     Timestamp             `json:"time"`
	Resolver ResolverInfo          `json:"resolver"`
	Domain   string                `json:"domain"`
	CNames   []DomainEntry[string] `json:"cnames,omitempty"`
	IPs      []DomainIP            `json:"ips"`
}

func (s *DomainLookup) Expired(extraTTL time.Duration) bool {
	ageSeconds := int(time.Since(s.Time.Time()).Seconds() - extraTTL.Seconds())
	for _, ip := range s.IPs {
		if ip.Expired(ageSeconds) {
			return true
		}
	}
	return false
}

type DomainIP struct {
	IP          IPv4                  `json:"ip"`
	TTL         uint32                `json:"ttl"`
	PTR         []DomainEntry[string] `json:"ptr,omitempty"`
	SOA         []DomainEntry[string] `json:"soa,omitempty"`
	PTRResolver ResolverInfo          `json:"ptr_resolver"`
}

func (s *DomainIP) Expired(ageSeconds int) bool {
	if int(s.TTL) <= ageSeconds {
		return true
	}
	for _, it := range s.PTR {
		if it.Expired(ageSeconds) {
			return true
		}
	}
	for _, it := range s.SOA {
		if it.Expired(ageSeconds) {
			return true
		}
	}
	return false
}

type DomainEntry[T comparable] struct {
	Name T      `json:"name"`
	TTL  uint32 `json:"ttl"`
}

func (s *DomainEntry[T]) Expired(ageSeconds int) bool {
	return int(s.TTL) <= ageSeconds
}

type ResolverInfo struct {
	Name     string  `json:"name"`
	Duration float64 `json:"duration"`
}
