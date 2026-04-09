package conntrack

import (
	"bytes"
	"cmp"
	"slices"
)

func SortBuckets(buckets []Bucket) {
	slices.SortFunc(buckets, func(a, b Bucket) int {
		return cmp.Compare(a.TimeRange.Start, b.TimeRange.Start)
	})
}

func SortBucketEntries(entries []BucketEntry) {
	slices.SortFunc(entries, func(a, b BucketEntry) int {
		return cmp.Or(
			bytes.Compare(a.SrcIP[:], b.SrcIP[:]),
			cmp.Compare(a.Protocol, b.Protocol),
			bytes.Compare(a.DstIP[:], b.DstIP[:]),
			cmp.Compare(a.DstPort, b.DstPort),
		)
	})
}
