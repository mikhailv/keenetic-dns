SSH_PATH ?= root@192.168.2.1:/opt
SSH_PATH_KEENETIC = $(SSH_PATH)/keenetic-dns
SSH_PORT ?= 222

.PHONY: all
all: lint build

.PHONY: build
build: build-agent build-dns-server

.PHONY: build-agent
build-agent:
	$(MAKE) -C agent

.PHONY: build-dns-server
build-dns-server:
	$(MAKE) -C dns-server

.PHONY: lint
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0 run -v

.PHONY: upload
upload: upload-config upload-agent upload-dns-server

.PHONY: upload-config
upload-config:
	scp -p -P $(SSH_PORT) dns-server/config.yaml $(SSH_PATH_KEENETIC)/

.PHONY: upload-agent
upload-agent:
	scp -p -P $(SSH_PORT) agent/agent $(SSH_PATH_KEENETIC)/update/

.PHONY: upload-dns-server
upload-dns-server:
	scp -p -P $(SSH_PORT) dns-server/dns-server $(SSH_PATH_KEENETIC)/update/

.PHONY: upload-deploy
upload-deploy:
	scp -p -P $(SSH_PORT) deploy/init.d/* $(SSH_PATH)/etc/init.d/
	scp -p -P $(SSH_PORT) deploy/*.sh $(SSH_PATH_KEENETIC)/
