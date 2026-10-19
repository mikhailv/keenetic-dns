package conntrack

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/chunkstore"
)

type pendingMigration struct {
	path      string
	timeRange TimeRange
	version   byte
}

func (s *fileStore) scan(ctx context.Context, dir string) ([]pendingMigration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var pending []pendingMigration
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if dir == s.dir {
				sub, err := s.scan(ctx, path)
				if err != nil {
					return nil, err
				}
				pending = append(pending, sub...)
			}
			continue
		}
		if p, ok := s.scanFile(path, e.Name()); ok {
			if p.version < encodingVersion {
				pending = append(pending, p)
			}
		}
	}
	slices.SortFunc(pending, func(a, b pendingMigration) int {
		return cmp.Compare(a.timeRange.Start, b.timeRange.Start)
	})
	return pending, nil
}

func (s *fileStore) scanFile(path, name string) (pendingMigration, bool) {
	tr := chunkstore.ParseChunkName(name, chunkFileSuffix)
	if !tr.Valid() {
		return pendingMigration{}, false
	}
	_, version, err := s.loadFile(path, true)
	if err != nil {
		if errors.Is(err, chunkstore.ErrCorrupt) {
			s.logger.Error("chunk file is corrupt and will be skipped, remove it to silence this",
				"path", path, "err", err)
		} else {
			s.logger.Error("failed to load version of chunk", "path", path, "err", err)
		}
		s.RemoveFile(tr)
		return pendingMigration{}, false
	}
	if newPath := s.ChunkPath(tr); newPath != path {
		if !s.moveChunkFile(path, newPath) {
			return pendingMigration{}, false
		}
		path = newPath
		s.SetFiles(nil)
	}
	return pendingMigration{path: path, timeRange: tr, version: version}, true
}

func (s *fileStore) scanAndMigrate(ctx context.Context) {
	defer close(s.scanDone)
	start := time.Now()
	pending, err := s.scan(ctx, s.dir)
	if err != nil {
		s.logger.Info("chunk scan interrupted", "err", err)
		return
	}
	s.logger.Info("chunk files scanned", "pending_migration", len(pending), "duration", time.Since(start))
	if len(pending) == 0 {
		return
	}
	if err := s.migrateFiles(ctx, pending); err != nil {
		s.logger.Info("chunk migration interrupted, the rest is migrated on next start", "err", err)
	}
}

func (s *fileStore) migrateFiles(ctx context.Context, pending []pendingMigration) error {
	start := time.Now()
	migrated := 0
	for _, p := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.migrateChunkFile(p) {
			migrated++
		} else {
			s.prevConnIndex = nil
		}
	}
	s.logger.Info("chunk migration finished", "migrated", migrated, "total", len(pending), "duration", time.Since(start))
	return nil
}

func (s *fileStore) migrateChunkFile(p pendingMigration) bool {
	chunk, version, err := s.loadFile(p.path, false)
	if err != nil {
		s.logger.Error("failed to load chunk file", "path", p.path, "err", err)
		return false
	}
	if version >= encodingVersion {
		return false
	}
	if version < 4 {
		s.assignConnIndices(&chunk)
	}
	if err := s.SaveFile(p.path, chunk); err != nil {
		s.logger.Error("failed to migrate chunk file", "path", p.path, "err", err)
		return false
	}
	s.logger.Debug("chunk file migrated", "path", p.path, "old_version", version)
	return true
}

type migrationConnKey struct {
	ConnKey
	ID uint64
}

func (s *fileStore) assignConnIndices(chunk *Chunk) {
	cur := make(map[migrationConnKey]uint64, len(s.prevConnIndex))
	next := uint64(chunk.TimeRange.Start) * connIndexPerSecond
	for _, bucket := range chunk.Buckets {
		for i := range bucket.Entries {
			entry := &bucket.Entries[i]
			for j, id := range entry.ConnIDs {
				key := migrationConnKey{ConnKey: entry.ConnKey, ID: id}
				idx, ok := cur[key]
				if !ok {
					if idx, ok = s.prevConnIndex[key]; !ok {
						idx = next
						next++
					}
					cur[key] = idx
				}
				entry.ConnIDs[j] = idx
			}
			slices.Sort(entry.ConnIDs)
		}
	}
	s.prevConnIndex = cur
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
		return s.SaveFile(path, chunk)
	}
	return nil
}
