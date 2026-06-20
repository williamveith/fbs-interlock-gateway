APP=fbs-interlock-gateway
SERVICE_DIR := services
BUILD_DIR := build
MAC_DIR := $(BUILD_DIR)/darwin
LINUX_DIR := $(BUILD_DIR)/linux

CONFIGS := config.yaml

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: run fmt build-mac build-pi deploy logs status clean

run:
	go run . -config config.yaml

fmt:
	go fmt ./...

build-mac: fmt
	mkdir -p "$(MAC_DIR)"
	cp "$(CONFIGS)" "$(MAC_DIR)/"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(MAC_DIR)/$(APP)" .

build-pi: fmt
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	if [ -d "$(SERVICE_DIR)" ]; then cp -R "$(SERVICE_DIR)" "$(LINUX_DIR)/"; fi
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" .

build-linux-amd64: fmt
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	if [ -d "$(SERVICE_DIR)" ]; then cp -R "$(SERVICE_DIR)" "$(LINUX_DIR)/"; fi
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" .

clean:
	rm -rf "$(BUILD_DIR)"
	go clean