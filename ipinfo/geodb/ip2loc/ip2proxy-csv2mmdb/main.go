// Command csv2mmdb converts IP2PROXY-LITE-PX12.CIDR.CSV to MMDB format.
//
// Usage:
//
//	go run ./ipinfo/geodb/ip2loc/csv2mmdb -in IP2PROXY-LITE-PX12.CIDR.CSV -out IP2PROXY-LITE-PX12.mmdb
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"strconv"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

// CSV columns (IP2PROXY-LITE-PX12.CIDR.CSV):
//
//	0: CIDR Range
//	1: Proxy Type
//	2: Country Code
//	3: Country Name
//	4: Region Name
//	5: City Name
//	6: ISP
//	7: Domain
//	8: Usage Type
//	9: ASN
//	10: Last Seen
//	11: Threat
//	12: Residential
//	13: Provider
//	14: Fraud Score

func main() {
	var inPath, outPath string
	flag.StringVar(&inPath, "in", "IP2PROXY-LITE-PX12.CIDR.CSV", "input CSV file")
	flag.StringVar(&outPath, "out", "IP2PROXY-LITE-PX12.mmdb", "output MMDB file")
	flag.Parse()

	writer, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "IP2Proxy-PX12",
		Description:  map[string]string{"en": "IP2Proxy LITE PX12"},
		RecordSize:   28,
	})
	if err != nil {
		log.Fatalf("creating mmdb writer: %v", err)
	}

	count, err := loadCSV(writer, inPath)
	if err != nil {
		log.Fatalf("loading CSV: %v", err)
	}

	if err := writeMMDB(writer, outPath); err != nil {
		log.Fatalf("writing MMDB: %v", err)
	}

	log.Printf("wrote %d networks to %s", count, outPath)
}

func loadCSV(writer *mmdbwriter.Tree, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	var count int
	for {
		row, readErr := r.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return 0, readErr
		}
		if len(row) < 15 {
			return 0, errors.New("expected 15 columns")
		}

		_, network, parseErr := net.ParseCIDR(row[0])
		if parseErr != nil {
			return 0, parseErr
		}

		record := buildRecord(row)

		if insertErr := writer.Insert(network, record); insertErr != nil {
			return 0, insertErr
		}
		count++
	}
	return count, nil
}

func buildRecord(row []string) mmdbtype.Map {
	record := mmdbtype.Map{}

	addField := func(name, value string) {
		if value != "" && value != "-" {
			record[mmdbtype.String(name)] = mmdbtype.String(value)
		}
	}

	addField("proxy_type", row[1])
	addField("country_code", row[2])
	addField("country_name", row[3])
	addField("region_name", row[4])
	addField("city_name", row[5])
	addField("isp", row[6])
	addField("domain", row[7])
	addField("usage_type", row[8])
	addField("asn", row[9])
	addField("last_seen", row[10])
	addField("threat", row[11])
	addField("residential", row[12])
	addField("provider", row[13])

	if score, err := strconv.Atoi(row[14]); err == nil {
		record["fraud_score"] = mmdbtype.Uint16(score)
	}

	return record
}

func writeMMDB(writer *mmdbwriter.Tree, path string) error {
	return util.SaveToFileFunc(path, func(w io.Writer) error {
		_, err := writer.WriteTo(w)
		return err
	})
}
