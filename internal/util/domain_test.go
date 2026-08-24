package util_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

func TestFQDN(t *testing.T) {
	assert.Equal(t, "example.com.", util.FQDN("example.com"))
	assert.Equal(t, "example.com.", util.FQDN("example.com."))
	assert.Empty(t, util.FQDN(""))
}

func TestTrimFQDN(t *testing.T) {
	assert.Equal(t, "example.com", util.TrimFQDN("example.com."))
	assert.Equal(t, "example.com", util.TrimFQDN("example.com"))
	assert.Empty(t, util.TrimFQDN(""))
}
