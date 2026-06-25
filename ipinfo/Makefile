TARGET_OS ?= linux
TARGET_ARCH ?=

IMAGE := ghcr.io/mikhailv/ipinfo:latest

.PHONY: all
all: build

.PHONY: build
build:
	env GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -o ipinfo ./cmd

.PHONY: lint
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3 run

.PHONY: image
image:
	docker build -f Dockerfile -t $(IMAGE) ..

.PHONY: push
push:
	docker push $(IMAGE)

.PHONY: release
release: image push
