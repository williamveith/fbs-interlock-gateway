# FBS Interlock Gateway

`fbs-interlock-gateway` is a small Go service that lets an FBS interlock server control networked interlocks through a local gateway.

The gateway sits between FBS and each physical network interlock. FBS talks to the gateway over HTTP. The gateway translates those requests into Shelly-style HTTP RPC calls and returns the simple JSON state response that FBS expects.

Real deployment configuration must stay out of Git. Do not commit `config.yaml` or `config.yaml.bak`.

## System architecture

```text
+------------+        HTTP         +-------------------------+       HTTP RPC        +----------------------+
|            |  /status /on /off   |                         |   Switch.GetStatus    |                      |
| FBS Server | <-----------------> |  fbs-interlock-gateway  | <-------------------> | Network Interlock    |
|            |                     |                         |      Switch.Set       | Relay / Control Box  |
+------------+                     +-------------------------+                       +----------------------+
                                                                                                |
                                                                                                v
                                                                                      Tool enable / monitor
                                                                                      circuit changes state


                     Local-only admin UI
                 http://127.0.0.1:18090
                              |
                              v
                  config.yaml editor / status view
```

## What this service does

The gateway exposes one HTTP listener per configured tool/interlock. Each listener port represents one interlock target.

Example layout using placeholders only:

```text
FBS Server
  -> http://<gateway-host>:<tool-1-port>/status
  -> fbs-interlock-gateway
  -> http://<interlock-host-1>/rpc/Switch.GetStatus?id=<switch-id>

FBS Server
  -> http://<gateway-host>:<tool-2-port>/on
  -> fbs-interlock-gateway
  -> http://<interlock-host-2>/rpc/Switch.Set?id=<switch-id>&on=true
```

The gateway is the only service FBS needs to know about. The individual interlock hostnames/IPs live only in the local `config.yaml` on the deployment machine.

## Communication path

### 1. FBS Server to Gateway

FBS sends HTTP requests to the gateway:

```text
http://<gateway-host>:<port>/status
http://<gateway-host>:<port>/on
http://<gateway-host>:<port>/off
```

The gateway accepts several common request formats for on/off commands, including path-based and query-based values such as:

```text
/on
/off
?turn=on
?turn=off
?state=1
?state=0
?value=1
?value=0
```

The gateway responds to FBS with:

```json
{"Success":1,"State":1}
```

or:

```json
{"Success":1,"State":0}
```

`State: 1` means the interlock output is on.

`State: 0` means the interlock output is off.

### 2. Gateway to Network Interlock

For status requests, the gateway asks the interlock for its current output state:

```text
http://<interlock-host>/rpc/Switch.GetStatus?id=<switch-id>
```

For command requests, the gateway sets the output state:

```text
http://<interlock-host>/rpc/Switch.Set?id=<switch-id>&on=true
http://<interlock-host>/rpc/Switch.Set?id=<switch-id>&on=false
```

### 3. Interlock to Tool

The network interlock changes the state of the configured relay/control output. The physical wiring determines what the relay controls, such as a monitor circuit, enable circuit, or another non-destructive control line.

The software path is:

```text
FBS event
  -> FBS HTTP request
  -> gateway listener for that tool
  -> interlock HTTP RPC command
  -> relay output changes state
  -> tool control circuit changes state
```

## Admin UI

The gateway includes a built-in web admin UI.

By default, it listens on:

```text
http://127.0.0.1:18090
```

The admin UI is embedded into the Go binary using Go's `embed` package. The deployed Linux machine does not need a separate `web/` directory at runtime.

The admin UI currently provides:

- live status table for configured tools
- Shelly connection/output status
- editable config table
- add-tool support
- config save support
- automatic restart request after saving
- styled interface with a fixed footer

The admin UI is intentionally local-only by default. Do not expose it directly to the network unless access is restricted with firewall rules or another access-control layer.

### Admin address flag

The admin UI address can be changed with:

```bash
./fbs-interlock-gateway -config config.yaml -admin 127.0.0.1:18090
```

To disable the admin UI:

```bash
./fbs-interlock-gateway -config config.yaml -admin ""
```

### Remote admin access by SSH tunnel

For a remote Debian/Linux gateway, keep the admin UI bound to `127.0.0.1` and use an SSH tunnel from your workstation:

```bash
ssh -L 18090:127.0.0.1:18090 fbs-gateway@<gateway-host>
```

Then open this on your workstation:

```text
http://127.0.0.1:18090
```

## Admin API

The admin UI uses these local API endpoints:

```text
GET  /api/config
PUT  /api/config
GET  /api/status
POST /api/restart
```

### `GET /api/config`

Returns the currently loaded config as JSON.

### `PUT /api/config`

Accepts the edited config as JSON, validates it, writes it back to `config.yaml`, and creates `config.yaml.bak` from the previous file when possible.

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

If an interlock cannot be reached, `connected` is `false` and the error message is included.

### `POST /api/restart`

Requests a clean gateway restart after a config save.

In production, the gateway exits and systemd is expected to restart it according to the service restart policy. When testing locally with `go run`, the process will exit unless you run it inside a restart loop.

## Configuration

The service loads `config.yaml` from the path provided with the `-config` flag.

If no `-config` flag is provided, the gateway looks for `config.yaml` next to the executable.

In the systemd deployment, the service starts with:

```text
-config /opt/fbs-interlock-gateway/config.yaml
```

`config.yaml` is intentionally not tracked by Git because it contains deployment-specific information such as interlock hostnames, addresses, ports, and future credentials.

### Create a starter config

Create a starter `config.yaml` with:

```bash
make init-config
```

This target does not overwrite an existing `config.yaml`.

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

Fill in real values before running the gateway.

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
    username: null
    password: null
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
| `defaults.safe_state_on_error` | State reported back to FBS if the interlock cannot be reached. Usually `off`. |
| `tools[].interlock_name`       | Human-readable tool/interlock name used in logs and the admin UI.             |
| `tools[].ip`                   | Hostname or IP address of the network interlock. Keep real values out of Git. |
| `tools[].port`                 | Gateway listener port for that FBS tool/interlock.                            |
| `tools[].switch_id`            | Interlock switch/relay ID. For Shelly 1 Mini Gen3 this is usually `0`.        |
| `tools[].username`             | Reserved for future authenticated interlocks.                                 |
| `tools[].password`             | Reserved for future authenticated interlocks. Do not commit real passwords.   |
| `tools[].enabled`              | Whether this gateway listener should start.                                   |

## Config validation

The admin API validates edited config before saving.

Validation checks include:

- missing `interlock_name`
- missing `ip`
- invalid ports
- duplicate ports
- invalid `switch_id`

If validation fails, the config is not written.

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

Build release binaries and checksums:

```bash
make release
```

Build only the Linux AMD64 release asset:

```bash
make release-linux-amd64
```

Build only the Linux ARM64 release asset:

```bash
make release-linux-arm64
```

Clean build outputs:

```bash
make clean
```

Use the exact targets available in the included `Makefile`.

## Build output

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
```

Generated files:

```text
build/linux/fbs-interlock-gateway.service
build/linux/install.sh
build/linux/update.sh
build/linux/fbs-interlock-gateway-update.service
build/linux/fbs-interlock-gateway-update.timer
```

Do not manually edit generated files. Edit the templates or Makefile variables instead.

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

Create a local config if needed:

```bash
make init-config
```

Run locally:

```bash
go run . -config config.yaml
```

Or specify the admin UI address explicitly:

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

Test the interlock directly first:

```bash
curl "http://<interlock-host>/rpc/Switch.GetStatus?id=<switch-id>"
curl "http://<interlock-host>/rpc/Switch.Set?id=<switch-id>&on=true"
curl "http://<interlock-host>/rpc/Switch.Set?id=<switch-id>&on=false"
```

Then test through the gateway:

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

The admin UI can request a restart after saving config. On Linux, systemd should restart the service. On macOS, `go run` will simply exit.

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

If installing manually, create the install directory:

```bash
sudo mkdir -p /opt/fbs-interlock-gateway
```

Copy the binary and local config:

```bash
sudo cp build/linux/fbs-interlock-gateway /opt/fbs-interlock-gateway/fbs-interlock-gateway
sudo cp config.yaml /opt/fbs-interlock-gateway/config.yaml
sudo chmod +x /opt/fbs-interlock-gateway/fbs-interlock-gateway
```

Install the generated systemd service:

```bash
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
2. Applies default values when fields are omitted.
3. Starts the local admin UI unless disabled.
4. Starts one HTTP server per enabled tool.
5. Maps each gateway port to one configured interlock.
6. Logs inbound FBS requests.
7. Logs outbound FBS responses.
8. Shuts down cleanly on interrupt, systemd stop, or admin restart request.

If a tool is disabled, the gateway skips that listener.

If an interlock cannot be reached, the gateway logs the error and reports the configured safe state back to FBS.

## Logging

Incoming FBS requests are logged with:

```text
FBS_IN
```

Outgoing FBS responses are logged with:

```text
FBS_OUT
```

When running under systemd:

```bash
journalctl -u fbs-interlock-gateway.service -f
```

## Git safety

These files must stay ignored:

```gitignore
build
config.yaml
config.yaml.bak
```

Do not commit:

- real hostnames
- IP addresses
- usernames
- passwords
- internal network names
- deployment-specific interlock mappings
- generated config backups

Only commit safe source files, examples, templates, and documentation.

## Security notes

The current implementation uses Shelly-style HTTP RPC. Keep the gateway and interlocks on the intended internal network.

The admin UI can edit `config.yaml` and request a restart. Keep it bound to `127.0.0.1` unless you have a specific reason to expose it and have applied firewall or access-control protections.

FBS-facing tool ports should be restricted so only the FBS server can reach them.

## Notes

- One gateway process can manage multiple interlocks.
- Each interlock gets its own gateway listener port.
- FBS communicates only with the gateway.
- The gateway communicates with each interlock using HTTP RPC.
- Real deployment mappings live only in local `config.yaml`.
- The web UI is embedded into the binary.
- The admin UI defaults to `127.0.0.1:18090`.
- The systemd service and update files are generated from templates.
- The generated service name and executable path are based on the Makefile `APP` value.
