package conntrack

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/gzip"
)

type Store interface {
	Load(ctx context.Context, timeRange TimeRange) iter.Seq2[Chunk, error]
	Save(ctx context.Context, chunk Chunk) error
	Close() error
}

// NewFileStore creates a Store that persists chunks as gzip+CBOR files
// in the given directory. Each chunk is stored as `{start}-{end}.dat`.
func NewFileStore(dir string) Store {
	return &fileStore{dir: dir}
}

var _ Store = (*fileStore)(nil)

type fileStore struct {
	dir string
}

func (s *fileStore) chunkPath(tr TimeRange) string {
	return filepath.Join(s.dir, fmt.Sprintf("%d-%d.dat", tr.Start, tr.End))
}

func (s *fileStore) Save(_ context.Context, chunk Chunk) (err error) {
	path := s.chunkPath(chunk.TimeRange)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer handleError(f.Close, &err)
	return saveChunk(f, chunk)
}

func (s *fileStore) Load(ctx context.Context, tr TimeRange) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		files, err := s.listChunks(tr)
		if err != nil {
			yield(Chunk{}, err)
			return
		}
		for _, fi := range files {
			if err := ctx.Err(); err != nil {
				yield(Chunk{}, err)
				return
			}
			chunk, err := s.loadFile(filepath.Join(s.dir, fi.name))
			if !yield(chunk, err) {
				return
			}
		}
	}
}

type chunkFile struct {
	name      string
	timeRange TimeRange
}

func (s *fileStore) listChunks(tr TimeRange) ([]chunkFile, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}
	var files []chunkFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".dat") {
			continue
		}
		before, after, ok := strings.Cut(strings.TrimSuffix(name, ".dat"), "-")
		if !ok {
			continue
		}
		start, err1 := strconv.ParseUint(before, 10, 32)
		end, err2 := strconv.ParseUint(after, 10, 32)
		if err1 != nil || err2 != nil {
			continue
		}
		ftr := TimeRange{Start: Timestamp(start), End: Timestamp(end)}
		if !ftr.Intersects(tr) {
			continue
		}
		files = append(files, chunkFile{name: name, timeRange: ftr})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].timeRange.Start < files[j].timeRange.Start
	})
	return files, nil
}

func (s *fileStore) loadFile(path string) (chunk Chunk, err error) {
	f, err := os.Open(path)
	if err != nil {
		return Chunk{}, fmt.Errorf("open file: %w", err)
	}
	defer handleError(f.Close, &err)
	err = loadChunk(f, &chunk)
	return chunk, err
}

func (s *fileStore) Close() error { return nil }

func loadChunk(r io.Reader, chunk *Chunk) error {
	gz, err := gzip.NewReader(bufio.NewReader(r))
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer handleError(gz.Close, &err)

	if err := cbor.NewDecoder(gz).Decode(chunk); err != nil {
		return fmt.Errorf("decode chunk: %w", err)
	}
	return nil
}

func saveChunk(w io.Writer, chunk Chunk) (err error) {
	bufWriter := bufio.NewWriter(w)
	defer handleError(bufWriter.Flush, &err)
	gz := gzip.NewWriter(bufWriter)
	defer handleError(gz.Close, &err)

	if err := cbor.NewEncoder(gz).Encode(&chunk); err != nil {
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
