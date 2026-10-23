package ipinfo

import (
	"errors"
	"time"
)

const (
	DefaultTTL        = 24 * time.Hour
	DefaultTimeout    = 2 * time.Second
	DefaultRetryAfter = time.Minute
)

type Config struct {
	URL        string        `yaml:"url"`
	TTL        time.Duration `yaml:"ttl"`
	Timeout    time.Duration `yaml:"timeout"`
	RetryAfter time.Duration `yaml:"retry_after"`
}

func (c *Config) SetDefaults() {
	if c.TTL == 0 {
		c.TTL = DefaultTTL
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.RetryAfter == 0 {
		c.RetryAfter = DefaultRetryAfter
	}
}

func (c *Config) Validate() error {
	if c.TTL < 0 || c.Timeout < 0 || c.RetryAfter < 0 {
		return errors.New("ipinfo: ttl, timeout and retry_after must not be negative")
	}
	return nil
}
