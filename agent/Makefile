PWD = $(shell pwd)
SHELL := env PATH=$(PATH):$(PWD)/tools bash

GOOS ?= linux
GOARCH ?=

.PHONY: all
all: tools generate-rpc build

.PHONY: build
build:
	env GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o agent ./cmd

.PHONY: lint
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0 run -v

.PHONY: generate-rpc
generate-rpc:
	buf lint
	buf generate

.PHONY: tools
tools:
	$(MAKE) -C tools
