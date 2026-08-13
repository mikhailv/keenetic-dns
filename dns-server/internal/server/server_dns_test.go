package server

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/server/ctxutil"
	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

type stubResolver struct {
	mu        sync.Mutex
	clientIP  types.IPv4
	hasClient bool
}

func (*stubResolver) Name() string { return "stub" }
func (*stubResolver) Close() error { return nil }

func (s *stubResolver) Resolve(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	ip, ok := ctxutil.GetDNSQueryClientIP(ctx)
	s.mu.Lock()
	s.clientIP, s.hasClient = ip, ok
	s.mu.Unlock()

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

	resolver := &stubResolver{}
	srv := NewDNSServer(addr, slog.New(slog.DiscardHandler), resolver)

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

			resolver.mu.Lock()
			clientIP, hasClient := resolver.clientIP, resolver.hasClient
			resolver.mu.Unlock()
			if !hasClient {
				t.Fatalf("no client IP in context over %s", network)
			}
			if got := clientIP.String(); got != "127.0.0.1" {
				t.Fatalf("client IP over %s = %q, want 127.0.0.1", network, got)
			}
		})
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type %T", l.Addr())
	}
	return addr.Port
}

func waitListening(t *testing.T, addr string) {
	t.Helper()
	dialer := net.Dialer{Timeout: 100 * time.Millisecond}
	for range 100 {
		conn, err := dialer.DialContext(t.Context(), "tcp", addr)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not start listening on %s", addr)
}
