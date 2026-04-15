package conntrack

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		tr := parseChunkFilename(e.Name())
		if !tr.Valid() {
			continue
		}
		newPath := s.chunkPath(tr, ".bin")
		_, version, err := s.loadFile(path, true)
		if err != nil {
			s.logger.Error("failed to load version of chunk", "path", path, "err", err)
			continue
		}
		if version < encodingVersion {
			if !s.migrateChunkFile(path, newPath, version) {
				continue
			}
		} else if newPath != path {
			if !s.moveChunkFile(path, newPath) {
				continue
			}
		}
		files[tr] = newPath
	}
	return nil
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
	defer handleError(srcFile.Close, &resErr)

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer handleError(dstFile.Close, &resErr)

	_, err = io.Copy(dstFile, srcFile)
	return err
}
