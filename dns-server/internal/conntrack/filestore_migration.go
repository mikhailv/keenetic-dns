package conntrack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

func (s *fileStore) migrate(ctx context.Context, dir string, files map[TimeRange]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read dir: %w", err)
	}

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if dir == s.dir {
				if err := s.migrate(ctx, path, files); err != nil {
					return err
				}
			}
			continue
		}
		if tr, newPath, ok := s.migrateFile(path, e.Name()); ok {
			files[tr] = newPath
		}
	}
	return nil
}

func (s *fileStore) migrateFile(path, name string) (TimeRange, string, bool) {
	tr := parseChunkFilename(name)
	if !tr.Valid() {
		return TimeRange{}, "", false
	}
	newPath := s.chunkPath(tr, ".bin")
	_, version, err := s.loadFile(path, true)
	if err != nil {
		if errors.Is(err, errCorruptChunk) {
			s.logger.Error("chunk file is corrupt and will be skipped, remove it to silence this",
				"path", path, "err", err)
		} else {
			s.logger.Error("failed to load version of chunk", "path", path, "err", err)
		}
		return TimeRange{}, "", false
	}
	switch {
	case version < encodingVersion:
		if !s.migrateChunkFile(path, newPath, version) {
			return TimeRange{}, "", false
		}
	case newPath != path:
		if !s.moveChunkFile(path, newPath) {
			return TimeRange{}, "", false
		}
	}
	return tr, newPath, true
}

func (s *fileStore) migrateChunkFile(path, savePath string, version byte) bool {
	chunk, _, err := s.loadFile(path, false)
	if err != nil {
		s.logger.Error("failed to load chunk file", "path", path, "err", err)
		return false
	}
	bakFile := fmt.Sprintf("%s.v%d.bak", path, version)
	if path == savePath {
		if err := copyFile(path, bakFile); err != nil {
			s.logger.Error("failed to create backup file", "path", path, "err", err)
			return false
		}
	}
	if err := s.saveFile(savePath, chunk); err != nil {
		s.logger.Error("failed to migrate chunk file", "path", path, "err", err)
		return false
	}
	if path != savePath {
		if err := os.Rename(path, bakFile); err != nil {
			s.logger.Error("failed to rename to backup file", "path", path, "err", err)
			return true // save succeeded, just backup rename failed
		}
	}
	s.logger.Info("chunk file migrated", "path", savePath, "bak_path", bakFile, "old_version", version)
	return true
}

func (s *fileStore) moveChunkFile(path, newPath string) bool {
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		s.logger.Error("failed to create dir for chunk file", "path", newPath, "err", err)
		return false
	}
	if err := os.Rename(path, newPath); err != nil {
		s.logger.Error("failed to move chunk file", "path", newPath, "old_path", path, "err", err)
		return false
	}
	s.logger.Info("chunk file moved", "old_path", path, "new_path", newPath)
	return true
}

func copyFile(src, dst string) (resErr error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer util.HandleError(srcFile.Close, &resErr)

	return util.SaveToFile(dst, util.SaveFileConfig{Reader: srcFile})
}

func (s *fileStore) deleteEntriesFromChunkFile(path string, del func(BucketEntry) bool) error { //nolint:unused //ignore
	chunk, _, err := s.loadFile(path, false)
	if err != nil {
		return err
	}
	deleted := false
	for i, bucket := range chunk.Buckets {
		entries := bucket.Entries
		bucket.Entries = slices.DeleteFunc(entries, del)
		if len(bucket.Entries) != len(entries) {
			chunk.Buckets[i] = bucket
			deleted = true
		}
	}
	if deleted {
		return s.saveFile(path, chunk)
	}
	return nil
}
