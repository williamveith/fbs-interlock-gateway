APP := fbs-interlock-gateway
CMD := ./cmd/$(APP)

SERVICE_DIR_WINDOWS := services/windows
SERVICE_DIR_LINUX := services/linux
SERVICE_DIR_MACOS := services/macos

BUILD_DIR := build
MAC_DIR := $(BUILD_DIR)/darwin
MAC_ARM64_DIR := $(MAC_DIR)/arm64
MAC_AMD64_DIR := $(MAC_DIR)/amd64
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
MACOS_INSTALL_GUIDE := macOS Install Instructions.md

# =========================
# LINUX SERVICE CONFIGS
# =========================
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

# =========================
# WINDOWS SERVICE CONFIGS
# =========================
WINDOWS_INSTALL_DIR ?= C:/FBS/$(APP)

WINDOWS_START_TEMPLATE := $(SERVICE_DIR_WINDOWS)/start.bat.in
WINDOWS_INSTALL_BAT_TEMPLATE := $(SERVICE_DIR_WINDOWS)/install.bat.in
WINDOWS_INSTALL_PS1_TEMPLATE := $(SERVICE_DIR_WINDOWS)/install.ps1.in
WINDOWS_UNINSTALL_BAT_TEMPLATE := $(SERVICE_DIR_WINDOWS)/uninstall.bat.in
WINDOWS_UNINSTALL_PS1_TEMPLATE := $(SERVICE_DIR_WINDOWS)/uninstall.ps1.in

WINDOWS_START_OUT := $(WINDOWS_DIR)/start.bat
WINDOWS_INSTALL_BAT_OUT := $(WINDOWS_DIR)/install.bat
WINDOWS_INSTALL_PS1_OUT := $(WINDOWS_DIR)/install.ps1
WINDOWS_UNINSTALL_BAT_OUT := $(WINDOWS_DIR)/uninstall.bat
WINDOWS_UNINSTALL_PS1_OUT := $(WINDOWS_DIR)/uninstall.ps1

WINDOWS_DEPLOYMENT_FILES := \
	$(WINDOWS_START_OUT) \
	$(WINDOWS_INSTALL_BAT_OUT) \
	$(WINDOWS_INSTALL_PS1_OUT) \
	$(WINDOWS_UNINSTALL_BAT_OUT) \
	$(WINDOWS_UNINSTALL_PS1_OUT)

# =========================
# MACOS SERVICE CONFIGS
# =========================
MACOS_INSTALL_DIR ?= /usr/local/libexec/$(APP)
MACOS_CONFIG_DIR ?= /Library/Application Support/$(APP)
MACOS_CONFIG_PATH ?= $(MACOS_CONFIG_DIR)/config.yaml
MACOS_LOG_DIR ?= /Library/Logs/$(APP)
MACOS_SERVICE_USER ?= _fbs-gateway
MACOS_SERVICE_GROUP ?= $(MACOS_SERVICE_USER)
MACOS_LAUNCHD_LABEL ?= com.williamveith.$(APP)

MACOS_INSTALL_TEMPLATE := $(SERVICE_DIR_MACOS)/install-macos.sh.in
MACOS_START_TEMPLATE := $(SERVICE_DIR_MACOS)/start.sh.in
MACOS_UNINSTALL_TEMPLATE := $(SERVICE_DIR_MACOS)/uninstall-macos.sh.in
MACOS_PLIST_TEMPLATE := $(SERVICE_DIR_MACOS)/com.williamveith.fbs-interlock-gateway.plist.in

MACOS_ARM64_INSTALL_OUT := $(MAC_ARM64_DIR)/install.sh
MACOS_ARM64_START_OUT := $(MAC_ARM64_DIR)/start.sh
MACOS_ARM64_UNINSTALL_OUT := $(MAC_ARM64_DIR)/uninstall.sh
MACOS_ARM64_PLIST_OUT := $(MAC_ARM64_DIR)/$(MACOS_LAUNCHD_LABEL).plist

MACOS_AMD64_INSTALL_OUT := $(MAC_AMD64_DIR)/install.sh
MACOS_AMD64_START_OUT := $(MAC_AMD64_DIR)/start.sh
MACOS_AMD64_UNINSTALL_OUT := $(MAC_AMD64_DIR)/uninstall.sh
MACOS_AMD64_PLIST_OUT := $(MAC_AMD64_DIR)/$(MACOS_LAUNCHD_LABEL).plist

MACOS_ARM64_DEPLOYMENT_FILES := \
	$(MACOS_ARM64_INSTALL_OUT) \
	$(MACOS_ARM64_START_OUT) \
	$(MACOS_ARM64_UNINSTALL_OUT) \
	$(MACOS_ARM64_PLIST_OUT)

MACOS_AMD64_DEPLOYMENT_FILES := \
	$(MACOS_AMD64_INSTALL_OUT) \
	$(MACOS_AMD64_START_OUT) \
	$(MACOS_AMD64_UNINSTALL_OUT) \
	$(MACOS_AMD64_PLIST_OUT)

RELEASE_DIR := $(BUILD_DIR)/release
LINUX_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-linux-amd64
LINUX_ARM64_ASSET := $(RELEASE_DIR)/$(APP)-linux-arm64
WINDOWS_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-windows-amd64.exe
DARWIN_ARM64_ASSET := $(RELEASE_DIR)/$(APP)-darwin-arm64
DARWIN_AMD64_ASSET := $(RELEASE_DIR)/$(APP)-darwin-amd64

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
	build-darwin-arm64 \
	build-darwin-amd64 \
	build-linux-arm64 \
	build-linux-amd64 \
	build-windows-amd64 \
	windows-deployment-files \
	macos-arm64-deployment-files \
	macos-amd64-deployment-files \
	macos-deployment-files \
	release-linux-amd64 \
	release-linux-arm64 \
	release-windows-amd64 \
	release-darwin-arm64 \
	release-darwin-amd64 \
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

build: build-darwin-arm64

build-mac: build-darwin-arm64

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

$(WINDOWS_START_OUT): $(WINDOWS_START_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		"$(WINDOWS_START_TEMPLATE)" > "$@"

$(WINDOWS_INSTALL_BAT_OUT): $(WINDOWS_INSTALL_BAT_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		"$(WINDOWS_INSTALL_BAT_TEMPLATE)" > "$@"

$(WINDOWS_INSTALL_PS1_OUT): $(WINDOWS_INSTALL_PS1_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		"$(WINDOWS_INSTALL_PS1_TEMPLATE)" > "$@"

$(WINDOWS_UNINSTALL_BAT_OUT): $(WINDOWS_UNINSTALL_BAT_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		"$(WINDOWS_UNINSTALL_BAT_TEMPLATE)" > "$@"

$(WINDOWS_UNINSTALL_PS1_OUT): $(WINDOWS_UNINSTALL_PS1_TEMPLATE) Makefile
	mkdir -p "$(WINDOWS_DIR)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@WINDOWS_INSTALL_DIR@|$(WINDOWS_INSTALL_DIR)|g' \
		"$(WINDOWS_UNINSTALL_PS1_TEMPLATE)" > "$@"

windows-deployment-files: $(WINDOWS_DEPLOYMENT_FILES)


$(MACOS_ARM64_INSTALL_OUT) $(MACOS_AMD64_INSTALL_OUT): $(MACOS_INSTALL_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_CONFIG_DIR@|$(MACOS_CONFIG_DIR)|g' \
		-e 's|@MACOS_CONFIG_PATH@|$(MACOS_CONFIG_PATH)|g' \
		-e 's|@MACOS_LOG_DIR@|$(MACOS_LOG_DIR)|g' \
		-e 's|@MACOS_SERVICE_USER@|$(MACOS_SERVICE_USER)|g' \
		-e 's|@MACOS_SERVICE_GROUP@|$(MACOS_SERVICE_GROUP)|g' \
		-e 's|@MACOS_LAUNCHD_LABEL@|$(MACOS_LAUNCHD_LABEL)|g' \
		"$(MACOS_INSTALL_TEMPLATE)" > "$@"
	chmod +x "$@"

$(MACOS_ARM64_START_OUT) $(MACOS_AMD64_START_OUT): $(MACOS_START_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_CONFIG_PATH@|$(MACOS_CONFIG_PATH)|g' \
		"$(MACOS_START_TEMPLATE)" > "$@"
	chmod +x "$@"

$(MACOS_ARM64_UNINSTALL_OUT) $(MACOS_AMD64_UNINSTALL_OUT): $(MACOS_UNINSTALL_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@APP@|$(APP)|g' \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_CONFIG_PATH@|$(MACOS_CONFIG_PATH)|g' \
		-e 's|@MACOS_LOG_DIR@|$(MACOS_LOG_DIR)|g' \
		-e 's|@MACOS_LAUNCHD_LABEL@|$(MACOS_LAUNCHD_LABEL)|g' \
		"$(MACOS_UNINSTALL_TEMPLATE)" > "$@"
	chmod +x "$@"

$(MACOS_ARM64_PLIST_OUT) $(MACOS_AMD64_PLIST_OUT): $(MACOS_PLIST_TEMPLATE) Makefile
	mkdir -p "$(@D)"
	sed \
		-e 's|@MACOS_INSTALL_DIR@|$(MACOS_INSTALL_DIR)|g' \
		-e 's|@MACOS_LOG_DIR@|$(MACOS_LOG_DIR)|g' \
		-e 's|@MACOS_SERVICE_USER@|$(MACOS_SERVICE_USER)|g' \
		-e 's|@MACOS_SERVICE_GROUP@|$(MACOS_SERVICE_GROUP)|g' \
		-e 's|@MACOS_LAUNCHD_LABEL@|$(MACOS_LAUNCHD_LABEL)|g' \
		"$(MACOS_PLIST_TEMPLATE)" > "$@"

macos-arm64-deployment-files: $(MACOS_ARM64_DEPLOYMENT_FILES)

macos-amd64-deployment-files: $(MACOS_AMD64_DEPLOYMENT_FILES)

macos-deployment-files: \
	macos-arm64-deployment-files \
	macos-amd64-deployment-files

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

build-darwin-arm64: fmt $(MACOS_ARM64_DEPLOYMENT_FILES)
	mkdir -p "$(MAC_ARM64_DIR)"
	cp "$(CONFIGS)" "$(MAC_ARM64_DIR)/"
	cp "$(DEPLOYMENT_GUIDES_DIR)/$(MACOS_INSTALL_GUIDE)" "$(MAC_ARM64_DIR)/"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(MAC_ARM64_DIR)/$(APP)" \
		$(CMD)

build-darwin-amd64: fmt $(MACOS_AMD64_DEPLOYMENT_FILES)
	mkdir -p "$(MAC_AMD64_DIR)"
	cp "$(CONFIGS)" "$(MAC_AMD64_DIR)/"
	cp "$(DEPLOYMENT_GUIDES_DIR)/$(MACOS_INSTALL_GUIDE)" "$(MAC_AMD64_DIR)/"
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(MAC_AMD64_DIR)/$(APP)" \
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

build-windows-amd64: fmt $(WINDOWS_DEPLOYMENT_FILES)
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

release-darwin-arm64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(DARWIN_ARM64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-darwin-arm64" > \
		"$(APP)-darwin-arm64.sha256"

release-darwin-amd64:
	mkdir -p "$(RELEASE_DIR)"
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(DARWIN_AMD64_ASSET)" \
		$(CMD)
	cd "$(RELEASE_DIR)" && \
		$(SHA256SUM) "$(APP)-darwin-amd64" > \
		"$(APP)-darwin-amd64.sha256"

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
	bash -n \
		"$(INSTALL_TEMPLATE)" \
		"$(UPDATE_TEMPLATE)" \
		"$(MACOS_INSTALL_TEMPLATE)" \
		"$(MACOS_START_TEMPLATE)" \
		"$(MACOS_UNINSTALL_TEMPLATE)"
	@if command -v plutil >/dev/null 2>&1; then \
		plutil -lint "$(MACOS_PLIST_TEMPLATE)"; \
	elif command -v python3 >/dev/null 2>&1; then \
		python3 -c 'import plistlib; plistlib.load(open("$(MACOS_PLIST_TEMPLATE)", "rb"))'; \
	else \
		echo "Skipping plist validation: plutil and python3 unavailable."; \
	fi

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
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-darwin-arm64" \
		$(CMD)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(BUILD_DIR)/ci/$(APP)-darwin-amd64" \
		$(CMD)

verify: fmt-check tidy-check vet test-race scripts-check build-check

release: \
	verify \
	release-linux-amd64 \
	release-linux-arm64 \
	release-windows-amd64 \
	release-darwin-arm64 \
	release-darwin-amd64

# =========================
# UTILITIES
# =========================

shelly-auth:
	@chmod +x scripts/set-shelly-auth.sh
	@./scripts/set-shelly-auth.sh

clean:
	rm -rf "$(BUILD_DIR)"
	go clean