package keenetic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOutput(t *testing.T) {
	t.Run("skip escape sequences", func(t *testing.T) {
		// act
		//nolint:staticcheck // ignore it
		res, err := ParseOutput(`[K
             host: 
                  mac: c8:4d:44:31:b2:91
                  via: c8:4d:44:31:b2:91
                   ip: 192.168.2.99

[K`)
		require.NoError(t, err)

		// assert
		assert.Equal(t, []Object{
			{"host": Object{
				"mac": "c8:4d:44:31:b2:91",
				"via": "c8:4d:44:31:b2:91",
				"ip":  "192.168.2.99",
			}},
		}, res)
	})

	t.Run("parse multiline string", func(t *testing.T) {
		// act
		res, err := ParseOutput(`
             host: 
               region: EA
                speed: 
          description: Keenetic Speedster (NDMS 4.03.C.6.0-5): KN-
                       3010
             firmware: 4.03.C.6.0-5
`)
		require.NoError(t, err)

		// assert
		assert.Equal(t, []Object{
			{"host": Object{
				"region":      "EA",
				"speed":       "",
				"description": "Keenetic Speedster (NDMS 4.03.C.6.0-5): KN-3010",
				"firmware":    "4.03.C.6.0-5",
			}},
		}, res)
	})
}
