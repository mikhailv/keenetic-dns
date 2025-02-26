package resolver

import (
	"slices"

	"github.com/miekg/dns"
)

func HasSingleQuestion(msg *dns.Msg, types ...uint16) bool {
	if len(msg.Question) != 1 {
		return false
	}
	return len(types) == 0 || slices.Contains(types, msg.Question[0].Qtype)
}

func isSucceededResponse(resp *dns.Msg) bool {
	return resp != nil && resp.Response && len(resp.Answer) > 0 && resp.Rcode == dns.RcodeSuccess
}

func RefusedResponse(req *dns.Msg) *dns.Msg {
	resp := &dns.Msg{}
	resp.SetRcode(req, dns.RcodeRefused)
	return resp
}
