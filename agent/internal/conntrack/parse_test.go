package conntrack

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/agent/internal/api"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

func TestParse(t *testing.T) {
	content := `tcp      6 422 ESTABLISHED src=192.168.2.91 dst=91.105.192.100 sport=36838 dport=443 packets=7 bytes=658 src=91.105.192.100 dst=10.10.11.245 sport=443 dport=36838 packets=4 bytes=499 [ASSURED] [FASTNAT] mark=0 nmark=0 sc=0 ifw=35 ifl=31 mac=00:e0:20:50:23:48 slan attrs= use=2 id=12345
udp      17 143 src=192.168.2.4 dst=192.168.2.1 sport=49776 dport=53 packets=9 bytes=738 src=192.168.2.1 dst=192.168.2.4 sport=53 dport=49776 packets=9 bytes=2570 [ASSURED] mark=0 nmark=0 sc=0 ifl=31 mac=50:ff:20:b2:75:78 slan attrs= use=2 id=67890
icmp     1 26 src=192.168.2.83 dst=213.180.193.230 type=8 code=0 id=1638 packets=33462 bytes=936936 src=213.180.193.230 dst=77.40.49.132 type=0 code=0 id=1638 packets=33404 bytes=935312 [FASTNAT] [RTCACHE o31/r32] mark=0 nmark=256 sc=0 ifw=32 ifl=31 mac=b8:87:6e:19:66:e1 slan attrs= use=2 id=99999
tcp      6 27 TIME_WAIT src=192.168.2.1 dst=192.168.2.4 sport=52102 dport=443 packets=19 bytes=1990 src=192.168.2.4 dst=192.168.2.1 sport=443 dport=52102 packets=25 bytes=23413 [ASSURED] [FASTNAT] mark=0 nmark=0 sc=0 nomac swan no_if attrs= use=2 id=11111
`

	want := []api.ConntrackEntry{
		{
			Protocol:     "tcp",
			State:        util.Ptr("ESTABLISHED"),
			Ttl:          util.Ptr(422),
			SrcIp:        "192.168.2.91",
			DstIp:        "91.105.192.100",
			SrcPort:      util.Ptr(uint16(36838)),
			DstPort:      util.Ptr(uint16(443)),
			PacketsOrig:  7,
			BytesOrig:    658,
			PacketsReply: 4,
			BytesReply:   499,
			Mark:         util.Ptr(0),
			Assured:      util.Ptr(true),
			Id:           util.Ptr(uint32(12345)),
		},
		{
			Protocol:     "udp",
			Ttl:          util.Ptr(143),
			SrcIp:        "192.168.2.4",
			DstIp:        "192.168.2.1",
			SrcPort:      util.Ptr(uint16(49776)),
			DstPort:      util.Ptr(uint16(53)),
			PacketsOrig:  9,
			BytesOrig:    738,
			PacketsReply: 9,
			BytesReply:   2570,
			Mark:         util.Ptr(0),
			Assured:      util.Ptr(true),
			Id:           util.Ptr(uint32(67890)),
		},
		{
			Protocol:     "icmp",
			Ttl:          util.Ptr(26),
			SrcIp:        "192.168.2.83",
			DstIp:        "213.180.193.230",
			PacketsOrig:  33462,
			BytesOrig:    936936,
			PacketsReply: 33404,
			BytesReply:   935312,
			Mark:         util.Ptr(0),
			Id:           util.Ptr(uint32(99999)),
		},
		{
			Protocol:     "tcp",
			State:        util.Ptr("TIME_WAIT"),
			Ttl:          util.Ptr(27),
			SrcIp:        "192.168.2.1",
			DstIp:        "192.168.2.4",
			SrcPort:      util.Ptr(uint16(52102)),
			DstPort:      util.Ptr(uint16(443)),
			PacketsOrig:  19,
			BytesOrig:    1990,
			PacketsReply: 25,
			BytesReply:   23413,
			Mark:         util.Ptr(0),
			Assured:      util.Ptr(true),
			Id:           util.Ptr(uint32(11111)),
		},
	}

	got := Parse(content)
	require.Len(t, got, len(want))
	assert.Equal(t, want, got)
}

func TestParseEmpty(t *testing.T) {
	assert.Empty(t, Parse(""))
}
