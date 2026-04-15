package conntrack

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/gzip"
)

type Store interface {
	Init(ctx context.Context) error
	Load(ctx context.Context, timeRange TimeRange) iter.Seq2[Chunk, error]
	Save(ctx context.Context, chunk Chunk) error
	Close() error
}

// NewFileStore creates a Store that persists chunks as gzip-compressed binary
// files in the given directory. Files are organized into date-based
// subdirectories (yyyy-mm-dd) with each chunk stored as `{start}-{end}.bin`.
func NewFileStore(dir string, logger *slog.Logger) Store {
	return &fileStore{dir: dir, logger: logger}
}

var _ Store = (*fileStore)(nil)

type fileStore struct {
	dir    string
	logger *slog.Logger
	mu     sync.RWMutex
	files  map[TimeRange]string
}

func (s *fileStore) Init(ctx context.Context) error {
	files := make(map[TimeRange]string)
	if err := s.migrate(ctx, s.dir, files); err != nil {
		return err
	}
	s.mu.Lock()
	s.files = files
	s.mu.Unlock()
	return nil
}

func (s *fileStore) Save(_ context.Context, chunk Chunk) error {
	path := s.chunkPath(chunk.TimeRange, ".bin")
	if err := s.saveFile(path, chunk); err != nil {
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
			chunk, _, err := s.loadFile(path, false)
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

func (s *fileStore) chunkPath(tr TimeRange, suffix string) string {
	dateDir := tr.Start.Time().UTC().Format(time.DateOnly)
	return filepath.Join(s.dir, dateDir, fmt.Sprintf("%d-%d%s", tr.Start, tr.End, suffix))
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
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(s.dir, e.Name())
		subEntries, err := os.ReadDir(subDir)
		if err != nil {
			return nil, fmt.Errorf("read subdir %s: %w", e.Name(), err)
		}
		for _, se := range subEntries {
			if se.IsDir() {
				continue
			}
			tr := parseChunkFilename(se.Name())
			if !tr.Valid() {
				continue
			}
			res[tr] = filepath.Join(subDir, se.Name())
		}
	}
	return res, nil
}

func (s *fileStore) saveFile(path string, chunk Chunk) (resErr error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer handleError(f.Close, &resErr)
	return encodeChunk(f, chunk)
}

func (s *fileStore) loadFile(path string, onlyVersion bool) (chunk Chunk, version byte, resErr error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Chunk{}, 0, err
		}
		return Chunk{}, 0, fmt.Errorf("open file: %w", err)
	}
	defer handleError(f.Close, &resErr)
	version, err = decodeChunk(f, &chunk, onlyVersion)
	return chunk, version, err
}

func (s *fileStore) Close() error { return nil }

func parseChunkFilename(name string) TimeRange {
	if !strings.HasSuffix(name, ".bin") {
		return TimeRange{}
	}
	before, after, ok := strings.Cut(strings.TrimSuffix(name, ".bin"), "-")
	if !ok {
		return TimeRange{}
	}
	start, err1 := strconv.ParseUint(before, 10, 32)
	end, err2 := strconv.ParseUint(after, 10, 32)
	if err1 != nil || err2 != nil {
		return TimeRange{}
	}
	return TimeRange{
		Start: Timestamp(start),
		End:   Timestamp(end),
	}
}

func decodeChunk(r io.Reader, chunk *Chunk, onlyVersion bool) (version byte, resErr error) {
	gz, err := gzip.NewReader(bufio.NewReader(r))
	if err != nil {
		return 0, fmt.Errorf("gzip reader: %w", err)
	}
	defer handleError(gz.Close, &resErr)

	decoder := newChunkDecoder(gz)
	if err := decoder.Decode(chunk, onlyVersion); err != nil {
		return 0, fmt.Errorf("decode chunk: %w", err)
	}
	return decoder.version, nil
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
