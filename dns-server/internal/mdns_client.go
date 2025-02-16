package internal

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/miekg/dns"
	"github.com/pion/mdns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

var _ DNSResolver = (*mdnsClient)(nil)

type mdnsClient struct {
	timeout time.Duration
	conn    struct {
		sync.RWMutex
		*mdns.Conn
	}
}

func newMDNSClient(timeout time.Duration) *mdnsClient {
	return &mdnsClient{
		timeout: timeout,
	}
}

func (s *mdnsClient) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackDuration("mdns_client.resolve")()
	conn, err := s.connection()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	answer, src, err := conn.Query(ctx, strings.TrimRight(msg.Question[0].Name, "."))
	cancel()
	if err != nil {
		return nil, err
	}

	resp := msg.Copy()
	resp.Response = true
	resp.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{
			Name:     answer.Name.String(),
			Rrtype:   dns.TypeA,
			Class:    dns.ClassINET,
			Ttl:      answer.TTL,
			Rdlength: answer.Length,
		},
		A: src.(*net.IPAddr).IP, //nolint:errcheck // no need to check type
	}}
	return resp, nil
}

func (s *mdnsClient) connection() (*mdns.Conn, error) {
	s.conn.RLock()
	if s.conn.Conn != nil {
		return s.conn.Conn, nil
	}
	s.conn.RUnlock()

	s.conn.Lock()
	defer s.conn.Unlock()
	if s.conn.Conn != nil {
		return s.conn.Conn, nil
	}

	addr, err := net.ResolveUDPAddr("udp4", mdns.DefaultAddress)
	if err != nil {
		return nil, err
	}
	pconn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	conn, err := mdns.Server(ipv4.NewPacketConn(pconn), &mdns.Config{})
	if err != nil {
		return nil, err
	}
	s.conn.Conn = conn
	return conn, nil
}
