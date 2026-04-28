TARGET_OS ?= linux
TARGET_ARCH ?=

TOOLS = ../tools
OAPI_CODEGEN = $(TOOLS)/oapi-codegen

.PHONY: all
all: build

.PHONY: build
build: generate
	env GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -o agent ./cmd

.PHONY: lint
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3 run

.PHONY: generate
generate: $(OAPI_CODEGEN)
	go generate ./...

$(OAPI_CODEGEN):
	$(MAKE) -C $(TOOLS) oapi-codegen
