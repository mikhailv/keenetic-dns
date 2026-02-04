PWD = $(shell pwd)
SHELL := env PATH=$(PATH):$(PWD)/tools bash

GOOS ?= linux
GOARCH ?=

.PHONY: all
all: tools generate-rpc build

.PHONY: build
build:
	env GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o agent ./cmd

.PHONY: generate-rpc
generate-rpc:
	buf lint
	buf generate

.PHONY: tools
tools:
	$(MAKE) -C tools
