package chunkstore

import (
	"context"
	"iter"
)

type Store[T Chunk] interface {
	Init(ctx context.Context) error
	Load(ctx context.Context, timeRange TimeRange) iter.Seq2[T, error]
	Save(ctx context.Context, chunk T) error
	Close() error
}
