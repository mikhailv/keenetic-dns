package dnssvc

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

func isSucceededResponse(resp *dns.Msg, qtype uint16) bool {
	if resp == nil || !resp.Response || resp.Rcode != dns.RcodeSuccess {
		return false
	}
	if qtype == dns.TypeANY {
		return len(resp.Answer) > 0
	}
	return slices.ContainsFunc(resp.Answer, func(rr dns.RR) bool {
		return rr.Header().Rrtype == qtype
	})
}

func RefusedResponse(req *dns.Msg) *dns.Msg {
	resp := &dns.Msg{}
	resp.SetRcode(req, dns.RcodeRefused)
	return resp
}
