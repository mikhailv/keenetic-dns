package domainstats

import (
	"fmt"
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

func (c *Config) Validate(name string) error {
	if !c.Enabled {
		return nil
	}
	if c.DataDir == "" {
		return fmt.Errorf("%s: data_dir must be set", name)
	}
	if c.FlushInterval > c.ChunkDuration {
		return fmt.Errorf("%s: flush_interval must not exceed chunk_duration", name)
	}
	return nil
}
