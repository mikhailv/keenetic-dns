package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_normalizeFQDN(t *testing.T) {
	assert.Exactly(t, "example.com.", normalizeFQDN("example.com"))
	assert.Exactly(t, "example.com.", normalizeFQDN(".example.com."))
}

func Test_isDomainSuffix(t *testing.T) {
	assert.True(t, isDomainSuffix("domain.com", "domain.com"))
	assert.True(t, isDomainSuffix(".domain.com", "domain.com"))
	assert.True(t, isDomainSuffix("sub.domain.com", "domain.com"))
	assert.True(t, isDomainSuffix("sub.domain.com.", "domain.com."))
	assert.False(t, isDomainSuffix("sub.domain.com.", "example.com."))
}
