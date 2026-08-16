package conntrack

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/klauspost/compress/gzip"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const chunkFileSuffix = ".bin"

// NewFileStore creates a Store that persists chunks as gzip-compressed binary
// files in the given directory. Files are organized into date-based
// subdirectories (yyyy-mm-dd) with each chunk stored as `{start}-{end}.bin`.
func NewFileStore(dir string, logger *slog.Logger) Store {
	return newFileStore(dir, logger)
}

func newFileStore(dir string, logger *slog.Logger) *fileStore {
	return &fileStore{
		FileStore: chunkstore.NewFileStore(dir, chunkstore.Codec[Chunk]{
			Suffix: chunkFileSuffix,
			Encode: encodeChunk,
			Decode: func(r io.Reader, _ chunkstore.TimeRange) (Chunk, error) {
				var chunk Chunk
				_, err := decodeChunk(r, &chunk, false)
				return chunk, err
			},
		}),
		dir:    dir,
		logger: logger,
	}
}

var _ Store = (*fileStore)(nil)

type fileStore struct {
	*chunkstore.FileStore[Chunk]
	dir    string
	logger *slog.Logger
}

func (s *fileStore) Init(ctx context.Context) error {
	files := make(map[TimeRange]string)
	if err := s.migrate(ctx, s.dir, files); err != nil {
		return err
	}
	s.SetFiles(files)
	return nil
}

func (s *fileStore) loadFile(path string, onlyVersion bool) (chunk Chunk, version byte, resErr error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Chunk{}, 0, err
		}
		return Chunk{}, 0, fmt.Errorf("open file: %w", err)
	}
	defer util.HandleError(f.Close, &resErr)
	version, err = decodeChunk(f, &chunk, onlyVersion)
	return chunk, version, err
}

func decodeChunk(r io.Reader, chunk *Chunk, onlyVersion bool) (version byte, resErr error) {
	gz, err := gzip.NewReader(bufio.NewReader(r))
	if err != nil {
		return 0, fmt.Errorf("%w: gzip reader: %w", chunkstore.ErrCorrupt, err)
	}
	defer util.HandleError(gz.Close, &resErr)

	decoder := newChunkDecoder(gz)
	if err := decoder.Decode(chunk, onlyVersion); err != nil {
		return 0, fmt.Errorf("decode chunk: %w", err)
	}
	return decoder.version, nil
}

func encodeChunk(w io.Writer, chunk Chunk) (resErr error) {
	bufWriter := bufio.NewWriter(w)
	defer util.HandleError(bufWriter.Flush, &resErr)
	gz := gzip.NewWriter(bufWriter)
	defer util.HandleError(gz.Close, &resErr)

	encoder := newChunkEncoder(gz)
	if err := encoder.Encode(&chunk); err != nil {
		return fmt.Errorf("encode chunk: %w", err)
	}
	return nil
}
