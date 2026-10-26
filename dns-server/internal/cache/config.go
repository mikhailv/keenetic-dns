package cache

import (
	"errors"
	"time"
)

type Config struct {
	ExtraTTL time.Duration `yaml:"extra_ttl"`
}

func (c *Config) SetDefaults() {}

func (c *Config) Validate() error {
	if c.ExtraTTL < 0 {
		return errors.New("dns cache: extra_ttl must not be negative")
	}
	return nil
}
