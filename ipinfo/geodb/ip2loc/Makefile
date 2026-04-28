TOOLS = ../../../tools
MMDBCONVERT = $(TOOLS)/mmdbconvert

GEO_MMDB_FILES = IP2LOCATION-LITE-ASN.MMDB IP2LOCATION-LITE-DB11.MMDB
PROXY_CSV_FILE = IP2PROXY-LITE-PX12.CIDR.CSV

OUTPUT_GEO_MMDB_FILE = ../ip2location.mmdb
OUTPUT_GEO_PARQUET_FILES = ../ip2location_ipv4.parquet ../ip2location_ipv6.parquet

OUTPUT_PROXY_MMDB_FILE = ../ip2proxy.mmdb
OUTPUT_PROXY_PARQUET_FILE = ../ip2proxy_ipv4.parquet ../ip2proxy_ipv6.parquet

OUTPUT_MMDB_FILES = $(OUTPUT_GEO_MMDB_FILE) $(OUTPUT_PROXY_MMDB_FILE)
OUTPUT_PARQUET_FILES = $(OUTPUT_GEO_PARQUET_FILES) $(OUTPUT_PROXY_PARQUET_FILE)

.PHONY: all parquet mmdb clean

all: mmdb parquet

mmdb: $(OUTPUT_MMDB_FILES)

parquet: $(OUTPUT_PARQUET_FILES)

$(OUTPUT_GEO_MMDB_FILE): $(MMDBCONVERT) convert_geo_mmdb.toml $(GEO_MMDB_FILES)
	$(MMDBCONVERT) convert_geo_mmdb.toml

$(OUTPUT_GEO_PARQUET_FILES): $(MMDBCONVERT) convert_geo_parquet.toml $(GEO_MMDB_FILES)
	$(MMDBCONVERT) convert_geo_parquet.toml

$(OUTPUT_PROXY_MMDB_FILE): $(PROXY_CSV_FILE)
	go run ./ip2proxy-csv2mmdb -in $(PROXY_CSV_FILE) -out ../ip2proxy.mmdb

$(OUTPUT_PROXY_PARQUET_FILE): $(MMDBCONVERT) convert_proxy_parquet.toml $(OUTPUT_PROXY_MMDB_FILE)
	$(MMDBCONVERT) convert_proxy_parquet.toml

clean:
	rm -f $(OUTPUT_MMDB_FILES) $(OUTPUT_GEO_PARQUET_FILES)

$(MMDBCONVERT):
	$(MAKE) -C $(TOOLS) mmdbconvert
