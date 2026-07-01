#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

source .env

DATABASES=(DB11LITEMMDB DBASNLITEMMDB PX12LITECIDR)

STATE_DIR=.state
mkdir -p "$STATE_DIR"

# Set FORCE=1 to re-check/re-download regardless of the stored fingerprint.
FORCE="${FORCE:-}"

# Note: IP2Location LITE rate-limits downloads to 5 per file per 24h, and even
# the lightweight probe below counts against that quota. We check every run and
# simply fail if the limit is hit.

# Remove any partially downloaded archive on exit, preserving the exit status
# (a bare test as the trap's last command would otherwise clobber it).
zipfile=
cleanup() {
    local rc=$?
    [[ -n "$zipfile" && -f "$zipfile" ]] && rm -f "$zipfile"
    exit "$rc"
}
trap cleanup EXIT

# Fingerprint a remote file so we can detect a new release without downloading
# the whole archive. The endpoint 302-redirects to a presigned Cloudflare R2 URL
# that only accepts GET (HEAD returns 403), so we do a 1-byte ranged GET and read
# the object's stable identity headers: ETag, Last-Modified and the total size
# from Content-Range (bytes 0-0/<size>). Echoes empty if no headers are usable.
remote_fingerprint() {
    local url="$1"
    curl -fsSL -r 0-0 -D - -o /dev/null "$url" 2>/dev/null \
        | tr -d '\r' \
        | grep -iE '^(etag|last-modified|content-range):' \
        | sort -u \
        || true
}

updated=0

for db in "${DATABASES[@]}"; do
    url="https://www.ip2location.com/download?code=$db&token=$IP2LOCATION_TOKEN"
    state_file="$STATE_DIR/$db.fingerprint"

    echo "Checking $db..."
    fingerprint=$(remote_fingerprint "$url")

    if [[ -z "$FORCE" && -n "$fingerprint" && -f "$state_file" ]] \
        && [[ "$(cat "$state_file")" == "$fingerprint" ]]; then
        echo "$db is up to date, skipping."
        continue
    fi

    echo "Downloading $db..."
    zipfile="${db}.zip"
    wget -q --show-progress -O "$zipfile" "$url"

    # The download can come back as a tiny non-zip body instead of an archive.
    # A known rate-limit page ("THIS FILE CAN ONLY BE DOWNLOADED 5 TIMES WITHIN
    # 24 HOURS") is reported and skipped without failing the run; any other
    # unexpected content is treated as an error before clobbering the database.
    if ! unzip -tqq "$zipfile" >/dev/null 2>&1; then
        if grep -qi 'CAN ONLY BE DOWNLOADED' "$zipfile"; then
            echo "WARNING: $db is rate-limited, skipping:" >&2
            head -c 200 "$zipfile" >&2
            echo >&2
            rm -f "$zipfile"
            zipfile=
            continue
        fi
        echo "ERROR: $db download is not a valid zip archive:" >&2
        head -c 200 "$zipfile" >&2
        echo >&2
        exit 1
    fi

    echo "Extracting database from $zipfile..."
    unzip -o -j "$zipfile" -x '*.TXT' 'README*' 'LICENSE*'
    rm -f "$zipfile"
    zipfile=

    if [[ -n "$fingerprint" ]]; then
        printf '%s' "$fingerprint" > "$state_file"
    fi
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
