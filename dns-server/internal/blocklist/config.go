package blocklist

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/types"
)

const (
	DefaultRefreshInterval = 24 * time.Hour
	DefaultDownloadTimeout = 2 * time.Minute
	DefaultDataDir         = "blocklist"
)

type Config struct {
	Enabled         bool          `yaml:"enabled"`
	Mode            Mode          `yaml:"mode"`
	DataDir         string        `yaml:"data_dir"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	DownloadTimeout time.Duration `yaml:"download_timeout"`
	Lists           []List        `yaml:"lists"`
	Groups          []Group       `yaml:"groups"`
}

type List struct {
	Name      string   `yaml:"name"`
	Enabled   bool     `yaml:"enabled"`
	URLs      []string `yaml:"urls"`
	AllowURLs []string `yaml:"allow_urls"`
	Allow     []string `yaml:"allow"`
	Deny      []string `yaml:"deny"`
}

func (l *List) Empty() bool {
	return len(l.URLs) == 0 && len(l.AllowURLs) == 0 && len(l.Allow) == 0 && len(l.Deny) == 0
}

type Group struct {
	Name    string   `yaml:"name"`
	Clients []string `yaml:"clients"`
	Lists   []string `yaml:"lists"`
}

func (c *Config) State() []ListState {
	lists := c.EnabledLists()
	state := make([]ListState, 0, len(lists))
	for _, list := range lists {
		state = append(state, ListState{
			Name:      list.Name,
			URLs:      normalizeStateEntries(list.URLs, false),
			AllowURLs: normalizeStateEntries(list.AllowURLs, false),
			Allow:     normalizeStateEntries(list.Allow, true),
			Deny:      normalizeStateEntries(list.Deny, true),
		})
	}
	slices.SortStableFunc(state, func(a, b ListState) int { return strings.Compare(a.Name, b.Name) })
	return state
}

func normalizeStateEntries(entries []string, lower bool) []string {
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if lower {
			e = strings.ToLower(e)
		}
		if e != "" {
			out = append(out, e)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (c *Config) EnabledLists() []List {
	lists := make([]List, 0, len(c.Lists))
	for _, list := range c.Lists {
		if list.Enabled {
			lists = append(lists, list)
		}
	}
	return lists
}

func (c *Config) SetDefaults() {
	if c.DataDir == "" {
		c.DataDir = DefaultDataDir
	}
	if c.RefreshInterval <= 0 {
		c.RefreshInterval = DefaultRefreshInterval
	}
	if c.DownloadTimeout <= 0 {
		c.DownloadTimeout = DefaultDownloadTimeout
	}
}

func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	names := map[string]bool{}
	for _, list := range c.Lists {
		if list.Name == "" {
			return errors.New("blocking: every list needs a name")
		}
		if names[list.Name] {
			return fmt.Errorf("blocking: duplicate list name %q", list.Name)
		}
		names[list.Name] = true
		if list.Enabled && list.Empty() {
			return fmt.Errorf("blocking: list %q is enabled but has no urls, allow_urls, allow or deny", list.Name)
		}
	}
	switch enabled := len(c.EnabledLists()); {
	case enabled == 0:
		return errors.New("blocking: enabled but no list is enabled")
	case enabled > MaxLists:
		return fmt.Errorf("blocking: %d lists enabled, at most %d are supported", enabled, MaxLists)
	}
	for _, group := range c.Groups {
		if group.Name == "" {
			return errors.New("blocking: every group needs a name")
		}
		if len(group.Clients) == 0 {
			return fmt.Errorf("blocking: group %q has no clients", group.Name)
		}
		for _, client := range group.Clients {
			if _, err := types.ParseIPv4(client); err != nil {
				return fmt.Errorf("blocking: group %q has invalid client %q: %w", group.Name, client, err)
			}
		}
		for _, name := range group.Lists {
			if !names[name] {
				return fmt.Errorf("blocking: group %q names unknown list %q", group.Name, name)
			}
		}
	}
	return nil
}
