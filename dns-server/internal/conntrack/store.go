package conntrack

import (
	"context"
	"iter"
)

type Store interface {
	Init(ctx context.Context) error
	Load(ctx context.Context, timeRange TimeRange) iter.Seq2[Chunk, error]
	Save(ctx context.Context, chunk Chunk) error
	Close() error
}
