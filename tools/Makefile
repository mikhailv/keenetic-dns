DIR := $(shell pwd)

.PHONY: all
all: oapi-codegen mmdbconvert

oapi-codegen:
	$(call go-get-tool,'$(DIR)/oapi-codegen','github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.6.0')

mmdbconvert:
	$(call go-get-tool,'$(DIR)/mmdbconvert','github.com/maxmind/mmdbconvert/cmd/mmdbconvert@v0.1.0')

.PHONY: clean
clean:
	rm -f ./oapi-codegen

# will run 'go install' for package $2 and install it as $1.
define go-get-tool
{ \
	set -e ;\
	echo "Building $(2) >>> $(1)" ;\
	GOBIN=$(shell dirname $(1)) go install $(2) ;\
}
endef
