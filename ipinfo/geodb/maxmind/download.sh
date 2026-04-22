#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

source .env

DATABASES=(GeoLite2-ASN GeoLite2-City)
#DATABASES=(GeoLite2-ASN GeoLite2-City GeoLite2-Country)

for db in "${DATABASES[@]}"; do
    echo "Downloading $db..."
    wget --content-disposition --user="$MAXMIND_USER_ID" --password="$MAXMIND_KEY" \
        "https://download.maxmind.com/geoip/databases/$db/download?suffix=tar.gz"
    tarball=$(ls -t "${db}"_*.tar.gz 2>/dev/null | head -1)
    echo "Extracting .mmdb from $tarball..."
    tar xzf "$tarball" --wildcards '*.mmdb' --strip-components=1
    rm "$tarball"
    echo "$db done."
done

echo "All databases updated."
ls -l *.mmdb
