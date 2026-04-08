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

	t.Run("parse real ndmc output", func(t *testing.T) {
		// Real ndmc "show device-list" output (reduced).
		// Section headers have trailing space after ": " (e.g. "host: ").
		res, err := ParseOutput(`
             host: 
                  mac: dc:03:98:81:d1:04
                   ip: 192.168.2.18
             hostname: LGwebOSTV
                 name: LGwebOSTV

            interface: 
                       id: Bridge0
                     name: Home
              description: Home network

                 dhcp: 
                  expires: 4866

           registered: yes
               active: yes
              rxbytes: 280232502
              txbytes: 4729253

                  mws: 
                      cid: e0b91322-8299-11ea-8f23-9317f81e1607
                       ap: WifiMaster1/AccessPoint0
                      psm: no
                     rssi: -41

               uptime: 20335
           first-seen: 20330
            last-seen: 15

             host: 
                  mac: 70:89:76:dc:16:53
                   ip: 192.168.2.89
             hostname: 
                 name: 

            interface: 
                       id: Bridge0
                     name: Home

           registered: no
               active: yes
               uptime: 35622
`)
		require.NoError(t, err)
		assert.Equal(t, []Object{
			{"host": Object{
				"mac":        "dc:03:98:81:d1:04",
				"ip":         "192.168.2.18",
				"hostname":   "LGwebOSTV",
				"name":       "LGwebOSTV",
				"registered": "yes",
				"active":     "yes",
				"rxbytes":    "280232502",
				"txbytes":    "4729253",
				"uptime":     "20335",
				"first-seen": "20330",
				"last-seen":  "15",
				"interface": Object{
					"id":          "Bridge0",
					"name":        "Home",
					"description": "Home network",
				},
				"dhcp": Object{
					"expires": "4866",
				},
				"mws": Object{
					"cid":  "e0b91322-8299-11ea-8f23-9317f81e1607",
					"ap":   "WifiMaster1/AccessPoint0",
					"psm":  "no",
					"rssi": "-41",
				},
			}},
			{"host": Object{
				"mac":        "70:89:76:dc:16:53",
				"ip":         "192.168.2.89",
				"hostname":   "",
				"name":       "",
				"registered": "no",
				"active":     "yes",
				"uptime":     "35622",
				"interface": Object{
					"id":   "Bridge0",
					"name": "Home",
				},
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
