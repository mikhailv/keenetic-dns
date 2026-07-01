.PHONY: all update parquet mmdb clean download

all: mmdb parquet

# Download fresh source databases (skipping unchanged ones) and rebuild only
# the outputs whose sources changed.
update:
	make -C ip2loc update
	make -C maxmind update

download:
	(cd ip2loc && ./download.sh)
	(cd maxmind && ./download.sh)

mmdb:
	make -C ip2loc mmdb
	make -C maxmind mmdb

parquet:
	make -C ip2loc parquet
	make -C maxmind parquet

clean:
	make -C ip2loc clean
	make -C maxmind clean
