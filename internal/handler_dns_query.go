package internal

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func (s *DoHServer) handleDNSQuery(w http.ResponseWriter, req *http.Request) (statusCode int, err error) {
	if req.Header.Get("Content-Type") != dnsMessageMediaType || req.Header.Get("Accept") != dnsMessageMediaType {
		return http.StatusBadRequest, errors.New("unexpected request format")
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to read body: %w", err)
	}

	var dnsReq dns.Msg
	if err = dnsReq.Unpack(body); err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to unpack DNS request: %w", err)
	}

	dnsResp, dsnRespBytes, err := s.dohService.Send(req.Context(), dnsReq)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to send DNS request: %w", err)
	}

	if dnsResp.Question[0].Qtype == dns.TypeA {
		domain := strings.TrimRight(dnsResp.Question[0].Name, ".")
		if iface, ok := s.config.Routing.LookupHost(domain); ok {
			for _, it := range dnsResp.Answer {
				if a, ok := it.(*dns.A); ok {
					rec := NewDNSRecord(domain, NewIPv4(a.A), time.Duration(a.Hdr.Ttl)*time.Second)
					s.ipRoutes.AddRoute(req.Context(), rec, iface)
				}
			}
		}
		fmt.Printf("\t%s\t%d ip\n", domain, len(dnsResp.Answer))
	}

	w.Header().Set("Content-Type", dnsMessageMediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(dsnRespBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dsnRespBytes)

	return http.StatusOK, nil
}
