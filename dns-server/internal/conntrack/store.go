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
	dir string
}

func (s *fileStore) chunkPath(tr TimeRange, suffix string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%d-%d%s", tr.Start, tr.End, suffix))
}

func (s *fileStore) Save(_ context.Context, chunk Chunk) (err error) {
	return s.saveFile(s.chunkPath(chunk.TimeRange, ".bin"), chunk, encodeChunk)
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
			chunk, err := s.loadFile(fi.path, decodeChunk)
			if !yield(chunk, err) {
				return
			}
		}
	}
}

type chunkFile struct {
	path      string
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
		if !ftr.Intersects(tr) {
			continue
		}
		files = append(files, chunkFile{
			path:      filepath.Join(s.dir, name),
			timeRange: ftr,
		})
	}
	slices.SortFunc(files, func(a, b chunkFile) int {
		return cmp.Compare(a.timeRange.Start, b.timeRange.Start)
	})
	return files, nil
}

func (s *fileStore) saveFile(path string, chunk Chunk, encoder func(w io.Writer, chunk Chunk) error) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer handleError(f.Close, &err)
	return encoder(f, chunk)
}

func (s *fileStore) loadFile(path string, decoder func(r io.Reader, chunk *Chunk) error) (chunk Chunk, err error) {
	f, err := os.Open(path)
	if err != nil {
		return Chunk{}, fmt.Errorf("open file: %w", err)
	}
	defer handleError(f.Close, &err)
	err = decoder(f, &chunk)
	return chunk, err
}

func (s *fileStore) Close() error { return nil }

func decodeChunk(r io.Reader, chunk *Chunk) error {
	gz, err := gzip.NewReader(bufio.NewReader(r))
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer handleError(gz.Close, &err)

	decoder := chunkDecoder{r: newByteReader(gz)}
	if err := decoder.Decode(chunk); err != nil {
		return fmt.Errorf("decode chunk: %w", err)
	}
	return nil
}

func encodeChunk(w io.Writer, chunk Chunk) (err error) {
	bufWriter := bufio.NewWriter(w)
	defer handleError(bufWriter.Flush, &err)
	gz := gzip.NewWriter(bufWriter)
	defer handleError(gz.Close, &err)

	encoder := chunkEncoder{w: gz}
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
