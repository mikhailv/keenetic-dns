package conntrack

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/klauspost/compress/gzip"
)

type Store interface {
	Load(ctx context.Context, timeRange TimeRange) iter.Seq2[Chunk, error]
	Save(ctx context.Context, chunk Chunk) error
	Close() error
}

// NewFileStore creates a Store that persists chunks as gzip+CBOR files
// in the given directory. Each chunk is stored as `{start}-{end}.bin`.
func NewFileStore(dir string) Store {
	return &fileStore{dir: dir}
}

var _ Store = (*fileStore)(nil)

type fileStore struct {
	dir   string
	mu    sync.RWMutex
	files map[TimeRange]string
}

func (s *fileStore) chunkPath(tr TimeRange, suffix string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%d-%d%s", tr.Start, tr.End, suffix))
}

func (s *fileStore) Save(_ context.Context, chunk Chunk) error {
	path := s.chunkPath(chunk.TimeRange, ".bin")
	if err := s.saveFile(path, chunk, encodeChunk); err != nil {
		return err
	}
	s.mu.Lock()
	if s.files != nil {
		s.files[chunk.TimeRange] = path
	}
	s.mu.Unlock()
	return nil
}

func (s *fileStore) Load(ctx context.Context, tr TimeRange) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		files, err := s.listChunks(tr)
		if err != nil {
			yield(Chunk{}, err)
			return
		}
		for _, path := range files {
			if err := ctx.Err(); err != nil {
				yield(Chunk{}, err)
				return
			}
			chunk, err := s.loadFile(path, decodeChunk)
			if os.IsNotExist(err) {
				s.mu.Lock()
				s.files = nil // invalidate file cache
				s.mu.Unlock()
				continue
			}
			if !yield(chunk, err) {
				return
			}
		}
	}
}

func (s *fileStore) listChunks(tr TimeRange) ([]string, error) {
	s.mu.RLock()
	files := s.files
	s.mu.RUnlock()

	if files == nil {
		var err error
		files, err = s.listAllChunks()
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.files = files
		s.mu.Unlock()
	}

	ranges := make([]TimeRange, 0, 5)
	for ftr := range files {
		if ftr.Intersects(tr) {
			ranges = append(ranges, ftr)
		}
	}
	slices.SortFunc(ranges, func(a, b TimeRange) int {
		return cmp.Compare(a.Start, b.Start)
	})
	res := make([]string, len(ranges))
	for i, r := range ranges {
		res[i] = files[r]
	}
	return res, nil
}

func (s *fileStore) listAllChunks() (map[TimeRange]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil //nolint:nilnil // ignore
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}
	res := map[TimeRange]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".bin") {
			continue
		}
		before, after, ok := strings.Cut(strings.TrimSuffix(name, ".bin"), "-")
		if !ok {
			continue
		}
		start, err1 := strconv.ParseUint(before, 10, 32)
		end, err2 := strconv.ParseUint(after, 10, 32)
		if err1 != nil || err2 != nil {
			continue
		}
		ftr := TimeRange{Start: Timestamp(start), End: Timestamp(end)}
		res[ftr] = filepath.Join(s.dir, name)
	}
	return res, nil
}

func (s *fileStore) saveFile(path string, chunk Chunk, encoder func(w io.Writer, chunk Chunk) error) (resErr error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer handleError(f.Close, &resErr)
	return encoder(f, chunk)
}

func (s *fileStore) loadFile(path string, decoder func(r io.Reader, chunk *Chunk) error) (chunk Chunk, resErr error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Chunk{}, err
		}
		return Chunk{}, fmt.Errorf("open file: %w", err)
	}
	defer handleError(f.Close, &resErr)
	err = decoder(f, &chunk)
	return chunk, err
}

func (s *fileStore) Close() error { return nil }

func decodeChunk(r io.Reader, chunk *Chunk) (resErr error) {
	gz, err := gzip.NewReader(bufio.NewReader(r))
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer handleError(gz.Close, &resErr)

	decoder := newChunkDecoder(gz)
	if err := decoder.Decode(chunk); err != nil {
		return fmt.Errorf("decode chunk: %w", err)
	}
	return nil
}

func encodeChunk(w io.Writer, chunk Chunk) (resErr error) {
	bufWriter := bufio.NewWriter(w)
	defer handleError(bufWriter.Flush, &resErr)
	gz := gzip.NewWriter(bufWriter)
	defer handleError(gz.Close, &resErr)

	encoder := newChunkEncoder(gz)
	if err := encoder.Encode(&chunk); err != nil {
		return fmt.Errorf("encode chunk: %w", err)
	}
	return nil
}

func handleError(fn func() error, err *error) {
	fnErr := fn()
	if *err == nil {
		*err = fnErr
	}
}
