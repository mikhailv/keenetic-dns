package conntrack

import (
	"fmt"
	"time"
)

const (
	DefaultPollInterval   = 5 * time.Second
	DefaultBucketInterval = time.Minute
	DefaultChunkInterval  = time.Hour
	DefaultCacheDuration  = 3 * time.Hour
	DefaultSaveInterval   = 10 * time.Minute
	DefaultDataDir        = "conntrack"
)

// Config holds configuration for the conntrack tracker.
type Config struct {
	PollInterval   time.Duration `yaml:"poll_interval"`
	BucketInterval time.Duration `yaml:"bucket_interval"`
	ChunkInterval  time.Duration `yaml:"chunk_interval"`
	CacheDuration  time.Duration `yaml:"cache_duration"`
	SaveInterval   time.Duration `yaml:"save_interval"`
	DataDir        string        `yaml:"data_dir"`
}

func (c *Config) SetDefaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = DefaultPollInterval
	}
	if c.BucketInterval <= 0 {
		c.BucketInterval = DefaultBucketInterval
	}
	if c.ChunkInterval <= 0 {
		c.ChunkInterval = DefaultChunkInterval
	}
	if c.CacheDuration <= 0 {
		c.CacheDuration = DefaultCacheDuration
	}
	if c.SaveInterval <= 0 {
		c.SaveInterval = DefaultSaveInterval
	}
	if c.DataDir == "" {
		c.DataDir = DefaultDataDir
	}
}

func (c *Config) Validate() error {
	if c.PollInterval > c.BucketInterval {
		return fmt.Errorf("conntrack: poll_interval %s must not exceed bucket_interval %s", c.PollInterval, c.BucketInterval)
	}
	if c.BucketInterval > c.ChunkInterval {
		return fmt.Errorf("conntrack: bucket_interval %s must not exceed chunk_interval %s", c.BucketInterval, c.ChunkInterval)
	}
	if c.ChunkInterval%c.BucketInterval != 0 {
		return fmt.Errorf("conntrack: chunk_interval %s must be a whole number of bucket_interval %s", c.ChunkInterval, c.BucketInterval)
	}
	return nil
}
