package lookup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDomainTree_ExactMatch(t *testing.T) {
	b := NewDomainTreeBuilder[string]()
	b.Add("youtube.com.", "yt")
	tree := b.Build()

	v, ok := tree.Get("youtube.com.")
	assert.True(t, ok)
	assert.Equal(t, "yt", v)
}

func TestDomainTree_SuffixMatch(t *testing.T) {
	b := NewDomainTreeBuilder[string]()
	b.Add("youtube.com.", "yt")
	tree := b.Build()

	v, ok := tree.Get("www.youtube.com.")
	assert.True(t, ok)
	assert.Equal(t, "yt", v)

	v, ok = tree.Get("deep.sub.youtube.com.")
	assert.True(t, ok)
	assert.Equal(t, "yt", v)
}

func TestDomainTree_LeadingDotMatch(t *testing.T) {
	b := NewDomainTreeBuilder[string]()
	b.Add(".ru", "t")
	tree := b.Build()

	v, ok := tree.Get("ya.ru.")
	assert.True(t, ok)
	assert.Equal(t, "t", v)
}

func TestDomainTree_LongestMatch(t *testing.T) {
	b := NewDomainTreeBuilder[string]()
	b.Add("com.", "generic")
	b.Add("youtube.com.", "yt")
	b.Add("music.youtube.com.", "yt-music")
	tree := b.Build()

	v, _ := tree.Get("youtube.com.")
	assert.Equal(t, "yt", v)

	v, _ = tree.Get("www.youtube.com.")
	assert.Equal(t, "yt", v)

	v, _ = tree.Get("music.youtube.com.")
	assert.Equal(t, "yt-music", v)

	v, _ = tree.Get("sub.music.youtube.com.")
	assert.Equal(t, "yt-music", v)

	v, _ = tree.Get("example.com.")
	assert.Equal(t, "generic", v)
}

func TestDomainTree_NoMatch(t *testing.T) {
	b := NewDomainTreeBuilder[string]()
	b.Add("youtube.com.", "yt")
	tree := b.Build()

	_, ok := tree.Get("example.com.")
	assert.False(t, ok)

	_, ok = tree.Get("notyoutube.com.")
	assert.False(t, ok)
}

func TestDomainTree_Has(t *testing.T) {
	b := NewDomainTreeBuilder[string]()
	b.Add("youtube.com.", "yt")
	tree := b.Build()

	assert.True(t, tree.Has("youtube.com."))
	assert.True(t, tree.Has("www.youtube.com."))
	assert.False(t, tree.Has("example.com."))
}

func TestDomainTree_WithoutTrailingDot(t *testing.T) {
	b := NewDomainTreeBuilder[string]()
	b.Add("youtube.com", "yt")
	tree := b.Build()

	v, ok := tree.Get("www.youtube.com")
	assert.True(t, ok)
	assert.Equal(t, "yt", v)

	v, ok = tree.Get("www.youtube.com.")
	assert.True(t, ok)
	assert.Equal(t, "yt", v)
}

func TestDomainTree_IgnoresCase(t *testing.T) {
	b := NewDomainTreeBuilder[string]()
	b.Add("youtube.com", "yt")
	b.Add("Ads.Example.COM", "ads")
	tree := b.Build()

	for _, domain := range []string{"WWW.YouTube.com.", "www.youtube.COM", "Www.YouTube.Com."} {
		v, ok := tree.Get(domain)
		assert.True(t, ok, domain)
		assert.Equal(t, "yt", v, domain)
	}

	v, ok := tree.Get("ads.example.com.")
	assert.True(t, ok, "a rule written with capitals matches a lowercase query")
	assert.Equal(t, "ads", v)
}

func TestDomainTree_Empty(t *testing.T) {
	tree := NewDomainTreeBuilder[string]().Build()
	_, ok := tree.Get("anything.com.")
	assert.False(t, ok)
}
