package internal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sort"

	"github.com/parquet-go/parquet-go"
)

var errMissingColumns = errors.New("parquet file missing start_int/end_int columns")

var _ Resolver = (*ParquetResolver)(nil)

type ParquetResolver struct {
	logger   *slog.Logger
	name     string
	file     *os.File
	pf       *parquet.File
	startIdx int // column index of start_int
	endIdx   int // column index of end_int
	sorted   bool
}

func NewParquetResolver(name string, path string, logger *slog.Logger) (*ParquetResolver, error) {
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

	r := &ParquetResolver{
		logger:   logger,
		name:     name,
		file:     f,
		pf:       pf,
		startIdx: -1,
		endIdx:   -1,
	}

	for i, col := range pf.Root().Columns() {
		switch col.Name() {
		case "start_int":
			r.startIdx = i
		case "end_int":
			r.endIdx = i
		}
	}
	if r.startIdx < 0 || r.endIdx < 0 {
		_ = f.Close()
		return nil, errMissingColumns
	}

	r.sorted = r.checkStartIntSorted()

	if r.sorted {
		logger.Info("start_int is sorted ascending, using binary search")
	} else {
		logger.Warn("start_int is NOT sorted, falling back to linear scan")
	}

	return r, nil
}

func (r *ParquetResolver) Name() string {
	return r.name
}

func (r *ParquetResolver) Lookup(ip net.IP, info *IPInfo) error {
	ip4 := ip.To4()
	if ip4 == nil {
		return errSupportedOnlyIPv4
	}
	target := int64(binary.BigEndian.Uint32(ip4))
	info.IP = ip.String()
	if r.sorted {
		return r.lookupSorted(target, info)
	}
	return r.lookupLinear(target, info)
}

// lookupSorted uses binary search across row groups and parquet.Search within
// a row group to quickly locate the IP range containing the target.
func (r *ParquetResolver) lookupSorted(target int64, info *IPInfo) error {
	rowGroups := r.pf.RowGroups()

	// Binary search: find the last row group where min(start_int) <= target.
	rgIdx := sort.Search(len(rowGroups), func(i int) bool {
		ci, err := rowGroups[i].ColumnChunks()[r.startIdx].ColumnIndex()
		if err != nil {
			return false
		}
		return ci.MinValue(0).Int64() > target
	}) - 1
	if rgIdx < 0 {
		return nil
	}

	rg := rowGroups[rgIdx]
	startChunk := rg.ColumnChunks()[r.startIdx]
	startCI, err := startChunk.ColumnIndex()
	if err != nil {
		return fmt.Errorf("reading start_int column index: %w", err)
	}

	// Use parquet.Search to find the page containing the target value.
	// Search does a binary search on the column index for ascending data.
	targetVal := parquet.Int64Value(target)
	pageIdx := parquet.Search(startCI, targetVal, startChunk.Type())

	// If target falls between pages (no page has MinValue <= target <= MaxValue),
	// Search returns NumPages. The matching row is in the last page where
	// MaxValue(start_int) <= target.
	if pageIdx >= startCI.NumPages() {
		pageIdx = startCI.NumPages() - 1
	}

	// The matching row has start_int <= target, which could be in the found page
	// or the one before it (if target equals a page boundary).
	startPage := pageIdx
	if startPage > 0 && startCI.MinValue(startPage).Int64() > target {
		startPage--
	}

	offsetIdx, err := startChunk.OffsetIndex()
	if err != nil {
		return fmt.Errorf("reading start_int offset index: %w", err)
	}

	firstRow := offsetIdx.FirstRowIndex(startPage)
	return r.lookupRow(rg, firstRow, target, info)
}

// lookupLinear scans all row groups sequentially (fallback for unsorted data).
// IP ranges don't overlap, so the first non-skipped row group either contains
// the match or the IP isn't in the dataset.
func (r *ParquetResolver) lookupLinear(target int64, info *IPInfo) error {
	for _, rg := range r.pf.RowGroups() {
		skip, err := r.skipRowGroup(rg, target)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		return r.lookupRow(rg, -1, target, info)
	}
	return nil
}

// lookupRow scans rows in the row group starting from firstRow (or the beginning
// if firstRow < 0). Fills info if a matching row is found.
func (r *ParquetResolver) lookupRow(rg parquet.RowGroup, firstRow int64, target int64, info *IPInfo) error {
	reader := parquet.NewGenericRowGroupReader[parquetRow](rg)
	defer reader.Close()

	if firstRow >= 0 {
		if err := reader.SeekToRow(firstRow); err != nil {
			return fmt.Errorf("seeking to row %d: %w", firstRow, err)
		}
	}

	var buf [10]parquetRow
	for {
		n, err := reader.Read(buf[:])
		for i := range n {
			if buf[i].StartInt > target {
				return nil
			}
			if target <= buf[i].EndInt {
				buf[i].fillIPInfo(info)
				return nil
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
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

func (r *ParquetResolver) checkStartIntSorted() bool {
	rowGroups := r.pf.RowGroups()
	if len(rowGroups) == 0 {
		return false
	}

	prevMax := int64(-1)
	for i, rg := range rowGroups {
		// Check sorting columns metadata if available.
		if sortCols := rg.SortingColumns(); len(sortCols) > 0 {
			hasSortedStart := false
			for _, sc := range sortCols {
				if len(sc.Path()) == 1 && sc.Path()[0] == "start_int" && !sc.Descending() {
					hasSortedStart = true
					break
				}
			}
			if !hasSortedStart {
				r.logger.Warn("row group has sorting columns but start_int is not ascending", "rowGroup", i)
				return false
			}
		}

		// Verify column index boundary order.
		ci, err := rg.ColumnChunks()[r.startIdx].ColumnIndex()
		if err != nil {
			r.logger.Warn("cannot read start_int column index", "rowGroup", i, "err", err)
			return false
		}
		// With multiple pages, boundary order must be ascending.
		// Single-page row groups trivially satisfy this.
		if ci.NumPages() > 1 && !ci.IsAscending() {
			r.logger.Warn("start_int column index not ascending", "rowGroup", i)
			return false
		}

		// Verify row groups are ordered relative to each other.
		curMin := ci.MinValue(0).Int64()
		if curMin <= prevMax {
			r.logger.Warn("start_int not sorted across row groups",
				"rowGroup", i, "min", curMin, "prevMax", prevMax)
			return false
		}
		prevMax = ci.MaxValue(ci.NumPages() - 1).Int64()
	}
	return true
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

func (r *parquetRow) fillIPInfo(info *IPInfo) {
	info.Network = r.Network
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
}
