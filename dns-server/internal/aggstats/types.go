package aggstats

import (
	"github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"
)

type (
	TimeRange = chunkstore.TimeRange
	Timestamp = chunkstore.Timestamp
)

const maxChunkSeconds = 1 << 16

type Chunk[K comparable, V any] struct {
	TimeRange TimeRange
	Entries   []Entry[K, V]
}

func (c Chunk[K, V]) Range() TimeRange {
	return c.TimeRange
}

type Entry[K comparable, V any] struct {
	Key   K `tsv:",inline"`
	Value V `tsv:",inline"`
}
