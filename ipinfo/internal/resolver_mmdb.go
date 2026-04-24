package internal

import (
	"fmt"
	"net"

	"github.com/oschwald/maxminddb-golang"
)

// mmdbRecord mirrors the geolite2.mmdb structure produced by convert_mmdb.toml.
type mmdbRecord struct {
	ASN                *ASN         `maxminddb:"asn"`
	Continent          *Continent   `maxminddb:"continent"`
	Country            *Country     `maxminddb:"country"`
	RegisteredCountry  *Country     `maxminddb:"registered_country"`
	RepresentedCountry *Country     `maxminddb:"represented_country"`
	City               *City        `maxminddb:"city"`
	Subdivision1       *Subdivision `maxminddb:"subdivision1"`
	Subdivision2       *Subdivision `maxminddb:"subdivision2"`
	Postal             *Postal      `maxminddb:"postal"`
	Location           *Location    `maxminddb:"location"`
}

var _ Resolver = (*MMDBResolver)(nil)

type MMDBResolver struct {
	name string
	db   *maxminddb.Reader
}

func NewMMDBResolver(name, path string) (*MMDBResolver, error) {
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening mmdb: %w", err)
	}
	return &MMDBResolver{name: name, db: db}, nil
}

func (r *MMDBResolver) Name() string {
	return r.name
}

func (r *MMDBResolver) Lookup(ip net.IP, info *IPInfo) error {
	var rec mmdbRecord
	network, ok, err := r.db.LookupNetwork(ip, &rec)
	if err != nil {
		return fmt.Errorf("mmdb lookup: %w", err)
	}
	if !ok {
		info.IP = ip.String()
		return nil
	}
	var subdivisions []Subdivision
	for _, it := range []*Subdivision{rec.Subdivision1, rec.Subdivision2} {
		if it != nil {
			subdivisions = append(subdivisions, *it)
		}
	}
	*info = IPInfo{
		IP:                 ip.String(),
		Network:            network.String(),
		ASN:                rec.ASN,
		Continent:          rec.Continent,
		Country:            rec.Country,
		RegisteredCountry:  rec.RegisteredCountry,
		RepresentedCountry: rec.RepresentedCountry,
		City:               rec.City,
		Subdivisions:       subdivisions,
		Postal:             rec.Postal,
		Location:           rec.Location,
	}
	return nil
}

func (r *MMDBResolver) Close() error {
	return r.db.Close()
}
