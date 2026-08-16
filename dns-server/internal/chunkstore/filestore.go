package chunkstore

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

var ErrCorrupt = errors.New("corrupt chunk file")

type Chunk interface {
	Range() TimeRange
}

type Codec[T Chunk] struct {
	Suffix string
	Encode func(w io.Writer, chunk T) error
	Decode func(r io.Reader, tr TimeRange) (T, error)
}

func NewFileStore[T Chunk](dir string, codec Codec[T]) *FileStore[T] {
	return &FileStore[T]{dir: dir, codec: codec}
}

type FileStore[T Chunk] struct {
	dir   string
	codec Codec[T]
	mu    sync.RWMutex
	files map[TimeRange]string
}

func (s *FileStore[T]) Init(context.Context) error {
	files, err := s.ScanDir()
	if err != nil {
		return err
	}
	s.SetFiles(files)
	return nil
}

func (s *FileStore[T]) Close() error { return nil }

func (s *FileStore[T]) SetFiles(files map[TimeRange]string) {
	s.mu.Lock()
	s.files = files
	s.mu.Unlock()
}

func (s *FileStore[T]) Save(_ context.Context, chunk T) error {
	path := s.ChunkPath(chunk.Range())
	if err := s.SaveFile(path, chunk); err != nil {
		return err
	}
	s.mu.Lock()
	if s.files != nil {
		s.files[chunk.Range()] = path
	}
	s.mu.Unlock()
	return nil
}

func (s *FileStore[T]) Load(ctx context.Context, tr TimeRange) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		files, err := s.listChunks(tr)
		if err != nil {
			yield(zero, err)
			return
		}
		for _, path := range files {
			if err := ctx.Err(); err != nil {
				yield(zero, err)
				return
			}
			chunk, err := s.LoadFile(path)
			if os.IsNotExist(err) {
				s.SetFiles(nil)
				continue
			}
			if !yield(chunk, err) {
				return
			}
		}
	}
}

func (s *FileStore[T]) SaveFile(path string, chunk T) error {
	return util.SaveToFileFunc(path, func(w io.Writer) error {
		return s.codec.Encode(w, chunk)
	})
}

func (s *FileStore[T]) ChunkPath(tr TimeRange) string {
	dateDir := tr.Start.Time().UTC().Format(time.DateOnly)
	return filepath.Join(s.dir, dateDir, fmt.Sprintf("%d-%d%s", tr.Start, tr.End, s.codec.Suffix))
}

func (s *FileStore[T]) LoadFile(path string) (chunk T, resErr error) {
	var zero T
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, err
		}
		return zero, fmt.Errorf("open file: %w", err)
	}
	defer util.HandleError(f.Close, &resErr)
	return s.codec.Decode(f, ParseChunkName(filepath.Base(path), s.codec.Suffix))
}

func (s *FileStore[T]) ScanDir() (map[TimeRange]string, error) {
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
			tr := ParseChunkName(se.Name(), s.codec.Suffix)
			if !tr.Valid() {
				continue
			}
			res[tr] = filepath.Join(subDir, se.Name())
		}
	}
	return res, nil
}

func (s *FileStore[T]) listChunks(tr TimeRange) ([]string, error) {
	s.mu.RLock()
	files := maps.Clone(s.files)
	s.mu.RUnlock()

	if files == nil {
		var err error
		files, err = s.ScanDir()
		if err != nil {
			return nil, err
		}
		s.SetFiles(files)
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

func ParseChunkName(name, suffix string) TimeRange {
	if !strings.HasSuffix(name, suffix) {
		return TimeRange{}
	}
	before, after, ok := strings.Cut(strings.TrimSuffix(name, suffix), "-")
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
