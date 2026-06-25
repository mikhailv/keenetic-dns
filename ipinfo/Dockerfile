# Dockerfile for the ipinfo subservice.
#
# The Go module is rooted at the repository root, so this image MUST be built
# with the repo root as the build context (not the ipinfo/ directory):
#
#   docker build -f ipinfo/Dockerfile -t ipinfo .
#
# The geo/proxy database files (*.mmdb, *.parquet) are gitignored and produced
# locally under ipinfo/geodb/ (see ipinfo/geodb/Makefile). They must exist on
# disk before building; they are baked into the image alongside the binary.

# ---- build stage ------------------------------------------------------------
FROM golang:1.26.4-alpine AS build

WORKDIR /src

# Resolve modules first so the layer caches across source-only changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# Static, CGO-free binary (maxminddb + parquet-go are pure Go).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/ipinfo ./ipinfo/cmd

# ---- runtime stage ----------------------------------------------------------
FROM alpine:3.21

# Run as an unprivileged user.
RUN addgroup -S app && adduser -S -G app app
USER app

WORKDIR /app

# Binary and data files share the same directory inside the image.
#
# Layer ordering matters for registry push dedup: copy the large, rarely-changing
# database files FIRST so their layers stay cache-stable, and the binary (which
# changes every build) LAST. If the binary came first it would bust the cache of
# every layer after it, forcing the ~300MB of data to re-upload on every push.
COPY ipinfo/geodb/geolite2.mmdb ipinfo/geodb/geolite2_ipv4.parquet ./
COPY ipinfo/geodb/ip2proxy.mmdb ipinfo/geodb/ip2proxy_ipv4.parquet ./
COPY --from=build /out/ipinfo ./ipinfo

EXPOSE 8080

# Database flags point at the co-located files and are always applied.
# CMD holds overridable runtime flags (override the whole CMD to change them,
# e.g. set -cache-ttl=30m, -addr=..., or add -debug).
ENTRYPOINT ["/app/ipinfo", \
    "-geo-mmdb=/app/geolite2.mmdb", \
    "-geo-parquet=/app/geolite2_ipv4.parquet", \
    "-proxy-mmdb=/app/ip2proxy.mmdb", \
    "-proxy-parquet=/app/ip2proxy_ipv4.parquet"]

CMD ["-addr=0.0.0.0:8080", "-cache-ttl=1h"]
