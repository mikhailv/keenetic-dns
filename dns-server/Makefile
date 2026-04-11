TARGET_OS ?= linux
TARGET_ARCH ?=

.PHONY: all
all: build

.PHONY: build
build: generate build-web build-bin

.PHONY: build-web
build-web:
	$(MAKE) -C web build

.PHONY: lint
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3 run -v
	$(MAKE) -C web format lint

.PHONY: build-bin
build-bin:
	# flags to reduce binary size (affect on pprof results): -trimpath -ldflags="-s -w"
	env GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -o dns-server ./cmd

.PHONY: generate
generate:
	go generate ./...
