package lookup

import (
	"slices"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

// IPTree is an immutable stride-4 (nibble) trie for longest-prefix IPv4 matching.
// Each level corresponds to one nibble (4 bits), giving at most 8 lookups per query.
type IPTree[V any] struct {
	root *ipNode[V]
}

type ipNode[V any] struct {
	entries [16]ipEntry[V]
}

type ipEntry[V any] struct {
	child    *ipNode[V]
	value    V
	hasValue bool
}

// ipNibble returns the i-th nibble (4 bits) of an IPv4 address (0 = most significant).
func ipNibble(ip types.IPv4, i int) byte {
	b := ip[i/2]
	if i%2 == 0 {
		return b >> 4
	}
	return b & 0x0F
}

// Get returns the value associated with the longest matching prefix.
func (t *IPTree[V]) Get(ip types.IPv4) (V, bool) {
	if t.root == nil {
		var zero V
		return zero, false
	}
	var bestValue V
	var found bool
	node := t.root
	for i := range 8 {
		entry := &node.entries[ipNibble(ip, i)]
		if entry.hasValue {
			bestValue = entry.value
			found = true
		}
		if entry.child == nil {
			break
		}
		node = entry.child
	}
	return bestValue, found
}

// Has reports whether ip matches any prefix in the tree.
func (t *IPTree[V]) Has(ip types.IPv4) bool {
	_, ok := t.Get(ip)
	return ok
}

// IPTreeBuilder builds an immutable IPTree.
type IPTreeBuilder[V any] struct {
	items []ipBuildItem[V]
}

type ipBuildItem[V any] struct {
	prefix types.IPv4
	value  V
}

func NewIPTreeBuilder[V any]() *IPTreeBuilder[V] {
	return &IPTreeBuilder[V]{}
}

// Add inserts an IPv4 prefix with an associated value.
// The IPv4 may include a prefix length (e.g. "10.0.0.0/8"); /32 hosts work too.
func (b *IPTreeBuilder[V]) Add(prefix types.IPv4, value V) {
	b.items = append(b.items, ipBuildItem[V]{prefix: prefix, value: value})
}

// Build creates an immutable IPTree from the builder's contents.
// Prefixes are sorted by length (shortest first) so that longer (more specific)
// prefixes overwrite shorter ones during expansion.
func (b *IPTreeBuilder[V]) Build() *IPTree[V] {
	if len(b.items) == 0 {
		return &IPTree[V]{}
	}
	slices.SortStableFunc(b.items, func(a, b ipBuildItem[V]) int {
		return a.prefix.Prefix() - b.prefix.Prefix()
	})
	root := &ipNode[V]{}
	for _, item := range b.items {
		insertIPPrefix(root, item.prefix, item.value)
	}
	return &IPTree[V]{root: root}
}

func insertIPPrefix[V any](root *ipNode[V], prefix types.IPv4, value V) {
	prefixLen := prefix.Prefix()
	fullNibbles := prefixLen / 4
	remainBits := prefixLen % 4

	// Number of child hops before setting values.
	walkDepth := fullNibbles
	if remainBits == 0 && fullNibbles > 0 {
		walkDepth = fullNibbles - 1
	}

	node := root
	for i := range walkDepth {
		entry := &node.entries[ipNibble(prefix, i)]
		if entry.child == nil {
			entry.child = &ipNode[V]{}
		}
		node = entry.child
	}

	if remainBits == 0 {
		if fullNibbles == 0 {
			// /0: default route — match all entries at root.
			for i := range node.entries {
				node.entries[i].value = value
				node.entries[i].hasValue = true
			}
		} else {
			// Exact nibble boundary: single entry.
			nib := ipNibble(prefix, fullNibbles-1)
			node.entries[nib].value = value
			node.entries[nib].hasValue = true
		}
	} else {
		// Mid-nibble prefix: expand to all matching nibble values.
		shift := uint(4 - remainBits)
		nib := ipNibble(prefix, fullNibbles)
		base := (nib >> shift) << shift
		count := byte(1) << shift
		for j := range count {
			node.entries[base|j].value = value
			node.entries[base|j].hasValue = true
		}
	}
}
