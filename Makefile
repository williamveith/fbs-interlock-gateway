APP := fbs-interlock-gateway
CMD := ./cmd/$(APP)

SERVICE_DIR := services
BUILD_DIR := build
MAC_DIR := $(BUILD_DIR)/darwin
LINUX_DIR := $(BUILD_DIR)/linux
WINDOWS_DIR := $(BUILD_DIR)/windows

CONFIGS := config.yaml
CONFIG_DIR ?= /etc/$(APP)
CONFIG_PATH ?= $(CONFIG_DIR)/$(CONFIGS)

INSTALL_DIR ?= /opt/$(APP)
SERVICE_USER ?= fbs-gateway
SERVICE_GROUP ?= $(SERVICE_USER)

DEPLOYMENT_GUIDES_DIR := deployment guides
LINUX_INSTALL_GUIDE := Linux Install Instructions.md
WINDOWS_INSTALL_GUIDE := Windows Install Instructions.md

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

WINDOWS_INSTALL_DIR ?= C:/FBS/$(APP)
START_WINDOWS_TEMPLATE := $(SERVICE_DIR)/start-windows.bat.in
START_WINDOWS_OUT := $(WINDOWS_DIR)/start.bat

RELEASE_DIR := $(BUILD_DIR)/release
LINUX_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-linux-amd64
LINUX_ARM64_ASSET := $(RELEASE_DIR)/$(APP)-linux-arm64
WINDOWS_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-windows-amd64.exe

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
SHA256SUM := shasum -a 256
else
SHA256SUM := sha256sum
endif

.PHONY: run fmt test init-config build build-mac build-linux-arm64 build-linux-amd64 build-windows-amd64 start-windows release-linux-amd64 release-linux-arm64 release-windows-amd64 release shelly-auth clean

run:
	go run $(CMD) -config $(CONFIGS)

fmt:
	go fmt ./...

test:
	go test ./...

build: build-mac

$(SERVICE_OUT): $(SERVICE_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@CONFIG_DIR@|$(CONFIG_DIR)|g' \
		-e 's|@CONFIG_PATH@|$(CONFIG_PATH)|g' \
		-e 's|@SERVICE_USER@|$(SERVICE_USER)|g' \
		-e 's|@SERVICE_GROUP@|$(SERVICE_GROUP)|g' \
		"$(SERVICE_TEMPLATE)" > "$@"

$(INSTALL_OUT): $(INSTALL_TEMPLATE) Makefile
	mkdir -p "$(LINUX_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@INSTALL_DIR@|$(INSTALL_DIR)|g' \
		-e 's|@CONFIG_DIR@|$(CONFIG_DIR)|g' \
		-e 's|@CONFIG_PATH@|$(CONFIG_PATH)|g' \
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

$(START_WINDOWS_OUT): $(START_WINDOWS_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		"$(START_WINDOWS_TEMPLATE)" > "$@"

start-windows: $(START_WINDOWS_OUT)

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
		-o "$(MAC_DIR)/$(APP)" \
		$(CMD)

build-linux-arm64: fmt $(SERVICE_OUT) $(INSTALL_OUT) $(UPDATE_OUT) $(UPDATE_SERVICE_OUT) $(UPDATE_TIMER_OUT)
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	cp "$(DEPLOYMENT_GUIDES_DIR)/$(LINUX_INSTALL_GUIDE)" "$(LINUX_DIR)/"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" \
		$(CMD)

build-linux-amd64: fmt $(SERVICE_OUT) $(INSTALL_OUT) $(UPDATE_OUT) $(UPDATE_SERVICE_OUT) $(UPDATE_TIMER_OUT)
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	cp "$(DEPLOYMENT_GUIDES_DIR)/$(LINUX_INSTALL_GUIDE)" "$(LINUX_DIR)/"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)" \
		$(CMD)

build-windows-amd64: fmt $(START_WINDOWS_OUT)
	mkdir -p "$(WINDOWS_DIR)"
	cp "$(CONFIGS)" "$(WINDOWS_DIR)/"
	cp "$(DEPLOYMENT_GUIDES_DIR)/$(WINDOWS_INSTALL_GUIDE)" "$(WINDOWS_DIR)/"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(WINDOWS_DIR)/$(APP).exe" \
		$(CMD)

release-linux-amd64: fmt
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_AMD64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && $(SHA256SUM) "$(APP)-linux-amd64" > "$(APP)-linux-amd64.sha256"

release-linux-arm64: fmt
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_ARM64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && $(SHA256SUM) "$(APP)-linux-arm64" > "$(APP)-linux-arm64.sha256"

release-windows-amd64: fmt
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(WINDOWS_AMD64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && $(SHA256SUM) "$(APP)-windows-amd64.exe" > "$(APP)-windows-amd64.exe.sha256"

release: release-linux-amd64 release-linux-arm64 release-windows-amd64

shelly-auth:
	@chmod +x scripts/set-shelly-auth.sh
	@./scripts/set-shelly-auth.sh

clean:
	rm -rf "$(BUILD_DIR)"
	go clean