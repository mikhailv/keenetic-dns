package aggstats

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/klauspost/compress/gzip"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"
	"github.com/mikhailv/keenetic-dns/internal/tsv"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const chunkFileSuffix = ".tsv.gz"

func NewFileStore[K comparable, V any](dir string, normalize func(*K), logger *slog.Logger) Store[K, V] {
	return &fileStore[K, V]{
		FileStore: chunkstore.NewFileStore(dir, chunkstore.Codec[Chunk[K, V]]{
			Suffix: chunkFileSuffix,
			Encode: encodeChunk[K, V],
			Decode: func(r io.Reader, tr TimeRange) (Chunk[K, V], error) {
				return decodeChunk[K, V](r, tr, normalize)
			},
		}),
		logger: logger,
	}
}

type fileStore[K comparable, V any] struct {
	*chunkstore.FileStore[Chunk[K, V]]
	logger *slog.Logger
}

func (s *fileStore[K, V]) Save(ctx context.Context, chunk Chunk[K, V]) error {
	if !chunk.TimeRange.Valid() {
		return fmt.Errorf("invalid chunk time range %s", chunk.TimeRange)
	}
	if d := int(chunk.TimeRange.End-chunk.TimeRange.Start) + 1; d > maxChunkSeconds {
		return fmt.Errorf("chunk of %ds exceeds the %ds addressable by offsets", d, maxChunkSeconds)
	}
	return s.FileStore.Save(ctx, chunk)
}

func encodeChunk[K comparable, V any](w io.Writer, chunk Chunk[K, V]) (resErr error) {
	bufWriter := bufio.NewWriter(w)
	defer util.HandleError(bufWriter.Flush, &resErr)
	gz := gzip.NewWriter(bufWriter)
	defer util.HandleError(gz.Close, &resErr)

	tsvWriter := tsv.NewWriter[Entry[K, V]](gz)
	for i := range chunk.Entries {
		if err := tsvWriter.Write(chunk.Entries[i]); err != nil {
			return fmt.Errorf("write entry: %w", err)
		}
	}
	return tsvWriter.Close()
}

func decodeChunk[K comparable, V any](r io.Reader, tr TimeRange, normalize func(*K)) (chunk Chunk[K, V], resErr error) {
	chunk.TimeRange = tr
	gz, err := gzip.NewReader(bufio.NewReader(r))
	if err != nil {
		return chunk, fmt.Errorf("%w: gzip reader: %w", chunkstore.ErrCorrupt, err)
	}
	defer util.HandleError(gz.Close, &resErr)

	for entry, err := range tsv.NewReader[Entry[K, V]](gz).Iterator() {
		if err != nil {
			return chunk, fmt.Errorf("%w: read entry: %w", chunkstore.ErrCorrupt, err)
		}
		if normalize != nil {
			normalize(&entry.Key)
		}
		chunk.Entries = append(chunk.Entries, entry)
	}
	return chunk, nil
}
