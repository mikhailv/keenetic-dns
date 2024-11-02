GOOS ?= linux
GOARCH ?= mipsle
GOMIPS ?= softfloat

.PHONY: all
all: build

.PHONY: build
build: build-web build-bin

.PHONY: build-web
build-web:
	cd web && npm run build

.PHONY: build-bin
build-bin:
	# flags to reduce binary size (affect on pprof results): -trimpath -ldflags="-s -w"
	env GOOS=$(GOOS) GOARCH=$(GOARCH) GOMIPS=$(GOMIPS) go build -o dns-server ./cmd
