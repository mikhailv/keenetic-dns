package lookup

import (
	"encoding/binary"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func ip(s string) types.IPv4 {
	return types.MustParseIPv4(s)
}

func TestIPRangeMap_ExactHost(t *testing.T) {
	b := NewIPRangeMapBuilder[string]()
	b.Add(ip("8.6.112.100"), "host")
	tree := b.Build()

	v, ok := tree.Get(ip("8.6.112.100"))
	assert.True(t, ok)
	assert.Equal(t, "host", v)

	_, ok = tree.Get(ip("8.6.112.101"))
	assert.False(t, ok)
}

func TestIPRangeMap_SubnetMatch(t *testing.T) {
	b := NewIPRangeMapBuilder[string]()
	b.Add(ip("8.6.112.0/24"), "subnet")
	tree := b.Build()

	v, ok := tree.Get(ip("8.6.112.100"))
	assert.True(t, ok)
	assert.Equal(t, "subnet", v)

	v, ok = tree.Get(ip("8.6.112.0"))
	assert.True(t, ok)
	assert.Equal(t, "subnet", v)

	v, ok = tree.Get(ip("8.6.112.255"))
	assert.True(t, ok)
	assert.Equal(t, "subnet", v)

	_, ok = tree.Get(ip("8.6.113.0"))
	assert.False(t, ok)

	_, ok = tree.Get(ip("8.6.111.255"))
	assert.False(t, ok)
}

func TestIPRangeMap_LongestPrefix(t *testing.T) {
	b := NewIPRangeMapBuilder[string]()
	b.Add(ip("10.0.0.0/8"), "big")
	b.Add(ip("10.1.0.0/16"), "medium")
	b.Add(ip("10.1.2.0/24"), "small")
	tree := b.Build()

	v, _ := tree.Get(ip("10.1.2.3"))
	assert.Equal(t, "small", v)

	v, _ = tree.Get(ip("10.1.3.3"))
	assert.Equal(t, "medium", v)

	v, _ = tree.Get(ip("10.2.0.0"))
	assert.Equal(t, "big", v)

	_, ok := tree.Get(ip("11.0.0.0"))
	assert.False(t, ok)
}

func TestIPRangeMap_Has(t *testing.T) {
	b := NewIPRangeMapBuilder[string]()
	b.Add(ip("192.168.1.0/24"), "lan")
	tree := b.Build()

	assert.True(t, tree.Has(ip("192.168.1.1")))
	assert.False(t, tree.Has(ip("192.168.2.1")))
}

func TestIPRangeMap_MidOctetPrefix(t *testing.T) {
	b := NewIPRangeMapBuilder[string]()
	b.Add(ip("10.1.0.0/20"), "slash20")
	tree := b.Build()

	// 10.1.0.0/20 covers 10.1.0.0 - 10.1.15.255
	v, ok := tree.Get(ip("10.1.0.1"))
	assert.True(t, ok)
	assert.Equal(t, "slash20", v)

	v, ok = tree.Get(ip("10.1.15.255"))
	assert.True(t, ok)
	assert.Equal(t, "slash20", v)

	_, ok = tree.Get(ip("10.1.16.0"))
	assert.False(t, ok)
}

func TestIPRangeMap_OverlappingMidOctet(t *testing.T) {
	b := NewIPRangeMapBuilder[string]()
	b.Add(ip("10.1.0.0/20"), "slash20")
	b.Add(ip("10.1.2.0/24"), "slash24")
	tree := b.Build()

	v, _ := tree.Get(ip("10.1.2.5"))
	assert.Equal(t, "slash24", v)

	v, _ = tree.Get(ip("10.1.3.5"))
	assert.Equal(t, "slash20", v)
}

func TestIPRangeMap_Empty(t *testing.T) {
	tree := NewIPRangeMapBuilder[string]().Build()
	_, ok := tree.Get(ip("1.2.3.4"))
	assert.False(t, ok)
}

func randomPrefixes(rng *rand.Rand, n int) []types.IPv4 {
	res := make([]types.IPv4, 0, n)
	for len(res) < n {
		var addr uint32
		if len(res) > 0 && rng.IntN(3) == 0 {
			addr = res[rng.IntN(len(res))].Uint32() ^ rng.Uint32()>>uint(8+rng.IntN(24))
		} else {
			addr = rng.Uint32()
		}
		var p types.IPv4
		binary.BigEndian.PutUint32(p[:4], addr)
		p[4] = byte(8 + rng.IntN(25))
		res = append(res, p)
	}
	return res
}

func bruteForceLongest(prefixes []types.IPv4, addr uint32) (int, bool) {
	best, bestLen := -1, -1
	for i, p := range prefixes {
		start, end := prefixRange(p)
		if start <= addr && addr <= end && p.Prefix() > bestLen {
			best, bestLen = i, p.Prefix()
		}
	}
	return best, best >= 0
}

func TestIPRangeMap_MatchesBruteForce(t *testing.T) {
	rng := newTestRand()
	for range 20 {
		prefixes := randomPrefixes(rng, 300)
		b := NewIPRangeMapBuilder[int]()
		for i, p := range prefixes {
			b.Add(p, i)
		}
		tree := b.Build()
		probe := func(addr uint32) {
			var ip types.IPv4
			binary.BigEndian.PutUint32(ip[:4], addr)
			ip[4] = 32
			want, wantOK := bruteForceLongest(prefixes, addr)
			got, gotOK := tree.Get(ip)
			require.Equal(t, wantOK, gotOK, "addr %08x", addr)
			if wantOK {
				require.Equal(t, prefixes[want].Prefix(), prefixes[got].Prefix(), "addr %08x", addr)
				start, end := prefixRange(prefixes[got])
				require.True(t, start <= addr && addr <= end, "addr %08x", addr)
			}
		}
		for _, p := range prefixes {
			start, end := prefixRange(p)
			for _, addr := range []uint32{start, end, start - 1, end + 1, start + (end-start)/2} {
				probe(addr)
			}
		}
		for range 2000 {
			probe(rng.Uint32())
		}
	}
}

func BenchmarkIPRangeMap_Build(b *testing.B) {
	for _, n := range []int{100, 1_000, 10_000, 100_000} {
		prefixes := randomPrefixes(newTestRand(), n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for b.Loop() {
				tb := NewIPRangeMapBuilder[int]()
				for i, p := range prefixes {
					tb.Add(p, i)
				}
				tb.Build()
			}
		})
	}
}

func BenchmarkIPRangeMap_Get(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 100_000} {
		rng := newTestRand()
		tb := NewIPRangeMapBuilder[int]()
		for i, p := range randomPrefixes(rng, n) {
			tb.Add(p, i)
		}
		tree := tb.Build()
		addrs := make([]types.IPv4, 1024)
		for i := range addrs {
			binary.BigEndian.PutUint32(addrs[i][:4], rng.Uint32())
			addrs[i][4] = 32
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			i := 0
			for b.Loop() {
				tree.Get(addrs[i&1023])
				i++
			}
		})
	}
}

func newTestRand() *rand.Rand {
	return rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a fixed seed keeps failures reproducible
}
