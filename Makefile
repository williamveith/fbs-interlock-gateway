APP := fbs-interlock-gateway
CMD := ./cmd/$(APP)

SERVICE_DIR_WINDOWS := services/windows
SERVICE_DIR_LINUX := services/linux
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

FBS_SOURCE_IP=146.6.76.61
FBS_PORT_RANGE=8081:8981

DEPLOYMENT_GUIDES_DIR := deployment guides
LINUX_INSTALL_GUIDE := Linux Install Instructions.md
WINDOWS_INSTALL_GUIDE := Windows Install Instructions.md

SERVICE_TEMPLATE := $(SERVICE_DIR_LINUX)/app.service.in
SERVICE_OUT := $(LINUX_DIR)/$(APP).service

INSTALL_TEMPLATE := $(SERVICE_DIR_LINUX)/install-linux.sh.in
INSTALL_OUT := $(LINUX_DIR)/install.sh

UPDATE_TEMPLATE := $(SERVICE_DIR_LINUX)/update-linux.sh.in
UPDATE_OUT := $(LINUX_DIR)/update.sh

UPDATE_SERVICE_TEMPLATE := $(SERVICE_DIR_LINUX)/update.service.in
UPDATE_TIMER_TEMPLATE := $(SERVICE_DIR_LINUX)/update.timer.in

UPDATE_SERVICE_OUT := $(LINUX_DIR)/$(APP)-update.service
UPDATE_TIMER_OUT := $(LINUX_DIR)/$(APP)-update.timer

WINDOWS_INSTALL_DIR ?= C:/FBS/$(APP)
START_WINDOWS_TEMPLATE := $(SERVICE_DIR_WINDOWS)/start-windows.bat.in
START_WINDOWS_OUT := $(WINDOWS_DIR)/start.bat

RELEASE_DIR := $(BUILD_DIR)/release
LINUX_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-linux-amd64
LINUX_ARM64_ASSET := $(RELEASE_DIR)/$(APP)-linux-arm64
WINDOWS_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-windows-amd64.exe

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
SHA256SUM := shasum -a 256
else
SHA256SUM := sha256sum
endif

.PHONY: \
	run \
	fmt \
	fmt-check \
	tidy-check \
	vet \
	test \
	test-race \
	scripts-check \
	build-check \
	verify \
	init-config \
	build \
	build-mac \
	build-linux-arm64 \
	build-linux-amd64 \
	build-windows-amd64 \
	start-windows \
	release-linux-amd64 \
	release-linux-arm64 \
	release-windows-amd64 \
	release \
	shelly-auth \
	clean

# =========================
# DEVELOPMENT
# =========================

run:
	go run $(CMD) -config $(CONFIGS)

fmt:
	go fmt ./...

test:
	go test -count=1 ./...

build: build-mac

# =========================
# GENERATED DEPLOYMENT FILES
# =========================

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
		-e 's|@FBS_SOURCE_IP@|$(FBS_SOURCE_IP)|g' \
		-e 's|@FBS_PORT_RANGE@|$(FBS_PORT_RANGE)|g' \
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

# =========================
# CONFIGURATION
# =========================

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

# =========================
# DEPLOYMENT BUILDS
# =========================

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

# =========================
# RELEASE BUILDS
# =========================

release-linux-amd64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_AMD64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-linux-amd64" > "$(APP)-linux-amd64.sha256"

release-linux-arm64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_ARM64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-linux-arm64" > "$(APP)-linux-arm64.sha256"

release-windows-amd64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(WINDOWS_AMD64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-windows-amd64.exe" > "$(APP)-windows-amd64.exe.sha256"

# =========================
# VALIDATION
# =========================

fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "The following Go files are not formatted:"; \
		echo "$$files"; \
		exit 1; \
	fi

tidy-check:
	@set -eu; \
	tmp_dir="$$(mktemp -d)"; \
	cp go.mod go.sum "$$tmp_dir/"; \
	trap 'cp "$$tmp_dir/go.mod" go.mod; cp "$$tmp_dir/go.sum" go.sum; rm -rf "$$tmp_dir"' EXIT; \
	go mod tidy; \
	diff -u "$$tmp_dir/go.mod" go.mod; \
	diff -u "$$tmp_dir/go.sum" go.sum

vet:
	go vet ./...

test-race:
	go test -race -count=1 ./...

scripts-check:
	bash -n scripts/*.sh
	bash -n "$(INSTALL_TEMPLATE)" "$(UPDATE_TEMPLATE)"

build-check:
	mkdir -p "$(BUILD_DIR)/ci"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-linux-amd64" \
		$(CMD)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-linux-arm64" \
		$(CMD)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-windows-amd64.exe" \
		$(CMD)

verify: fmt-check tidy-check vet test-race scripts-check build-check

release: verify release-linux-amd64 release-linux-arm64 release-windows-amd64

# =========================
# UTILITIES
# =========================

shelly-auth:
	@chmod +x scripts/set-shelly-auth.sh
	@./scripts/set-shelly-auth.sh

clean:
	rm -rf "$(BUILD_DIR)"
	go clean