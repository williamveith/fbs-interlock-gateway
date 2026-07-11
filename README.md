# FBS Interlock Gateway

`fbs-interlock-gateway` is a Go service that lets the FBS interlock system control networked tool interlocks through a local gateway server.

The gateway receives FBS HTTP interlock requests, maps each request to a configured tool port, communicates with the assigned Shelly relay/control box over Shelly HTTP RPC, and returns the simple JSON state response expected by FBS.

This project is designed for internal facility deployment where FBS communicates only with the gateway, and the gateway communicates only with the configured Shelly devices.

## Current release: v2.5.0

v2.5.0 is a reliability, validation, and release-automation release. It keeps the established FBS-to-Shelly control path while adding automated tests, race-detector validation, cross-platform build checks, guarded GitHub releases, and substantial Admin UI reliability improvements.

Current capabilities include:

- one gateway listener port per configured tool
- FBS-compatible `/status`, `/on`, and `/off` endpoints
- supported query-based on/off command formats
- Shelly Gen2/Gen3 HTTP RPC control
- optional Shelly HTTP Digest Authentication
- interactive Shelly authentication setup script
- embedded admin UI bound to localhost by default
- concurrent live-status checks across configured Shelly devices
- visible disconnected/error states for unreachable devices
- atomic configuration writes with `.bak` backups
- Debian/Linux systemd packaging
- generated Linux installer and update service/timer
- Linux AMD64 and ARM64 builds
- Windows AMD64 packaging
- macOS ARM64 development builds
- GitHub Actions CI for pull requests and pushes to `main`
- a guarded tag-and-release workflow
- a unified `make verify` validation gate

Existing v2.4.0 configuration files remain compatible with v2.5.0. No configuration migration is required.

## Production security model

The deployed security model is layered:

```text
Official FBS source IP
  -> allowed by Linux firewall
  -> gateway tool port
  -> gateway optionally authenticates to Shelly using Digest Auth
  -> Shelly relay/control box changes output state
  -> tool monitor / enable / interlock circuit changes state
```

The production deployment should deny inbound traffic by default. The only inbound gateway traffic allowed for tool control should come from the official FBS interlock source IP.

The gateway binary and generated installer do **not** configure UFW automatically. The firewall rules in this README are deployment-policy examples and must be applied and verified by the operator.

Shelly devices can use HTTP Digest Authentication. When authentication is enabled on a Shelly, direct RPC control requires valid Shelly credentials. The gateway stores those credentials locally in `config.yaml` and uses them only when communicating with the configured Shelly device.

The admin UI binds to `127.0.0.1:18090` by default. This keeps configuration editing local to the gateway host unless an operator intentionally uses an SSH tunnel.

## System architecture

```text
+------------+        HTTP         +-----------------------+       HTTP RPC + optional Digest Auth       +---------------------+
|            |  /status /on /off   |                       |   Switch.GetStatus / Switch.Set    |                     |
| FBS Server | <-----------------> | fbs-interlock-gateway | <--------------------------------> | Shelly Relay Box    |
|            |                     |                       |                                    | / Network Interlock |
+------------+                     +-----------------------+                                    +---------------------+
                                                                                                           |
                                                                                                           v
                                                                                                 Tool monitor / enable
                                                                                                 circuit changes state

   +------------------------+
   |  Local-only admin UI   |
   | http://127.0.0.1:18090 |
   +------------------------+
                |
                v
config.yaml editor / status view / restart request
```

## Repository layout

v2.5.0 uses a modular Go layout:

```text
cmd/fbs-interlock-gateway/
  main.go                 application entry point and CLI flags

internal/admin/
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

.github/workflows/
  CI and guarded release automation
```

## What the gateway does

The gateway exposes one HTTP listener per configured tool/interlock. Each listener port represents one interlock target.

Example path:

```text
FBS
  -> http://<gateway-host>:<tool-port>/on
  -> fbs-interlock-gateway
  -> http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=true
  -> Shelly relay output changes state
```

FBS only needs the gateway hostname and the assigned gateway port for each tool. Shelly hostnames, switch IDs, and authentication credentials live in the local gateway configuration.

## FBS-facing endpoints

FBS sends HTTP requests to the gateway:

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

Gateway responses are intentionally simple for FBS compatibility:

```json
{"Success":1,"State":1}
```

```json
{"Success":1,"State":0}
```

`State: 1` means the interlock output is on.

`State: 0` means the interlock output is off.

> **Important:** `defaults.safe_state_on_error` controls the state reported to FBS and shown by the Admin API when the Shelly cannot be reached. It does not prove the physical state of an unreachable relay. Hardware interlocks and fail-safe circuitry must remain the authoritative safety mechanism.

## Gateway-to-Shelly communication

For status requests, the gateway asks the Shelly for its current output state:

```text
http://<shelly-host>/rpc/Switch.GetStatus?id=<switch-id>
```

For command requests, the gateway sets the Shelly output state:

```text
http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=true
http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=false
```

When `username` and `password` are configured for a tool, the gateway uses Shelly HTTP Digest Authentication. The first request receives the Shelly authentication challenge, then the gateway retries with the correct Digest Authorization header.

When `username` and `password` are blank or `null`, the gateway uses unauthenticated Shelly RPC for devices that have not been configured with Shelly authentication.

## Shelly authentication

Shelly Gen2/Gen3 devices support local HTTP Digest Authentication. This project supports authenticated Shelly RPC while keeping the FBS-facing gateway response format unchanged.

Authenticated tool config example:

```yaml
bind: "0.0.0.0"

defaults:
  timeout_ms: 800
  safe_state_on_error: "off"

tools:
  - interlock_name: "EQU-EXAMPLE-TOOL-01"
    ip: "shelly-device.example.local"
    port: 8081
    switch_id: 0
    username: "admin"
    password: "local-device-password"
    enabled: true
```

Unauthenticated tool config example:

```yaml
tools:
  - interlock_name: "EQU-EXAMPLE-TOOL-02"
    ip: "shelly-device-2.example.local"
    port: 8082
    switch_id: 0
    username: null
    password: null
    enabled: true
```

### Shelly auth setup script

The repository includes an interactive helper script:

```text
scripts/set-shelly-auth.sh
```

The script prompts for:

- Shelly host/IP
- new Shelly auth password
- current Shelly auth password, blank for devices with auth disabled or blank auth

Run it with:

```bash
chmod +x scripts/set-shelly-auth.sh
./scripts/set-shelly-auth.sh
```

or through the Makefile target when present:

```bash
make shelly-auth
```

The script performs this sequence:

1. Reads Shelly device info from `/rpc/Shelly.GetDeviceInfo`.
2. Extracts the Shelly realm/device ID.
3. Computes the Shelly `ha1` value for the `admin` user.
4. Calls `Shelly.SetAuth`.
5. Verifies authenticated access with `Switch.GetStatus`.

## Gateway firewall

The Linux gateway uses UFW to restrict inbound traffic.

Production rule model:

```text
Default incoming traffic: denied
Default outgoing traffic: allowed
Allowed inbound FBS traffic: [FBS IP Address] -> TCP ports 8081:8981
```

The FBS source IP is:

```text
[FBS IP Address]
```

The deployed gateway port range is:

```text
8081:8981/tcp
```

The wide port range is intentional. It reserves a large gateway tool-port block for future tool deployments without requiring repeated firewall changes.

Production UFW command set:

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow from [FBS IP Address] to any port 8081:8981 proto tcp
sudo ufw enable
sudo ufw status verbose
```

Expected UFW status pattern:

```text
Status: active
Default: deny (incoming), allow (outgoing), disabled (routed)

To              Action      From
--              ------      ----
8081:8981/tcp   ALLOW IN    [FBS IP Address]
```

This firewall rule means users on the network cannot call gateway URLs such as:

```text
http://<gateway-host>:8082/on
http://<gateway-host>:8082/off
```

Only the official FBS interlock source IP can reach the gateway tool ports.

## Admin UI

The gateway includes a built-in web admin UI embedded into the Go binary with Go's `embed` package.

Default admin UI address:

```text
http://127.0.0.1:18090
```

The deployed machine does not need a separate `web/` directory at runtime.

The Admin UI provides:

- a live status table for all configured tools
- Shelly connection and output status
- disconnected/error reporting for unreachable Shelly devices
- concurrent status checks, so a large device list is not checked sequentially
- prevention of overlapping polling requests
- visible errors when config or status requests fail
- editable configuration fields
- add-tool support
- validated config saves
- atomic config replacement and `config.yaml.bak` creation
- an automatic restart request after a successful save
- HTML escaping for tool names, hostnames, and error text
- cache-disabled requests for live API data

Disabled tools remain visible in configuration/status data but are not contacted.

### Admin address flag

Set the admin UI address explicitly:

```bash
./fbs-interlock-gateway -config config.yaml -admin 127.0.0.1:18090
```

Disable the admin UI:

```bash
./fbs-interlock-gateway -config config.yaml -admin ""
```

### Remote admin access by SSH tunnel

Keep the Admin UI bound to `127.0.0.1` and tunnel into it from a workstation:

```bash
ssh -L 18090:127.0.0.1:18090 fbs-gateway@<gateway-host>
```

Then open this on the workstation:

```text
http://127.0.0.1:18090
```

## Admin API

The admin UI uses local API endpoints:

```text
GET  /api/config
PUT  /api/config
GET  /api/status
POST /api/restart
```

### `GET /api/config`

Returns the currently loaded config as JSON.

### `PUT /api/config`

Accepts edited config as JSON, validates it, writes it back to `config.yaml`, and creates `config.yaml.bak` from the previous file when possible.

The save operation writes through a temporary file before renaming it into place.

### `GET /api/status`

Returns live status information for configured tools:

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

When an interlock cannot be reached, `connected` is `false` and the error message is included.

### `POST /api/restart`

Requests a clean gateway restart after a config save.

In production, the gateway exits and systemd restarts it according to the service restart policy. During local testing with `go run`, the process exits.

## Configuration

The service loads YAML configuration from the path provided with the `-config` flag.

When `-config` is omitted, the gateway looks for `config.yaml` in the same directory as the executable.

The generated v2.5.0 systemd service uses:

```text
-config /etc/fbs-interlock-gateway/config.yaml
```

The production executable is installed separately at:

```text
/opt/fbs-interlock-gateway/fbs-interlock-gateway
```

The generated installer creates `/etc/fbs-interlock-gateway`, installs the config with mode `0640`, and assigns it to the `fbs-gateway` service account. An existing production config is preserved during reinstall.

`config.yaml`, `config.yaml.bak`, and the `build/` directory are excluded from Git.

### Create a starter config

Create a starter `config.yaml` with:

```bash
make init-config
```

This target leaves an existing `config.yaml` unchanged.

Generated starter config:

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

Safe example:

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

## Config fields

| Field                          | Purpose                                                                       |
| ------------------------------ | ----------------------------------------------------------------------------- |
| `bind`                         | Address the FBS-facing gateway listeners bind to. Use `0.0.0.0` to listen on all interfaces. |
| `defaults.timeout_ms`          | HTTP timeout for interlock requests.                                          |
| `defaults.safe_state_on_error` | State reported back to FBS when the interlock cannot be reached. Usually `off`. |
| `tools[].interlock_name`       | Human-readable tool/interlock name used in logs and the admin UI.             |
| `tools[].ip`                   | Hostname or IP address of the Shelly/network interlock.                       |
| `tools[].port`                 | Gateway listener port for that FBS tool/interlock.                            |
| `tools[].switch_id`            | Shelly switch/relay ID. For Shelly 1 Mini Gen3 this is usually `0`.           |
| `tools[].username`             | Shelly RPC username. For Shelly Gen2/Gen3 this is usually `admin`.            |
| `tools[].password`             | Shelly RPC password used by the gateway for Digest Auth.                      |
| `tools[].enabled`              | Whether this gateway listener starts.                                         |

## Config validation

The admin API validates edited config before saving.

Validation checks include:

- missing `interlock_name`
- missing `ip`
- invalid ports
- duplicate ports
- invalid `switch_id`

When validation fails, the config is not written.

## Development and validation

v2.5.0 requires Go 1.22.

Create a local configuration before running deployment build targets because those targets copy `config.yaml` into the build output:

```bash
make init-config
```

Format source files:

```bash
make fmt
```

Run the normal test suite:

```bash
make test
```

Run the race-detector test suite:

```bash
make test-race
```

Run the complete v2.5.0 validation gate:

```bash
make verify
```

`make verify` performs:

1. `gofmt` verification without modifying source files
2. `go.mod` and `go.sum` consistency validation
3. `go vet`
4. all Go tests under the race detector
5. shell-script syntax validation
6. Linux AMD64 build validation
7. Linux ARM64 build validation
8. Windows AMD64 build validation

The individual validation targets are:

```bash
make fmt-check
make tidy-check
make vet
make test-race
make scripts-check
make build-check
```

## Building

Build for macOS Apple Silicon:

```bash
make build-mac
```

Build for Linux ARM64:

```bash
make build-linux-arm64
```

Build for Linux AMD64:

```bash
make build-linux-amd64
```

Build for Windows AMD64:

```bash
make build-windows-amd64
```

Build all release binaries and checksums through the full validation gate:

```bash
make release VERSION=v2.5.0
```

Build individual release assets without the aggregate release target:

```bash
make release-linux-amd64 VERSION=v2.5.0
make release-linux-arm64 VERSION=v2.5.0
make release-windows-amd64 VERSION=v2.5.0
```

Clean build outputs:

```bash
make clean
```

Display embedded build metadata:

```bash
./build/darwin/fbs-interlock-gateway -version
```

A release binary prints output in this form:

```text
fbs-interlock-gateway version=v2.5.0 commit=<commit> date=<UTC-build-time>
```

## Build output

### macOS ARM64 development build

```text
build/darwin/
  fbs-interlock-gateway
  config.yaml
```

### Linux deployment build

```text
build/linux/
  fbs-interlock-gateway
  config.yaml
  Linux Install Instructions.md
  fbs-interlock-gateway.service
  install.sh
  update.sh
  fbs-interlock-gateway-update.service
  fbs-interlock-gateway-update.timer
```

### Windows AMD64 deployment build

```text
build/windows/
  fbs-interlock-gateway.exe
  config.yaml
  Windows Install Instructions.md
  start.bat
```

### Release assets

Release targets place binaries and SHA-256 checksum files under:

```text
build/release/
```

Expected release output:

```text
build/release/
  fbs-interlock-gateway-linux-amd64
  fbs-interlock-gateway-linux-amd64.sha256
  fbs-interlock-gateway-linux-arm64
  fbs-interlock-gateway-linux-arm64.sha256
  fbs-interlock-gateway-windows-amd64.exe
  fbs-interlock-gateway-windows-amd64.exe.sha256
```

## Service and update templates

Deployment files are generated from templates in `services/`.

Templates:

```text
services/app.service.in
services/install-linux.sh.in
services/update-linux.sh.in
services/update.service.in
services/update.timer.in
services/start-windows.bat.in
```

Generated Linux files:

```text
build/linux/fbs-interlock-gateway.service
build/linux/install.sh
build/linux/update.sh
build/linux/fbs-interlock-gateway-update.service
build/linux/fbs-interlock-gateway-update.timer
```

Generated Windows helper:

```text
build/windows/start.bat
```

Generated files are build artifacts. Edit the templates or Makefile variables rather than editing generated copies.

Current defaults:

| Setting | Value |
| --- | --- |
| Application name | `fbs-interlock-gateway` |
| Command package | `./cmd/fbs-interlock-gateway` |
| Linux binary directory | `/opt/fbs-interlock-gateway` |
| Linux config directory | `/etc/fbs-interlock-gateway` |
| Linux config path | `/etc/fbs-interlock-gateway/config.yaml` |
| Linux service user/group | `fbs-gateway` |
| Windows install directory used by `start.bat` | `C:\FBS\fbs-interlock-gateway` |

## Continuous integration and releases

The v2.5.0 CI workflow runs for pull requests and pushes to `main`. It:

1. installs the Go version declared in `go.mod`
2. runs `make verify`
3. smoke-tests the Linux AMD64 binary with `-version`
4. inspects the generated Linux and Windows binary formats

The manually triggered release workflow:

1. requires the `main` branch
2. validates the requested version
3. rejects an existing tag
4. runs `make verify`
5. builds all release assets
6. verifies asset existence and SHA-256 checksums
7. verifies binary formats and architectures
8. verifies embedded version and commit metadata
9. confirms that the build did not modify tracked files
10. creates an annotated tag and GitHub release only after validation succeeds

## Local testing

Create a local config:

```bash
make init-config
```

Run through the Makefile:

```bash
make run
```

Equivalent explicit command:

```bash
go run ./cmd/fbs-interlock-gateway -config ./config.yaml
```

Run with an explicit Admin UI address:

```bash
go run ./cmd/fbs-interlock-gateway \
  -config ./config.yaml \
  -admin 127.0.0.1:18090
```

Print version metadata and exit:

```bash
go run ./cmd/fbs-interlock-gateway -version
```

Open the Admin UI:

```text
http://127.0.0.1:18090
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

Test an authenticated Shelly directly:

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

Expected gateway response:

```json
{"Success":1,"State":1}
```

or:

```json
{"Success":1,"State":0}
```

### Local restart testing on macOS

The Admin UI can request a restart after saving config. Under systemd, the service restart policy starts the process again. During `go run`, the process exits.

To simulate production restart behavior locally:

```bash
while true; do
  go run ./cmd/fbs-interlock-gateway -config ./config.yaml
  echo "gateway exited; restarting in 2 seconds..."
  sleep 2
done
```

## Deployment on Debian/Linux

Build for the deployment architecture:

```bash
make build-linux-amd64
```

or:

```bash
make build-linux-arm64
```

Copy the complete generated `build/linux/` directory to the deployment machine and run:

```bash
cd build/linux
sudo ./install.sh
```

The installer can also request administrator privileges through `pkexec` when available.

### Installed layout

```text
/opt/fbs-interlock-gateway/
  fbs-interlock-gateway
  update.sh

/etc/fbs-interlock-gateway/
  config.yaml
  config.yaml.bak        created after a successful Admin UI save when possible

/etc/systemd/system/
  fbs-interlock-gateway.service
  fbs-interlock-gateway-update.service
  fbs-interlock-gateway-update.timer
```

The installer:

- creates the `fbs-gateway` system user and group when needed
- installs the binary under `/opt/fbs-interlock-gateway`
- creates `/etc/fbs-interlock-gateway`
- preserves an existing production config
- applies restrictive config ownership and permissions
- installs and enables the main systemd service
- installs and enables the update timer when its generated files are present
- restarts the gateway service

Check the service:

```bash
sudo systemctl status fbs-interlock-gateway.service --no-pager --full
sudo systemctl is-active fbs-interlock-gateway.service
sudo journalctl -u fbs-interlock-gateway.service -f
```

### Automatic update behavior

The generated update timer runs five minutes after boot and then hourly. It downloads the latest matching Linux release asset and checksum, verifies SHA-256, backs up the installed binary, installs the new binary, and rolls back when the service fails to start.

Inspect the timer:

```bash
sudo systemctl status fbs-interlock-gateway-update.timer
sudo systemctl list-timers fbs-interlock-gateway-update.timer
```

Disable automatic updates when releases must be approved manually:

```bash
sudo systemctl disable --now fbs-interlock-gateway-update.timer
```

Run an update manually:

```bash
sudo /opt/fbs-interlock-gateway/update.sh
```

## Deployment on Windows AMD64

Build the Windows package:

```bash
make build-windows-amd64
```

Copy the contents of `build/windows/` to:

```text
C:\FBS\fbs-interlock-gateway
```

Edit the copied `config.yaml`, then launch:

```text
start.bat
```

The generated batch file starts `fbs-interlock-gateway.exe` minimized. Because no `-config` argument is supplied, the executable loads `config.yaml` from the same directory.

## Runtime behavior

On startup, the gateway:

1. parses `-config`, `-admin`, and `-version`
2. loads configuration from the explicit path or beside the executable
3. applies default values for omitted optional fields
4. validates enabled tools
5. starts the local Admin UI unless disabled
6. starts one HTTP server per enabled tool
7. maps each gateway port to one configured Shelly/network interlock
8. logs inbound FBS requests and outbound responses
9. authenticates to Shelly devices when credentials are configured
10. reports the configured safe state when a Shelly request fails
11. shuts down cleanly on interrupt, SIGTERM, Admin UI restart request, or server error

Disabled tools are skipped during FBS listener startup. The Admin status API still includes them but does not contact them.

### Port ownership warning

Before starting each enabled tool listener, v2.5.0 attempts to clear any process already listening on that configured port. Use dedicated gateway ports and verify that no unrelated service is assigned to them.

### Configuration reload behavior

A successful Admin UI save updates the stored configuration atomically and returns `restart_required: true`. The UI then requests a process restart so that listener ports and runtime clients are rebuilt from the new configuration.

## Logging

Incoming FBS requests are logged with:

```text
FBS_IN
```

Outgoing FBS responses are logged with:

```text
FBS_OUT
```

Systemd logs:

```bash
journalctl -u fbs-interlock-gateway.service -f
```

## Repository safety

Ignored local deployment files:

```gitignore
build
config.yaml
config.yaml.bak
```

Committed repository content includes source code, templates, setup helpers, and documentation. Local deployment configuration remains on the deployment machine.

## Operational notes

- One gateway process can manage multiple interlocks.
- Each enabled interlock gets its own gateway listener port.
- Supported FBS paths are `/status`, `/on`, and `/off`.
- Query-based on/off command formats remain supported.
- FBS communicates only with the gateway.
- The gateway communicates with each Shelly/network interlock using HTTP RPC.
- Authenticated Shelly devices use HTTP Digest Authentication.
- The UFW rule shown in this README is an operator-managed deployment control; it is not installed automatically.
- Real deployment mappings and Shelly credentials live in local `config.yaml`.
- Linux production config lives at `/etc/fbs-interlock-gateway/config.yaml`.
- The web UI is embedded into the binary.
- The Admin UI defaults to `127.0.0.1:18090`.
- Admin status checks run concurrently in v2.5.0.
- Unreachable devices appear as disconnected with an error and the configured reported safe state.
- The generated Linux installer enables the update timer when its files are included.
- `make verify` is the aggregate validation gate used by CI and release builds.
- Release assets support Linux AMD64, Linux ARM64, and Windows AMD64.
- macOS ARM64 is provided as a development build target.
