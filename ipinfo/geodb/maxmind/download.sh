#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

source .env

DATABASES=(GeoLite2-ASN GeoLite2-City)
#DATABASES=(GeoLite2-ASN GeoLite2-City GeoLite2-Country)

STATE_DIR=.state
mkdir -p "$STATE_DIR"

# Set FORCE=1 to re-download regardless of the stored checksum.
FORCE="${FORCE:-}"

updated=0

for db in "${DATABASES[@]}"; do
    base_url="https://download.maxmind.com/geoip/databases/$db/download"
    state_file="$STATE_DIR/$db.sha256"

    echo "Checking $db..."
    # MaxMind serves a small companion checksum file; use it to detect new releases
    # without downloading the (large) tarball every time.
    remote_sha=$(curl -fsSL --user "$MAXMIND_USER_ID:$MAXMIND_KEY" \
        "$base_url?suffix=tar.gz.sha256")

    if [[ -z "$FORCE" && -f "$state_file" ]] && [[ "$(cat "$state_file")" == "$remote_sha" ]]; then
        echo "$db is up to date, skipping."
        continue
    fi

    echo "Downloading $db..."
    wget -q --show-progress --content-disposition --user="$MAXMIND_USER_ID" --password="$MAXMIND_KEY" \
        "$base_url?suffix=tar.gz"
    tarball=$(ls -t "${db}"_*.tar.gz 2>/dev/null | head -1)

    echo "Verifying checksum of $tarball..."
    echo "${remote_sha%% *}  $tarball" | sha256sum -c -

    echo "Extracting .mmdb from $tarball..."
    tar xzf "$tarball" --wildcards '*.mmdb' --strip-components=1
    rm "$tarball"

    printf '%s' "$remote_sha" > "$state_file"
    updated=1
    echo "$db done."
done

if [[ "$updated" -eq 1 ]]; then
    echo "All databases updated."
    exit 0
else
    echo "Everything already up to date, nothing downloaded."
    # Exit 3 signals "nothing new" so callers (Makefile) can skip the rebuild.
    exit 3
fi
