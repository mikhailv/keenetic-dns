package lookup

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

func ip(s string) types.IPv4 {
	return types.MustParseIPv4(s)
}

func TestIPTree_ExactHost(t *testing.T) {
	b := NewIPTreeBuilder[string]()
	b.Add(ip("8.6.112.100"), "host")
	tree := b.Build()

	v, ok := tree.Get(ip("8.6.112.100"))
	assert.True(t, ok)
	assert.Equal(t, "host", v)

	_, ok = tree.Get(ip("8.6.112.101"))
	assert.False(t, ok)
}

func TestIPTree_SubnetMatch(t *testing.T) {
	b := NewIPTreeBuilder[string]()
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

func TestIPTree_LongestPrefix(t *testing.T) {
	b := NewIPTreeBuilder[string]()
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

func TestIPTree_Has(t *testing.T) {
	b := NewIPTreeBuilder[string]()
	b.Add(ip("192.168.1.0/24"), "lan")
	tree := b.Build()

	assert.True(t, tree.Has(ip("192.168.1.1")))
	assert.False(t, tree.Has(ip("192.168.2.1")))
}

func TestIPTree_MidOctetPrefix(t *testing.T) {
	b := NewIPTreeBuilder[string]()
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

func TestIPTree_OverlappingMidOctet(t *testing.T) {
	b := NewIPTreeBuilder[string]()
	b.Add(ip("10.1.0.0/20"), "slash20")
	b.Add(ip("10.1.2.0/24"), "slash24")
	tree := b.Build()

	v, _ := tree.Get(ip("10.1.2.5"))
	assert.Equal(t, "slash24", v)

	v, _ = tree.Get(ip("10.1.3.5"))
	assert.Equal(t, "slash20", v)
}

func TestIPTree_Empty(t *testing.T) {
	tree := NewIPTreeBuilder[string]().Build()
	_, ok := tree.Get(ip("1.2.3.4"))
	assert.False(t, ok)
}
