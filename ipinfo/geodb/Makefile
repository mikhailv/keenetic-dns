.PHONY: all parquet mmdb clean

all: mmdb parquet

mmdb:
	make -C ip2loc mmdb
	make -C maxmind mmdb

parquet:
	make -C ip2loc parquet
	make -C maxmind parquet

clean:
	make -C ip2loc clean
	make -C maxmind clean
