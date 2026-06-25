.PHONY: all parquet mmdb clean

all: mmdb parquet

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
