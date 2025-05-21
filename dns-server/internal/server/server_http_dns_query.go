package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
)

func (s *HTTPServer) handleDNSQueryGET(w http.ResponseWriter, req *http.Request) (statusCode int, err error) {
	if req.Header.Get("Accept") != dnsMessageMediaType {
		return http.StatusBadRequest, errors.New("doh: unexpected request format")
	}

	query := req.URL.Query()
	if !query.Has("dns") {
		return http.StatusBadRequest, errors.New("doh: missing 'dns' query parameter")
	}

	dnsReqBody, err := base64.URLEncoding.DecodeString(req.URL.Query().Get("dns"))
	if err != nil {
		return http.StatusBadRequest, errors.New("doh: failed to decode base64 encoded dns request")
	}

	ctx := ctxutil.WithDNSQueryRemoteAddr(req.Context(), req.RemoteAddr)
	return s.handleDNSRequest(ctx, dnsReqBody, w)
}

func (s *HTTPServer) handleDNSQueryPOST(w http.ResponseWriter, req *http.Request) (statusCode int, err error) {
	if req.Header.Get("Content-Type") != dnsMessageMediaType || req.Header.Get("Accept") != dnsMessageMediaType {
		return http.StatusBadRequest, errors.New("doh: unexpected request format")
	}

	dnsReqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("doh: failed to read body: %w", err)
	}

	ctx := ctxutil.WithDNSQueryRemoteAddr(req.Context(), req.RemoteAddr)
	return s.handleDNSRequest(ctx, dnsReqBody, w)
}

func (s *HTTPServer) handleDNSRequest(ctx context.Context, reqBody []byte, w http.ResponseWriter) (statusCode int, err error) {
	var dnsReq dns.Msg
	if err = dnsReq.Unpack(reqBody); err != nil {
		return http.StatusBadRequest, fmt.Errorf("doh: failed to unpack DNS request: %w", err)
	}

	dnsResp, err := s.resolver.Resolve(ctx, &dnsReq)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("doh: failed to send DNS request: %w", err)
	}

	dnsRespBytes, err := dnsResp.Pack()
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("doh: failed to pack DNS response: %w", err)
	}

	w.Header().Set("Content-Type", dnsMessageMediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(dnsRespBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dnsRespBytes)

	return http.StatusOK, nil
}
