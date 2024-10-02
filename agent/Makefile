PWD = $(shell pwd)
SHELL := env PATH=$(PATH):$(PWD)/tools bash

.PHONY: all
all: tools generate-rpc build

.PHONY: build
build:
	env GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -o agent ./cmd

.PHONY: generate-rpc
generate-rpc:
	buf lint
	buf generate

.PHONY: tools
tools:
	$(MAKE) -C tools
