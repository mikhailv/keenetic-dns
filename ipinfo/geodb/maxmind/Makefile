TOOLS = ../../../tools
MMDBCONVERT = $(TOOLS)/mmdbconvert

MMDB_FILES = GeoLite2-ASN.mmdb GeoLite2-City.mmdb

OUTPUT_MMDB_FILE = ../geolite2.mmdb
OUTPUT_PARQUET_FILES = ../geolite2_ipv4.parquet ../geolite2_ipv6.parquet

.PHONY: all parquet mmdb clean

all: mmdb parquet

mmdb: $(OUTPUT_MMDB_FILE)

parquet: $(OUTPUT_PARQUET_FILES)

$(OUTPUT_MMDB_FILE): $(MMDBCONVERT) convert_mmdb.toml $(MMDB_FILES)
	$(MMDBCONVERT) convert_mmdb.toml

$(OUTPUT_PARQUET_FILES): $(MMDBCONVERT) convert_parquet.toml $(MMDB_FILES)
	$(MMDBCONVERT) convert_parquet.toml

clean:
	rm -f $(OUTPUT_MMDB_FILE) $(OUTPUT_PARQUET_FILES)

$(MMDBCONVERT):
	$(MAKE) -C $(TOOLS) mmdbconvert
