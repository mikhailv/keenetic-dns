package blockstats

import (
	"errors"
	"time"
)

const (
	DefaultChunkDuration = time.Hour
	DefaultFlushInterval = 5 * time.Minute
	DefaultMaxEntries    = 50_000
)

type Config struct {
	Enabled       bool          `yaml:"enabled"`
	DataDir       string        `yaml:"data_dir"`
	ChunkDuration time.Duration `yaml:"chunk_duration"`
	FlushInterval time.Duration `yaml:"flush_interval"`
	MaxEntries    int           `yaml:"max_entries"`
}

func (c *Config) SetDefaults() {
	if c.DataDir == "" {
		c.DataDir = "blockstats"
	}
	if c.ChunkDuration <= 0 {
		c.ChunkDuration = DefaultChunkDuration
	}
	if c.ChunkDuration > maxChunkSeconds*time.Second {
		c.ChunkDuration = maxChunkSeconds * time.Second
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = DefaultFlushInterval
	}
	if c.MaxEntries <= 0 {
		c.MaxEntries = DefaultMaxEntries
	}
}

func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.FlushInterval > c.ChunkDuration {
		return errors.New("blocking stats: flush_interval must not exceed chunk_duration")
	}
	return nil
}
