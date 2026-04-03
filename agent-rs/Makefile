TARGET ?= aarch64-unknown-linux-musl

.PHONY: all
all: build

.PHONY: build
build:
	cargo build --release --target $(TARGET)

.PHONY: build-local
build-local:
	cargo build --release

.PHONY: fmt
fmt:
	cargo fmt

.PHONY: lint
lint:
	cargo fmt --check
	cargo clippy -- -D warnings

.PHONY: test
test:
	cargo test

.PHONY: clean
clean:
	cargo clean
