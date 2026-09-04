package dnssvc

import (
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
)

func response(qtype uint16, answer ...dns.RR) *dns.Msg {
	req := &dns.Msg{}
	req.SetQuestion("tr.rbxcdn.com.", qtype)
	resp := &dns.Msg{}
	resp.SetReply(req)
	resp.Answer = answer
	return resp
}

func cname(name, target string) dns.RR {
	return &dns.CNAME{
		Hdr:    dns.RR_Header{Name: name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60},
		Target: target,
	}
}

func a(name, ip string) dns.RR {
	return &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP(ip),
	}
}

func TestIsSucceededResponse(t *testing.T) {
	tests := []struct {
		name  string
		qtype uint16
		resp  *dns.Msg
		want  bool
	}{
		{
			name:  "nil",
			qtype: dns.TypeA,
			resp:  nil,
		},
		{
			name:  "empty answer",
			qtype: dns.TypeA,
			resp:  response(dns.TypeA),
		},
		{
			name:  "cname only",
			qtype: dns.TypeA,
			resp:  response(dns.TypeA, cname("tr.rbxcdn.com.", "trns1.rbxcdn.com.")),
		},
		{
			name:  "complete cname chain",
			qtype: dns.TypeA,
			resp: response(dns.TypeA,
				cname("tr.rbxcdn.com.", "trns1.rbxcdn.com."),
				cname("trns1.rbxcdn.com.", "traws.rbxcdn.com."),
				cname("traws.rbxcdn.com.", "d77muyc5iodv8.cloudfront.net."),
				a("d77muyc5iodv8.cloudfront.net.", "3.164.230.91"),
			),
			want: true,
		},
		{
			name:  "address only",
			qtype: dns.TypeA,
			resp:  response(dns.TypeA, a("tr.rbxcdn.com.", "3.164.230.91")),
			want:  true,
		},
		{
			name:  "cname answering a cname query",
			qtype: dns.TypeCNAME,
			resp:  response(dns.TypeCNAME, cname("tr.rbxcdn.com.", "trns1.rbxcdn.com.")),
			want:  true,
		},
		{
			name:  "any query",
			qtype: dns.TypeANY,
			resp:  response(dns.TypeANY, cname("tr.rbxcdn.com.", "trns1.rbxcdn.com.")),
			want:  true,
		},
		{
			name:  "any query with empty answer",
			qtype: dns.TypeANY,
			resp:  response(dns.TypeANY),
		},
		{
			name:  "address for a different type",
			qtype: dns.TypeAAAA,
			resp:  response(dns.TypeAAAA, a("tr.rbxcdn.com.", "3.164.230.95")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSucceededResponse(tt.resp, tt.qtype))
		})
	}
}

func TestIsSucceededResponse_ServFail(t *testing.T) {
	resp := response(dns.TypeA, a("tr.rbxcdn.com.", "3.164.230.91"))
	resp.Rcode = dns.RcodeServerFailure
	assert.False(t, isSucceededResponse(resp, dns.TypeA))
}

func TestIsSucceededResponse_EchoedQuestionIsIgnored(t *testing.T) {
	resp := response(dns.TypeAAAA, a("tr.rbxcdn.com.", "3.164.230.91"))
	assert.True(t, isSucceededResponse(resp, dns.TypeA), "the type asked for decides, not the one the response echoes")

	resp.Question = nil
	assert.True(t, isSucceededResponse(resp, dns.TypeA))
}
