package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

var queryStatuses = map[string]func(*types.DNSQuery) bool{
	"reused":   (*types.DNSQuery).Reused,
	"blocked":  (*types.DNSQuery).IsBlocked,
	"routed":   (*types.DNSQuery).Routed,
	"excluded": (*types.DNSQuery).Excluded,
	"direct":   (*types.DNSQuery).Direct,
}

func (s *HTTPServer) filterQueries(_ *http.Request, query url.Values) FilterFunc[types.DNSQuery] {
	domain := strings.TrimSpace(query.Get("domain"))
	search := strings.ToLower(strings.TrimSpace(query.Get("search")))
	client, hasClient := parseQueryClient(query.Get("client"))
	statuses := parseQueryStatuses(query.Get("status"))
	if domain == "" && search == "" && !hasClient && len(statuses) == 0 {
		return nil
	}
	var searchClients util.Set[types.IPv4]
	if search != "" {
		searchClients = s.clientsMatching(search)
	}
	return func(val types.DNSQuery) bool {
		if len(statuses) > 0 && !matchesAnyQueryStatus(&val, statuses) {
			return false
		}
		if hasClient && val.ClientIP != client {
			return false
		}
		if search != "" && !matchesQuerySearch(&val, search, searchClients) {
			return false
		}
		if domain != "" && val.Domain != domain {
			return false
		}
		return true
	}
}

func parseQueryClient(value string) (types.IPv4, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return types.IPv4{}, false
	}
	ip, err := types.ParseIPv4(value)
	if err != nil {
		return types.IPv4{}, false
	}
	return ip, true
}

func parseQueryStatuses(value string) []func(*types.DNSQuery) bool {
	var res []func(*types.DNSQuery) bool
	for name := range strings.SplitSeq(value, ",") {
		if match, ok := queryStatuses[strings.TrimSpace(name)]; ok {
			res = append(res, match)
		}
	}
	return res
}

func matchesAnyQueryStatus(val *types.DNSQuery, statuses []func(*types.DNSQuery) bool) bool {
	for _, match := range statuses {
		if match(val) {
			return true
		}
	}
	return false
}

func matchesQuerySearch(val *types.DNSQuery, search string, searchClients util.Set[types.IPv4]) bool {
	if containsFold(val.Domain, search) || strings.Contains(val.ID.String(), search) {
		return true
	}
	if strings.Contains(val.ClientIP.String(), search) || searchClients.Has(val.ClientIP) {
		return true
	}
	if val.Blocked != nil && (containsFold(val.Blocked.Domain, search) || containsFold(val.Blocked.List, search)) {
		return true
	}
	if val.Lookup != nil {
		for _, cname := range val.Lookup.CNames {
			if containsFold(cname.Name, search) {
				return true
			}
		}
		for _, ip := range val.Lookup.IPs {
			if strings.Contains(ip.IP.String(), search) {
				return true
			}
			for _, ptr := range ip.PTR {
				if containsFold(ptr.Name, search) {
					return true
				}
			}
		}
	}
	for ip := range val.IPRoutings {
		if strings.Contains(ip.String(), search) {
			return true
		}
	}
	return false
}

func containsFold(value, lowerSearch string) bool {
	return strings.Contains(strings.ToLower(value), lowerSearch)
}

func (s *HTTPServer) clientsMatching(search string) util.Set[types.IPv4] {
	hosts, err := s.hostList()
	if err != nil {
		return nil
	}
	var res util.Set[types.IPv4]
	for _, host := range hosts {
		if host.Ip == nil {
			continue
		}
		if !matchesHostName(host.Name, search) && !matchesHostName(host.Hostname, search) {
			continue
		}
		if ip, err := types.ParseIPv4(*host.Ip); err == nil {
			res.Add(ip)
		}
	}
	return res
}

func matchesHostName(name *string, search string) bool {
	return name != nil && containsFold(*name, search)
}
