package server

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"
	"slices"
	"strconv"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/conntrack"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

var errInvalidRange = httpError("invalid time range")

type httpError string

func (e httpError) Error() string { return string(e) }

type conntrackBucketsResponse struct {
	TimeRange         conntrack.TimeRange `json:"time_range"`
	Interval          uint                `json:"interval"`
	RequestedInterval uint                `json:"requested_interval"`
	Buckets           []conntrack.Bucket  `json:"buckets"`
}

func (s *HTTPServer) handleListConntrackBuckets(w http.ResponseWriter, req *http.Request) (int, error) {
	q := req.URL.Query()
	from, err := strconv.ParseUint(q.Get("from"), 10, 32)
	if err != nil {
		return http.StatusBadRequest, err
	}
	to, err := strconv.ParseUint(q.Get("to"), 10, 32)
	if err != nil {
		return http.StatusBadRequest, err
	}
	intervalReq, err := strconv.ParseUint(q.Get("interval"), 10, 32)
	if err != nil {
		return http.StatusBadRequest, err
	}
	tr := conntrack.TimeRange{Start: conntrack.Timestamp(from), End: conntrack.Timestamp(to)}
	if !tr.Valid() {
		return http.StatusBadRequest, errInvalidRange
	}

	nativeInterval := s.conntrackTracker.BucketDuration()
	interval := effectiveInterval(uint(intervalReq), nativeInterval)
	tr = adjustTimeRange(tr, interval)

	resp := conntrackBucketsResponse{
		TimeRange:         tr,
		Interval:          interval,
		RequestedInterval: uint(intervalReq),
	}

	bucketSeq := iterateBuckets(req.Context(), s.conntrackTracker, tr)
	if interval != nativeInterval {
		bucketSeq = aggregateBuckets(bucketSeq, interval)
	}
	bucketSeq = fillGapsInBucketSeq(bucketSeq, tr, interval)

	resp.Buckets, err = util.Seq2ToSlice(int(1+uint(tr.End-tr.Start+1)/interval), bucketSeq)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp) //nolint:errchkjson // ignore
	return http.StatusOK, nil
}

func adjustTimeRange(tr conntrack.TimeRange, interval uint) conntrack.TimeRange {
	intervalTs := conntrack.Timestamp(interval)
	tr.Start -= tr.Start % intervalTs
	tr.End -= tr.End%intervalTs + intervalTs - 1
	return tr
}

// effectiveInterval clamps and rounds the requested interval (in seconds)
// up to the nearest multiple of native.
func effectiveInterval(req, native uint) uint {
	return max(native, (req+native/2)/native*native)
}

type bucketAgg struct {
	timeRange conntrack.TimeRange
	entries   map[conntrack.ConnKey]*conntrack.BucketEntry
}

func (s *bucketAgg) toBucket() conntrack.Bucket {
	entries := make([]conntrack.BucketEntry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, *e)
	}
	conntrack.SortBucketEntries(entries)
	return conntrack.Bucket{
		TimeRange: s.timeRange,
		Entries:   entries,
	}
}

func iterateBuckets(ctx context.Context, tracker *conntrack.Tracker, tr conntrack.TimeRange) iter.Seq2[conntrack.Bucket, error] {
	return func(yield func(conntrack.Bucket, error) bool) {
		for chunk, err := range tracker.IterateChunks(ctx, tr) {
			if err != nil {
				if !yield(conntrack.Bucket{}, err) {
					return
				}
				continue
			}
			for _, bucket := range chunk.Buckets {
				if bucket.TimeRange.Intersects(tr) && !yield(bucket, nil) {
					return
				}
			}
		}
	}
}

//nolint:gocognit // ignore
func aggregateBuckets(bucketSeq iter.Seq2[conntrack.Bucket, error], interval uint) iter.Seq2[conntrack.Bucket, error] {
	return func(yield func(conntrack.Bucket, error) bool) {
		agg := bucketAgg{
			entries: map[conntrack.ConnKey]*conntrack.BucketEntry{},
		}
		for bucket, err := range bucketSeq {
			if err != nil {
				if !yield(bucket, err) {
					return
				}
				continue
			}

			aggStart := conntrack.Timestamp(uint(bucket.TimeRange.Start) / interval * interval)
			if agg.timeRange.Start != aggStart {
				if !agg.timeRange.IsZero() {
					if !yield(agg.toBucket(), nil) {
						return
					}
					clear(agg.entries)
				}
				agg.timeRange = conntrack.TimeRange{
					Start: aggStart,
					End:   aggStart + conntrack.Timestamp(interval) - 1,
				}
				bucket = bucket.Clone()
				for i, e := range bucket.Entries {
					agg.entries[e.ConnKey] = &bucket.Entries[i]
				}
				continue
			}

			for _, entry := range bucket.Entries {
				if ae, ok := agg.entries[entry.ConnKey]; ok {
					ae.Add(entry.ConnStat)
					ae.ConnIDs = append(ae.ConnIDs, entry.ConnIDs...)
					slices.Sort(ae.ConnIDs)
					ae.ConnIDs = slices.Compact(ae.ConnIDs) // remove duplicates
				} else {
					agg.entries[entry.ConnKey] = util.Ptr(entry.Clone())
				}
			}
		}
		if !agg.timeRange.IsZero() {
			yield(agg.toBucket(), nil)
		}
	}
}

func fillGapsInBucketSeq(bucketSeq iter.Seq2[conntrack.Bucket, error], tr conntrack.TimeRange, interval uint) iter.Seq2[conntrack.Bucket, error] {
	return func(yield func(conntrack.Bucket, error) bool) {
		startTime := tr.Start

		yieldEmptyBucket := func() bool {
			emptyBucket := conntrack.Bucket{
				TimeRange: conntrack.TimeRange{
					Start: startTime,
					End:   startTime + conntrack.Timestamp(interval) - 1,
				},
				Entries: []conntrack.BucketEntry{},
			}
			startTime += conntrack.Timestamp(interval)
			return yield(emptyBucket, nil)
		}

		for bucket, err := range bucketSeq {
			if err != nil {
				if !yield(bucket, err) {
					return
				}
				continue
			}
			// fill gap before bucket
			for bucket.TimeRange.Start > startTime {
				if !yieldEmptyBucket() {
					return
				}
			}
			if !yield(bucket, nil) {
				return
			}
			startTime += conntrack.Timestamp(interval)
		}

		// fill gap after last bucket
		for startTime <= tr.End {
			if !yieldEmptyBucket() {
				return
			}
		}
	}
}
