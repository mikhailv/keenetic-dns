package util

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
)

type SaveFileConfig struct {
	DirPerm      os.FileMode
	FilePerm     os.FileMode
	Limit        int64
	Reader       io.Reader
	Saver        func(w io.Writer) error
	BeforeCommit func(tmpFile string) error
}

func (c *SaveFileConfig) setDefaults() {
	if c.DirPerm == 0 {
		c.DirPerm = 0o755
	}
	if c.FilePerm == 0 {
		c.FilePerm = 0o644
	}
}

func (c *SaveFileConfig) validate() error {
	if (c.Reader == nil) == (c.Saver == nil) {
		return errors.New("exactly one of Reader or Saver must be set")
	}
	if c.Limit != 0 && c.Reader == nil {
		return errors.New("Limit applies to Reader only")
	}
	if c.Limit < 0 {
		return fmt.Errorf("Limit must not be negative, got %d", c.Limit)
	}
	return nil
}

func SaveToFile(file string, cfg SaveFileConfig) error {
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(file), cfg.DirPerm); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	tmp := fmt.Sprintf("%s.tmp-%x", file, rand.Uint64())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, cfg.FilePerm)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	if err := cfg.writeTo(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("sync file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close file: %w", err)
	}

	if cfg.BeforeCommit != nil {
		if err := cfg.BeforeCommit(tmp); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, file); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit file: %w", err)
	}
	return nil
}

func SaveToFileFunc(file string, saver func(w io.Writer) error) error {
	return SaveToFile(file, SaveFileConfig{Saver: saver})
}

func (c *SaveFileConfig) writeTo(w io.Writer) error {
	if c.Saver != nil {
		return c.Saver(w)
	}

	src := c.Reader
	if c.Limit > 0 {
		src = io.LimitReader(src, c.Limit+1)
	}
	written, err := io.Copy(w, src)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	if c.Limit > 0 && written > c.Limit {
		return fmt.Errorf("input exceeds %d bytes", c.Limit)
	}
	return nil
}
