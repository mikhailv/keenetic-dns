package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

var _ DNSResolver = (*dohClient)(nil)

type dohClient struct {
	url    string
	client http.Client
}

func NewDoHClient(url string, timeout time.Duration) DNSResolver {
	return &dohClient{
		url: url,
		client: http.Client{
			Timeout: timeout,
		},
	}
}

func (s *dohClient) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackDuration("doh_client.resolve")()

	reqBody, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("doh_client: failed to pack request message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", s.url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("doh_client: failed to create request: %w", err)
	}
	req.Header.Set("Accept", dnsMessageMediaType)
	req.Header.Set("Content-Type", dnsMessageMediaType)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doh_client: failed to send request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doh_client: unexpected status code: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("doh_client: failed to read response body: %w", err)
	}
	var res dns.Msg
	if err = res.Unpack(respBody); err != nil {
		return nil, fmt.Errorf("doh_client: failed to unpack response message: %w", err)
	}
	return &res, nil
}
