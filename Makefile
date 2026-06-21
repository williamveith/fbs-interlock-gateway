APP := fbs-interlock-gateway
SERVICE_DIR := services
BUILD_DIR := build
MAC_DIR := $(BUILD_DIR)/darwin
LINUX_DIR := $(BUILD_DIR)/linux

CONFIGS := config.yaml

INSTALL_DIR ?= /opt/$(APP)
SERVICE_USER ?= fbs-gateway
SERVICE_GROUP ?= $(SERVICE_USER)
SERVICE_TEMPLATE := $(SERVICE_DIR)/app.service.in
SERVICE_OUT := $(LINUX_DIR)/$(APP).service
INSTALL_TEMPLATE := $(SERVICE_DIR)/install-linux.sh.in
INSTALL_OUT := $(LINUX_DIR)/install.sh

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: run fmt build-mac build-pi build-linux-amd64 clean

run:
	go run . -config config.yaml

fmt:
	go fmt ./...

$(SERVICE_OUT): $(SERVICE_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(SERVICE_TEMPLATE)" > "$@"

$(INSTALL_OUT): $(INSTALL_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(INSTALL_TEMPLATE)" > "$@"
	chmod +x "$@"

build-mac: fmt
	mkdir -p "$(MAC_DIR)"
	cp "$(CONFIGS)" "$(MAC_DIR)/"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(MAC_DIR)/$(APP)" .

build-pi: fmt $(SERVICE_OUT) $(INSTALL_OUT)
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" .

build-linux-amd64: fmt $(SERVICE_OUT) $(INSTALL_OUT)
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" .

clean:
	rm -rf "$(BUILD_DIR)"
	go clean