package internal

// IPInfo represents the geo information for an IP address.
// JSON field names match the geolite2.mmdb output structure.
type IPInfo struct {
	IP                 string        `json:"ip"`
	ASN                *ASN          `json:"asn,omitempty"`
	Continent          *Continent    `json:"continent,omitempty"`
	Country            *Country      `json:"country,omitempty"`
	RegisteredCountry  *Country      `json:"registered_country,omitempty"`
	RepresentedCountry *Country      `json:"represented_country,omitempty"`
	City               *City         `json:"city,omitempty"`
	Subdivisions       []Subdivision `json:"subdivisions,omitempty"`
	Postal             *Postal       `json:"postal,omitempty"`
	Location           *Location     `json:"location,omitempty"`

	Debug struct {
		LookupTime float64 `json:"lookup_time"`
		Resolver   string  `json:"resolver"`
	} `json:"debug"`
}

type ASN struct {
	Number       uint   `json:"number" maxminddb:"number"`
	Organization string `json:"organization" maxminddb:"organization"`
}

type GeoArea struct {
	GeoNameID uint   `json:"geoname_id" maxminddb:"geoname_id"`
	Name      string `json:"name" maxminddb:"name"`
}

type Continent struct {
	Code string `json:"code" maxminddb:"code"`
	GeoArea
}

type Country struct {
	ISOCode string `json:"iso_code" maxminddb:"iso_code"`
	GeoArea
}

type City struct {
	GeoArea
}

type Subdivision struct {
	ISOCode string `json:"iso_code" maxminddb:"iso_code"`
	GeoArea
}

type Postal struct {
	Code string `json:"code" maxminddb:"code"`
}

type Location struct {
	Latitude       float64 `json:"latitude" maxminddb:"latitude"`
	Longitude      float64 `json:"longitude" maxminddb:"longitude"`
	AccuracyRadius uint16  `json:"accuracy_radius" maxminddb:"accuracy_radius"`
	TimeZone       string  `json:"time_zone" maxminddb:"time_zone"`
}
