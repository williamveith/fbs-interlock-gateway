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

UPDATE_TEMPLATE := $(SERVICE_DIR)/update-linux.sh.in
UPDATE_OUT := $(LINUX_DIR)/update.sh

UPDATE_SERVICE_TEMPLATE := $(SERVICE_DIR)/update.service.in
UPDATE_TIMER_TEMPLATE := $(SERVICE_DIR)/update.timer.in

UPDATE_SERVICE_OUT := $(LINUX_DIR)/$(APP)-update.service
UPDATE_TIMER_OUT := $(LINUX_DIR)/$(APP)-update.timer

RELEASE_DIR := $(BUILD_DIR)/release
LINUX_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-linux-amd64
LINUX_ARM64_ASSET := $(RELEASE_DIR)/$(APP)-linux-arm64

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: run fmt build-mac build-linux-arm64 build-linux-amd64 release-linux-amd64 release-linux-arm64 release clean

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

$(UPDATE_OUT): $(UPDATE_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(UPDATE_TEMPLATE)" > "$@"
	chmod +x "$@"

$(UPDATE_SERVICE_OUT): $(UPDATE_SERVICE_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(UPDATE_SERVICE_TEMPLATE)" > "$@"

$(UPDATE_TIMER_OUT): $(UPDATE_TIMER_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(UPDATE_TIMER_TEMPLATE)" > "$@"

init-config:
	@if [ -f "$(CONFIGS)" ]; then \
		echo "$(CONFIGS) already exists; not overwriting."; \
	else \
		echo "Creating $(CONFIGS)"; \
		printf '%s\n' \
			'bind: 0.0.0.0' \
			'' \
			'defaults:' \
			'  timeout_ms: 800' \
			'  safe_state_on_error: "off"' \
			'' \
			'tools:' \
			'  - interlock_name:' \
			'    ip:' \
			'    port:' \
			'    switch_id:' \
			'    username:' \
			'    password:' \
			'    enabled:' \
			> "$(CONFIGS)"; \
	fi

build-mac: fmt
	mkdir -p "$(MAC_DIR)"
	cp "$(CONFIGS)" "$(MAC_DIR)/"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(MAC_DIR)/$(APP)" .

build-linux-arm64: fmt $(SERVICE_OUT) $(INSTALL_OUT) $(UPDATE_OUT) $(UPDATE_SERVICE_OUT) $(UPDATE_TIMER_OUT)
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" .

build-linux-amd64: fmt $(SERVICE_OUT) $(INSTALL_OUT) $(UPDATE_OUT) $(UPDATE_SERVICE_OUT) $(UPDATE_TIMER_OUT)
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" .

release-linux-amd64: fmt
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_AMD64_ASSET)" .
	cd "$(RELEASE_DIR)" && sha256sum "$(APP)-linux-amd64" > "$(APP)-linux-amd64.sha256"

release-linux-arm64: fmt
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_ARM64_ASSET)" .
	cd "$(RELEASE_DIR)" && sha256sum "$(APP)-linux-arm64" > "$(APP)-linux-arm64.sha256"

release: release-linux-amd64 release-linux-arm64

clean:
	rm -rf "$(BUILD_DIR)"
	go clean