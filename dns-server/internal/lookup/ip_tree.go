package lookup

import (
	"encoding/binary"
	"slices"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

// IPTree is an immutable structure for longest-prefix IPv4 matching.
// It decomposes the IP space into sorted non-overlapping intervals,
// each annotated with the longest matching prefix value.
// Lookup is a single binary search: O(log N).
type IPTree[V any] struct {
	bounds []uint32 // sorted interval start points
	values []ipValue[V]
}

type ipValue[V any] struct {
	value    V
	hasValue bool
}

// Get returns the value associated with the longest matching prefix.
func (t *IPTree[V]) Get(ip types.IPv4) (V, bool) {
	if len(t.bounds) == 0 {
		var zero V
		return zero, false
	}
	addr := ipToUint32(ip)
	// Find the rightmost bound <= addr.
	i, ok := slices.BinarySearch(t.bounds, addr)
	if !ok {
		i-- // addr < bounds[i], so the interval is i-1
	}
	if i < 0 {
		var zero V
		return zero, false
	}
	v := &t.values[i]
	return v.value, v.hasValue
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
func (b *IPTreeBuilder[V]) Add(prefix types.IPv4, value V) {
	b.items = append(b.items, ipBuildItem[V]{prefix: prefix, value: value})
}

// Build creates an immutable IPTree from the builder's contents.
func (b *IPTreeBuilder[V]) Build() *IPTree[V] {
	if len(b.items) == 0 {
		return &IPTree[V]{}
	}

	// Sort prefixes by length descending so the first match in a scan is the longest.
	slices.SortStableFunc(b.items, func(a, b ipBuildItem[V]) int {
		return b.prefix.Prefix() - a.prefix.Prefix()
	})

	// Collect all interval boundary points.
	boundSet := make(map[uint32]struct{}, len(b.items)*2)
	boundSet[0] = struct{}{} // start of IP space
	for _, item := range b.items {
		start, end := prefixRange(item.prefix)
		boundSet[start] = struct{}{}
		if end < 0xFFFFFFFF {
			boundSet[end+1] = struct{}{}
		}
	}

	bounds := make([]uint32, 0, len(boundSet))
	for bp := range boundSet {
		bounds = append(bounds, bp)
	}
	slices.Sort(bounds)

	// For each interval, find the longest matching prefix.
	values := make([]ipValue[V], len(bounds))
	for i, bp := range bounds {
		for _, item := range b.items {
			start, end := prefixRange(item.prefix)
			if start <= bp && bp <= end {
				// First match is longest prefix (sorted by length desc).
				values[i] = ipValue[V]{value: item.value, hasValue: true}
				break
			}
		}
	}

	return &IPTree[V]{bounds: bounds, values: values}
}

// prefixRange returns the start and end (inclusive) uint32 addresses for a CIDR prefix.
func prefixRange(prefix types.IPv4) (start, end uint32) {
	addr := ipToUint32(prefix)
	bits := uint(prefix.Prefix())
	if bits == 0 {
		return 0, 0xFFFFFFFF
	}
	mask := uint32(0xFFFFFFFF) << (32 - bits)
	start = addr & mask
	end = start | ^mask
	return start, end
}

func ipToUint32(ip types.IPv4) uint32 {
	return binary.BigEndian.Uint32(ip[:4])
}
