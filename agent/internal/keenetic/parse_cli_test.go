package keenetic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOutput(t *testing.T) {
	t.Run("skip escape sequences", func(t *testing.T) {
		// act
		//nolint:staticcheck // ignore it
		res := ParseOutput(`[K
             host: 
                  mac: c8:4d:44:31:b2:91
                  via: c8:4d:44:31:b2:91
                   ip: 192.168.2.99

[K`)

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
		res := ParseOutput(`
             host: 
               region: EA
          description: Keenetic Speedster (NDMS 4.03.C.6.0-5): KN-
                       3010
             firmware: 4.03.C.6.0-5
`)

		// assert
		assert.Equal(t, []Object{
			{"host": Object{
				"region":      "EA",
				"description": "Keenetic Speedster (NDMS 4.03.C.6.0-5): KN-3010",
				"firmware":    "4.03.C.6.0-5",
			}},
		}, res)
	})
}
