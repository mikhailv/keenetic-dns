package lookup

import (
	"cmp"
	"slices"
	"sort"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

// IPRangeMap is an immutable structure for longest-prefix IPv4 matching.
// It decomposes the IP space into sorted non-overlapping intervals,
// each annotated with the longest matching prefix value.
// Lookup is a single binary search: O(log N).
type IPRangeMap[V any] struct {
	intervals []ipInterval[V]
}

type ipInterval[V any] struct {
	start uint32
	end   uint32
	value V
}

// Get returns the value associated with the longest matching prefix.
func (t IPRangeMap[V]) Get(ip types.IPv4) (V, bool) {
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

// Empty reports whether the tree contains no entries.
func (t IPRangeMap[V]) Empty() bool {
	return len(t.intervals) == 0
}

// Has reports whether ip matches any prefix in the tree.
func (t IPRangeMap[V]) Has(ip types.IPv4) bool {
	_, ok := t.Get(ip)
	return ok
}

// IPRangeMapBuilder builds an immutable IPRangeMap.
type IPRangeMapBuilder[V any] struct {
	items []ipBuildItem[V]
}

type ipBuildItem[V any] struct {
	prefix types.IPv4
	value  V
}

func NewIPRangeMapBuilder[V any]() IPRangeMapBuilder[V] {
	return IPRangeMapBuilder[V]{}
}

// Add inserts an IPv4 prefix with an associated value.
func (b *IPRangeMapBuilder[V]) Add(prefix types.IPv4, value V) {
	b.items = append(b.items, ipBuildItem[V]{prefix: prefix, value: value})
}

// Build creates an immutable IPRangeMap from the builder's contents.
func (b IPRangeMapBuilder[V]) Build() IPRangeMap[V] {
	if len(b.items) == 0 {
		return IPRangeMap[V]{}
	}

	ranges := make([]ipRange[V], len(b.items))
	for i, item := range b.items {
		start, end := prefixRange(item.prefix)
		ranges[i] = ipRange[V]{start: start, end: end, value: item.value}
	}
	slices.SortStableFunc(ranges, func(a, b ipRange[V]) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(b.end, a.end))
	})

	intervals := make([]ipInterval[V], 0, len(ranges)*2)
	emit := func(from uint64, r *ipRange[V], to uint64) {
		if from <= to {
			intervals = append(intervals, ipInterval[V]{start: uint32(from), end: uint32(to), value: r.value})
		}
	}
	var open []ipRange[V]
	var cursor uint64
	for i, r := range ranges {
		if i > 0 && r.start == ranges[i-1].start && r.end == ranges[i-1].end {
			continue
		}
		for len(open) > 0 && open[len(open)-1].end < r.start {
			top := open[len(open)-1]
			emit(cursor, &top, uint64(top.end))
			cursor = max(cursor, uint64(top.end)+1)
			open = open[:len(open)-1]
		}
		if len(open) > 0 {
			emit(cursor, &open[len(open)-1], uint64(r.start)-1)
		}
		open = append(open, r)
		cursor = uint64(r.start)
	}
	for len(open) > 0 {
		top := open[len(open)-1]
		emit(cursor, &top, uint64(top.end))
		cursor = max(cursor, uint64(top.end)+1)
		open = open[:len(open)-1]
	}

	return IPRangeMap[V]{intervals: intervals}
}

type ipRange[V any] struct {
	start uint32
	end   uint32
	value V
}

// prefixRange returns the start and end (inclusive) uint32 addresses for a CIDR prefix.
func prefixRange(prefix types.IPv4) (start, end uint32) {
	addr := prefix.Uint32()
	mask := uint32(0xFFFFFFFF) << (32 - uint(prefix.Prefix()))
	start = addr & mask
	end = start | ^mask
	return start, end
}
