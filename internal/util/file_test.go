package util

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveToFile_ConfigValidation(t *testing.T) {
	tests := map[string]struct {
		cfg SaveFileConfig
		err string
	}{
		"neither source": {
			cfg: SaveFileConfig{},
			err: "exactly one of Reader or Saver",
		},
		"both sources": {
			cfg: SaveFileConfig{
				Reader: strings.NewReader("x"),
				Saver:  func(io.Writer) error { return nil },
			},
			err: "exactly one of Reader or Saver",
		},
		"limit without reader": {
			cfg: SaveFileConfig{
				Limit: 10,
				Saver: func(io.Writer) error { return nil },
			},
			err: "Limit applies to Reader only",
		},
		"negative limit": {
			cfg: SaveFileConfig{Reader: strings.NewReader("x"), Limit: -1},
			err: "must not be negative",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out")
			require.ErrorContains(t, SaveToFile(path, tt.cfg), tt.err)

			_, statErr := os.Stat(path)
			assert.True(t, os.IsNotExist(statErr), "a rejected config must not create the file")
		})
	}
}

func TestSaveToFile_Limit(t *testing.T) {
	tests := map[string]struct {
		body    string
		limit   int64
		wantErr bool
	}{
		"under limit":      {body: "abcd", limit: 8},
		"exactly at limit": {body: "abcdefgh", limit: 8},
		"over limit":       {body: "abcdefghi", limit: 8, wantErr: true},
		"no limit":         {body: strings.Repeat("x", 100)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out")
			err := SaveToFile(path, SaveFileConfig{
				Reader: strings.NewReader(tt.body),
				Limit:  tt.limit,
			})
			if tt.wantErr {
				require.ErrorContains(t, err, "exceeds")
				_, statErr := os.Stat(path)
				assert.True(t, os.IsNotExist(statErr), "an oversize input must not be committed")
				return
			}
			require.NoError(t, err)
			data, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.body, string(data))
		})
	}
}

func TestSaveToFile_BeforeCommitRejectionKeepsPreviousFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))

	sentinel := errors.New("rejected")
	err := SaveToFile(path, SaveFileConfig{
		Reader:       strings.NewReader("replacement"),
		BeforeCommit: func(string) error { return sentinel },
	})
	require.ErrorIs(t, err, sentinel)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original", string(data), "a rejected save must not replace the file")

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the temporary file must be cleaned up")
}

func TestSaveToFile_BeforeCommitSeesNewContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))

	var seen string
	err := SaveToFile(path, SaveFileConfig{
		Reader: strings.NewReader("replacement"),
		BeforeCommit: func(tmpFile string) error {
			assert.NotEqual(t, path, tmpFile, "BeforeCommit must receive the temporary path")
			data, readErr := os.ReadFile(tmpFile)
			seen = string(data)
			return readErr
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "replacement", seen, "BeforeCommit must see the new contents, not the old")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "replacement", string(data))
}

func TestSaveToFile_SaverThenBeforeCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")

	var written int
	var validated string
	err := SaveToFile(path, SaveFileConfig{
		Saver: func(w io.Writer) error {
			var writeErr error
			written, writeErr = io.WriteString(w, "abcde")
			return writeErr
		},
		BeforeCommit: func(tmpFile string) error {
			data, readErr := os.ReadFile(tmpFile)
			validated = string(data)
			return readErr
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 5, written)
	assert.Equal(t, "abcde", validated, "BeforeCommit runs after Saver and sees what it wrote")
}

func TestSaveToFile_SaverFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()

	sentinel := errors.New("boom")
	err := SaveToFile(filepath.Join(dir, "out"), SaveFileConfig{
		Saver: func(w io.Writer) error {
			_, _ = io.WriteString(w, "partial")
			return sentinel
		},
	})
	require.ErrorIs(t, err, sentinel)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestSaveToFile_ReplacesRatherThanRewrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")

	require.NoError(t, SaveToFile(path, SaveFileConfig{Reader: strings.NewReader("first")}))
	before, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, SaveToFile(path, SaveFileConfig{Reader: strings.NewReader("second")}))
	after, err := os.Stat(path)
	require.NoError(t, err)

	assert.False(t, os.SameFile(before, after), "save must rename a new file into place")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "second", string(data))
}

func TestSaveToFile_CreatesMissingDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "out")
	require.NoError(t, SaveToFile(path, SaveFileConfig{
		Reader:   strings.NewReader("data"),
		FilePerm: 0o600,
	}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
