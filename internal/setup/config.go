package setup

import "errors"

const (
	DefaultLogMaxSizeMB  = 20
	DefaultLogMaxBackups = 10
)

type LogFile struct {
	Path       string `yaml:"path"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
}

func (c *LogFile) SetDefaults() {
	if c.MaxSizeMB == 0 {
		c.MaxSizeMB = DefaultLogMaxSizeMB
	}
	if c.MaxBackups == 0 {
		c.MaxBackups = DefaultLogMaxBackups
	}
}

func (c *LogFile) Validate() error {
	if c.MaxSizeMB < 0 {
		return errors.New("log file max_size_mb must not be negative")
	}
	if c.MaxBackups < 0 {
		return errors.New("log file max_backups must not be negative")
	}
	return nil
}
