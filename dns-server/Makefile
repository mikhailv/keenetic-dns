.PHONY: all
all: build

.PHONY: build
build: build-web build-bin

.PHONY: build-web
build-web:
	cd web && npm run build

.PHONY: build-bin
build-bin:
	env GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -o dns-server ./cmd
	#env GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -trimpath -ldflags="-s -w" .
