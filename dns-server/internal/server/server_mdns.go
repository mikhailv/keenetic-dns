package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"

	"golang.org/x/net/ipv4"

	"github.com/miekg/dns"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/config"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const (
	mdnsAddr             = "224.0.0.251:5353"
	mdnsDomain           = "local."
	dnsSDBrowse          = "_services._dns-sd._udp.local."
	mdnsServiceRecordTTL = 4500 // PTR, TXT
	mdnsHostRecordTTL    = 120
)

type mdnsService struct {
	InstanceFQDN string // "My Service._http._tcp.local."
	ServiceFQDN  string // "_http._tcp.local."
	HostFQDN     string // "myhost.local."
	Port         uint16
	IPs          []net.IP
	TXT          []string // "key=value"
}

type MDNSServer struct {
	logger   *slog.Logger
	cfg      *util.Dynamic[[]config.MDNSService]
	iface    string
	services atomic.Pointer[[]*mdnsService]
}

func NewMDNSServer(logger *slog.Logger, iface string, cfg *util.Dynamic[[]config.MDNSService]) *MDNSServer {
	return &MDNSServer{
		logger: logger,
		cfg:    cfg,
		iface:  iface,
	}
}

func (s *MDNSServer) Serve(ctx context.Context) error {
	iface, err := net.InterfaceByName(s.iface)
	if err != nil {
		return fmt.Errorf("mdns: interface %q: %w", s.iface, err)
	}

	addr, err := net.ResolveUDPAddr("udp4", mdnsAddr)
	if err != nil {
		return fmt.Errorf("mdns: resolve addr: %w", err)
	}

	conn, err := net.ListenMulticastUDP("udp4", iface, addr)
	if err != nil {
		return fmt.Errorf("mdns: listen: %w", err)
	}
	defer conn.Close()

	p := ipv4.NewPacketConn(conn)
	if err = p.SetMulticastLoopback(true); err != nil {
		return fmt.Errorf("mdns: set multicast loopback: %w", err)
	}

	s.logger.Info("server starting...", "iface", s.iface)

	s.refresh()
	s.cfg.Listen(s.refresh)

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	return s.serveLoop(ctx, conn)
}

func (s *MDNSServer) serveLoop(ctx context.Context, conn *net.UDPConn) error {
	mcastDst := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

	var buf [64 << 10]byte
	for {
		n, src, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // it's ok to return nil, context canceled
			}
			s.logger.Warn("mdns read error", "err", err)
			continue
		}

		var msg dns.Msg
		if err = msg.Unpack(buf[:n]); err != nil {
			continue
		}

		if msg.Response {
			continue
		}

		resp := s.handleQuery(&msg)
		if resp == nil {
			continue
		}

		data, err := resp.Pack()
		if err != nil {
			s.logger.Warn("mdns pack error", "err", err)
			continue
		}

		// RFC 6762: respond to multicast unless QU (unicast-question) bit is set.
		dst := mcastDst
		if hasUnicastQuestion(&msg) {
			dst = src
		}
		if _, err = conn.WriteToUDP(data, dst); err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // it's ok to return nil, context canceled
			}
			s.logger.Warn("mdns write error", "err", err)
		}
	}
}

func (s *MDNSServer) refresh() {
	serviceDefs := s.cfg.Get()
	services := make([]*mdnsService, 0, len(serviceDefs))
	for _, def := range serviceDefs {
		host := def.Host
		if !strings.HasSuffix(host, ".") {
			host += "."
		}
		if !strings.HasSuffix(host, ".local.") {
			host += "local."
		}

		svcType := def.Service + "." + mdnsDomain
		instance := escapeMDNSName(def.Name) + "." + svcType

		var txt []string
		if len(def.TXT) == 0 {
			txt = []string{""}
		} else {
			txt = make([]string, 0, len(def.TXT))
			for k, v := range def.TXT {
				txt = append(txt, k+"="+v)
			}
		}

		svc := &mdnsService{
			InstanceFQDN: instance,
			ServiceFQDN:  svcType,
			HostFQDN:     host,
			Port:         uint16(def.Port),
			IPs:          def.IP,
			TXT:          txt,
		}
		services = append(services, svc)

		s.logger.Info("registered service",
			"host", def.Host, "port", def.Port, "ip", def.IP, "name", def.Name)
	}

	s.services.Store(&services)
}

func (s *MDNSServer) handleQuery(query *dns.Msg) *dns.Msg { //nolint:cyclop,funlen,gocognit // ok
	services := *s.services.Load()

	var resp dns.Msg
	resp.SetReply(query)
	resp.Authoritative = true
	resp.RecursionAvailable = false

	for _, q := range query.Question {
		qname := strings.ToLower(q.Name)

		switch {
		case q.Qtype == dns.TypePTR && qname == dnsSDBrowse:
			// DNS-SD service type enumeration
			seen := map[string]bool{}
			for _, svc := range services {
				if seen[svc.ServiceFQDN] {
					continue
				}
				seen[svc.ServiceFQDN] = true
				resp.Answer = append(resp.Answer, &dns.PTR{
					Hdr: dns.RR_Header{
						Name:   dnsSDBrowse,
						Rrtype: dns.TypePTR,
						Class:  dns.ClassINET,
						Ttl:    mdnsServiceRecordTTL,
					},
					Ptr: svc.ServiceFQDN,
				})
			}

		case q.Qtype == dns.TypePTR:
			// Service instance enumeration
			for _, svc := range services {
				if strings.EqualFold(svc.ServiceFQDN, qname) {
					resp.Answer = append(resp.Answer, &dns.PTR{
						Hdr: dns.RR_Header{
							Name:   svc.ServiceFQDN,
							Rrtype: dns.TypePTR,
							Class:  dns.ClassINET,
							Ttl:    mdnsServiceRecordTTL,
						},
						Ptr: svc.InstanceFQDN,
					})
					s.addServiceRecords(&resp, svc)
				}
			}

		case q.Qtype == dns.TypeSRV:
			for _, svc := range services {
				if strings.EqualFold(svc.InstanceFQDN, qname) {
					resp.Answer = append(resp.Answer, &dns.SRV{
						Hdr: dns.RR_Header{
							Name:   svc.InstanceFQDN,
							Rrtype: dns.TypeSRV,
							Class:  dns.ClassINET,
							Ttl:    mdnsHostRecordTTL,
						},
						Port:   svc.Port,
						Target: svc.HostFQDN,
					})
					s.addAddressRecords(&resp.Extra, svc, dns.TypeA)
					s.addAddressRecords(&resp.Extra, svc, dns.TypeAAAA)
				}
			}

		case q.Qtype == dns.TypeTXT:
			for _, svc := range services {
				if strings.EqualFold(svc.InstanceFQDN, qname) {
					resp.Answer = append(resp.Answer, &dns.TXT{
						Hdr: dns.RR_Header{
							Name:   svc.InstanceFQDN,
							Rrtype: dns.TypeTXT,
							Class:  dns.ClassINET,
							Ttl:    mdnsServiceRecordTTL,
						},
						Txt: svc.TXT,
					})
				}
			}

		case q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA:
			for _, svc := range services {
				if strings.EqualFold(svc.HostFQDN, qname) {
					s.addAddressRecords(&resp.Answer, svc, q.Qtype)
				}
			}

		case q.Qtype == dns.TypeANY:
			for _, svc := range services {
				if strings.EqualFold(svc.InstanceFQDN, qname) {
					s.addServiceRecords(&resp, svc)
				} else if strings.EqualFold(svc.HostFQDN, qname) {
					s.addAddressRecords(&resp.Answer, svc, dns.TypeA)
					s.addAddressRecords(&resp.Answer, svc, dns.TypeAAAA)
				}
			}
		}
	}

	if len(resp.Answer) == 0 && len(resp.Extra) == 0 {
		return nil
	}

	return &resp
}

func (s *MDNSServer) addServiceRecords(resp *dns.Msg, svc *mdnsService) {
	resp.Extra = append(resp.Extra, &dns.SRV{
		Hdr: dns.RR_Header{
			Name:   svc.InstanceFQDN,
			Rrtype: dns.TypeSRV,
			Class:  dns.ClassINET,
			Ttl:    mdnsHostRecordTTL,
		},
		Port:   svc.Port,
		Target: svc.HostFQDN,
	})
	resp.Extra = append(resp.Extra, &dns.TXT{
		Hdr: dns.RR_Header{
			Name:   svc.InstanceFQDN,
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
			Ttl:    mdnsServiceRecordTTL,
		},
		Txt: svc.TXT,
	})
	s.addAddressRecords(&resp.Extra, svc, dns.TypeA)
	s.addAddressRecords(&resp.Extra, svc, dns.TypeAAAA)
}

func (s *MDNSServer) addAddressRecords(records *[]dns.RR, svc *mdnsService, qtype uint16) {
	for _, ip := range svc.IPs {
		switch qtype {
		case dns.TypeA:
			if ip4 := ip.To4(); ip4 != nil {
				*records = append(*records, &dns.A{
					Hdr: dns.RR_Header{
						Name:   svc.HostFQDN,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    mdnsHostRecordTTL,
					},
					A: ip4,
				})
			}
		case dns.TypeAAAA:
			if ip16 := ip.To16(); ip16 != nil && ip.To4() == nil {
				*records = append(*records, &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   svc.HostFQDN,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    mdnsHostRecordTTL,
					},
					AAAA: ip16,
				})
			}
		default:
			panic("unexpected query type: " + dns.TypeToString[qtype])
		}
	}
}

// hasUnicastQuestion returns true if any question has the QU bit set (RFC 6762 §5.4).
func hasUnicastQuestion(msg *dns.Msg) bool {
	for _, q := range msg.Question {
		if q.Qclass&(1<<15) != 0 {
			return true
		}
	}
	return false
}

// escapeMDNSName escapes dots in mDNS service instance names.
func escapeMDNSName(name string) string {
	return strings.ReplaceAll(name, ".", "\\.")
}
