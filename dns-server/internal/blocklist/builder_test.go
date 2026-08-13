package blocklist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_MergesMasksForSharedDomain(t *testing.T) {
	b := NewBuilder()
	first, err := b.AddList("first", "")
	require.NoError(t, err)
	second, err := b.AddList("second", "")
	require.NoError(t, err)

	b.Add(Rule{Domain: "shared.example.com", Action: Block, Subdomains: true}, first)
	b.Add(Rule{Domain: "shared.example.com", Action: Block, Subdomains: true}, second)
	idx := writeAndOpen(t, b)

	assert.Equal(t, 1, idx.Domains(), "duplicate domains must merge into one entry")

	for _, mask := range []uint32{maskOf(first), maskOf(second), maskOf(first, second)} {
		match, ok := idx.Lookup("shared.example.com", mask)
		require.True(t, ok)
		assert.True(t, match.Blocked())
	}
}

func TestBuilder_AddListRejectsTooManyLists(t *testing.T) {
	b := NewBuilder()
	for i := range MaxLists {
		_, err := b.AddList("list"+string(rune('a'+i)), "")
		require.NoError(t, err)
	}
	_, err := b.AddList("one-too-many", "")
	assert.Error(t, err)
}
