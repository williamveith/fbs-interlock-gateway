# FBS Interlock Gateway

`fbs-interlock-gateway` is a Go service that lets the FBS interlock system control networked tool interlocks through a local gateway server.

The gateway receives FBS HTTP interlock requests, maps each request to a configured tool listener, communicates with the assigned Shelly relay or network interlock over HTTP or HTTPS RPC, and returns the simple JSON state response expected by FBS. Shelly communication can use HTTP Digest Authentication, mutual TLS, or both.

The project is intended for controlled facility deployments where FBS communicates with the gateway and the gateway communicates with configured interlock devices. A shared in-memory status store lets normal FBS traffic passively update the local Admin UI without creating recurring Shelly status traffic.

## Table of Contents

- [Capabilities](#capabilities)
- [Safety and security model](#safety-and-security-model)
  - [Platform firewall behavior](#platform-firewall-behavior)
- [System architecture](#system-architecture)
- [Repository layout](#repository-layout)
- [FBS-facing behavior](#fbs-facing-behavior)
  - [Endpoints](#endpoints)
- [Gateway-to-Shelly communication](#gateway-to-shelly-communication)
  - [Connection, authentication, and recovery behavior](#connection-authentication-and-recovery-behavior)
- [Shelly authentication helper](#shelly-authentication-helper)
- [Shelly mutual TLS](#shelly-mutual-tls)
  - [Certificate roles](#certificate-roles)
  - [Generate the certificate authorities and gateway identity](#generate-the-certificate-authorities-and-gateway-identity)
  - [Generate a Shelly server certificate](#generate-a-shelly-server-certificate)
- [Admin UI](#admin-ui)
  - [Passive status model](#passive-status-model)
  - [Admin server protections](#admin-server-protections)
  - [Admin address flag](#admin-address-flag)
  - [Remote access with an SSH tunnel](#remote-access-with-an-ssh-tunnel)
- [Admin API](#admin-api)
  - [GET /api/config](#get-apiconfig)
  - [PUT /api/config](#put-apiconfig)
  - [GET /api/status](#get-apistatus)
  - [GET /api/status?refresh=1](#get-apistatusrefresh1)
  - [POST /api/restart](#post-apirestart)
- [Configuration](#configuration)
  - [Config fields](#config-fields)
  - [Validation](#validation)
- [Development and validation](#development-and-validation)
- [Building deployment packages](#building-deployment-packages)
  - [Prepare Linux runtime TLS files](#prepare-linux-runtime-tls-files)
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
- Shelly Gen2/Gen3 RPC over per-tool `http` or `https`
- optional Shelly HTTP Digest Authentication with reusable digest sessions
- optional mutual TLS with Shelly server verification and gateway client authentication
- certificate-generation helpers for the private CAs, gateway identity, and per-device Shelly identities
- bounded connection reuse and per-device request serialization
- transient status retry and controlled recovery for persistent Shelly HTTP `423` and `429` responses
- request-phase diagnostics for DNS, TCP, TLS, response-header, and response-body failures
- an embedded Admin UI bound to localhost by default
- a shared status store passively updated by normal FBS traffic
- cache-only Admin UI polling that does not contact Shelly devices
- explicit on-demand fleet refresh with bounded worker concurrency
- visible disconnected, safe-output, protocol, and error states
- editable per-tool protocol and gateway mutual-TLS file paths
- password masking and preservation in the Admin API
- validated, atomic configuration writes with `.bak` backups
- Linux AMD64 and ARM64 deployment packages with runtime TLS files
- separate production and development Linux installers
- Windows AMD64 deployment packages
- macOS ARM64 and AMD64 deployment packages
- Linux systemd supervision and checksum-aware automatic update support
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
  -> Shelly RPC over HTTP or HTTPS
       -> optional mutual TLS
       -> optional Digest Authentication
  -> Shelly relay or network interlock
  -> tool monitor / enable / interlock circuit
```

Important operational rules:

- Hardware interlocks and fail-safe circuitry remain authoritative.
- `defaults.safe_state_on_error` controls the state reported by software when a device cannot be reached. It does not prove the physical relay state.
- A successful `Switch.Set` result updates the Admin status cache to the requested state, but only a later `Switch.GetStatus` independently verifies the device output.
- Production gateway ports should be reachable only from the authorized FBS source.
- The Admin UI should remain bound to a loopback address unless remote access is intentionally secured.
- Shelly credentials are stored locally in `config.yaml`.
- Gateway TLS private keys and private CA material must remain outside version control.
- The Linux deployment contains only the gateway runtime trust certificate, client certificate, and client private key. CA private keys, CSRs, and per-device Shelly private keys remain on the certificate-management machine.
- Real credentials, certificates, private keys, and production mappings must not be committed to the repository.

### Platform firewall behavior

The deployment mechanisms differ by operating system:

| Platform | Installer behavior |
| --- | --- |
| Linux | Installs UFW when needed, sets default incoming traffic to deny, allows outgoing traffic, permits the configured FBS source IP to the configured gateway port range, and enables UFW. Both the production and development installers apply this behavior. |
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
+------------+        HTTP         +-----------------------+   HTTP or HTTPS RPC    +---------------------+
|            |  /status /on /off   |                       |  Digest and/or mTLS    |                     |
| FBS Server | <-----------------> | fbs-interlock-gateway | <--------------------> | Shelly Relay Box    |
|            |                     |                       |                        | / Network Interlock |
+------------+                     +-----------------------+                        +---------------------+
                                           |                                                |
                                           | successful or failed result                    v
                                           v                                      Tool monitor / enable
                                  +--------------------+                           circuit changes state
                                  | Shared status store|
                                  +--------------------+
                                           ^
                                           |
                         cache-only polling|  explicit fleet refresh
                                           |
                                  +------------------------+
                                  |  Local-only Admin UI   |
                                  | http://127.0.0.1:18090 |
                                  +------------------------+
```

Normal FBS traffic updates only the affected tool row in the shared status store. The browser reads that in-memory snapshot every three seconds without contacting any Shelly. The **Refresh Status** action explicitly runs a bounded fleet-wide `Switch.GetStatus` scan.

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
  YAML loading, relative-path resolution, defaults, validation,
  atomic writes, and backups

internal/fbs/
  FBS-compatible HTTP request handling, responses, and status recording

internal/gateway/
  application lifecycle, listener startup, restart, and shared dependencies

internal/process/
  listener-port process utilities

internal/shelly/
  HTTP/HTTPS RPC client, Digest Authentication, mutual TLS,
  retry, request timing, and controlled recovery

internal/status/
  revision-protected shared in-memory Admin status store

scripts/tls/
  private-CA, gateway-client, and per-Shelly certificate generation

services/linux/
  systemd, production installer, development installer, and updater templates

services/windows/
  installer, startup, and uninstaller templates

services/macos/
  installer, startup, uninstaller, and LaunchDaemon templates

deployment guides/
  platform-specific installation instructions

.github/workflows/
  CI and guarded release automation
```

The generated `pki/` and `tls/` directories are intentionally ignored by Git.

## FBS-facing behavior

The gateway exposes one HTTP listener per enabled tool. Each configured listener port represents one interlock target.

Example request path for an HTTPS Shelly:

```text
FBS
  -> http://<gateway-host>:<tool-port>/on
  -> fbs-interlock-gateway
  -> https://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=true
  -> Shelly output command succeeds
  -> shared Admin status row is updated
  -> FBS receives the requested state
```

FBS only needs the gateway hostname and assigned gateway port. Shelly hostnames, per-tool protocols, switch IDs, credentials, certificates, and enable states remain in the local gateway configuration.

The FBS server does not impose a fixed three-second response write deadline. The Shelly client's configured end-to-end timeout controls the device operation, allowing valid HTTPS and mutual-TLS requests to complete when `defaults.timeout_ms` is intentionally greater than three seconds.

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

Responses remain intentionally simple for FBS compatibility:

```json
{"Success":1,"State":1}
```

```json
{"Success":1,"State":0}
```

`State: 1` means the reported interlock output is on.

`State: 0` means the reported interlock output is off.

A successful `/status`, `/on`, or `/off` operation updates the corresponding Admin status row. A failed Shelly operation records `connected: false`, the configured safe output, and the error before returning the safe state to FBS.

## Gateway-to-Shelly communication

Each tool selects its RPC scheme with `tools[].protocol`. Supported values are `http` and `https`; an omitted value remains backward-compatible and defaults to `http`.

Status requests use:

```text
<protocol>://<shelly-host>/rpc/Switch.GetStatus?id=<switch-id>
```

Command requests use:

```text
<protocol>://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=true
<protocol>://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=false
```

The `tools[].ip` field contains only the hostname or address. Do not include `http://`, `https://`, a path, or a port in that field.

For HTTPS tools, the gateway:

- verifies the Shelly server certificate against `defaults.shelly_tls.server_ca_file`
- validates the certificate hostname against the configured Shelly hostname
- presents `defaults.shelly_tls.client_cert_file` and `client_key_file` during the mutual-TLS handshake
- requires TLS 1.2 or newer
- reuses idle HTTP connections and caches TLS sessions where supported

When a username and password are configured, the gateway also uses HTTP Digest Authentication. Mutual TLS and Digest Authentication can be enabled together.

When credentials are blank or `null`, the gateway performs the RPC without a Digest Authorization header.

### Connection, authentication, and recovery behavior

Each Shelly request attempt is bounded by `defaults.timeout_ms`. DNS, TCP connect, TLS handshake, and response-header phases are bounded within that attempt. `Switch.GetStatus` may perform one additional transient-network retry after a short delay.

Additional behavior:

- requests to the same Shelly protocol/host are serialized so a previous response is fully consumed before the next RPC begins
- Digest challenges are retained in a per-device session, allowing subsequent requests to send a preemptive Authorization header with an increasing nonce count
- digest sessions expire after 55 minutes or 30,000 nonce uses and are replaced when the device returns a new challenge
- a transient status-network failure is retried once after 150 milliseconds
- certificate validation errors are not retried
- HTTP `429 Too Many Requests` waits two seconds and retries once
- a persistent HTTP `429` or an HTTP `423 Locked` schedules one controlled `Shelly.Reboot` request
- reboot recovery uses a 500-millisecond device reboot delay, suppresses duplicate in-flight attempts, and enforces a five-minute per-device cooldown
- error responses are bounded to 4 KiB
- network failures identify the observed phase such as DNS lookup, TCP connect, TLS handshake, response headers, or response body, and report whether the connection was reused or the TLS session resumed

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

The runtime client caches the resulting Digest challenge state per device, reducing repeated unauthenticated challenge round trips.

## Shelly mutual TLS

The project uses two private certificate authorities so server and client trust are separated.

### Certificate roles

| File | Role | Installed or uploaded to |
| --- | --- | --- |
| `server-ca.crt` | Trust anchor used by the gateway to verify Shelly HTTPS server certificates. | Gateway runtime |
| `server-ca.key` | Signs per-device Shelly server certificates. | Certificate-management machine only |
| `client-ca.crt` | Trust anchor used by each Shelly to verify the gateway client certificate. | Shelly devices |
| `client-ca.key` | Signs the gateway client certificate. | Certificate-management machine only |
| `gateway-client.crt` | Gateway mutual-TLS client identity. | Gateway runtime |
| `gateway-client.key` | Gateway client private key. | Gateway runtime |
| `shelly-server.crt` | Unique HTTPS server identity for one Shelly hostname. | Matching Shelly device |
| `shelly-server.key` | Private key for that Shelly server certificate. | Matching Shelly device |

### Generate the certificate authorities and gateway identity

Generate both private CAs:

```bash
make ca
```

This creates:

```text
pki/ca/
├── server-ca.crt
├── server-ca.key
├── client-ca.crt
└── client-ca.key
```

The CA script refuses to overwrite existing CA material. By default, CA certificates are valid for 3,650 days; set `CA_VALID_DAYS` to override that duration.

Generate the gateway client identity:

```bash
make gateway-cert
```

This creates the full gateway material under `pki/gateway/` and stages only the runtime files under. Gateway and Shelly leaf certificates default to 825 days; set `CERT_VALID_DAYS` to override that duration:

```text
tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

Upload `pki/ca/client-ca.crt` to every Shelly as the CA authorized to verify the gateway client certificate.

### Generate a Shelly server certificate

Run:

```bash
make shelly-cert
```

The helper prompts for:

- the interlock name, used as the output-directory name
- the Shelly UT Austin Dynamic DNS hostname, such as `2c41389b0d77.dynamic.utexas.edu`

The hostname is normalized to lowercase and must not include a URL scheme, path, or port. The generated certificate contains the DDNS hostname as both its common name and DNS subject alternative name; no IP address is embedded.

Output is written to:

```text
pki/shellys/<interlock-name>/
├── shelly-server.cnf
├── shelly-server.csr
├── shelly-server.crt
└── shelly-server.key
```

Install the generated `shelly-server.crt` and `shelly-server.key` on the matching device. Keep all CA private keys and generated device private keys securely outside the repository.

## Admin UI

The Admin UI is embedded in the Go executable with Go's `embed` package.

Default address:

```text
http://127.0.0.1:18090
```

No separate runtime `web/` directory is required.

The interface provides:

- a status table for all configured tools
- connected, disconnected, output, error, and protocol states
- one explicit fleet refresh when the page initially loads
- a **Refresh Status** button for on-demand fleet verification
- cache-only status polling every three seconds
- passive row updates from normal FBS `/status`, `/on`, and `/off` traffic
- paused cache polling while the page is hidden
- an immediate cache read when the page becomes visible again
- prevention of overlapping browser requests and duplicate fleet refreshes
- bounded manual status checks with at most four concurrent Shelly requests
- editable configuration fields, including per-tool `http` or `https`
- editable mutual-TLS server CA, client certificate, and client-key paths
- automatic selection of the next available listener port
- duplicate-name, duplicate-address, duplicate-port, protocol, TLS-path, and field validation
- add and delete tool controls
- password-set indicators without returning stored passwords
- explicit password replacement or clearing
- notifications for loading, validation, save, refresh, and restart results
- safe text rendering for names, addresses, and error messages
- cache-disabled API requests
- an automatic restart request after a successful save

Disabled tools remain visible in configuration and status data but are not contacted.

### Passive status model

The Admin UI is not a second device-control loop.

```text
FBS request
  -> Shelly operation
  -> gateway records the result for that tool
  -> browser reads the in-memory snapshot
```

An ordinary browser status poll does not contact a Shelly. This preserves a near-live operational view without restoring the previous fleet-wide status scan every three seconds.

The first page load and the **Refresh Status** button use an explicit fleet refresh. During that refresh, the browser checks the in-memory snapshot once per second until the server reports completion. Those checks do not start additional device requests.

The shared store uses monotonically increasing revisions. A slow manual refresh that started earlier cannot overwrite a newer FBS result that completed later.

A successful `/on` or `/off` result represents the state accepted by `Switch.Set`. Use **Refresh Status** when an independent `Switch.GetStatus` verification is required.

### Admin server protections

The Admin server includes:

- explicit HTTP method handling
- strict single-object JSON decoding
- rejection of unknown JSON fields
- a request body size limit
- read, write, idle, header, and shutdown timeouts
- a two-minute explicit-refresh timeout
- at most four concurrent device-status requests during a fleet refresh
- one active fleet refresh at a time
- revision-protected merging of refresh results into the shared store
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
GET  /api/status?refresh=1
POST /api/restart
```

### `GET /api/config`

Returns the loaded configuration without returning stored passwords. The response includes normalized per-tool protocols and the gateway `defaults.shelly_tls` paths.

A tool with a stored password reports:

```json
{
  "password_set": true
}
```

An omitted tool protocol is returned as `http`.

### `PUT /api/config`

Accepts edited configuration, validates it, preserves existing passwords unless replacement or clearing is explicitly requested, writes the file atomically, and creates `config.yaml.bak` from the previous file when possible.

The request can update:

- per-tool `http` or `https` protocol
- gateway Shelly-server CA path
- gateway client-certificate path
- gateway client-key path

A successful response indicates that a restart is required.

### `GET /api/status`

Returns the current shared in-memory snapshot and does not contact Shelly devices:

```json
[
  {
    "interlock_name": "EQU-EXAMPLE-TOOL-01",
    "ip": "interlock-01.example.local",
    "protocol": "https",
    "port": 8081,
    "switch_id": 0,
    "enabled": true,
    "connected": true,
    "output": false
  }
]
```

The response includes:

```text
X-Status-Refresh-In-Progress: false
```

Enabled tools begin with the configured safe output and `status not yet refreshed` until an FBS operation or explicit fleet refresh records a result. Disabled tools remain visible without a placeholder error.

When an interlock operation fails, `connected` is `false`, the configured safe output is reported, and the error is included.

### `GET /api/status?refresh=1`

Starts one asynchronous fleet-wide `Switch.GetStatus` refresh and immediately returns the current snapshot.

While the refresh is active, responses include:

```text
X-Status-Refresh-In-Progress: true
```

Additional explicit refresh requests share the in-progress scan rather than creating duplicate Shelly traffic. The browser performs cache-only follow-up reads until the header changes to `false`.

Refresh results are merged by tool and revision. A result from an older scan cannot overwrite a newer FBS-derived status.

### `POST /api/restart`

Requests a clean process restart. The platform service supervisor starts the process again in an installed deployment.

## Configuration

The service loads YAML from the path supplied with `-config`.

When `-config` is omitted, the gateway looks for `config.yaml` beside the executable.

Relative mutual-TLS paths are resolved against the directory containing the loaded configuration file. For example, when the installed config is `/etc/fbs-interlock-gateway/config.yaml`, `./tls/server-ca.crt` resolves to `/etc/fbs-interlock-gateway/tls/server-ca.crt`.

Create a starter config:

```bash
make init-config
```

The target preserves an existing local `config.yaml`. The generated starter uses a 10-second Shelly operation timeout and relative Linux-compatible TLS paths:

```yaml
bind: 0.0.0.0

defaults:
  timeout_ms: 10000
  safe_state_on_error: "off"
  shelly_tls:
    server_ca_file: "./tls/server-ca.crt"
    client_cert_file: "./tls/gateway-client.crt"
    client_key_file: "./tls/gateway-client.key"

tools:
  - interlock_name:
    ip:
    port:
    switch_id:
    username:
    password:
    enabled:
```

Example with HTTP and HTTPS tools:

```yaml
bind: "0.0.0.0"

defaults:
  timeout_ms: 10000
  safe_state_on_error: "off"
  shelly_tls:
    server_ca_file: "./tls/server-ca.crt"
    client_cert_file: "./tls/gateway-client.crt"
    client_key_file: "./tls/gateway-client.key"

tools:
  - interlock_name: "EQU-EXAMPLE-TOOL-01"
    ip: "interlock-01.example.local"
    protocol: "https"
    port: 8081
    switch_id: 0
    username: "admin"
    password: "example-password"
    enabled: true

  - interlock_name: "EQU-EXAMPLE-TOOL-02"
    ip: "interlock-02.example.local"
    protocol: "http"
    port: 8082
    switch_id: 0
    username: null
    password: null
    enabled: true
```

Existing configurations that omit `tools[].protocol` continue to use `http`. Existing configurations that omit `defaults.shelly_tls` continue to run for HTTP-only tools.

When any TLS path is populated, the gateway initializes its TLS client from those files during startup. When an enabled tool uses `https`, all three TLS path fields are required.

### Config fields

| Field | Purpose |
| --- | --- |
| `bind` | Address used by FBS-facing listeners. Use `0.0.0.0` to listen on all interfaces. |
| `defaults.timeout_ms` | Timeout applied to each Shelly request attempt. The starter config uses `10000` milliseconds. |
| `defaults.safe_state_on_error` | State reported when an interlock cannot be reached. Usually `off`. |
| `defaults.shelly_tls.server_ca_file` | CA certificate used to verify HTTPS Shelly server certificates. |
| `defaults.shelly_tls.client_cert_file` | Gateway client certificate presented to HTTPS Shellies. |
| `defaults.shelly_tls.client_key_file` | Private key matching the gateway client certificate. |
| `tools[].interlock_name` | Tool or interlock name used in logs and the Admin UI. |
| `tools[].ip` | Shelly or network-interlock hostname/IP without a URL scheme, path, or port. |
| `tools[].protocol` | Shelly RPC scheme: `http` or `https`. Omitted values default to `http`. |
| `tools[].port` | FBS-facing gateway listener port. |
| `tools[].switch_id` | Shelly relay ID. This is commonly `0` for a single-output device. |
| `tools[].username` | Optional Shelly RPC username. |
| `tools[].password` | Optional Shelly RPC password used for Digest Authentication. |
| `tools[].enabled` | Whether the gateway starts the listener and contacts the device. |

### Validation

Validation includes:

- required, nonblank interlock names
- required device addresses
- rejection of URL schemes in `tools[].ip`
- `http` or `https` protocol normalization and validation
- listener ports within `8081` through `8981`
- duplicate listener ports
- valid nonnegative switch IDs
- required TLS file paths when an applicable tool uses HTTPS
- valid defaults

Invalid configuration is not written. Missing or unreadable configured TLS files also prevent the gateway from starting.

## Development and validation

Use the Go version declared in `go.mod`.

Create a local configuration:

```bash
make init-config
```

Generate local TLS material when the config contains the starter TLS paths:

```bash
make ca
make gateway-cert
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
4. all Go tests under the race detector, including concurrent shared-status and Shelly-client tests
5. shell syntax checks for top-level helpers, TLS helpers, Linux production and development installers, the Linux updater, and macOS templates
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

TLS helper targets:

```bash
make ca
make gateway-cert
make shelly-cert
```

The private `pki/` and staged runtime `tls/` directories are ignored by Git.

## Building deployment packages

### Prepare Linux runtime TLS files

Linux deployment builds require these repository-local runtime files:

```text
tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

Generate them with:

```bash
make ca
make gateway-cert
```

The Linux build fails before packaging if any required runtime TLS file is missing. Only the three runtime files are copied into the deployment directory; the complete `pki/` directory and all CA private keys remain excluded.

### Linux

```bash
make build-linux-amd64
make build-linux-arm64
```

Both commands generate `build/linux/`. Run only the architecture target needed for the deployment machine after `make clean`.

The normal `install.sh` installs and enables managed automatic updates. `install-dev.sh` pauses existing update units and installs the current build without installing the updater, preventing a development binary from being immediately replaced during testing.

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

The automatic runtime-TLS packaging described above currently applies to Linux deployment builds. HTTPS deployments on other platforms must place the configured certificate files at paths readable by the service account.

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
├── install-dev.sh
├── update.sh
├── fbs-interlock-gateway-update.service
├── fbs-interlock-gateway-update.timer
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── Linux Install Instructions.md
```

The staged gateway private key is copied with mode `0600`; the two certificates are copied with mode `0644`.

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

Release binaries contain only the executable and checksum. Generated TLS trust, certificates, and private keys are never included in GitHub release assets.

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
├── install-linux-dev.sh.in
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
├── config.yaml.bak
└── tls/
    ├── server-ca.crt
    ├── gateway-client.crt
    └── gateway-client.key

/etc/systemd/system/
├── fbs-interlock-gateway.service
├── fbs-interlock-gateway-update.service
└── fbs-interlock-gateway-update.timer
```

The production Linux installer:

- verifies or installs `lsof`, `curl`, `ca-certificates`, and `ufw`
- configures and enables UFW
- verifies that all three packaged gateway TLS files exist
- creates the service user and group when needed
- installs the executable and service files
- preserves an existing production config
- installs new TLS files but preserves existing installed TLS files on reinstallation
- sets installed TLS files to `root:<service-group>` with mode `0640`
- verifies that the service account can read each TLS file
- applies restrictive ownership and permissions
- enables and starts the gateway
- enables the update timer when updater files are present

The development Linux installer performs the same dependency, firewall, binary, config, TLS, and service installation, but it:

- disables and stops existing automatic update units
- does not install update files
- leaves automatic updates disabled during development testing
- instructs the operator to run the normal installer to restore managed updates

The systemd service:

- starts after the network is online
- runs under the configured service account
- uses `/etc/fbs-interlock-gateway` as its working directory
- resolves relative config paths such as `./tls/...` from the configuration directory
- writes logs to journald
- restarts after exits
- waits two seconds between starts
- limits rapid restart attempts
- applies `NoNewPrivileges=true`

The hourly updater changes only the executable. It does not replace the installed configuration or TLS files.

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
- creates the configuration directory owned by the service account
- installs or preserves `config.yaml` with service-account ownership and mode `0640`
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

The updater now avoids downloading or reinstalling an unchanged release:

1. selects the matching Linux release asset
2. downloads the small release checksum file first
3. validates that the checksum contains a SHA-256 value
4. computes the installed binary checksum
5. exits without downloading or restarting when the installed checksum already matches
6. downloads the binary only when it differs or is missing
7. verifies the downloaded binary against the release checksum
8. backs up the installed executable
9. installs the new executable
10. verifies the checksum again after installation
11. restarts the service only if it was active before the update
12. rolls back when the installed checksum is wrong or the service fails to start

The updater changes only the application binary. It does not modify `config.yaml` or the installed TLS files.

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

Use the generated `install-dev.sh` when testing a development build on a machine that normally receives automatic updates.

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

If the generated TLS paths remain configured, generate the local certificate material:

```bash
make ca
make gateway-cert
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

Read the Admin cache without contacting Shellies:

```bash
curl -i "http://127.0.0.1:18090/api/status"
```

Start an explicit fleet refresh:

```bash
curl -i "http://127.0.0.1:18090/api/status?refresh=1"
```

The `X-Status-Refresh-In-Progress` header reports whether the background scan is active.

Read configuration:

```bash
curl -s "http://127.0.0.1:18090/api/config"
```

Test an HTTP Shelly directly:

```bash
curl "http://<shelly-host>/rpc/Switch.GetStatus?id=<switch-id>"
curl "http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=true"
curl "http://<shelly-host>/rpc/Switch.Set?id=<switch-id>&on=false"
```

Test Digest Authentication over HTTP:

```bash
curl --anyauth -u "admin:<password>" \
  "http://<shelly-host>/rpc/Switch.GetStatus?id=<switch-id>"
```

Test mutual TLS directly from the development machine:

```bash
curl \
  --cacert ./tls/server-ca.crt \
  --cert ./tls/gateway-client.crt \
  --key ./tls/gateway-client.key \
  "https://<shelly-ddns-host>/rpc/Switch.GetStatus?id=<switch-id>"
```

Add `--anyauth -u "admin:<password>"` when the HTTPS Shelly also requires Digest Authentication.

Test through the gateway:

```bash
curl "http://<gateway-host>:<port>/status"
curl "http://<gateway-host>:<port>/on"
curl "http://<gateway-host>:<port>/off"
```

## Runtime behavior

On startup, the gateway:

1. parses `-config`, `-admin`, and `-version`
2. resolves and loads configuration from the explicit path or beside the executable
3. resolves relative TLS file paths against the configuration directory
4. applies defaults
5. initializes the shared status store with safe-output placeholders
6. loads the configured Shelly server CA and gateway client identity when TLS is configured
7. validates enabled tools, protocols, listener ports, and required TLS fields
8. starts the Admin server unless disabled
9. starts one FBS-facing HTTP listener per enabled tool
10. maps each gateway port to one configured interlock
11. logs inbound FBS requests and outbound responses
12. uses HTTP or HTTPS, optional mutual TLS, and optional Digest Authentication per tool
13. records every completed Shelly result in the shared Admin status store
14. reports the configured safe state and records the error when a device request fails
15. shuts down cleanly on interrupt, termination, Admin restart request, or server error

During operation:

- ordinary Admin status polling reads memory only
- an explicit Admin refresh queries enabled tools with at most four workers
- FBS requests passively update only the affected tool row
- per-device revisions prevent stale scan results from replacing newer FBS results
- requests to one Shelly are serialized and may reuse an idle connection and Digest session
- transient status failures may receive one retry
- persistent Shelly HTTP `423` or `429` conditions may schedule a cooldown-protected reboot request

Disabled tools do not receive FBS listeners and are not contacted by explicit status refreshes.

### Port ownership warning

Before starting an enabled listener, the gateway may clear a process already using that configured port. Use dedicated gateway ports and confirm that unrelated services do not use the configured range.

### Configuration reload behavior

A successful Admin UI save writes the updated configuration and requests a process restart. The installed platform supervisor rebuilds runtime listeners, the shared status store, the Shelly transport, TLS trust, and Digest state by starting the process again.

## Logging

Gateway FBS request logs use:

```text
FBS_IN
FBS_OUT
```

Shelly-client operational logs may include:

```text
shelly_status_retry
shelly_authentication_throttled
shelly_reboot_scheduled
shelly_reboot_suppressed
shelly_reboot_requested
shelly_reboot_failed
```

Network errors include the observed request phase and elapsed time, for example:

```text
phase=dns_lookup
phase=tcp_connect
phase=tls_handshake
phase=response_headers
phase=response_body
```

They also report whether the HTTP connection was reused and whether the TLS session resumed.

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
.DS_Store
build
/tls/
pki
config.yaml
config.yaml.bak
```

`pki/` contains CA private keys, gateway certificate requests, and per-device Shelly keys. `/tls/` contains the staged gateway runtime trust and identity files. Both directories are intentionally excluded from version control.

Committed content includes source code, tests, certificate templates and generation helpers, service templates, workflows, and documentation. Production configuration, credentials, generated certificates, and private keys remain on controlled development or deployment machines.
