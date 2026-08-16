package blocklist

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildIndex(t *testing.T, rules ...Rule) *Index {
	t.Helper()
	b := NewBuilder()
	id, err := b.AddList("test", "https://example.invalid/list.txt")
	require.NoError(t, err)
	for _, rule := range rules {
		b.Add(rule, id)
	}
	return writeAndOpen(t, b)
}

func writeAndOpen(t *testing.T, b *Builder) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "blocklist.fst")
	require.NoError(t, b.Write(path, time.Unix(1754784000, 0)))

	idx, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, idx.Close()) })
	return idx
}

const allLists = ^uint32(0)

func block(domain string, subdomains bool) Rule {
	return Rule{Domain: domain, Action: Block, Subdomains: subdomains}
}

func maskOf(ids ...int) uint32 {
	var mask uint32
	for _, id := range ids {
		mask |= uint32(1) << id
	}
	return mask
}

func TestIndex_ExactRuleMatchesOnlyItself(t *testing.T) {
	idx := buildIndex(t, block("ads.example.com", false))

	m, ok := idx.Lookup("ads.example.com", allLists)
	require.True(t, ok)
	assert.True(t, m.Blocked())
	assert.Equal(t, "test", m.List)

	_, ok = idx.Lookup("sub.ads.example.com", allLists)
	assert.False(t, ok, "hosts-format entry must not cover subdomains")
}

func TestIndex_WildcardRuleCoversSubdomains(t *testing.T) {
	idx := buildIndex(t, block("example.com", true))

	for _, domain := range []string{"example.com", "ads.example.com", "a.b.c.example.com"} {
		m, ok := idx.Lookup(domain, allLists)
		require.True(t, ok, domain)
		assert.True(t, m.Blocked(), domain)
	}
}

func TestIndex_NoMatch(t *testing.T) {
	idx := buildIndex(t, block("example.com", true))

	for _, domain := range []string{"example.org", "notexample.com", "com", "example.com.evil.net"} {
		_, ok := idx.Lookup(domain, allLists)
		assert.False(t, ok, domain)
	}
}

func TestIndex_LongestMatchWins(t *testing.T) {
	idx := buildIndex(t,
		block("example.com", true),
		Rule{Domain: "cdn.example.com", Action: Allow, Subdomains: true},
	)

	m, ok := idx.Lookup("ads.example.com", allLists)
	require.True(t, ok)
	assert.True(t, m.Blocked())

	m, ok = idx.Lookup("cdn.example.com", allLists)
	require.True(t, ok)
	assert.False(t, m.Blocked())

	m, ok = idx.Lookup("img.cdn.example.com", allLists)
	require.True(t, ok)
	assert.False(t, m.Blocked())
}

func TestIndex_ParentExactRuleDoesNotShadowChild(t *testing.T) {
	idx := buildIndex(t, block("example.com", false), block("com", true))

	m, ok := idx.Lookup("ads.example.com", allLists)
	require.True(t, ok)
	assert.True(t, m.Blocked())
}

func TestIndex_QueryNormalization(t *testing.T) {
	idx := buildIndex(t, block("example.com", true))

	for _, domain := range []string{"ADS.Example.COM", "ads.example.com.", "  ads.example.com  "} {
		m, ok := idx.Lookup(domain, allLists)
		require.True(t, ok, domain)
		assert.True(t, m.Blocked(), domain)
	}
}

func TestIndex_EmptyQuery(t *testing.T) {
	idx := buildIndex(t, block("example.com", true))

	_, ok := idx.Lookup("", allLists)
	assert.False(t, ok)
}

func TestIndex_AllowWinsOverBlockOnSameDomain(t *testing.T) {
	idx := buildIndex(t,
		block("example.com", true),
		Rule{Domain: "example.com", Action: Allow, Subdomains: true},
	)

	m, ok := idx.Lookup("example.com", allLists)
	require.True(t, ok)
	assert.False(t, m.Blocked(), "allow must survive deduplication")
}

func TestIndex_WildcardRulePreferredOverExactDuplicate(t *testing.T) {
	idx := buildIndex(t, block("example.com", false), block("example.com", true))

	m, ok := idx.Lookup("ads.example.com", allLists)
	require.True(t, ok)
	assert.True(t, m.Blocked(), "broader duplicate must survive deduplication")
}

func TestIndex_ListAttribution(t *testing.T) {
	b := NewBuilder()
	ads, err := b.AddList("ads-list", "https://example.invalid/ads.txt")
	require.NoError(t, err)
	malware, err := b.AddList("malware-list", "")
	require.NoError(t, err)
	b.Add(block("ads.example.com", true), ads)
	b.Add(block("evil.example.net", true), malware)
	idx := writeAndOpen(t, b)

	m, ok := idx.Lookup("x.ads.example.com", allLists)
	require.True(t, ok)
	assert.Equal(t, "ads-list", m.List)

	m, ok = idx.Lookup("evil.example.net", allLists)
	require.True(t, ok)
	assert.Equal(t, "malware-list", m.List)
}

func TestIndex_MidLabelWildcardBecomesRegex(t *testing.T) {
	b := NewBuilder()
	id, err := b.AddList("test", "")
	require.NoError(t, err)
	b.Add(block("ads*.example.com", true), id)
	assert.Equal(t, 0, b.RuleCount(), "wildcard rule must not enter the FST")
	assert.Equal(t, 1, b.RegexpCount())
	idx := writeAndOpen(t, b)

	for _, domain := range []string{"ads1.example.com", "ads.example.com", "x.ads42.example.com"} {
		m, ok := idx.Lookup(domain, allLists)
		require.True(t, ok, domain)
		assert.True(t, m.Blocked(), domain)
		assert.NotEmpty(t, m.Pattern, domain)
	}
	_, ok := idx.Lookup("ads.other.example.com", allLists)
	assert.False(t, ok)
}

func TestIndex_ExactRuleWinsOverRegex(t *testing.T) {
	b := NewBuilder()
	id, err := b.AddList("test", "")
	require.NoError(t, err)
	b.Add(block("ads*.example.com", true), id)
	b.Add(Rule{Domain: "ads1.example.com", Action: Allow, Subdomains: true}, id)
	idx := writeAndOpen(t, b)

	m, ok := idx.Lookup("ads1.example.com", allLists)
	require.True(t, ok)
	assert.False(t, m.Blocked(), "stored allow rule must win over a regex block")
}

func TestIndex_Manifest(t *testing.T) {
	idx := buildIndex(t, block("a.example.com", true), block("b.example.com", true))

	assert.Equal(t, 2, idx.Domains())
	assert.Equal(t, "2025-08-10T00:00:00Z", idx.BuiltAt())
}

func TestIndex_OpenMissingFiles(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "absent.fst"))
	assert.Error(t, err)
}

func TestManifestPath(t *testing.T) {
	assert.Equal(t, "/opt/var/blocklist.json", ManifestPath("/opt/var/blocklist.fst"))
}

func TestReverseLabels(t *testing.T) {
	tests := map[string]string{
		"ads.example.com": "com.example.ads",
		"example.com":     "com.example",
		"com":             "com",
		"":                "",
		"a.b.c.d":         "d.c.b.a",
	}
	for in, want := range tests {
		assert.Equal(t, want, reverseLabels(in), in)
	}
}

func TestPackValueRoundTrip(t *testing.T) {
	tests := []listMasks{
		{},
		{blockSub: 1},
		{blockExact: 1},
		{allowSub: 1},
		{allowExact: 1},
		{blockSub: 0b1010, blockExact: 0b0101, allowSub: 0b1100, allowExact: 0b0011},
		{blockSub: maskBits32, blockExact: maskBits32, allowSub: maskBits32, allowExact: maskBits32},
	}
	for _, tt := range tests {
		assert.Equal(t, tt, unpackValue(packValue(tt)))
	}
}

func TestPackValueStaysSmallForCommonRule(t *testing.T) {
	assert.Equal(t, uint64(1), packValue(listMasks{blockSub: 1 << 0}))
	assert.Less(t, packValue(listMasks{blockSub: 1 << 3}), uint64(128))
}

func TestIndex_ClientMaskHidesForeignLists(t *testing.T) {
	b := NewBuilder()
	ads, err := b.AddList("ads", "")
	require.NoError(t, err)
	extra, err := b.AddList("extra", "")
	require.NoError(t, err)

	b.Add(Rule{Domain: "tracker.example.com", Action: Block, Subdomains: true}, ads)
	b.Add(Rule{Domain: "gaming.example.net", Action: Block, Subdomains: true}, extra)
	idx := writeAndOpen(t, b)

	match, ok := idx.Lookup("tracker.example.com", maskOf(ads))
	require.True(t, ok)
	assert.True(t, match.Blocked())
	assert.Equal(t, "ads", match.List)

	_, ok = idx.Lookup("gaming.example.net", maskOf(ads))
	assert.False(t, ok, "a list outside the client's mask must be invisible")

	match, ok = idx.Lookup("gaming.example.net", maskOf(ads, extra))
	require.True(t, ok)
	assert.True(t, match.Blocked())
	assert.Equal(t, "extra", match.List)
}

func TestIndex_UnpermittedDeeperRuleDoesNotShadow(t *testing.T) {
	b := NewBuilder()
	broad, err := b.AddList("broad", "")
	require.NoError(t, err)
	narrow, err := b.AddList("narrow", "")
	require.NoError(t, err)

	b.Add(Rule{Domain: "example.com", Action: Block, Subdomains: true}, broad)
	b.Add(Rule{Domain: "ads.example.com", Action: Allow, Subdomains: true}, narrow)
	idx := writeAndOpen(t, b)

	match, ok := idx.Lookup("ads.example.com", maskOf(broad, narrow))
	require.True(t, ok)
	assert.False(t, match.Blocked())

	match, ok = idx.Lookup("ads.example.com", maskOf(broad))
	require.True(t, ok)
	assert.True(t, match.Blocked())
	assert.Equal(t, "broad", match.List)
}

func TestIndex_AllowListExempts(t *testing.T) {
	b := NewBuilder()
	ads, err := b.AddList("ads", "")
	require.NoError(t, err)
	rescue, err := b.AddList("rescue", "")
	require.NoError(t, err)

	b.Add(Rule{Domain: "example.com", Action: Block, Subdomains: true}, ads)
	b.Add(Rule{Domain: "shop.example.com", Action: Allow, Subdomains: true}, rescue)
	idx := writeAndOpen(t, b)

	match, ok := idx.Lookup("shop.example.com", maskOf(ads, rescue))
	require.True(t, ok)
	assert.False(t, match.Blocked(), "an allowlist entry must exempt, not block")
	assert.Equal(t, "rescue", match.List)

	match, ok = idx.Lookup("ads.example.com", maskOf(ads, rescue))
	require.True(t, ok)
	assert.True(t, match.Blocked())
}

func TestIndex_HandWrittenRules(t *testing.T) {
	b := NewBuilder()
	ads, err := b.AddList("ads", "")
	require.NoError(t, err)
	overrides, err := b.AddList("overrides", "")
	require.NoError(t, err)

	b.Add(Rule{Domain: "analytics.example.com", Action: Block, Subdomains: true}, ads)
	require.True(t, b.AddDomain("analytics.example.com", Allow, overrides))
	require.True(t, b.AddDomain("blocked.example.net", Block, overrides))
	idx := writeAndOpen(t, b)

	match, ok := idx.Lookup("analytics.example.com", maskOf(ads, overrides))
	require.True(t, ok)
	assert.False(t, match.Blocked())
	assert.Equal(t, "overrides", match.List)

	match, ok = idx.Lookup("sub.blocked.example.net", maskOf(overrides))
	require.True(t, ok)
	assert.True(t, match.Blocked())
	assert.Equal(t, "overrides", match.List)
}

func TestIndex_HandWrittenRulesFollowTheirList(t *testing.T) {
	b := NewBuilder()
	ads, err := b.AddList("ads", "")
	require.NoError(t, err)
	overrides, err := b.AddList("overrides", "")
	require.NoError(t, err)

	b.Add(Rule{Domain: "analytics.example.com", Action: Block, Subdomains: true}, ads)
	require.True(t, b.AddDomain("analytics.example.com", Allow, overrides))
	idx := writeAndOpen(t, b)

	match, ok := idx.Lookup("analytics.example.com", maskOf(ads))
	require.True(t, ok)
	assert.True(t, match.Blocked())
	assert.Equal(t, "ads", match.List)
}

func TestIndex_RegexRuleRespectsClientMask(t *testing.T) {
	b := NewBuilder()
	ads, err := b.AddList("ads", "")
	require.NoError(t, err)
	other, err := b.AddList("other", "")
	require.NoError(t, err)
	b.Add(Rule{Domain: "ads*.example.com", Action: Block, Subdomains: true}, other)
	idx := writeAndOpen(t, b)

	_, ok := idx.Lookup("ads1.example.com", maskOf(ads))
	assert.False(t, ok, "a regex rule from an unpermitted list must not match")

	match, ok := idx.Lookup("ads1.example.com", maskOf(ads, other))
	require.True(t, ok)
	assert.True(t, match.Blocked())
}

func TestIndex_SubdomainsIsPerList(t *testing.T) {
	b := NewBuilder()
	hosts, err := b.AddList("hosts-list", "")
	require.NoError(t, err)
	wildcard, err := b.AddList("wildcard-list", "")
	require.NoError(t, err)

	b.Add(Rule{Domain: "example.com", Action: Block, Subdomains: false}, hosts)
	b.Add(Rule{Domain: "example.com", Action: Block, Subdomains: true}, wildcard)
	idx := writeAndOpen(t, b)

	_, ok := idx.Lookup("www.example.com", maskOf(hosts))
	assert.False(t, ok, "an exact-only list must not block a subdomain")

	m, ok := idx.Lookup("example.com", maskOf(hosts))
	require.True(t, ok, "the exact rule still matches the domain itself")
	assert.True(t, m.Blocked())
	assert.Equal(t, "hosts-list", m.List)

	m, ok = idx.Lookup("www.example.com", maskOf(wildcard))
	require.True(t, ok)
	assert.True(t, m.Blocked())
	assert.Equal(t, "wildcard-list", m.List)
}

func TestIndex_ExactAllowDoesNotRescueSubdomains(t *testing.T) {
	b := NewBuilder()
	id, err := b.AddList("list", "")
	require.NoError(t, err)

	b.Add(Rule{Domain: "example.com", Action: Block, Subdomains: true}, id)
	b.Add(Rule{Domain: "example.com", Action: Allow, Subdomains: false}, id)
	idx := writeAndOpen(t, b)

	m, ok := idx.Lookup("example.com", allLists)
	require.True(t, ok)
	assert.False(t, m.Blocked(), "the exact allow wins on the domain itself")

	m, ok = idx.Lookup("www.example.com", allLists)
	require.True(t, ok)
	assert.True(t, m.Blocked(), "but it must not reach subdomains")
}

func TestIndex_RejectsOtherFormatVersion(t *testing.T) {
	dir := t.TempDir()
	b := NewBuilder()
	id, err := b.AddList("test", "")
	require.NoError(t, err)
	b.Add(block("ads.example.com", true), id)
	indexPath := filepath.Join(dir, "blocklist.fst")
	require.NoError(t, b.Write(indexPath, time.Unix(1754784000, 0)))

	m, err := loadManifest(ManifestPath(indexPath))
	require.NoError(t, err)
	m.Version = manifestVersion + 1
	require.NoError(t, m.save(ManifestPath(indexPath)))

	_, err = Open(indexPath)
	require.ErrorContains(t, err, "index format")
}
