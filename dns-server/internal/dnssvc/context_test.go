package dnssvc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mikhailv/keenetic-dns/dns-server/internal/dnssvc"
)

func TestQueryStatus(t *testing.T) {
	ctx := dnssvc.WithQueryStatus(t.Context())
	assert.Equal(t, dnssvc.QueryStatus{}, dnssvc.GetQueryStatus(ctx))

	dnssvc.SetQueryRouted(ctx)
	assert.Equal(t, dnssvc.QueryStatus{Routed: true}, dnssvc.GetQueryStatus(ctx))

	dnssvc.SetQueryBlocked(ctx)
	assert.Equal(t, dnssvc.QueryStatus{Routed: true, Blocked: true}, dnssvc.GetQueryStatus(ctx))
}

func TestQueryStatus_WithoutHolder(t *testing.T) {
	ctx := t.Context()
	dnssvc.SetQueryBlocked(ctx)
	dnssvc.SetQueryRouted(ctx)
	assert.Equal(t, dnssvc.QueryStatus{}, dnssvc.GetQueryStatus(ctx), "setting without a holder is a no-op, not a panic")
}
