SSH_PATH ?= root@192.168.2.1:/opt
SSH_PATH_KEENETIC = $(SSH_PATH)/keenetic-dns
SSH_PORT ?= 22
GOARCH ?= arm64

.PHONY: all
all: lint govulncheck build

.PHONY: build
build: build-agent build-dns-server

.PHONY: build-agent
build-agent:
	GOARCH=$(GOARCH) $(MAKE) -C agent

.PHONY: build-dns-server
build-dns-server:
	GOARCH=$(GOARCH) $(MAKE) -C dns-server

.PHONY: lint
lint:
	$(MAKE) -C agent lint
	$(MAKE) -C dns-server lint

.PHONY: govulncheck
govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: upload
upload: upload-config upload-agent upload-dns-server

.PHONY: upload-config
upload-config:
	scp -pC -P $(SSH_PORT) dns-server/config.yaml $(SSH_PATH_KEENETIC)/

.PHONY: upload-agent
upload-agent:
	scp -pC -P $(SSH_PORT) agent/agent $(SSH_PATH_KEENETIC)/update/

.PHONY: upload-dns-server
upload-dns-server:
	scp -pC -P $(SSH_PORT) dns-server/dns-server $(SSH_PATH_KEENETIC)/update/

.PHONY: upload-deploy
upload-deploy:
	scp -pC -P $(SSH_PORT) deploy/init.d/* $(SSH_PATH)/etc/init.d/
	scp -pC -P $(SSH_PORT) deploy/*.sh $(SSH_PATH_KEENETIC)/
