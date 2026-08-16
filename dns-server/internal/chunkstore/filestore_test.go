package chunkstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"
)

type testChunk struct {
	TimeRange chunkstore.TimeRange
	Values    []int
}

func (c testChunk) Range() chunkstore.TimeRange { return c.TimeRange }

func newTestStore(dir string) *chunkstore.FileStore[testChunk] {
	return chunkstore.NewFileStore(dir, chunkstore.Codec[testChunk]{
		Suffix: ".json",
		Encode: func(w io.Writer, chunk testChunk) error {
			return json.NewEncoder(w).Encode(chunk.Values)
		},
		Decode: func(r io.Reader, tr chunkstore.TimeRange) (testChunk, error) {
			chunk := testChunk{TimeRange: tr}
			if err := json.NewDecoder(r).Decode(&chunk.Values); err != nil {
				return chunk, chunkstore.ErrCorrupt
			}
			return chunk, nil
		},
	})
}

func hourRange(start chunkstore.Timestamp) chunkstore.TimeRange {
	return chunkstore.TimeRange{Start: start, End: start + 3599}
}

func collect(t *testing.T, s *chunkstore.FileStore[testChunk], tr chunkstore.TimeRange) []testChunk {
	t.Helper()
	var res []testChunk
	for chunk, err := range s.Load(t.Context(), tr) {
		require.NoError(t, err)
		res = append(res, chunk)
	}
	return res
}

func TestFileStore_SaveLoad(t *testing.T) {
	s := newTestStore(t.TempDir())
	require.NoError(t, s.Init(t.Context()))

	first := testChunk{TimeRange: hourRange(3600), Values: []int{1, 2}}
	second := testChunk{TimeRange: hourRange(7200), Values: []int{3}}
	require.NoError(t, s.Save(t.Context(), second))
	require.NoError(t, s.Save(t.Context(), first))

	got := collect(t, s, chunkstore.TimeRange{Start: 3600, End: 10799})
	assert.Equal(t, []testChunk{first, second}, got, "chunks come back in start order")

	got = collect(t, s, hourRange(7200))
	assert.Equal(t, []testChunk{second}, got, "only intersecting chunks are read")
}

func TestFileStore_SaveUsesDateSubdir(t *testing.T) {
	dir := t.TempDir()
	s := newTestStore(dir)
	require.NoError(t, s.Init(t.Context()))

	tr := hourRange(1755000000)
	require.NoError(t, s.Save(t.Context(), testChunk{TimeRange: tr, Values: []int{1}}))

	want := filepath.Join(dir, tr.Start.Time().UTC().Format("2006-01-02"), "1755000000-1755003599.json")
	assert.FileExists(t, want)
	assert.Equal(t, want, s.ChunkPath(tr))
}

func TestFileStore_LoadRescansAfterFileDisappears(t *testing.T) {
	dir := t.TempDir()
	s := newTestStore(dir)
	require.NoError(t, s.Init(t.Context()))

	tr := hourRange(3600)
	require.NoError(t, s.Save(t.Context(), testChunk{TimeRange: tr, Values: []int{1}}))
	require.NoError(t, os.Remove(s.ChunkPath(tr)))

	assert.Empty(t, collect(t, s, tr), "a vanished chunk is skipped, not an error")

	other := testChunk{TimeRange: hourRange(7200), Values: []int{2}}
	require.NoError(t, s.Save(t.Context(), other))
	assert.Equal(t, []testChunk{other}, collect(t, s, chunkstore.TimeRange{Start: 3600, End: 10799}),
		"the invalidated cache is rebuilt from disk")
}

func TestFileStore_LoadReportsCorruptChunk(t *testing.T) {
	dir := t.TempDir()
	s := newTestStore(dir)
	require.NoError(t, s.Init(t.Context()))

	tr := hourRange(3600)
	require.NoError(t, s.Save(t.Context(), testChunk{TimeRange: tr, Values: []int{1}}))
	require.NoError(t, os.WriteFile(s.ChunkPath(tr), []byte("not json"), 0o600))

	var gotErr error
	for _, err := range s.Load(t.Context(), tr) {
		gotErr = err
	}
	assert.ErrorIs(t, gotErr, chunkstore.ErrCorrupt)
}

func TestFileStore_LoadStopsOnCanceledContext(t *testing.T) {
	s := newTestStore(t.TempDir())
	require.NoError(t, s.Init(t.Context()))
	require.NoError(t, s.Save(t.Context(), testChunk{TimeRange: hourRange(3600), Values: []int{1}}))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, err := range s.Load(ctx, hourRange(3600)) {
		assert.ErrorIs(t, err, context.Canceled)
	}
}

func TestFileStore_ScanDirSkipsUnparsableNames(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "2025-08-12")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	for _, name := range []string{"100-200.bin", "garbage.json", "100.json", "a-b.json", "100-200.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(subDir, name), []byte("[]"), 0o600))
	}

	files, err := newTestStore(dir).ScanDir()
	require.NoError(t, err)
	assert.Equal(t, map[chunkstore.TimeRange]string{
		{Start: 100, End: 200}: filepath.Join(subDir, "100-200.json"),
	}, files)
}

func TestFileStore_ScanDirMissingDir(t *testing.T) {
	files, err := newTestStore(filepath.Join(t.TempDir(), "absent")).ScanDir()
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestFileStore_SaveDoesNotWriteInPlace(t *testing.T) {
	s := newTestStore(t.TempDir())
	require.NoError(t, s.Init(t.Context()))

	tr := hourRange(3600)
	require.NoError(t, s.Save(t.Context(), testChunk{TimeRange: tr, Values: []int{1}}))

	path := s.ChunkPath(tr)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	statBefore, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, s.Save(t.Context(), testChunk{TimeRange: tr, Values: []int{1, 2, 3}}))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
	statAfter, err := os.Stat(path)
	require.NoError(t, err)
	assert.NotEqual(t, statBefore.Sys(), statAfter.Sys(), "rename replaces the file rather than truncating it")

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

func TestParseChunkName(t *testing.T) {
	assert.Equal(t, chunkstore.TimeRange{Start: 100, End: 200}, chunkstore.ParseChunkName("100-200.tsv.gz", ".tsv.gz"))
	for _, name := range []string{"100-200.bin", "garbage.tsv.gz", "100.tsv.gz", "a-b.tsv.gz", "-.tsv.gz"} {
		assert.False(t, chunkstore.ParseChunkName(name, ".tsv.gz").Valid(), name)
	}
}

func TestTimeRange(t *testing.T) {
	tr := chunkstore.TimeRange{Start: 100, End: 200}
	assert.True(t, tr.Valid())
	assert.False(t, tr.IsZero())
	assert.True(t, tr.InRange(100) && tr.InRange(200))
	assert.False(t, tr.InRange(99) || tr.InRange(201))
	assert.True(t, tr.Intersects(chunkstore.TimeRange{Start: 200, End: 300}))
	assert.False(t, tr.Intersects(chunkstore.TimeRange{Start: 201, End: 300}))
	assert.Equal(t, "100-200", tr.String())
	assert.False(t, chunkstore.TimeRange{Start: 200, End: 100}.Valid())

	data, err := json.Marshal(tr)
	require.NoError(t, err)
	assert.Equal(t, `{"start":100,"end":200}`, string(bytes.TrimSpace(data)))
}
