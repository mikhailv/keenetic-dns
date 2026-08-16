package domainstats

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

func NewFileStore(dir string, logger *slog.Logger) Store {
	return &fileStore{
		FileStore: chunkstore.NewFileStore(dir, chunkstore.Codec[Chunk]{
			Suffix: chunkFileSuffix,
			Encode: encodeChunk,
			Decode: decodeChunk,
		}),
		logger: logger,
	}
}

var _ Store = (*fileStore)(nil)

type fileStore struct {
	*chunkstore.FileStore[Chunk]
	logger *slog.Logger
}

func (s *fileStore) Save(ctx context.Context, chunk Chunk) error {
	if !chunk.TimeRange.Valid() {
		return fmt.Errorf("invalid chunk time range %s", chunk.TimeRange)
	}
	if d := int(chunk.TimeRange.End-chunk.TimeRange.Start) + 1; d > maxChunkSeconds {
		return fmt.Errorf("chunk of %ds exceeds the %ds addressable by offsets", d, maxChunkSeconds)
	}
	return s.FileStore.Save(ctx, chunk)
}

func encodeChunk(w io.Writer, chunk Chunk) (resErr error) {
	bufWriter := bufio.NewWriter(w)
	defer util.HandleError(bufWriter.Flush, &resErr)
	gz := gzip.NewWriter(bufWriter)
	defer util.HandleError(gz.Close, &resErr)

	tsvWriter := tsv.NewWriter[Entry](gz)
	for i := range chunk.Entries {
		if err := tsvWriter.Write(chunk.Entries[i]); err != nil {
			return fmt.Errorf("write entry: %w", err)
		}
	}
	return tsvWriter.Close()
}

func decodeChunk(r io.Reader, tr TimeRange) (chunk Chunk, resErr error) {
	chunk.TimeRange = tr
	gz, err := gzip.NewReader(bufio.NewReader(r))
	if err != nil {
		return chunk, fmt.Errorf("%w: gzip reader: %w", chunkstore.ErrCorrupt, err)
	}
	defer util.HandleError(gz.Close, &resErr)

	for entry, err := range tsv.NewReader[Entry](gz).Iterator() {
		if err != nil {
			return chunk, fmt.Errorf("%w: read entry: %w", chunkstore.ErrCorrupt, err)
		}
		chunk.Entries = append(chunk.Entries, entry)
	}
	return chunk, nil
}
