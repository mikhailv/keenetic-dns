package server

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type stubResolver struct{}

func (stubResolver) Name() string { return "stub" }
func (stubResolver) Close() error { return nil }

func (stubResolver) Resolve(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Answer = append(resp.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.IPv4(127, 0, 0, 1),
	})
	return resp, nil
}

func TestDNSServer_ServesUDPAndTCP(t *testing.T) {
	addr := "127.0.0.1:" + strconv.Itoa(freePort(t))

	srv := NewDNSServer(addr, slog.New(slog.DiscardHandler), stubResolver{})

	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-serveErr; err != nil {
			t.Errorf("Serve returned error: %v", err)
		}
	})

	waitListening(t, addr)

	for _, network := range []string{"udp", "tcp"} {
		t.Run(network, func(t *testing.T) {
			c := &dns.Client{Net: network, Timeout: 2 * time.Second}
			req := new(dns.Msg)
			req.SetQuestion("example.com.", dns.TypeA)

			resp, _, err := c.Exchange(req, addr)
			if err != nil {
				t.Fatalf("exchange over %s failed: %v", network, err)
			}
			if len(resp.Answer) != 1 {
				t.Fatalf("expected 1 answer over %s, got %d", network, len(resp.Answer))
			}
		})
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	defer l.Close() //nolint:errcheck
	return l.Addr().(*net.TCPAddr).Port
}

func waitListening(t *testing.T, addr string) {
	t.Helper()
	for range 100 {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close() //nolint:errcheck
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not start listening on %s", addr)
}
