#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

source .env

DATABASES=(DB11LITEMMDB DBASNLITEMMDB PX12LITECIDR)

for db in "${DATABASES[@]}"; do
    echo "Downloading $db..."
    zipfile="${db}.zip"
    wget -O "$zipfile" "https://www.ip2location.com/download?code=$db&token=$IP2LOCATION_TOKEN"
    echo "Extracting database from $zipfile..."
    unzip -o -j "$zipfile" -x '*.TXT' 'README*' 'LICENSE*'
    rm "$zipfile"
    echo "$db done."
done

echo "All databases updated."
