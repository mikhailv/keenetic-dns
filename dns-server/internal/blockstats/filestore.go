package blockstats

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/gzip"

	"github.com/mikhailv/keenetic-dns/internal/tsv"
	"github.com/mikhailv/keenetic-dns/internal/util"
)

const chunkFileSuffix = ".tsv.gz"

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

func (s *fileStore) Init(context.Context) error {
	files, err := s.listAllChunks()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.files = files
	s.mu.Unlock()
	return nil
}

func (s *fileStore) Close() error { return nil }

func (s *fileStore) Save(_ context.Context, chunk Chunk) error {
	if !chunk.TimeRange.Valid() {
		return fmt.Errorf("invalid chunk time range %s", chunk.TimeRange)
	}
	if d := int(chunk.TimeRange.End-chunk.TimeRange.Start) + 1; d > maxChunkSeconds {
		return fmt.Errorf("chunk of %ds exceeds the %ds addressable by offsets", d, maxChunkSeconds)
	}

	path := s.chunkPath(chunk.TimeRange)
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
			chunk, err := s.loadFile(path)
			if os.IsNotExist(err) {
				s.mu.Lock()
				s.files = nil // invalidate the file cache
				s.mu.Unlock()
				continue
			}
			if !yield(chunk, err) {
				return
			}
		}
	}
}

func (s *fileStore) chunkPath(tr TimeRange) string {
	dateDir := tr.Start.Time().UTC().Format(time.DateOnly)
	return filepath.Join(s.dir, dateDir, fmt.Sprintf("%d-%d%s", tr.Start, tr.End, chunkFileSuffix))
}

func (s *fileStore) listChunks(tr TimeRange) ([]string, error) {
	s.mu.RLock()
	files := maps.Clone(s.files)
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
		files = maps.Clone(files)
	}

	ranges := make([]TimeRange, 0, 8)
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
			return map[TimeRange]string{}, nil
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

func (s *fileStore) saveFile(path string, chunk Chunk) error {
	return util.SaveToFileFunc(path, func(w io.Writer) error {
		return encodeChunk(w, chunk)
	})
}

func (s *fileStore) loadFile(path string) (chunk Chunk, resErr error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Chunk{}, err
		}
		return Chunk{}, fmt.Errorf("open file: %w", err)
	}
	defer util.HandleError(f.Close, &resErr)

	chunk.TimeRange = parseChunkFilename(filepath.Base(path))
	entries, err := decodeEntries(f)
	chunk.Entries = entries
	return chunk, err
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

func decodeEntries(r io.Reader) (entries []Entry, resErr error) {
	gz, err := gzip.NewReader(bufio.NewReader(r))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer util.HandleError(gz.Close, &resErr)

	for entry, err := range tsv.NewReader[Entry](gz).Iterator() {
		if err != nil {
			return entries, fmt.Errorf("read entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseChunkFilename(name string) TimeRange {
	if !strings.HasSuffix(name, chunkFileSuffix) {
		return TimeRange{}
	}
	before, after, ok := strings.Cut(strings.TrimSuffix(name, chunkFileSuffix), "-")
	if !ok {
		return TimeRange{}
	}
	start, err1 := strconv.ParseUint(before, 10, 32)
	end, err2 := strconv.ParseUint(after, 10, 32)
	if err1 != nil || err2 != nil {
		return TimeRange{}
	}
	return TimeRange{Start: Timestamp(start), End: Timestamp(end)}
}
