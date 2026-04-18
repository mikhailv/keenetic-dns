package lookup

import (
	"slices"
	"sort"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

// IPTree is an immutable structure for longest-prefix IPv4 matching.
// It decomposes the IP space into sorted non-overlapping intervals,
// each annotated with the longest matching prefix value.
// Lookup is a single binary search: O(log N).
type IPTree[V any] struct {
	intervals []ipInterval[V]
}

type ipInterval[V any] struct {
	start uint32
	end   uint32
	value V
}

// Get returns the value associated with the longest matching prefix.
func (t IPTree[V]) Get(ip types.IPv4) (V, bool) {
	addr := ip.Uint32()
	// Find the rightmost interval with start <= addr.
	i := sort.Search(len(t.intervals), func(i int) bool {
		return t.intervals[i].start > addr
	}) - 1
	if i >= 0 && addr <= t.intervals[i].end {
		return t.intervals[i].value, true
	}
	var zero V
	return zero, false
}

// Has reports whether ip matches any prefix in the tree.
func (t IPTree[V]) Has(ip types.IPv4) bool {
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

func NewIPTreeBuilder[V any]() IPTreeBuilder[V] {
	return IPTreeBuilder[V]{}
}

// Add inserts an IPv4 prefix with an associated value.
func (b *IPTreeBuilder[V]) Add(prefix types.IPv4, value V) {
	b.items = append(b.items, ipBuildItem[V]{prefix: prefix, value: value})
}

// Build creates an immutable IPTree from the builder's contents.
func (b IPTreeBuilder[V]) Build() IPTree[V] {
	if len(b.items) == 0 {
		return IPTree[V]{}
	}

	// Sort prefixes by length descending so the first match in a scan is the longest.
	slices.SortStableFunc(b.items, func(a, b ipBuildItem[V]) int {
		return b.prefix.Prefix() - a.prefix.Prefix()
	})

	// Collect all interval boundary points.
	bounds := make([]uint32, 0, len(b.items)*2)
	for _, item := range b.items {
		start, end := prefixRange(item.prefix)
		bounds = append(bounds, start)
		if end < 0xFFFFFFFF {
			bounds = append(bounds, end+1)
		}
	}
	slices.Sort(bounds)
	bounds = slices.Compact(bounds)

	// For each interval, find the longest matching prefix.
	// Only keep intervals that have a match.
	var intervals []ipInterval[V]
	for i, bp := range bounds {
		var end uint32
		if i+1 < len(bounds) {
			end = bounds[i+1] - 1
		} else {
			end = 0xFFFFFFFF
		}
		for _, item := range b.items {
			start, e := prefixRange(item.prefix)
			if start <= bp && bp <= e {
				intervals = append(intervals, ipInterval[V]{start: bp, end: end, value: item.value})
				break
			}
		}
	}

	return IPTree[V]{intervals: intervals}
}

// prefixRange returns the start and end (inclusive) uint32 addresses for a CIDR prefix.
func prefixRange(prefix types.IPv4) (start, end uint32) {
	addr := prefix.Uint32()
	mask := uint32(0xFFFFFFFF) << (32 - uint(prefix.Prefix()))
	start = addr & mask
	end = start | ^mask
	return start, end
}
