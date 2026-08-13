package blocklist

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_EnabledLists(t *testing.T) {
	cfg := Config{Lists: []List{
		{Name: "on", Enabled: true, URLs: []string{"https://example.invalid/a.txt"}},
		{Name: "off", Enabled: false, URLs: []string{"https://example.invalid/b.txt"}},
		{Name: "on-no-urls", Enabled: true},
	}}

	names := make([]string, 0, 2)
	for _, list := range cfg.EnabledLists() {
		names = append(names, list.Name)
	}
	assert.Equal(t, []string{"on", "on-no-urls"}, names)
}

func TestConfig_ValidateRejectsEnabledListWithoutURLs(t *testing.T) {
	cfg := Config{Enabled: true, Lists: []List{{Name: "on-no-urls", Enabled: true}}}
	require.ErrorContains(t, cfg.Validate(), "no urls")
}

func TestConfig_ValidateRejectsNoEnabledLists(t *testing.T) {
	cfg := Config{Enabled: true, Lists: []List{
		{Name: "off", URLs: []string{"https://example.invalid/a.txt"}},
	}}
	require.ErrorContains(t, cfg.Validate(), "no list is enabled")
}

func TestConfig_ValidateRejectsDuplicateNames(t *testing.T) {
	cfg := Config{Enabled: true, Lists: []List{
		{Name: "dup", Enabled: true, URLs: []string{"https://example.invalid/a.txt"}},
		{Name: "dup", Enabled: true, URLs: []string{"https://example.invalid/b.txt"}},
	}}
	require.ErrorContains(t, cfg.Validate(), "duplicate list name")
}

func TestConfig_ValidateRejectsBadGroups(t *testing.T) {
	valid := List{Name: "ads", Enabled: true, URLs: []string{"https://example.invalid/a.txt"}}
	tests := map[string]Config{
		"unnamed group":  {Enabled: true, Lists: []List{valid}, Groups: []Group{{Clients: []string{"10.0.0.1"}}}},
		"no clients":     {Enabled: true, Lists: []List{valid}, Groups: []Group{{Name: "g"}}},
		"invalid client": {Enabled: true, Lists: []List{valid}, Groups: []Group{{Name: "g", Clients: []string{"nope"}}}},
		"unknown list":   {Enabled: true, Lists: []List{valid}, Groups: []Group{{Name: "g", Clients: []string{"10.0.0.1"}, Lists: []string{"typo"}}}},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, cfg.Validate())
		})
	}
}

func TestConfig_ValidateSkippedWhenDisabled(t *testing.T) {
	cfg := Config{Lists: []List{{Name: "", Enabled: true}}}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_SetDefaults(t *testing.T) {
	var cfg Config
	cfg.SetDefaults()
	assert.Equal(t, DefaultDataDir, cfg.DataDir)
	assert.Equal(t, DefaultRefreshInterval, cfg.RefreshInterval)
	assert.Equal(t, DefaultDownloadTimeout, cfg.DownloadTimeout)
}

func TestConfig_StateIgnoresPresentationDifferences(t *testing.T) {
	base := Config{Enabled: true, Lists: []List{
		{Name: "a", Enabled: true, URLs: []string{"https://example.invalid/1.txt"}, Deny: []string{"b.example.com", "a.example.com"}},
		{Name: "b", Enabled: true, Allow: []string{"c.example.com"}},
	}}
	same := Config{Enabled: true, Lists: []List{
		{Name: "b", Enabled: true, Allow: []string{" C.Example.COM "}},
		{Name: "a", Enabled: true, URLs: []string{"https://example.invalid/1.txt"}, Deny: []string{"A.example.com", "b.example.com", "a.example.com"}},
	}}

	assert.Equal(t, base.State(), same.State(), "order, case, padding and duplicates say the same thing")
}

func TestConfig_StateTracksIndexAffectingChanges(t *testing.T) {
	base := Config{Enabled: true, Lists: []List{
		{Name: "a", Enabled: true, URLs: []string{"https://example.invalid/1.txt"}},
	}}

	tests := map[string]func(c *Config){
		"allow added":     func(c *Config) { c.Lists[0].Allow = []string{"x.example.com"} },
		"deny added":      func(c *Config) { c.Lists[0].Deny = []string{"x.example.com"} },
		"url changed":     func(c *Config) { c.Lists[0].URLs = []string{"https://example.invalid/2.txt"} },
		"allow_url added": func(c *Config) { c.Lists[0].AllowURLs = []string{"https://example.invalid/a.txt"} },
		"list renamed":    func(c *Config) { c.Lists[0].Name = "renamed" },
		"list disabled":   func(c *Config) { c.Lists[0].Enabled = false },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := Config{Enabled: true, Lists: slices.Clone(base.Lists)}
			mutate(&changed)
			assert.NotEqual(t, base.State(), changed.State())
		})
	}
}

func TestConfig_StateIgnoresNonIndexSettings(t *testing.T) {
	base := Config{Enabled: true, Lists: []List{
		{Name: "a", Enabled: true, URLs: []string{"https://example.invalid/1.txt"}},
	}}
	other := base
	other.Mode = ModeNull
	other.RefreshInterval = time.Minute
	other.DataDir = "elsewhere"
	other.Groups = []Group{{Name: "g", Clients: []string{"10.0.0.1"}, Lists: []string{"a"}}}

	assert.Equal(t, base.State(), other.State())
}
