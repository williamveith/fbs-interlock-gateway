# FBS Interlock Gateway

`fbs-interlock-gateway` is a Go service that lets the FBS interlock system control networked tool interlocks through a local gateway server.

The gateway receives FBS HTTP interlock requests, maps each request to a configured tool listener, communicates with the assigned Shelly relay or network interlock over HTTP RPC, and returns the simple JSON state response expected by FBS.

The project is intended for controlled facility deployments where FBS communicates with the gateway and the gateway communicates with configured interlock devices.

## Table of Contents

- [Capabilities](#capabilities)
- [Safety and security model](#safety-and-security-model)
  - [Platform firewall behavior](#platform-firewall-behavior)
- [System architecture](#system-architecture)
- [Repository layout](#repository-layout)
- [FBS-facing behavior](#fbs-facing-behavior)
  - [Endpoints](#endpoints)
- [Gateway-to-Shelly communication](#gateway-to-shelly-communication)
- [Shelly authentication helper](#shelly-authentication-helper)
- [Admin UI](#admin-ui)
  - [Admin server protections](#admin-server-protections)
  - [Admin address flag](#admin-address-flag)
  - [Remote access with an SSH tunnel](#remote-access-with-an-ssh-tunnel)
- [Admin API](#admin-api)
  - [GET /api/config](#get-apiconfig)
  - [PUT /api/config](#put-apiconfig)
  - [GET /api/status](#get-apistatus)
  - [POST /api/restart](#post-apirestart)
- [Configuration](#configuration)
  - [Config fields](#config-fields)
  - [Validation](#validation)
- [Development and validation](#development-and-validation)
- [Building deployment packages](#building-deployment-packages)
  - [Linux](#linux)
  - [Windows AMD64](#windows-amd64)
  - [macOS Apple Silicon](#macos-apple-silicon)
  - [macOS Intel](#macos-intel)
  - [Generate template-derived files only](#generate-template-derived-files-only)
- [Deployment build output](#deployment-build-output)
  - [Linux](#linux-1)
  - [Windows](#windows)
  - [macOS ARM64](#macos-arm64)
  - [macOS AMD64](#macos-amd64)
- [Release binaries](#release-binaries)
- [Service templates and installed layouts](#service-templates-and-installed-layouts)
  - [Linux templates](#linux-templates)
  - [Windows templates](#windows-templates)
  - [macOS templates](#macos-templates)
- [Automatic Linux updates](#automatic-linux-updates)
- [Continuous integration](#continuous-integration)
- [Release workflow](#release-workflow)
- [Branch and pull-request workflow](#branch-and-pull-request-workflow)
- [Local testing](#local-testing)
- [Runtime behavior](#runtime-behavior)
  - [Port ownership warning](#port-ownership-warning)
  - [Configuration reload behavior](#configuration-reload-behavior)
- [Logging](#logging)
- [Repository safety](#repository-safety)

## Capabilities

- one FBS-facing listener port per configured tool
- FBS-compatible `/status`, `/on`, and `/off` endpoints
- supported query-based on/off command formats
- Shelly Gen2/Gen3 HTTP RPC control
- optional Shelly HTTP Digest Authentication
- an interactive Shelly authentication setup script
- an embedded Admin UI bound to localhost by default
- concurrent live-status checks with bounded worker concurrency
- visible disconnected and error states for unreachable devices
- password masking and preservation in the Admin API
- validated, atomic configuration writes with `.bak` backups
- Linux AMD64 and ARM64 deployment packages
- Windows AMD64 deployment packages
- macOS ARM64 and AMD64 deployment packages
- Linux systemd supervision and automatic update support
- Windows Task Scheduler supervision and restart handling
- macOS LaunchDaemon supervision
- cross-platform build validation in GitHub Actions
- GPG-signed release tags
- SHA-256 checksums for release binaries
- a unified `make verify` validation gate

## Safety and security model

The gateway controls access signals, but it is not a substitute for hardware safety controls.

```text
FBS server
  -> host or network firewall
  -> gateway tool listener
  -> optional Shelly Digest Authentication
  -> Shelly relay or network interlock
  -> tool monitor / enable / interlock circuit
```

Important operational rules:

- Hardware interlocks and fail-safe circuitry remain authoritative.
- `defaults.safe_state_on_error` controls the state reported by software when a device cannot be reached. It does not prove the physical relay state.
- Production gateway ports should be reachable only from the authorized FBS source.
- Shelly credentials are stored locally in `config.yaml`.
- The Admin UI should remain bound to a loopback address unless remote access is intentionally secured.
- Real credentials and production mappings must not be committed to the repository.

### Platform firewall behavior

The deployment mechanisms differ by operating system:

| Platform | Installer behavior |
| --- | --- |
| Linux | Installs UFW when needed, sets default incoming traffic to deny, allows outgoing traffic, permits the configured FBS source IP to the configured gateway port range, and enables UFW. |
| Windows | Adds an inbound Windows Firewall rule for the installed gateway executable. Apply additional network controls when source-IP restriction is required. |
| macOS | Adds the executable to the macOS Application Firewall allow list. The Application Firewall works by application and is not equivalent to a source-IP and port-range rule. Use a network firewall, a reviewed `pf` rule, or an application-level allowlist when source restriction is required. |

The Linux firewall values are generated from these Makefile variables:

```make
FBS_SOURCE_IP = <authorized-fbs-source>
FBS_PORT_RANGE = 8081:8981
```

Review them before building a production deployment.

## System architecture

```text
+------------+        HTTP         +-----------------------+  HTTP RPC + optional Digest Auth   +---------------------+
|            |  /status /on /off   |                       |   Switch.GetStatus / Switch.Set    |                     |
| FBS Server | <-----------------> | fbs-interlock-gateway | <--------------------------------> | Shelly Relay Box    |
|            |                     |                       |                                    | / Network Interlock |
+------------+                     +-----------------------+                                    +---------------------+
                                                                                                           |
                                                                                                           v
                                                                                                 Tool monitor / enable
                                                                                                 circuit changes state

   +------------------------+
   |  Local-only Admin UI   |
   | http://127.0.0.1:18090 |
   +------------------------+
                |
                v
configuration editor / status view / restart request
```

## Repository layout

```text
cmd/fbs-interlock-gateway/
  main.go
    application entry point, CLI flags, version output, signal handling

internal/admin/
  server.go
  server_test.go
  web/
    embedded Admin UI and local Admin API

internal/config/
  YAML loading, defaults, validation, atomic writes, and backups

internal/fbs/
  FBS-compatible HTTP request handling and responses

internal/gateway/
  application lifecycle, listener startup, restart, and shared state

internal/process/
  listener-port process utilities

internal/shelly/
  Shelly RPC client and Digest Authentication

services/linux/
  systemd, installer, and updater templates

services/windows/
  installer, startup, and uninstaller templates

services/macos/
  installer, startup, uninstaller, and LaunchDaemon templates

deployment guides/
  platform-specific installation instructions

.github/workflows/
  CI and guarded release automation
```

## FBS-facing behavior

The gateway exposes one HTTP listener per enabled tool. Each configured listener port represents one interlock target.

Example request path:

```text
FBS
  -> http://<gateway-host>:<tool-port>/on
  -> fbs-interlock-gateway
  -> http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=true
  -> Shelly output changes state
```

FBS only needs the gateway hostname and the assigned gateway port. Shelly hostnames, switch IDs, authentication credentials, and enable states remain in the local gateway configuration.

### Endpoints

```text
http://<gateway-host>:<port>/status
http://<gateway-host>:<port>/on
http://<gateway-host>:<port>/off
```

The gateway also accepts common query-based command formats:

```text
?turn=on
?turn=off
?state=1
?state=0
?value=1
?value=0
```

Responses are intentionally simple for FBS compatibility:

```json
{"Success":1,"State":1}
```

```json
{"Success":1,"State":0}
```

`State: 1` means the reported interlock output is on.

`State: 0` means the reported interlock output is off.

## Gateway-to-Shelly communication

Status requests use:

```text
http://<shelly-host>/rpc/Switch.GetStatus?id=<switch-id>
```

Command requests use:

```text
http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=true
http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=false
```

When a username and password are configured, the gateway responds to the Shelly authentication challenge and retries with an HTTP Digest Authorization header.

When credentials are blank or `null`, the gateway uses unauthenticated RPC.

## Shelly authentication helper

The repository includes:

```text
scripts/set-shelly-auth.sh
```

Run it directly:

```bash
chmod +x scripts/set-shelly-auth.sh
./scripts/set-shelly-auth.sh
```

or through Make:

```bash
make shelly-auth
```

The helper:

1. reads device information from `Shelly.GetDeviceInfo`
2. obtains the Shelly authentication realm
3. computes the `ha1` value for the `admin` account
4. calls `Shelly.SetAuth`
5. verifies authenticated access with `Switch.GetStatus`

## Admin UI

The Admin UI is embedded in the Go executable with Go's `embed` package.

Default address:

```text
http://127.0.0.1:18090
```

No separate runtime `web/` directory is required.

The interface provides:

- a live status table for configured tools
- connected, disconnected, output, and error states
- bounded concurrent status checks
- paused polling when the page is not visible
- prevention of overlapping requests
- editable configuration fields
- automatic selection of the next available listener port
- duplicate-name, duplicate-port, and field validation
- add and delete tool controls
- password-set indicators without returning stored passwords
- explicit password replacement or clearing
- notifications for loading, validation, save, and restart results
- safe text rendering for names, addresses, and error messages
- cache-disabled API requests
- an automatic restart request after a successful save

Disabled tools remain visible in configuration and status data but are not contacted.

### Admin server protections

The Admin server includes:

- explicit HTTP method handling
- strict single-object JSON decoding
- rejection of unknown JSON fields
- a request body size limit
- read, write, idle, header, and shutdown timeouts
- bounded concurrent device-status requests
- no-store headers for API responses
- same-origin checks for state-changing requests
- rejection of cross-site state-changing requests
- `Content-Security-Policy`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- restrictive referrer and permissions policies

Keep the Admin UI on loopback whenever possible.

### Admin address flag

Set an explicit address:

```bash
./fbs-interlock-gateway \
  -config config.yaml \
  -admin 127.0.0.1:18090
```

Disable the Admin UI:

```bash
./fbs-interlock-gateway \
  -config config.yaml \
  -admin ""
```

### Remote access with an SSH tunnel

```bash
ssh -L 18090:127.0.0.1:18090 fbs-gateway@<gateway-host>
```

Then open:

```text
http://127.0.0.1:18090
```

## Admin API

```text
GET  /api/config
PUT  /api/config
GET  /api/status
POST /api/restart
```

### `GET /api/config`

Returns the loaded configuration without returning stored passwords.

A tool with a stored password reports:

```json
{
  "password_set": true
}
```

### `PUT /api/config`

Accepts edited configuration, validates it, preserves existing passwords unless replacement or clearing is explicitly requested, writes the file atomically, and creates `config.yaml.bak` from the previous file when possible.

A successful response indicates that a restart is required.

### `GET /api/status`

Returns live status information for all configured tools:

```json
[
  {
    "interlock_name": "EQU-EXAMPLE-TOOL-01",
    "ip": "interlock-01.example.local",
    "port": 8081,
    "switch_id": 0,
    "enabled": true,
    "connected": true,
    "output": false
  }
]
```

When an enabled interlock cannot be reached, `connected` is `false`, the configured safe output is reported, and the error is included.

### `POST /api/restart`

Requests a clean process restart. The platform service supervisor starts the process again in an installed deployment.

## Configuration

The service loads YAML from the path supplied with `-config`.

When `-config` is omitted, the gateway looks for `config.yaml` beside the executable.

Create a starter config:

```bash
make init-config
```

The target preserves an existing local `config.yaml`.

Starter structure:

```yaml
bind: 0.0.0.0

defaults:
  timeout_ms: 800
  safe_state_on_error: "off"

tools:
  - interlock_name:
    ip:
    port:
    switch_id:
    username:
    password:
    enabled:
```

Example:

```yaml
bind: "0.0.0.0"

defaults:
  timeout_ms: 800
  safe_state_on_error: "off"

tools:
  - interlock_name: "EQU-EXAMPLE-TOOL-01"
    ip: "interlock-01.example.local"
    port: 8081
    switch_id: 0
    username: "admin"
    password: "example-password"
    enabled: true

  - interlock_name: "EQU-EXAMPLE-TOOL-02"
    ip: "interlock-02.example.local"
    port: 8082
    switch_id: 0
    username: null
    password: null
    enabled: true
```

### Config fields

| Field | Purpose |
| --- | --- |
| `bind` | Address used by FBS-facing listeners. Use `0.0.0.0` to listen on all interfaces. |
| `defaults.timeout_ms` | HTTP timeout for interlock requests. |
| `defaults.safe_state_on_error` | State reported when an interlock cannot be reached. Usually `off`. |
| `tools[].interlock_name` | Tool or interlock name used in logs and the Admin UI. |
| `tools[].ip` | Shelly or network-interlock hostname/IP. |
| `tools[].port` | FBS-facing gateway listener port. |
| `tools[].switch_id` | Shelly relay ID. This is commonly `0` for a single-output device. |
| `tools[].username` | Optional Shelly RPC username. |
| `tools[].password` | Optional Shelly RPC password used for Digest Authentication. |
| `tools[].enabled` | Whether the gateway starts the listener and contacts the device. |

### Validation

Validation includes:

- required interlock names
- required device addresses
- listener ports within `8081` through `8981`
- duplicate listener ports
- valid switch IDs
- valid defaults

Invalid configuration is not written.

## Development and validation

Use the Go version declared in `go.mod`.

Create a local configuration before deployment builds:

```bash
make init-config
```

Common targets:

```bash
make fmt
make test
make test-race
make vet
make verify
```

`make verify` runs:

1. `gofmt` verification without modifying source files
2. `go.mod` and `go.sum` consistency checks
3. `go vet`
4. all Go tests under the race detector
5. Linux and macOS shell-template syntax checks
6. macOS property-list validation when `plutil` or Python's `plistlib` is available
7. Linux AMD64 build validation
8. Linux ARM64 build validation
9. Windows AMD64 build validation
10. macOS ARM64 build validation
11. macOS AMD64 build validation

Individual validation targets:

```bash
make fmt-check
make tidy-check
make vet
make test-race
make scripts-check
make build-check
```

## Building deployment packages

### Linux

```bash
make build-linux-amd64
make build-linux-arm64
```

Both commands generate `build/linux/`. Run only the architecture target needed for the deployment machine after `make clean`.

### Windows AMD64

```bash
make build-windows-amd64
```

### macOS Apple Silicon

```bash
make build-darwin-arm64
```

`make build` and `make build-mac` are aliases for the Apple Silicon deployment build.

### macOS Intel

```bash
make build-darwin-amd64
```

### Generate template-derived files only

```bash
make windows-deployment-files
make macos-arm64-deployment-files
make macos-amd64-deployment-files
make macos-deployment-files
```

## Deployment build output

### Linux

```text
build/linux/
├── fbs-interlock-gateway
├── config.yaml
├── fbs-interlock-gateway.service
├── install.sh
├── update.sh
├── fbs-interlock-gateway-update.service
├── fbs-interlock-gateway-update.timer
└── Linux Install Instructions.md
```

### Windows

```text
build/windows/
├── fbs-interlock-gateway.exe
├── config.yaml
├── install.bat
├── install.ps1
├── start.bat
├── uninstall.bat
├── uninstall.ps1
└── Windows Install Instructions.md
```

### macOS ARM64

```text
build/darwin/arm64/
├── fbs-interlock-gateway
├── config.yaml
├── install.sh
├── start.sh
├── uninstall.sh
├── com.williamveith.fbs-interlock-gateway.plist
└── macOS Install Instructions.md
```

### macOS AMD64

```text
build/darwin/amd64/
├── fbs-interlock-gateway
├── config.yaml
├── install.sh
├── start.sh
├── uninstall.sh
├── com.williamveith.fbs-interlock-gateway.plist
└── macOS Install Instructions.md
```

Generated deployment files are build artifacts. Edit their templates or Makefile variables instead of editing generated copies.

## Release binaries

Build all release binaries and checksums through the validation gate:

```bash
make release VERSION=<version>
```

Build individual assets:

```bash
make release-linux-amd64 VERSION=<version>
make release-linux-arm64 VERSION=<version>
make release-windows-amd64 VERSION=<version>
make release-darwin-arm64 VERSION=<version>
make release-darwin-amd64 VERSION=<version>
```

Release files are written to:

```text
build/release/
├── fbs-interlock-gateway-linux-amd64
├── fbs-interlock-gateway-linux-amd64.sha256
├── fbs-interlock-gateway-linux-arm64
├── fbs-interlock-gateway-linux-arm64.sha256
├── fbs-interlock-gateway-windows-amd64.exe
├── fbs-interlock-gateway-windows-amd64.exe.sha256
├── fbs-interlock-gateway-darwin-arm64
├── fbs-interlock-gateway-darwin-arm64.sha256
├── fbs-interlock-gateway-darwin-amd64
└── fbs-interlock-gateway-darwin-amd64.sha256
```

Display embedded metadata:

```bash
./build/release/fbs-interlock-gateway-linux-amd64 -version
```

Output format:

```text
fbs-interlock-gateway version=<version> commit=<commit> date=<UTC-build-time>
```

## Service templates and installed layouts

### Linux templates

```text
services/linux/
├── app.service.in
├── install-linux.sh.in
├── update-linux.sh.in
├── update.service.in
└── update.timer.in
```

Installed layout:

```text
/opt/fbs-interlock-gateway/
├── fbs-interlock-gateway
└── update.sh

/etc/fbs-interlock-gateway/
├── config.yaml
└── config.yaml.bak

/etc/systemd/system/
├── fbs-interlock-gateway.service
├── fbs-interlock-gateway-update.service
└── fbs-interlock-gateway-update.timer
```

The Linux installer:

- verifies or installs `lsof`, `curl`, `ca-certificates`, and `ufw`
- configures and enables UFW
- creates the service user and group when needed
- installs the executable and service files
- preserves an existing production config
- applies restrictive ownership and permissions
- enables and starts the gateway
- enables the update timer when updater files are present

The systemd service:

- starts after the network is online
- runs under the configured service account
- writes logs to journald
- restarts after exits
- waits two seconds between starts
- limits rapid restart attempts
- applies `NoNewPrivileges=true`

### Windows templates

```text
services/windows/
├── install.bat.in
├── install.ps1.in
├── start.bat.in
├── uninstall.bat.in
└── uninstall.ps1.in
```

Installed layout:

```text
C:\FBS\fbs-interlock-gateway\
├── fbs-interlock-gateway.exe
├── config.yaml
├── start.bat
└── logs\
    └── gateway.log
```

The Windows installer:

- elevates through User Account Control
- copies the executable and startup wrapper
- preserves an existing production config
- creates the log directory
- adds a Windows Firewall rule
- registers a Task Scheduler job
- runs the task as `SYSTEM`
- starts the gateway at boot
- starts the gateway immediately
- checks the Admin API

The startup wrapper:

- passes the installed config path explicitly
- restarts the executable after two seconds
- limits rapid restart attempts
- writes process output and restart events to `gateway.log`

The uninstaller removes the task, firewall rule, and installed application files while preserving the production config.

### macOS templates

```text
services/macos/
├── com.williamveith.fbs-interlock-gateway.plist.in
├── install-macos.sh.in
├── start.sh.in
└── uninstall-macos.sh.in
```

Installed layout:

```text
/usr/local/libexec/fbs-interlock-gateway/
├── fbs-interlock-gateway
└── start.sh

/Library/Application Support/fbs-interlock-gateway/
└── config.yaml

/Library/LaunchDaemons/
└── com.williamveith.fbs-interlock-gateway.plist

/Library/Logs/fbs-interlock-gateway/
├── gateway.log
└── gateway-error.log
```

The macOS installer:

- verifies it is running on macOS
- validates the LaunchDaemon property list
- creates a hidden non-login service account when needed
- copies the executable and startup wrapper
- preserves an existing production config
- creates the log files
- installs and starts a system LaunchDaemon
- registers the executable with the Application Firewall
- checks the Admin API

The LaunchDaemon:

- starts before a user signs in
- runs under the dedicated service account
- keeps the gateway running
- throttles rapid restarts
- writes stdout and stderr to separate log files

The uninstaller unloads the LaunchDaemon and removes installed executable files while preserving configuration and logs.

Detailed procedures are in:

```text
deployment guides/Linux Install Instructions.md
deployment guides/Windows Install Instructions.md
deployment guides/macOS Install Instructions.md
```

## Automatic Linux updates

The generated Linux update timer runs after boot and then periodically.

The updater:

1. selects the matching Linux release asset
2. downloads the executable and checksum
3. verifies SHA-256
4. backs up the installed executable
5. installs the new executable
6. restarts the service
7. rolls back when the service fails to start

Inspect or disable the timer:

```bash
sudo systemctl status fbs-interlock-gateway-update.timer
sudo systemctl list-timers fbs-interlock-gateway-update.timer
sudo systemctl disable --now fbs-interlock-gateway-update.timer
```

Run an update manually:

```bash
sudo /opt/fbs-interlock-gateway/update.sh
```

## Continuous integration

The CI workflow runs for pull requests and pushes to `main`.

It:

1. checks out the repository
2. installs the Go version declared in `go.mod`
3. runs `make verify`
4. executes the Linux AMD64 binary with `-version`
5. verifies the generated formats for:
   - Linux AMD64
   - Linux ARM64
   - Windows AMD64
   - macOS ARM64
   - macOS AMD64

The macOS and Windows binaries are cross-compiled and format-checked on the Linux runner. They are not executed by that runner.

## Release workflow

Releases are created through the manually triggered **Validate, Tag, and Release** GitHub Actions workflow.

The workflow:

1. requires the `main` branch
2. checks out complete Git history and tags
3. imports the protected release-signing GPG key
4. validates the requested semantic version
5. rejects an existing tag
6. runs `make verify`
7. builds all supported release binaries
8. verifies asset existence
9. verifies every SHA-256 checksum
10. verifies binary formats and architectures
11. verifies embedded version and commit metadata using the Linux AMD64 binary
12. confirms that the build did not modify tracked files
13. creates and locally verifies a GPG-signed annotated tag
14. pushes the signed tag
15. creates a GitHub release with generated notes and all validated assets

Required release-environment secrets:

```text
GPG_PRIVATE_KEY
GPG_PASSPHRASE
```

Start the workflow with GitHub CLI:

```bash
gh workflow run release.yml \
  --ref main \
  -f version=<version>
```

After it succeeds:

```bash
git fetch origin --tags
git tag -v <version>
gh release view <version>
```

## Branch and pull-request workflow

Start from current `main`:

```bash
git switch main
git pull --ff-only origin main
```

Create a short-lived branch:

```bash
git switch -c feature/<description>
```

Validate before committing:

```bash
make fmt
make verify
git status
git diff
```

Commit and push:

```bash
git add -A
git commit -S -m "Describe the change"
git push --set-upstream origin feature/<description>
```

Open a pull request:

```bash
gh pr create \
  --base main \
  --head feature/<description> \
  --fill
```

After merging:

```bash
git switch main
git pull --ff-only origin main
git fetch --prune
git branch -d feature/<description>
```

## Local testing

Create a local config:

```bash
make init-config
```

Run through Make:

```bash
make run
```

Equivalent command:

```bash
go run ./cmd/fbs-interlock-gateway \
  -config ./config.yaml
```

Run with an explicit Admin address:

```bash
go run ./cmd/fbs-interlock-gateway \
  -config ./config.yaml \
  -admin 127.0.0.1:18090
```

Print build metadata:

```bash
go run ./cmd/fbs-interlock-gateway -version
```

Test the Admin API:

```bash
curl -s "http://127.0.0.1:18090/api/config"
curl -s "http://127.0.0.1:18090/api/status"
```

Test a Shelly directly:

```bash
curl "http://<shelly-host>/rpc/Switch.GetStatus?id=<switch-id>"
curl "http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=true"
curl "http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=false"
```

Test an authenticated Shelly:

```bash
curl --anyauth -u "admin:<password>" \
  "http://<shelly-host>/rpc/Switch.GetStatus?id=<switch-id>"
```

Test through the gateway:

```bash
curl "http://<gateway-host>:<port>/status"
curl "http://<gateway-host>:<port>/on"
curl "http://<gateway-host>:<port>/off"
```

## Runtime behavior

On startup, the gateway:

1. parses `-config`, `-admin`, and `-version`
2. loads configuration from the explicit path or beside the executable
3. applies defaults
4. validates enabled tools
5. starts the Admin server unless disabled
6. starts one FBS-facing HTTP listener per enabled tool
7. maps each gateway port to one configured interlock
8. logs inbound FBS requests and outbound responses
9. authenticates to Shelly devices when credentials are configured
10. reports the configured safe state when a device request fails
11. shuts down cleanly on interrupt, termination, Admin restart request, or server error

Disabled tools do not receive FBS listeners and are not contacted by status polling.

### Port ownership warning

Before starting an enabled listener, the gateway may clear a process already using that configured port. Use dedicated gateway ports and confirm that unrelated services do not use the configured range.

### Configuration reload behavior

A successful Admin UI save writes the updated configuration and requests a process restart. The installed platform supervisor rebuilds runtime listeners and clients by starting the process again.

## Logging

Gateway request logs use:

```text
FBS_IN
FBS_OUT
```

Platform logs:

```text
Linux:
  journalctl -u fbs-interlock-gateway.service -f

Windows:
  C:\FBS\fbs-interlock-gateway\logs\gateway.log

macOS:
  /Library/Logs/fbs-interlock-gateway/gateway.log
  /Library/Logs/fbs-interlock-gateway/gateway-error.log
```

## Repository safety

Ignored local artifacts:

```gitignore
build
config.yaml
config.yaml.bak
```

Committed content includes source code, tests, service templates, setup helpers, workflows, and documentation. Production configuration remains on the deployment machine.
