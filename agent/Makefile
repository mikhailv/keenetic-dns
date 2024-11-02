PWD = $(shell pwd)
SHELL := env PATH=$(PATH):$(PWD)/tools bash

GOOS ?= linux
GOARCH ?= mipsle
GOMIPS ?= softfloat

.PHONY: all
all: tools generate-rpc build

.PHONY: build
build:
	env GOOS=$(GOOS) GOARCH=$(GOARCH) GOMIPS=$(GOMIPS) go build -o agent ./cmd

.PHONY: generate-rpc
generate-rpc:
	buf lint
	buf generate

.PHONY: tools
tools:
	$(MAKE) -C tools
