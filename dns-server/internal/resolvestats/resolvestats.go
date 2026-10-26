package resolvestats

import (
	"context"
	"log/slog"
	"net"
	"strings"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/aggstats"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	OriginClient   = "client"
	OriginInternal = "internal"
)

type Key struct {
	Domain  string `tsv:"domain"`
	Address string `tsv:"address"`
	QType   string `tsv:"qtype"`
	Origin  string `tsv:"origin"`
}

type (
	Chunk = aggstats.Chunk[Key, aggstats.Count]
	Entry = aggstats.Entry[Key, aggstats.Count]
	Store = aggstats.Store[Key, aggstats.Count]
)

func NewFileStore(dir string, logger *slog.Logger) Store {
	return aggstats.NewFileStore[Key, aggstats.Count](dir, normalizeKey, logger)
}

func normalizeKey(k *Key) {
	k.Domain = util.FQDN(k.Domain)
}

type Recorder struct {
	rec *aggstats.Recorder[Key, aggstats.Count]
}

func NewRecorder(store Store, cfg aggstats.Config, logger *slog.Logger) *Recorder {
	return &Recorder{rec: aggstats.NewRecorder[Key, aggstats.Count](store, cfg, nil, logger)}
}

func (r *Recorder) RecordResponse(origin string, resp *dns.Msg) {
	if resp == nil || len(resp.Question) != 1 {
		return
	}
	for _, key := range answerKeys(resp) {
		key.Origin = origin
		r.rec.Record(key, func(v *aggstats.Count, offset uint16) {
			v.Add(offset)
		})
	}
}

func (r *Recorder) Start(ctx context.Context) util.Waiter {
	return r.rec.Start(ctx)
}

func (r *Recorder) Flush(ctx context.Context) error {
	return r.rec.Flush(ctx)
}

func (r *Recorder) Close() error {
	return r.rec.Close()
}

func answerKeys(resp *dns.Msg) []Key {
	qname := dns.CanonicalName(resp.Question[0].Name)
	var keys []Key
	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *dns.A:
			if !v.A.IsUnspecified() {
				keys = append(keys, Key{Domain: qname, Address: v.A.String(), QType: "A"})
			}
		case *dns.AAAA:
			if !v.AAAA.IsUnspecified() {
				keys = append(keys, Key{Domain: qname, Address: v.AAAA.String(), QType: "AAAA"})
			}
		case *dns.PTR:
			if ip := reverseIPv4(qname); ip != nil {
				keys = append(keys, Key{Domain: dns.CanonicalName(v.Ptr), Address: ip.String(), QType: "PTR"})
			}
		case *dns.HTTPS:
			keys = appendHints(keys, qname, "HTTPS", v.Value)
		case *dns.SVCB:
			keys = appendHints(keys, qname, "SVCB", v.Value)
		}
	}
	return keys
}

func appendHints(keys []Key, domain, qtype string, values []dns.SVCBKeyValue) []Key {
	for _, kv := range values {
		var hints []net.IP
		switch h := kv.(type) {
		case *dns.SVCBIPv4Hint:
			hints = h.Hint
		case *dns.SVCBIPv6Hint:
			hints = h.Hint
		}
		for _, ip := range hints {
			if !ip.IsUnspecified() {
				keys = append(keys, Key{Domain: domain, Address: ip.String(), QType: qtype})
			}
		}
	}
	return keys
}

func reverseIPv4(name string) net.IP {
	labels, ok := strings.CutSuffix(name, ".in-addr.arpa.")
	if !ok {
		return nil
	}
	octets := strings.Split(labels, ".")
	if len(octets) != 4 {
		return nil
	}
	for i, j := 0, len(octets)-1; i < j; i, j = i+1, j-1 {
		octets[i], octets[j] = octets[j], octets[i]
	}
	return net.ParseIP(strings.Join(octets, ".")).To4()
}
