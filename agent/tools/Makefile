DIR := $(shell pwd)

.PHONY: all
all: buf protoc-gen-connect-go

.PHONY: buf
buf:
	$(call go-get-tool,'$(DIR)/buf','github.com/bufbuild/buf/cmd/buf@v1.43.0')

.PHONY: protoc-gen-connect-go
protoc-gen-connect-go:
	$(call go-get-tool,'$(DIR)/protoc-gen-connect-go','connectrpc.com/connect/cmd/protoc-gen-connect-go@latest')

.PHONY: clean
clean:
	rm -f ./buf
	rm -f ./protoc-*

# will run 'go install' for package $2 and install it as $1.
define go-get-tool
@[ -f $(1) ] || { \
	set -e ;\
	echo "Building $(2) >>> $(1)" ;\
	GOBIN=$(shell dirname $(1)) go install $(2) ;\
}
endef
