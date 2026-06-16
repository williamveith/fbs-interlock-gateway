APP=fbs-interlock-gateway
PI_USER=wev222
PI_HOST=fbs-interlock-gateway.local
PI_DIR=/opt/fbs-interlock-ping raspberrypi.localgateway

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
		-o "$(MAC_DIR)/$(APP)-darwin-arm64" .

build-pi: fmt
	mkdir -p "$(LINUX_DIR)"
	cp "$(CONFIGS)" "$(LINUX_DIR)/"
	if [ -d "$(SERVICE_DIR)" ]; then cp -R "$(SERVICE_DIR)" "$(LINUX_DIR)/"; fi
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o "$(LINUX_DIR)/$(APP)-linux-arm64" .

deploy: build-pi
	scp "$(LINUX_DIR)/$(APP)-linux-arm64" $(PI_USER)@$(PI_HOST):/home/$(PI_USER)/$(APP)
	scp "$(LINUX_DIR)/config.yaml" $(PI_USER)@$(PI_HOST):/home/$(PI_USER)/config.yaml
	ssh $(PI_USER)@$(PI_HOST) '\
		sudo mkdir -p $(PI_DIR) && \
		sudo mv /home/$(PI_USER)/$(APP) $(PI_DIR)/$(APP) && \
		sudo mv /home/$(PI_USER)/config.yaml $(PI_DIR)/config.yaml && \
		sudo chown -R $(PI_USER):$(PI_USER) $(PI_DIR) && \
		chmod +x $(PI_DIR)/$(APP) && \
		sudo systemctl restart fbs-interlock-gateway.service \
	'

logs:
	ssh $(PI_USER)@$(PI_HOST) 'journalctl -u fbs-interlock-gateway.service -f'

status:
	ssh $(PI_USER)@$(PI_HOST) 'systemctl status fbs-interlock-gateway.service'

clean:
	rm -rf "$(BUILD_DIR)"
	go clean