package resolvestats

import (
	"bufio"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/klauspost/compress/gzip"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/aggstats"
)

func response(name string, qtype uint16, answers ...dns.RR) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(name, qtype)
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Answer = answers
	return resp
}

func hdr(name string, rrtype uint16) dns.RR_Header {
	return dns.RR_Header{Name: name, Rrtype: rrtype, Class: dns.ClassINET, Ttl: 60}
}

func TestAnswerKeys(t *testing.T) {
	tests := []struct {
		name string
		resp *dns.Msg
		want []Key
	}{
		{
			name: "A answers are recorded under the queried name, not the CNAME target",
			resp: response("WWW.YouTube.com.", dns.TypeA,
				&dns.CNAME{Hdr: hdr("www.youtube.com.", dns.TypeCNAME), Target: "youtube-ui.l.google.com."},
				&dns.A{Hdr: hdr("youtube-ui.l.google.com.", dns.TypeA), A: net.ParseIP("142.250.1.1")},
				&dns.A{Hdr: hdr("youtube-ui.l.google.com.", dns.TypeA), A: net.ParseIP("142.250.2.2")},
			),
			want: []Key{
				{Domain: "www.youtube.com.", Address: "142.250.1.1", QType: "A"},
				{Domain: "www.youtube.com.", Address: "142.250.2.2", QType: "A"},
			},
		},
		{
			name: "AAAA",
			resp: response("example.com.", dns.TypeAAAA, &dns.AAAA{Hdr: hdr("example.com.", dns.TypeAAAA), AAAA: net.ParseIP("2001:db8::1")}),
			want: []Key{{Domain: "example.com.", Address: "2001:db8::1", QType: "AAAA"}},
		},
		{
			name: "PTR maps the reversed name back to the address",
			resp: response("130.127.19.94.in-addr.arpa.", dns.TypePTR,
				&dns.PTR{Hdr: hdr("130.127.19.94.in-addr.arpa.", dns.TypePTR), Ptr: "Host.Example.NET."}),
			want: []Key{{Domain: "host.example.net.", Address: "94.19.127.130", QType: "PTR"}},
		},
		{
			name: "blocked answers carry no information",
			resp: response("ads.example.com.", dns.TypeA, &dns.A{Hdr: hdr("ads.example.com.", dns.TypeA), A: net.IPv4zero}),
		},
		{
			name: "malformed reverse names are skipped",
			resp: response("1.2.3.in-addr.arpa.", dns.TypePTR, &dns.PTR{Hdr: hdr("1.2.3.in-addr.arpa.", dns.TypePTR), Ptr: "x."}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, answerKeys(tt.resp))
		})
	}
}

func TestRecorder_WritesOriginAndCounts(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, s.Init(t.Context()))
	defer s.Close()

	r := NewRecorder(s, aggstats.Config{}, slog.New(slog.DiscardHandler))
	a := response("example.com.", dns.TypeA, &dns.A{Hdr: hdr("example.com.", dns.TypeA), A: net.ParseIP("93.184.216.34")})
	r.RecordResponse(OriginClient, a)
	r.RecordResponse(OriginClient, a)
	r.RecordResponse(OriginInternal, a)
	r.RecordResponse(OriginClient, nil)
	require.NoError(t, r.Close())

	var entries []Entry
	for c, err := range s.Load(t.Context(), aggstats.TimeRange{Start: 1, End: 1<<32 - 1}) {
		require.NoError(t, err)
		entries = append(entries, c.Entries...)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return len(a.Key.Origin) - len(b.Key.Origin) })
	assert.Equal(t, []Entry{
		{Key: Key{Domain: "example.com.", Address: "93.184.216.34", QType: "A", Origin: OriginClient}, Value: aggstats.Count{Count: 2}},
		{Key: Key{Domain: "example.com.", Address: "93.184.216.34", QType: "A", Origin: OriginInternal}, Value: aggstats.Count{Count: 1}},
	}, entries)

	files, err := filepath.Glob(filepath.Join(dir, "*", "*.tsv.gz"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	f, err := os.Open(files[0])
	require.NoError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(bufio.NewReader(f))
	require.NoError(t, err)
	header, err := bufio.NewReader(gz).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "domain\taddress\tqtype\torigin\tcount\n", header)
}
