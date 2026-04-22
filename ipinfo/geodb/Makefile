MMDBCONVERT ?= mmdbconvert

MMDB_FILES = maxmind/GeoLite2-ASN.mmdb maxmind/GeoLite2-City.mmdb
PARQUET_FILES = geolite2_ipv4.parquet geolite2_ipv6.parquet

.PHONY: all parquet mmdb clean

all: mmdb parquet

mmdb: geodb/geolite2.mmdb

parquet: $(PARQUET_FILES)

geolite2.mmdb: geodb/convert_mmdb.toml $(MMDB_FILES)
	$(MMDBCONVERT) convert_mmdb.toml

$(PARQUET_FILES): geodb/convert_parquet.toml $(MMDB_FILES)
	$(MMDBCONVERT) convert_parquet.toml

clean:
	rm -f geolite2.mmdb $(PARQUET_FILES)
