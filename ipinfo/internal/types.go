package internal

// IPLookup is the combined per-IP response: geo information and proxy information. Either field is omitted if the
// respective dataset has no record for the queried IP.
type IPLookup struct {
	Geo   *IPInfo    `json:"geo,omitempty"`
	Proxy *ProxyInfo `json:"proxy,omitempty"`
}

// QueryResponse is the paginated response shape served by the /query endpoints. HasMore is true when more matching rows
// exist past the returned page.
type QueryResponse[T any] struct {
	Items   []T  `json:"items"`
	HasMore bool `json:"has_more"`
}

// IPInfo represents the geo information for an IP address.
type IPInfo struct {
	Network            string        `json:"network,omitempty"`
	ASN                *ASN          `json:"asn,omitempty"`
	Continent          *Continent    `json:"continent,omitempty"`
	Country            *Country      `json:"country,omitempty"`
	RegisteredCountry  *Country      `json:"registered_country,omitempty"`
	RepresentedCountry *Country      `json:"represented_country,omitempty"`
	City               *City         `json:"city,omitempty"`
	Subdivisions       []Subdivision `json:"subdivisions,omitempty"`
	Postal             *Postal       `json:"postal,omitempty"`
	Location           *Location     `json:"location,omitempty"`
}

type ProxyInfo struct {
	Network     string `json:"network,omitempty"      parquet:"network,optional"`
	ProxyType   string `json:"proxy_type,omitempty"   maxminddb:"proxy_type"   parquet:"proxy_type,optional"`
	CountryCode string `json:"country_code,omitempty" maxminddb:"country_code" parquet:"country_code,optional"`
	CountryName string `json:"country_name,omitempty" maxminddb:"country_name" parquet:"country_name,optional"`
	RegionName  string `json:"region_name,omitempty"  maxminddb:"region_name"  parquet:"region_name,optional"`
	CityName    string `json:"city_name,omitempty"    maxminddb:"city_name"    parquet:"city_name,optional"`
	ISP         string `json:"isp,omitempty"          maxminddb:"isp"          parquet:"isp,optional"`
	Domain      string `json:"domain,omitempty"       maxminddb:"domain"       parquet:"domain,optional"`
	UsageType   string `json:"usage_type,omitempty"   maxminddb:"usage_type"   parquet:"usage_type,optional"`
	ASN         string `json:"asn,omitempty"          maxminddb:"asn"          parquet:"asn,optional"`
	LastSeen    string `json:"last_seen,omitempty"    maxminddb:"last_seen"    parquet:"last_seen,optional"`
	Threat      string `json:"threat,omitempty"       maxminddb:"threat"       parquet:"threat,optional"`
	Residential string `json:"residential,omitempty"  maxminddb:"residential"  parquet:"residential,optional"`
	Provider    string `json:"provider,omitempty"     maxminddb:"provider"     parquet:"provider,optional"`
	FraudScore  int64  `json:"fraud_score,omitempty"  maxminddb:"fraud_score"  parquet:"fraud_score,optional"`
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

// ToIPInfo flattens the GeoRecord into the response shape served by /ip/{ip}. Network is left blank because mmdb
// returns the network separately via LookupNetwork; populate it from the caller if needed.
func (r GeoRecord) ToIPInfo() IPInfo {
	var subdivisions []Subdivision
	if r.Subdivision1 != nil {
		subdivisions = append(subdivisions, *r.Subdivision1)
	}
	if r.Subdivision2 != nil {
		subdivisions = append(subdivisions, *r.Subdivision2)
	}
	return IPInfo{
		ASN:                r.ASN,
		Continent:          r.Continent,
		Country:            r.Country,
		RegisteredCountry:  r.RegisteredCountry,
		RepresentedCountry: r.RepresentedCountry,
		City:               r.City,
		Subdivisions:       subdivisions,
		Postal:             r.Postal,
		Location:           r.Location,
	}
}

// GeoRecord is the mmdb-shaped (nested) view of one GeoLite2 record.
type GeoRecord struct {
	ASN                *ASN         `maxminddb:"asn"                 json:"asn,omitempty"`
	Continent          *Continent   `maxminddb:"continent"           json:"continent,omitempty"`
	Country            *Country     `maxminddb:"country"             json:"country,omitempty"`
	RegisteredCountry  *Country     `maxminddb:"registered_country"  json:"registered_country,omitempty"`
	RepresentedCountry *Country     `maxminddb:"represented_country" json:"represented_country,omitempty"`
	City               *City        `maxminddb:"city"                json:"city,omitempty"`
	Subdivision1       *Subdivision `maxminddb:"subdivision1"        json:"subdivision_1,omitempty"`
	Subdivision2       *Subdivision `maxminddb:"subdivision2"        json:"subdivision_2,omitempty"`
	Postal             *Postal      `maxminddb:"postal"              json:"postal,omitempty"`
	Location           *Location    `maxminddb:"location"            json:"location,omitempty"`
}

// GeoRow is the parquet-shaped (flat) view of one GeoLite2 record. Includes the start_int/end_int columns so range
// lookups remain possible via parquetdb.
type GeoRow struct {
	StartInt int64  `parquet:"start_int"`
	EndInt   int64  `parquet:"end_int"`
	Network  string `parquet:"network,optional"`

	ASNNumber       int64  `parquet:"asn_number,optional"`
	ASNOrganization string `parquet:"asn_organization,optional"`

	ContinentCode      string `parquet:"continent_code,optional"`
	ContinentGeoNameID int64  `parquet:"continent_geoname_id,optional"`
	ContinentName      string `parquet:"continent_name,optional"`

	CountryISOCode   string `parquet:"country_iso_code,optional"`
	CountryGeoNameID int64  `parquet:"country_geoname_id,optional"`
	CountryName      string `parquet:"country_name,optional"`

	RegisteredCountryISOCode   string `parquet:"registered_country_iso_code,optional"`
	RegisteredCountryGeoNameID int64  `parquet:"registered_country_geoname_id,optional"`
	RegisteredCountryName      string `parquet:"registered_country_name,optional"`

	RepresentedCountryISOCode   string `parquet:"represented_country_iso_code,optional"`
	RepresentedCountryGeoNameID int64  `parquet:"represented_country_geoname_id,optional"`
	RepresentedCountryName      string `parquet:"represented_country_name,optional"`

	CityGeoNameID int64  `parquet:"city_geoname_id,optional"`
	CityName      string `parquet:"city_name,optional"`

	Subdivision1ISOCode   string `parquet:"subdivision_1_iso_code,optional"`
	Subdivision1GeoNameID int64  `parquet:"subdivision_1_geoname_id,optional"`
	Subdivision1Name      string `parquet:"subdivision_1_name,optional"`

	Subdivision2ISOCode   string `parquet:"subdivision_2_iso_code,optional"`
	Subdivision2GeoNameID int64  `parquet:"subdivision_2_geoname_id,optional"`
	Subdivision2Name      string `parquet:"subdivision_2_name,optional"`

	PostalCode string `parquet:"postal_code,optional"`

	Latitude       float64 `parquet:"latitude,optional"`
	Longitude      float64 `parquet:"longitude,optional"`
	AccuracyRadius int64   `parquet:"accuracy_radius,optional"`
	TimeZone       string  `parquet:"time_zone,optional"`
}

// ToIPInfo lifts the flat parquet row into the nested IPInfo response shape.
func (r GeoRow) ToIPInfo() IPInfo {
	info := IPInfo{Network: r.Network}
	if r.ASNNumber != 0 || r.ASNOrganization != "" {
		info.ASN = &ASN{
			Number:       uint(r.ASNNumber),
			Organization: r.ASNOrganization,
		}
	}
	if r.ContinentCode != "" {
		info.Continent = &Continent{
			Code:    r.ContinentCode,
			GeoArea: GeoArea{uint(r.ContinentGeoNameID), r.ContinentName},
		}
	}
	if r.CountryISOCode != "" {
		info.Country = &Country{
			ISOCode: r.CountryISOCode,
			GeoArea: GeoArea{uint(r.CountryGeoNameID), r.CountryName},
		}
	}
	if r.RegisteredCountryISOCode != "" {
		info.RegisteredCountry = &Country{
			ISOCode: r.RegisteredCountryISOCode,
			GeoArea: GeoArea{uint(r.RegisteredCountryGeoNameID), r.RegisteredCountryName},
		}
	}
	if r.RepresentedCountryISOCode != "" {
		info.RepresentedCountry = &Country{
			ISOCode: r.RepresentedCountryISOCode,
			GeoArea: GeoArea{uint(r.RepresentedCountryGeoNameID), r.RepresentedCountryName},
		}
	}
	if r.CityGeoNameID != 0 || r.CityName != "" {
		info.City = &City{
			GeoArea: GeoArea{uint(r.CityGeoNameID), r.CityName},
		}
	}
	var subdivisions []Subdivision
	if r.Subdivision1ISOCode != "" {
		subdivisions = append(subdivisions, Subdivision{
			ISOCode: r.Subdivision1ISOCode,
			GeoArea: GeoArea{uint(r.Subdivision1GeoNameID), r.Subdivision1Name},
		})
	}
	if r.Subdivision2ISOCode != "" {
		subdivisions = append(subdivisions, Subdivision{
			ISOCode: r.Subdivision2ISOCode,
			GeoArea: GeoArea{uint(r.Subdivision2GeoNameID), r.Subdivision2Name},
		})
	}
	info.Subdivisions = subdivisions
	if r.PostalCode != "" {
		info.Postal = &Postal{Code: r.PostalCode}
	}
	if r.Latitude != 0 || r.Longitude != 0 || r.TimeZone != "" {
		info.Location = &Location{
			Latitude:       r.Latitude,
			Longitude:      r.Longitude,
			AccuracyRadius: uint16(r.AccuracyRadius),
			TimeZone:       r.TimeZone,
		}
	}
	return info
}
