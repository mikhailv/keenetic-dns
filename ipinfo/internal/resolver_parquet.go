package internal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/parquet-go/parquet-go"
)

var _ Resolver = (*ParquetResolver)(nil)

type ParquetResolver struct {
	file     *os.File
	pf       *parquet.File
	startIdx int // column index of start_int
	endIdx   int // column index of end_int
}

func NewParquetResolver(path string) (*ParquetResolver, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening parquet file: %w", err)
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat parquet file: %w", err)
	}
	pf, err := parquet.OpenFile(f, stat.Size())
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("parsing parquet file: %w", err)
	}

	startIdx := -1
	endIdx := -1
	for i, col := range pf.Root().Columns() {
		switch col.Name() {
		case "start_int":
			startIdx = i
		case "end_int":
			endIdx = i
		}
	}
	if startIdx < 0 || endIdx < 0 {
		_ = f.Close()
		return nil, fmt.Errorf("parquet file missing start_int/end_int columns")
	}

	return &ParquetResolver{
		file:     f,
		pf:       pf,
		startIdx: startIdx,
		endIdx:   endIdx,
	}, nil
}

func (r *ParquetResolver) Name() string {
	return "parquet"
}

func (r *ParquetResolver) Lookup(ip net.IP) (*IPInfo, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("only IPv4 is supported")
	}
	target := int64(binary.BigEndian.Uint32(ip4))

	buf := make([]parquetRow, 256)
	for _, rg := range r.pf.RowGroups() {
		skip, err := r.skipRowGroup(rg, target)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		reader := parquet.NewGenericRowGroupReader[parquetRow](rg)
		for {
			n, err := reader.Read(buf)
			for i := range n {
				if buf[i].StartInt <= target && target <= buf[i].EndInt {
					_ = reader.Close()
					return buf[i].toIPInfo(ip), nil
				}
			}
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				_ = reader.Close()
				return nil, err
			}
		}
		_ = reader.Close()
	}

	return &IPInfo{IP: ip.String()}, nil
}

func (r *ParquetResolver) skipRowGroup(rg parquet.RowGroup, target int64) (bool, error) {
	chunks := rg.ColumnChunks()

	// If min(start_int) across all pages > target, skip.
	startCI, err := chunks[r.startIdx].ColumnIndex()
	if err != nil || !findStartIndex(startCI, target) {
		return true, err
	}

	// If max(end_int) across all pages < target, skip.
	endCI, err := chunks[r.endIdx].ColumnIndex()
	if err != nil || !findEndIndex(endCI, target) {
		return true, err
	}

	return false, nil
}

func findStartIndex(ci parquet.ColumnIndex, target int64) bool {
	for p := range ci.NumPages() {
		if ci.MinValue(p).Int64() <= target {
			return true
		}
	}
	return false
}

func findEndIndex(ci parquet.ColumnIndex, target int64) bool {
	for p := range ci.NumPages() {
		if ci.MaxValue(p).Int64() >= target {
			return true
		}
	}
	return false
}

func (r *ParquetResolver) Close() error {
	return r.file.Close()
}

type parquetRow struct {
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

func (r *parquetRow) toIPInfo(ip net.IP) *IPInfo {
	info := &IPInfo{IP: ip.String()}

	if r.ASNNumber != 0 || r.ASNOrganization != "" {
		info.ASN = &ASN{Number: uint(r.ASNNumber), Organization: r.ASNOrganization}
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
