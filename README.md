# FBS Interlock Gateway

`fbs-interlock-gateway` is a Go service that lets the FBS interlock system control networked tool interlocks through a local gateway server.

The gateway receives FBS HTTP interlock requests, maps each request to a configured tool port, communicates with the assigned Shelly relay/control box over Shelly HTTP RPC, and returns the simple JSON state response expected by FBS.

This project is designed for internal facility deployment where FBS communicates only with the gateway, and the gateway communicates only with the configured Shelly devices.

## Current release focus

Current production capabilities include:

- one gateway listener port per configured tool
- FBS-compatible `/status`, `/on`, and `/off` endpoints
- Shelly Gen2/Gen3 HTTP RPC control
- Shelly HTTP Digest Authentication support
- interactive Shelly authentication setup script
- local-only embedded admin UI
- editable local configuration file
- systemd deployment support for Debian/Linux
- Linux firewall model that allows only the official FBS source IP to reach gateway tool ports

## Production security model

The deployed security model is layered:

```text
Official FBS source IP
  -> allowed by Linux firewall
  -> gateway tool port
  -> gateway authenticates to Shelly using Digest Auth
  -> Shelly relay/control box changes output state
  -> tool monitor / enable / interlock circuit changes state
```

The gateway host denies inbound traffic by default. The only inbound gateway traffic allowed for tool control is from the official FBS interlock source IP.

Shelly devices use HTTP Digest Authentication. Direct Shelly RPC control requires valid Shelly credentials. The gateway stores those credentials locally in `config.yaml` and uses them only when communicating with the configured Shelly device.

The admin UI binds to `127.0.0.1:18090` by default. This keeps configuration editing local to the gateway host unless an operator intentionally uses an SSH tunnel.

## System architecture

```text
+------------+        HTTP         +-----------------------+       HTTP RPC + Digest Auth       +---------------------+
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

The deployed Linux machine does not need a separate `web/` directory at runtime.

The admin UI provides:

- live status table for configured tools
- Shelly connection/output status
- editable config table
- add-tool support
- config save support
- automatic restart request after saving
- styled interface with a fixed footer

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

Keep the admin UI bound to `127.0.0.1` and tunnel into it from a workstation:

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

The service loads `config.yaml` from the path provided with the `-config` flag.

Without a `-config` flag, the gateway looks for `config.yaml` next to the executable.

In the systemd deployment, the service starts with:

```text
-config /opt/fbs-interlock-gateway/config.yaml
```

`config.yaml` contains local deployment data and is not tracked by Git.

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

## Building

Format the code:

```bash
make fmt
```

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

Build release binaries and checksums:

```bash
make release
```

Build individual release assets:

```bash
make release-linux-amd64
make release-linux-arm64
make release-windows-amd64
```

Clean build outputs:

```bash
make clean
```

## Build output

Release targets place release binaries and SHA-256 checksum files under:

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

Linux build targets place the binary, local config copy, generated install script, generated update script, and generated systemd files under:

```text
build/linux/
```

Expected Linux build output:

```text
build/linux/
  fbs-interlock-gateway
  config.yaml
  fbs-interlock-gateway.service
  install.sh
  update.sh
  fbs-interlock-gateway-update.service
  fbs-interlock-gateway-update.timer
```

## Service and update templates

Systemd service and update files are generated from templates in `services/`.

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
build/windows/start-windows.bat
```

Generated files are build artifacts. Edit templates or Makefile variables instead.

Current app name:

```make
APP := fbs-interlock-gateway
```

Default install directory:

```text
/opt/fbs-interlock-gateway
```

Default service user:

```text
fbs-gateway
```

## Local testing

Create a local config:

```bash
make init-config
```

Run locally:

```bash
go run . -config config.yaml
```

Run locally with an explicit admin UI address:

```bash
go run . -config config.yaml -admin 127.0.0.1:18090
```

Open the admin UI:

```text
http://127.0.0.1:18090
```

Test the admin API:

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

The admin UI can request a restart after saving config. On Linux, systemd restarts the service. On macOS, `go run` exits.

To simulate production restart behavior locally:

```bash
while true; do
  go run . -config ./config.yaml
  echo "gateway exited; restarting in 2 seconds..."
  sleep 2
done
```

## Deployment on Debian/Linux

Build the Linux target for the deployment machine:

```bash
make build-linux-amd64
```

or:

```bash
make build-linux-arm64
```

Copy the generated Linux build folder to the deployment machine, then run the generated installer:

```bash
cd build/linux
./install.sh
```

Manual install path:

```bash
sudo mkdir -p /opt/fbs-interlock-gateway
sudo cp build/linux/fbs-interlock-gateway /opt/fbs-interlock-gateway/fbs-interlock-gateway
sudo cp config.yaml /opt/fbs-interlock-gateway/config.yaml
sudo chmod +x /opt/fbs-interlock-gateway/fbs-interlock-gateway
sudo cp build/linux/fbs-interlock-gateway.service /etc/systemd/system/fbs-interlock-gateway.service
sudo systemctl daemon-reload
sudo systemctl enable fbs-interlock-gateway.service
sudo systemctl restart fbs-interlock-gateway.service
```

Check status:

```bash
systemctl status fbs-interlock-gateway.service
journalctl -u fbs-interlock-gateway.service -f
```

## Runtime behavior

On startup, the gateway:

1. Loads `config.yaml` from the path provided with `-config`.
2. Applies default values for omitted optional fields.
3. Starts the local admin UI unless disabled.
4. Starts one HTTP server per enabled tool.
5. Maps each gateway port to one configured Shelly/network interlock.
6. Logs inbound FBS requests.
7. Logs outbound FBS responses.
8. Authenticates to Shelly devices when credentials are configured.
9. Reports the configured safe state back to FBS when an interlock cannot be reached.
10. Shuts down cleanly on interrupt, systemd stop, or admin restart request.

Disabled tools are skipped during listener startup.

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
- Each interlock gets its own gateway listener port.
- FBS communicates only with the gateway.
- The gateway communicates with each Shelly/network interlock using HTTP RPC.
- Authenticated Shelly devices use HTTP Digest Authentication.
- The gateway firewall allows only `[FBS IP Address]` to reach TCP ports `8081:8981`.
- Real deployment mappings live in local `config.yaml`.
- The web UI is embedded into the binary.
- The admin UI defaults to `127.0.0.1:18090`.
- The systemd service and update files are generated from templates.
- The generated service name and executable path are based on the Makefile `APP` value.
