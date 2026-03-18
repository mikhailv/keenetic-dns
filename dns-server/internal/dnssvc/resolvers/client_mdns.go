package resolvers

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/miekg/dns"
	"github.com/pion/mdns/v2"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/metrics"
)

var _ dnssvc.Resolver = (*mdnsClient)(nil)

type mdnsClient struct {
	name    string
	address string
	timeout time.Duration
	conn    struct {
		sync.RWMutex
		*mdns.Conn
	}
}

func NewMDNSClient(name string, address string, timeout time.Duration) dnssvc.Resolver {
	return &mdnsClient{
		name:    name,
		address: address,
		timeout: timeout,
	}
}

func (s *mdnsClient) Name() string {
	return s.name
}

func (s *mdnsClient) Resolve(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	defer metrics.TrackNamedDuration("mdns_client.resolve", s.name)()
	conn, err := s.connection()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	answer, src, err := conn.QueryAddr(ctx, strings.TrimRight(msg.Question[0].Name, "."))
	cancel()
	if err != nil {
		return nil, err
	}

	resp := &dns.Msg{}
	resp.SetRcode(msg, dns.RcodeSuccess)
	resp.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{
			Name:     answer.Name.String(),
			Rrtype:   dns.TypeA,
			Class:    dns.ClassINET,
			Ttl:      answer.TTL,
			Rdlength: answer.Length,
		},
		A: src.AsSlice(),
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

	addr, err := net.ResolveUDPAddr("udp4", s.address)
	if err != nil {
		return nil, err
	}
	pconn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	conn, err := mdns.Server(ipv4.NewPacketConn(pconn), nil, &mdns.Config{})
	if err != nil {
		return nil, err
	}
	s.conn.Conn = conn
	return conn, nil
}

func (s *mdnsClient) Close() error {
	s.conn.Lock()
	defer s.conn.Unlock()
	if s.conn.Conn != nil {
		return s.conn.Close()
	}
	return nil
}
