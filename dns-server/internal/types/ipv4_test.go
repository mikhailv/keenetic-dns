package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixMatch(t *testing.T) {
	assert.True(t, PrefixMatch(MustParseIPv4("8.6.112.0/24"), MustParseIPv4("8.6.112.100")))
	assert.False(t, PrefixMatch(MustParseIPv4("8.6.112.0/24"), MustParseIPv4("8.6.113.0")))
	assert.False(t, PrefixMatch(MustParseIPv4("8.6.112.0/24"), MustParseIPv4("8.6.111.0")))
}
