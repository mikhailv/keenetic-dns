TARGET_OS ?= linux
TARGET_ARCH ?=

.PHONY: all
all: build

.PHONY: build
build: generate
	env GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -o agent ./cmd

.PHONY: lint
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3 run -v

.PHONY: generate
generate:
	go generate ./...
