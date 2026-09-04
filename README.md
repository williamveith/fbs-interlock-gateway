---
title: "FBS Interlock Gateway"
subtitle: "Project Overview and Operations Reference"
author: "William Veith"
date: "2026-09-04"
lang: en-US
---

> **Purpose**
>
> This README provides the authoritative project-level overview for `fbs-interlock-gateway`, including system architecture, security boundaries, interlock hardware, configuration, Admin behavior, development, deployment packaging, updates, logging, testing, and release operations.

> **Safety and security boundary**
>
> The gateway controls software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. Production listener ports must remain restricted to the authorized FBS source, the Admin UI should remain bound to loopback, and generated credentials, certificates, private keys, and production mappings must remain outside version control.

# Table of Contents
- [Project Overview](#project-overview)
  - [Documentation Set](#documentation-set)
- [Capabilities](#capabilities)
- [Safety and Security Model](#safety-and-security-model)
  - [Platform Firewall Behavior](#platform-firewall-behavior)
  - [Release and Update Trust](#release-and-update-trust)
- [System Architecture](#system-architecture)
- [Interlock Hardware](#interlock-hardware)
- [Repository Layout](#repository-layout)
- [FBS-Facing Behavior](#fbs-facing-behavior)
  - [Endpoints](#endpoints)
- [Gateway-to-Shelly Communication](#gateway-to-shelly-communication)
  - [Connection, Authentication, Priority, and Recovery Behavior](#connection-authentication-priority-and-recovery-behavior)
- [Shelly Authentication Helper](#shelly-authentication-helper)
- [Shelly Mutual TLS](#shelly-mutual-tls)
  - [Certificate Roles](#certificate-roles)
  - [Generate the Certificate Authorities and Gateway Identity](#generate-the-certificate-authorities-and-gateway-identity)
  - [Generate a Shelly Server Certificate](#generate-a-shelly-server-certificate)
- [Admin UI](#admin-ui)
  - [Passive Status Model](#passive-status-model)
  - [FBS-Priority Scheduling](#fbs-priority-scheduling)
  - [Admin Server Protections](#admin-server-protections)
  - [Admin Address Flag](#admin-address-flag)
  - [Remote Access with an SSH Tunnel](#remote-access-with-an-ssh-tunnel)
- [Admin API](#admin-api)
  - [GET /api/config](#get-apiconfig)
  - [PUT /api/config](#put-apiconfig)
  - [GET /api/status](#get-apistatus)
  - [GET /api/status?refresh=1](#get-apistatusrefresh1)
  - [POST /api/restart](#post-apirestart)
- [Configuration storage](#configuration-storage)
  - [Storage model](#storage-model)
  - [First-run migration](#first-run-migration)
  - [Admin changes](#admin-changes)
  - [Import and export](#import-and-export)
  - [Installed paths](#installed-paths)
    - [Linux](#linux)
    - [Windows](#windows)
    - [macOS](#macos)
  - [Security](#security)
- [Configuration](#configuration)
  - [Config Fields](#config-fields)
  - [Validation](#validation)
  - [Configuration Ownership](#configuration-ownership)
- [Development and Validation](#development-and-validation)
- [Building Deployment Packages](#building-deployment-packages)
  - [Prepare Runtime TLS Files](#prepare-runtime-tls-files)
  - [Deployment-Guide PDF Requirements](#deployment-guide-pdf-requirements)
  - [Linux](#linux-1)
  - [Windows AMD64](#windows-amd64)
  - [macOS Apple Silicon](#macos-apple-silicon)
  - [macOS Intel](#macos-intel)
  - [Generate Template-Derived Files Only](#generate-template-derived-files-only)
  - [Generate Deployment-Guide PDFs Only](#generate-deployment-guide-pdfs-only)
- [Deployment Build Output](#deployment-build-output)
  - [Linux](#linux-2)
  - [Windows](#windows-1)
  - [macOS ARM64](#macos-arm64)
  - [macOS AMD64](#macos-amd64)
- [Release Binaries](#release-binaries)
- [Service Templates and Installed Layouts](#service-templates-and-installed-layouts)
  - [Linux Templates](#linux-templates)
  - [Windows Templates](#windows-templates)
  - [macOS Templates](#macos-templates)
- [Automatic Updates and Log Maintenance](#automatic-updates-and-log-maintenance)
- [Continuous Integration](#continuous-integration)
- [Release Workflow](#release-workflow)
- [Branch and Pull-Request Workflow](#branch-and-pull-request-workflow)
- [Local Testing](#local-testing)
- [Runtime Behavior](#runtime-behavior)
  - [Port Ownership Warning](#port-ownership-warning)
  - [Configuration Reload Behavior](#configuration-reload-behavior)
- [Logging](#logging)
- [Repository Safety](#repository-safety)
- [License](#License)

<div class="page-break"></div>

# Project Overview

`fbs-interlock-gateway` is a Go service that lets the FBS interlock system control networked tool interlocks through a local gateway server.

The gateway receives FBS HTTP interlock requests, maps each request to a configured tool listener, communicates with the assigned Shelly relay or network interlock over HTTP or HTTPS RPC, and returns the simple JSON state response expected by FBS. Shelly communication can use HTTP Digest Authentication, mutual TLS, or both.

The project is intended for controlled facility deployments where FBS communicates with the gateway and the gateway communicates with configured interlock devices. A shared in-memory status store lets normal FBS traffic passively update the local Admin UI without creating recurring Shelly status traffic.

```text
FBS server
    -> source-restricted gateway listener
    -> fbs-interlock-gateway
    -> Shelly RPC over HTTP or HTTPS
    -> Shelly-based interlock hardware
    -> tool interlock / monitor circuit
```

The repository covers the complete interlock system boundary: application behavior, device communication, security, service supervision, deployment, maintenance, physical wiring, enclosure labeling, and auditable device identity.

## Documentation Set

| Document | Scope |
| --- | --- |
| [Linux Installation and Operations Guide](<docs/deployment guides/Linux Install Instructions.md>) | Linux build, installation, systemd supervision, UFW, updates, logging, troubleshooting, and uninstall |
| [Windows Installation and Operations Guide](<docs/deployment guides/Windows Install Instructions.md>) | Windows deployment, Task Scheduler supervision, firewall controls, updates, logging, rollback, and uninstall |
| [macOS Installation and Operations Guide](<docs/deployment guides/macOS Install Instructions.md>) | macOS deployment, LaunchDaemons, Application Firewall, Packet Filter, updates, logging, rollback, and uninstall |
| [Shelly Interlock Hardware Guide](<docs/hardware/Shelly Interlock Hardware Guide.md>) | Junction-box materials, per-assembly bills of materials, wiring configurations, label artwork, QR device identity, fabrication, verification, and maintenance |
| [Security Policy](SECURITY.md) | Supported versions, private vulnerability reporting, operational-safety limits, secret handling, coordinated disclosure, and safe-harbor expectations |

Use this README for project-wide behavior and architecture, including the authoritative SQLite configuration-storage model. Use the platform guides for installation and operations, and use the hardware guide for physical interlock construction and audit documentation.

# Capabilities

- one FBS-facing listener port per configured tool
- FBS-compatible `/status`, `/on`, and `/off` endpoints
- supported query-based on/off command formats
- Shelly Gen2/Gen3 RPC over per-tool `http` or `https`
- optional Shelly HTTP Digest Authentication with reusable per-device digest sessions
- optional mutual TLS with Shelly server verification and gateway client authentication
- certificate-generation helpers for separate private server/client CAs, the gateway identity, and per-device Shelly identities
- canonical PKI source layout under `pki/`, with only required runtime trust/identity files copied into deployment builds
- version-controlled Shelly interlock wiring diagrams, junction-box label artwork, per-assembly bills of materials, hardware build documentation, and QR-encoded device identity records for rapid auditing
- shared HTTP connection reuse, TLS session caching, and per-device RPC serialization
- FBS-priority device scheduling that defers or cancels lower-priority Admin probes
- transient status retry for bounded timeout/connection failures and controlled recovery for persistent Shelly HTTP `423` and `429` responses
- request-phase diagnostics for DNS, TCP, TLS, response-header, and response-body failures
- an embedded Admin UI bound to localhost by default
- a shared status store passively updated by normal FBS traffic
- cache-only Admin UI polling that does not contact Shelly devices
- explicit on-demand fleet refresh with up to 32 concurrent workers
- incremental fleet-refresh publishing that preserves completed partial results
- revision-protected status merging so older Admin scans cannot overwrite newer FBS results
- visible disconnected, safe-output, protocol, and error states
- editable per-tool protocol and gateway mutual-TLS file paths
- password masking and preservation in the Admin API
- an authoritative local SQLite configuration store in `gateway.sqlite3`
- one-time migration from legacy `config.yaml`, followed by generated YAML rollback/compatibility mirrors with `.bak` preservation
- transactional configuration replacement with schema constraints and database integrity checks
- `config export` and `config import` commands, including redacted YAML export for review
- deep-cloned configuration snapshots that prevent unintended shared-state mutation
- Linux AMD64 and ARM64 deployment packages with runtime TLS files and rendered PDF guides
- Windows AMD64 deployment packages with runtime TLS files, production/development installers, managed updates, and rendered PDF guides
- macOS ARM64 and AMD64 deployment packages with runtime TLS files, production/development installers, managed updates, Packet Filter rules, and rendered PDF guides
- Linux systemd supervision, UFW source restriction, authenticated signed-checksum updates, downgrade protection, and standard or purge uninstall
- Windows Task Scheduler supervision under restricted runtime credentials, source-restricted Windows Firewall rules, authenticated signed-checksum updates, log rotation, downgrade protection, and rollback
- macOS LaunchDaemon supervision under a dedicated service account, Application Firewall registration, managed `pf` source restriction, authenticated signed-checksum updates, log rotation, downgrade protection, and rollback
- an embedded Ed25519 update-trust public key and dependency-free `verify-update-checksum` command used by installed updaters
- Go 1.26 development with `go vet`, Staticcheck, race testing, Bash syntax validation, ShellCheck, PowerShell validation, plist validation, and cross-platform build checks
- weekly Dependabot maintenance for Go modules and GitHub Actions
- full-commit-SHA-pinned GitHub Actions and a separately pinned reusable trusted-builder workflow
- SLSA v1 build provenance for release binaries and checksum files
- Ed25519 detached signatures over authenticated release checksum files
- GPG-signed annotated release tags
- immutable GitHub Releases with release/asset attestation verification
- a unified `make verify` validation gate

# Safety and Security Model

The gateway controls access signals, but it is not a substitute for hardware safety controls.

```text
FBS server
  -> host firewall source restriction
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
- Production gateway listener ports should be reachable only from the authorized FBS source.
- The Admin UI should remain bound to a loopback address unless remote access is intentionally secured.
- Shelly credentials are stored locally in the authoritative `gateway.sqlite3` database and can also appear in generated YAML rollback/compatibility mirrors and non-redacted exports. Protect all of these as sensitive configuration.
- Gateway TLS private keys and private CA material must remain outside version control.
- Deployment packages contain only the gateway runtime trust certificate, gateway client certificate, and gateway client private key. CA private keys, CSRs, `client-ca.crt`, and per-device Shelly private keys remain on the certificate-management machine.
- Existing installed SQLite configuration state, YAML rollback/compatibility state, and TLS identities are preserved during normal reinstallation on all supported platforms.
- Real credentials, certificates, private keys, and production mappings must not be committed to the repository.

## Platform Firewall Behavior

The production and development installers apply the same platform security boundary. Development mode disables managed release replacement; it does not weaken runtime firewall controls.

| Platform | Installer behavior |
| --- | --- |
| Linux | Installs UFW when needed, sets default incoming traffic to deny, allows outgoing traffic, permits the configured FBS source IP to the configured gateway TCP port range, and enables UFW. The uninstaller removes the gateway-specific UFW rule without disabling UFW or reversing system-wide defaults. |
| Windows | Sets the Domain, Private, and Public profiles' default inbound action to `Block`, removes the older unrestricted gateway rule, and adds an inbound rule restricted by executable, TCP listener range, and authorized FBS source IP. |
| macOS | Adds the executable to the Application Firewall allow list and installs a managed Packet Filter anchor that permits loopback, permits the authorized FBS source to the gateway listener range, and blocks other inbound TCP traffic to that range. The installer validates the complete candidate `pf.conf` and preserves unrelated rules. |

The generated platform rules use these Makefile variables:

```make
FBS_SOURCE_IP = <authorized-fbs-source>
FBS_PORT_RANGE = 8081:8981
```

Review them before building a production deployment. Keep the Admin UI on `127.0.0.1`; the generated listener-range rules are not a replacement for Admin authentication or remote-access controls.

## Release and Update Trust

Release integrity is intentionally split across build, publication, and deployed-update trust boundaries.

```text
source commit on main
        |
        v
pinned reusable trusted builder
        |
        +-> validated release binaries + SHA-256 files
        |
        +-> SLSA v1 provenance via GitHub OIDC/Sigstore
                    |
                    v
publish job re-verifies provenance
        |
        +-> Ed25519-signs each authenticated .sha256 file
        |
        +-> verifies the embedded updater public key
        |
        +-> creates and locally verifies a GPG-signed tag
                    |
                    v
immutable GitHub Release
        |
        +-> release attestation
        +-> exact asset attestation verification
                    |
                    v
installed gateway verifies .sha256.sig
before trusting an update checksum
```

Important trust properties:

- The release caller uses the reusable workflow in `williamveith/fbs-interlock-gateway-builder` at an exact full commit SHA.
- The trusted build receives only the permissions required for build/provenance work; the release GPG key and update-signing private key are not exposed to repository build commands.
- The publish job verifies SLSA provenance against the expected repository, exact source commit, `refs/heads/main`, exact trusted-builder workflow revision, SLSA provenance predicate, and a GitHub-hosted runner requirement before authorizing publication.
- Only after provenance verification are the small `.sha256` files signed with the release-environment Ed25519 update-signing key.
- The gateway executable embeds only the matching Ed25519 public key. The private update-signing key remains an environment secret and is never deployed.
- The `verify-update-checksum` command verifies the detached 64-byte Ed25519 signature over the exact checksum-file bytes, validates the SHA-256 format, and requires the signed asset filename to match the requested release asset.
- Production updaters use the currently installed trusted gateway executable to verify the checksum signature; a downloaded candidate is never allowed to authenticate itself.
- Automatic updates require stable `vMAJOR.MINOR.PATCH` metadata and reject authenticated downgrades. Reinstalling the same version is allowed so an authenticated release can repair a locally altered binary.
- GitHub release immutability and `gh release verify` / `gh release verify-asset` provide a separate publication-level check in addition to SLSA build provenance.

The repository's [Security Policy](SECURITY.md) defines vulnerability-reporting, operational-safety, secret-handling, and coordinated-disclosure expectations.

# System Architecture

```text
+------------+        HTTP         +-----------------------+                        +------------------+
|            |  /status /on /off   |                       |   HTTP or HTTPS RPC    |                  |
| FBS Server | <-----------------> | fbs-interlock-gateway | <--------------------> | Shelly Relay Box |
|            |                     |                       |   Digest and/or mTLS   |                  |
+------------+                     +-----------------------+                        +------------------+
                                                |                                             |
                                  successful or |                                             |
                                  failed result |                                             v
                                                v                                   Tool monitor / enable
                                    +---------------------+                         circuit changes state
                                    | Shared status store |
                                    +---------------------+
                                                ^
                             cache-only polling |
                         explicit fleet refresh | 
                                                |
                                   +------------------------+
                                   |  Local-only Admin UI   |
                                   | http://127.0.0.1:18090 |
                                   +------------------------+
```

Normal FBS traffic updates only the affected tool row in the shared status store. The browser reads that in-memory snapshot every three seconds without contacting any Shelly. The **Refresh Status** action explicitly runs a fleet-wide `Switch.GetStatus` scan with up to 32 workers and publishes each completed result immediately. FBS requests have priority over Admin probes for the same device.

# Interlock Hardware

The software is designed to operate with custom Shelly-based interlock junction boxes. Documentation on how to build the hardware interlocks is maintained under [`docs/hardware/`](<docs/hardware/Shelly Interlock Hardware Guide.md>).

The hardware documentation includes:

- a wiring configuration powered from an external 12 VDC supply
- a wiring configuration powered from the tool's 110–240 VAC supply
- junction-box label artwork for both power configurations
- a general materials list for building the interlock boxes
- bills of materials with procurement links and recorded per-assembly costs for both supported power configurations
- the recorded 30 W fiber-laser settings used to mark the box lids
- QR-code label data encoded as serialized JSON containing the Shelly device name, model number, and unique device ID
- a phone-based audit workflow for rapidly collecting and verifying installed-device identity information

Each junction-box QR code records the installed Shelly identity in a compact JSON object, such as `{"device":"Shelly 1 Gen4","model":"S4SW-001X16EU","id":"A085E3B5325C"}`. Scanning the label with a phone provides a fast, auditable record of the device type, model, and unique Shelly ID.

The hardware guide records estimated material costs of **$43.88 per external-12-VDC assembly** and **$34.93 per tool-powered 110–240 VAC assembly** using the source-workbook quantities and prices. Procurement prices are references rather than fixed quotations.

The diagrams document the associated interlock hardware but do not replace qualified electrical review, applicable codes, equipment ratings, or facility safety requirements.

# Repository Layout

```text
cmd/fbs-interlock-gateway/
  main.go
    application entry point, runtime/config CLI flags, version output,
    first-run migration, YAML import/export, and signal handling

internal/admin/
  server.go
  server_test.go
  web/
    embedded Admin UI, local Admin API, incremental fleet refresh,
    and FBS-priority Admin status behavior

internal/config/
  YAML loading/serialization, relative-path resolution, defaults,
  validation, deep cloning, and legacy file persistence support

internal/configstore/
  SQLite schema and migrations, transactional configuration replacement,
  integrity checks, stable tool IDs, and YAML compatibility mirrors

internal/fbs/
  FBS-compatible HTTP request handling, responses, and status recording

internal/gateway/
  application lifecycle, listener startup, restart, configuration ownership,
  and shared dependencies

internal/process/
  listener-port process utilities

internal/shelly/
  HTTP/HTTPS RPC client, Digest Authentication, mutual TLS,
  per-device request priority, retry, request timing, and controlled recovery

internal/status/
  revision-protected shared in-memory Admin status store

internal/updateauth/
  embedded Ed25519 update-trust public key, signed-checksum verifier,
  CLI command implementation, and security-focused tests

scripts/pandoc/
  inline-code wrapping filter used by deployment-guide PDF rendering

scripts/tls/
  private-CA, gateway-client, and per-Shelly certificate generation

services/linux/
  systemd service, production installer, development installer,
  uninstaller, and updater templates

services/windows/
  production/development launchers, PowerShell installer,
  startup supervisor, updater, and uninstaller templates

services/macos/
  production/development installers, startup wrapper, gateway and update
  LaunchDaemon property lists, Packet Filter anchor, updater, and uninstaller

docs/
  deployment guides/
    platform-specific Markdown installation and operations guides rendered to PDF
    during deployment builds
  hardware/
    Shelly interlock wiring diagrams, junction-box label artwork, and build notes

.github/dependabot.yml
  weekly Go-module and GitHub Actions dependency maintenance

.github/workflows/
  CI plus trusted-build, provenance, signing, immutable-release,
  and publication-verification automation

SECURITY.md
  vulnerability reporting, operational safety, disclosure, and safe-harbor policy
```

The generated `pki/` and `build/` directories are intentionally excluded from version control. Runtime TLS files are copied directly from the private `pki/` source tree into generated deployment directories; there is no repository-root runtime `tls/` staging directory.

# FBS-Facing Behavior

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

## Endpoints

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

# Gateway-to-Shelly Communication

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

## Connection, Authentication, Priority, and Recovery Behavior

Each Shelly request attempt is bounded by `defaults.timeout_ms`. DNS, TCP connect, TLS handshake, and response-header phases are bounded within that attempt. `Switch.GetStatus` may perform one additional transient-network retry after a short delay, so a failed status operation can consume two attempt budgets plus the retry delay.

Additional behavior:

- requests to the same Shelly protocol/host are serialized so a previous response is fully consumed before the next RPC begins
- requests to different Shellys can proceed concurrently
- normal FBS `/status`, `/on`, and `/off` operations use high-priority device acquisition
- Admin status probes are opportunistic and never wait behind an occupied device slot
- an arriving FBS operation cancels an active Admin probe for the same Shelly and takes the slot as soon as cancellation unwinds
- Admin deferral is treated as an expected scheduling result, not a connectivity failure
- the FBS operation records the authoritative result in the same shared status store used by the Admin UI
- Digest challenges are retained in a per-device session, allowing subsequent requests to send a preemptive Authorization header with an increasing nonce count
- digest sessions expire after 55 minutes or 30,000 nonce uses and are replaced when the device returns a new challenge
- retryable status failures are limited to network timeouts, EOF/unexpected EOF, connection reset, connection refused, and broken-pipe conditions and are retried once after 150 milliseconds
- deprecated `net.Error.Temporary()` classification is not used
- certificate validation errors, caller cancellation/deadline completion, and Admin deferrals are not retried
- HTTP `429 Too Many Requests` waits two seconds and retries once
- a persistent HTTP `429` or an HTTP `423 Locked` schedules one controlled `Shelly.Reboot` request
- reboot recovery uses a 500-millisecond device reboot delay, suppresses duplicate in-flight attempts, and enforces a five-minute per-device cooldown
- response and error bodies are bounded to 4 KiB
- network failures identify the observed phase such as DNS lookup, TCP connect, TLS handshake, response headers, or response body, and report whether the connection was reused or the TLS session resumed

# Shelly Authentication Helper

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

# Shelly Mutual TLS

The project uses two private certificate authorities so server and client trust are separated.

## Certificate Roles

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

## Generate the Certificate Authorities and Gateway Identity

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

The CA script refuses to overwrite existing CA material. By default, CA certificates are valid for 100 years; set `CA_VALID_DAYS` to override that duration.

Generate the gateway client identity:

```bash
make gateway-cert
```

This creates the full gateway material under `pki/gateway/`. Gateway and Shelly leaf certificates default to 100 years; set `CERT_VALID_DAYS` to override that duration. Upload `pki/ca/client-ca.crt` to every Shelly as the CA authorized to verify the gateway client certificate.

## Generate a Shelly Server Certificate

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

Install the generated `shelly-server.crt`, `shelly-server.key`, and `pki/ca/client-ca.crt` on the matching device. Keep all CA private keys and generated device private keys securely outside the repository.

# Admin UI

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
- up to 32 concurrent Shelly status workers during an explicit fleet refresh
- incremental row publication as each worker completes
- preservation of completed partial results if the overall refresh times out or is canceled
- FBS-priority scheduling that skips or cancels Admin probes when production traffic needs the same device
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

## Passive Status Model

The Admin UI is not a second device-control loop.

```text
FBS request
  -> Shelly operation
  -> gateway records the result for that tool
  -> browser reads the in-memory snapshot
```

An ordinary browser status poll does not contact a Shelly. This preserves a near-live operational view without restoring the previous fleet-wide status scan every three seconds.

The first page load and the **Refresh Status** button use an explicit fleet refresh. During that refresh, the browser checks the in-memory snapshot once per second until the server reports completion. Those checks do not start additional device requests.

Fleet workers write each successful or failed device result directly into the shared status store as soon as it completes. Healthy rows therefore become visible while slower devices are still pending, and completed rows remain available if the two-minute refresh context expires.

The shared store uses monotonically increasing revisions. A slow manual refresh that started earlier cannot overwrite a newer FBS result that completed later.

A successful `/on` or `/off` result represents the state accepted by `Switch.Set`. Use **Refresh Status** when an independent `Switch.GetStatus` verification is required.

## FBS-Priority Scheduling

Admin status probes use a distinct low-priority Shelly operation:

- an Admin probe does not queue behind another request
- an active Admin probe is canceled when an FBS operation for the same Shelly arrives
- the canceled Admin probe preserves the existing row instead of recording a false connectivity error
- the FBS status or command result updates the same shared row the Admin probe was attempting to refresh
- Digest nonce use and relay operations remain serialized because only one actual Shelly RPC is active per device

This keeps Admin fleet verification responsive without allowing it to consume the FBS request timeout budget.

## Admin Server Protections

The Admin server includes:

- explicit HTTP method handling
- strict single-object JSON decoding
- rejection of unknown JSON fields
- a request body size limit
- read, write, idle, header, and shutdown timeouts
- a two-minute explicit-refresh timeout
- at most 32 concurrent device-status requests during a fleet refresh
- one active fleet refresh at a time
- immediate per-tool publishing instead of fleet-sized all-or-nothing commits
- FBS-priority preemption of Admin device probes
- revision-protected merging of refresh results into the shared store
- no-store headers for API responses
- same-origin checks for state-changing requests
- rejection of cross-site state-changing requests
- `Content-Security-Policy`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- restrictive referrer and permissions policies

Keep the Admin UI on loopback whenever possible.

## Admin Address Flag

Set an explicit address:

```bash
./fbs-interlock-gateway \
  -config config.yaml \
  -db gateway.sqlite3 \
  -admin 127.0.0.1:18090
```

Disable the Admin UI:

```bash
./fbs-interlock-gateway \
  -config config.yaml \
  -db gateway.sqlite3 \
  -admin ""
```

## Remote Access with an SSH Tunnel

```bash
ssh -L 18090:127.0.0.1:18090 fbs-gateway@<gateway-host>
```

Then open:

```text
http://127.0.0.1:18090
```

# Admin API

```text
GET  /api/config
PUT  /api/config
GET  /api/status
GET  /api/status?refresh=1
POST /api/restart
```

## `GET /api/config`

Returns the loaded configuration without returning stored passwords. The response includes normalized per-tool protocols and the gateway `defaults.shelly_tls` paths.

A tool with a stored password reports:

```json
{
  "password_set": true
}
```

An omitted tool protocol is returned as `http`.

## `PUT /api/config`

Accepts edited configuration, validates it, preserves existing passwords unless replacement or clearing is explicitly requested, and replaces the authoritative configuration in one SQLite transaction. After a successful database commit, the configured compatibility mirror is regenerated as YAML and its previous contents are preserved as `config.yaml.bak` when possible. The SQLite commit remains authoritative even if the compatibility-mirror write later fails.

The request can update:

- per-tool `http` or `https` protocol
- gateway Shelly-server CA path
- gateway client-certificate path
- gateway client-key path

A successful response indicates that a restart is required.

## `GET /api/status`

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

## `GET /api/status?refresh=1`

Starts one asynchronous fleet-wide `Switch.GetStatus` refresh and immediately returns the current snapshot.

While the refresh is active, responses include:

```text
X-Status-Refresh-In-Progress: true
```

Additional explicit refresh requests share the in-progress scan rather than creating duplicate Shelly traffic. The browser performs cache-only follow-up reads until the header changes to `false`.

Refresh results are merged by tool and revision. A result from an older scan cannot overwrite a newer FBS-derived status.

## `POST /api/restart`

Requests a clean process restart. The platform service supervisor starts the process again in an installed deployment.

# Configuration storage

The gateway uses SQLite as the authoritative persistent configuration store.

The in-memory runtime model remains `config.Config`; SQLite is a persistence implementation detail and is not queried on each FBS or Shelly request.

## Storage model

- `gateway.sqlite3` is authoritative after initialization.
- `config.yaml` is accepted as a legacy first-run import source.
- The original YAML is left untouched during the first import.
- After a successful Admin configuration change, the gateway writes a generated YAML compatibility snapshot and moves the previous YAML to `config.yaml.bak`.
- Live relay/status state remains in memory and is not persisted to SQLite.
- TLS keys and certificates remain ordinary protected files; their paths are stored in the database.

The SQLite connection is intentionally conservative:

- one database connection
- rollback-journal (`DELETE`) mode
- `synchronous=FULL`
- foreign keys enabled
- 5-second busy timeout
- `PRAGMA quick_check(1)` on open
- schema versioning through `PRAGMA user_version`

The database must be local to the gateway host. Do not place `gateway.sqlite3` on NFS, SMB, or another network filesystem for active/passive sharing. Replicate configuration between gateway hosts explicitly instead.

## First-run migration

Fresh platform installers and generated service wrappers pass both the legacy YAML path and the platform-specific authoritative database path explicitly.

On Linux, for example:

```text
fbs-interlock-gateway \
  -config /etc/fbs-interlock-gateway/config.yaml \
  -db /var/lib/fbs-interlock-gateway/gateway.sqlite3
```

Windows `start.bat` and macOS `start.sh` likewise pass explicit `-config` and `-db` paths. The binary also accepts `FBS_GATEWAY_DB_PATH` when `-db` is omitted. If neither is supplied, the database defaults to `gateway.sqlite3` beside the selected `config.yaml`.

Startup behavior is deterministic:

1. Open and integrity-check `gateway.sqlite3`.
2. Apply any supported database schema migrations.
3. If the database already contains configuration, load it and ignore manual edits to `config.yaml`.
4. If the database is uninitialized, load and validate `config.yaml`.
5. Resolve relative TLS paths while loading the YAML.
6. Import the complete configuration in one SQLite transaction.
7. Re-load the imported configuration from SQLite before starting listeners.

If the database is uninitialized and the legacy YAML is absent or invalid, the gateway refuses to start.

## Admin changes

The existing Admin API remains unchanged. `PUT /api/config` still validates the complete proposed configuration before accepting it, but persistence is now a SQLite transaction rather than an authoritative YAML rewrite. A successful save still requests a process restart so listeners, TLS state, the Shelly transport, and status structures are rebuilt exactly as before. After the SQLite commit, the configured YAML compatibility mirror is regenerated and the previous YAML is preserved as `config.yaml.bak` when possible.

Database constraints additionally prevent invalid ports, invalid booleans, invalid protocols, invalid switch IDs, duplicate listener ports, and invalid singleton settings from being committed.

## Import and export

Export the authoritative database as YAML:

```text
fbs-interlock-gateway config export \
  -db /var/lib/fbs-interlock-gateway/gateway.sqlite3 \
  -output fleet.yaml
```

A redacted export for review can be created with:

```text
fbs-interlock-gateway config export \
  -db /var/lib/fbs-interlock-gateway/gateway.sqlite3 \
  -output fleet-redacted.yaml \
  -redact-secrets
```

A redacted export replaces stored passwords with the literal value `REDACTED`. Do not import a redacted review copy unless that literal value is intentionally supposed to become the stored password.

Import a complete YAML configuration transactionally:

```text
fbs-interlock-gateway config import \
  -db /var/lib/fbs-interlock-gateway/gateway.sqlite3 \
  -input fleet.yaml
```

Add `-mirror-config <path>` to `config import` when a generated compatibility snapshot should also be maintained.

The import validates and replaces the complete database configuration in one transaction. Restart the supervised gateway afterward so listeners, TLS state, Shelly transports, status structures, and per-device scheduling state are rebuilt from the imported configuration.

Exports written to a file are created atomically with private file permissions. Non-redacted exports contain stored credentials and must be protected accordingly.

## Installed paths

Recommended production paths are:

### Linux

```text
/opt/fbs-interlock-gateway/fbs-interlock-gateway
/etc/fbs-interlock-gateway/config.yaml            # legacy/rollback mirror
/etc/fbs-interlock-gateway/tls/...
/var/lib/fbs-interlock-gateway/gateway.sqlite3    # authoritative
```

### Windows

```text
C:\FBS\fbs-interlock-gateway\fbs-interlock-gateway.exe
C:\FBS\fbs-interlock-gateway\config.yaml          # legacy/rollback mirror
C:\FBS\fbs-interlock-gateway\tls\...
C:\FBS\fbs-interlock-gateway\gateway.sqlite3
```

### macOS

```text
/usr/local/libexec/fbs-interlock-gateway/fbs-interlock-gateway
/Library/Application Support/fbs-interlock-gateway/config.yaml
/Library/Application Support/fbs-interlock-gateway/tls/...
/Library/Application Support/fbs-interlock-gateway/gateway.sqlite3
```

Standard uninstall preserves the authoritative database, YAML rollback/compatibility state, and TLS identity. Purge removes the persistent database along with the other preserved application state.

## Security

SQLite changes editability and transactional behavior; it is not encryption.

Shelly passwords stored in `gateway.sqlite3` remain recoverable by an account that can read the database. Protect the database with the same OS-level access controls used for the existing production configuration. Do not commit production `gateway.sqlite3`, configuration exports containing credentials, or generated compatibility YAML files to source control.

# Configuration

The gateway's runtime configuration model remains `config.Config`. YAML is still used for first-run initialization, compatibility mirrors, and explicit import/export workflows, but SQLite is authoritative after initialization.

Create a starter YAML configuration for first-run/local initialization:

```bash
make init-config
```

The target preserves an existing local `config.yaml`. The generated starter uses a five-second Shelly request-attempt timeout and relative TLS paths that work with the installed layout on every supported platform. If `defaults.timeout_ms` is omitted from an existing configuration, the runtime default remains three seconds:

```yaml
bind: 0.0.0.0

defaults:
  timeout_ms: 5000
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
  timeout_ms: 5000
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

## Config Fields

| Field | Purpose |
| --- | --- |
| `bind` | Address used by FBS-facing listeners. Use `0.0.0.0` to listen on all interfaces. |
| `defaults.timeout_ms` | Timeout applied to each Shelly request attempt. The generated starter uses `5000` milliseconds; an omitted value defaults to `3000` milliseconds at runtime. A retried status operation can use a second attempt plus the 150-millisecond retry delay. |
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

## Validation

Application validation includes:

- required, nonblank interlock names
- required device addresses
- rejection of URL schemes in `tools[].ip`
- `http` or `https` protocol normalization and validation
- listener ports within `8081` through `8981`
- duplicate listener ports
- valid nonnegative switch IDs
- required TLS file paths when an applicable tool uses HTTPS
- valid defaults

SQLite constraints provide an additional persistence boundary for invalid ports, invalid booleans, invalid protocols, invalid switch IDs, duplicate listener ports, and invalid singleton settings.

Invalid configuration is not committed. Missing or unreadable configured TLS files also prevent the gateway from starting.

## Configuration Ownership

The gateway deep-clones configuration when it is accepted and whenever a snapshot is returned. The clone includes the `tools` backing array and the optional username/password string values. Callers therefore cannot mutate the gateway's internal configuration by editing a returned snapshot or by retaining references to a configuration supplied to the gateway.

# Development and Validation

The module declares **Go 1.26** in `go.mod`. Staticcheck is recorded as a Go tool dependency, so the repository uses the pinned module version through `go tool staticcheck` rather than relying on an arbitrary globally installed Staticcheck binary.

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
make staticcheck
make shellcheck
make verify
```

`make verify` runs:

1. `gofmt` verification without modifying source files
2. `go.mod` and `go.sum` consistency checks
3. `go vet ./...`
4. `go tool staticcheck ./...`
5. all Go tests under the race detector, including SQLite configuration round trips/migration, Admin/FBS preemption, incremental fleet refresh, independent configuration-copy, shared-status, Digest, TLS, retry/recovery, and signed-update authorization tests
6. Bash syntax checks for top-level helpers, TLS helpers, and Linux/macOS deployment scripts
7. ShellCheck across `scripts/` and `services/` shell source/templates
8. PowerShell parser validation for the Windows installer, updater, and uninstaller when `pwsh` is available
9. a platform-independent check for ambiguous PowerShell variable interpolation before a colon
10. validation of both macOS LaunchDaemon property lists with `plutil` or Python's `plistlib`
11. Linux AMD64 build validation
12. Linux ARM64 build validation
13. Windows AMD64 build validation
14. macOS ARM64 build validation
15. macOS AMD64 build validation

Individual validation targets:

```bash
make fmt-check
make tidy-check
make vet
make staticcheck
make test-race
make scripts-check
make shellcheck
make build-check
```

TLS helper targets:

```bash
make ca
make gateway-cert
make shelly-cert
```

Deployment package builds additionally require the PDF toolchain described below because the platform Markdown guides are rendered into the build directories.

The private `pki/` tree is ignored by Git. Do not commit generated CA material, gateway or Shelly private keys, production configuration, or release-signing secrets.

# Building Deployment Packages

## Prepare Runtime TLS Files

Every Linux, Windows, and macOS deployment build requires three runtime identity/trust files, sourced directly from the canonical PKI tree:

```text
pki/
├── ca/
│   └── server-ca.crt
└── gateway/
    ├── gateway-client.crt
    └── gateway-client.key
```

Generate them with:

```bash
make ca
make gateway-cert
```

The Makefile verifies those source files and copies only the required runtime material into the platform deployment directory's `tls/` subdirectory. There is no repository-root staging `tls/` directory.

The complete `pki/` directory, CA private keys, CSRs, `client-ca.crt`, and per-device Shelly private keys remain excluded from deployment packages. The copied gateway private key uses mode `0600` in generated build output; certificates use `0644`. Platform installers then apply the native installed permissions required by their service accounts.

## Deployment-Guide PDF Requirements

Deployment builds render every matching Markdown guide from `docs/deployment guides/` into a PDF in the corresponding platform build directory.

Default guide patterns:

```make
LINUX_DEPLOYMENT_GUIDE_PATTERN  = Linux*.md
WINDOWS_DEPLOYMENT_GUIDE_PATTERN = Windows*.md
MACOS_DEPLOYMENT_GUIDE_PATTERN  = macOS*.md
```

Required tools and defaults:

```make
PANDOC = pandoc
PDF_ENGINE = xelatex
PDF_MARGIN = 0.3in
PDF_FONT_SIZE = 12pt
PDF_MAIN_FONT = IBMPlexMono-Regular
PDF_MONO_FONT = IBMPlexMono-Regular
PANDOC_INLINE_CODE_FILTER = scripts/pandoc/wrap-inline-code.lua
```

Install Pandoc, XeLaTeX, the configured fonts, and the LaTeX support needed by `fvextra` before building deployment packages. The Lua filter wraps long inline-code spans, and `fvextra` enables line breaking in verbatim/code blocks so generated deployment PDFs do not run past the page width. Override a pattern or PDF variable at invocation time when needed, for example:

```bash
make build-linux-amd64 \
  LINUX_DEPLOYMENT_GUIDE_PATTERN='*.md' \
  PDF_MAIN_FONT='IBM Plex Mono'
```

The build fails when no guide matches the selected pattern.

## Linux

```bash
make build-linux-amd64
make build-linux-arm64
```

Both commands generate `build/linux/`. Run only the architecture target needed for the deployment machine after `make clean`.

The normal `install.sh` installs and enables managed automatic updates. `install-dev.sh` disables existing updater units and installs the current build without managed release replacement. Both apply the same service-account, TLS, firewall, and configuration protections. The package also includes `uninstall.sh`.

## Windows AMD64

```bash
make build-windows-amd64
```

The Windows package includes production and development launchers, the PowerShell installer, startup supervisor, authenticated signed-checksum updater, uninstaller, runtime TLS files, and rendered guide PDFs.

## macOS Apple Silicon

```bash
make build-darwin-arm64
```

`make build` and `make build-mac` remain aliases for the Apple Silicon deployment build.

## macOS Intel

```bash
make build-darwin-amd64
```

Both macOS packages include production and development installers, the startup wrapper, gateway and update LaunchDaemon property lists, authenticated signed-checksum updater, Packet Filter anchor, uninstaller, runtime TLS files, and rendered guide PDFs.

## Generate Template-Derived Files Only

```bash
make windows-deployment-files
make macos-arm64-deployment-files
make macos-amd64-deployment-files
make macos-deployment-files
```

These targets render scripts and service definitions but do not build the executable or copy runtime TLS material.

## Generate Deployment-Guide PDFs Only

```bash
make linux-deployment-guides
make windows-deployment-guides
make macos-arm64-deployment-guides
make macos-amd64-deployment-guides
```

Generated deployment files are build artifacts. Edit source templates, Markdown guides, or Makefile variables instead of editing generated copies.

# Deployment Build Output

Deployment packages continue to contain `config.yaml` because it is the first-run seed/rollback source. `gateway.sqlite3` is **not** prebuilt into deployment packages; the installed gateway creates and initializes the authoritative database on first start at the platform-specific persistent path.


## Linux

```text
build/linux/
├── fbs-interlock-gateway
├── config.yaml
├── fbs-interlock-gateway.service
├── install.sh
├── install-dev.sh
├── uninstall.sh
├── update.sh
├── fbs-interlock-gateway-update.service
├── fbs-interlock-gateway-update.timer
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── Linux Install Instructions.pdf
```

## Windows

```text
build/windows/
├── fbs-interlock-gateway.exe
├── config.yaml
├── install.bat
├── install-dev.bat
├── install.ps1
├── start.bat
├── update.bat
├── update.ps1
├── uninstall.bat
├── uninstall.ps1
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── Windows Install Instructions.pdf
```

## macOS ARM64

```text
build/darwin/arm64/
├── fbs-interlock-gateway
├── config.yaml
├── install.sh
├── install-dev.sh
├── start.sh
├── uninstall.sh
├── update.sh
├── com.williamveith.fbs-interlock-gateway.plist
├── com.williamveith.fbs-interlock-gateway-update.plist
├── com.williamveith.fbs-interlock-gateway.pf
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── macOS Install Instructions.pdf
```

## macOS AMD64

```text
build/darwin/amd64/
├── fbs-interlock-gateway
├── config.yaml
├── install.sh
├── install-dev.sh
├── start.sh
├── uninstall.sh
├── update.sh
├── com.williamveith.fbs-interlock-gateway.plist
├── com.williamveith.fbs-interlock-gateway-update.plist
├── com.williamveith.fbs-interlock-gateway.pf
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── macOS Install Instructions.pdf
```

The exact PDF filenames follow the matching source Markdown basenames. Generated deployment files are build artifacts; edit their templates, guides, or Makefile variables and rebuild.

# Release Binaries

Build all local release binaries and SHA-256 checksum files through the validation gate:

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

The local `make release` output contains executables and their `.sha256` files. The GitHub release workflow later adds detached Ed25519 `.sha256.sig` files **after** the trusted build and SLSA provenance have been verified. Generated TLS trust, certificates, CA material, and private keys are never GitHub release assets.

Local release files are written to:

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

A published production release contains the five executables, five checksum files, and five detached signatures:

```text
<asset>
<asset>.sha256
<asset>.sha256.sig
```

The signature is a raw Ed25519 detached signature over the exact bytes of the corresponding `.sha256` file. Updaters authenticate the checksum with the public key embedded in the currently installed gateway before the checksum is used to authorize a candidate binary.

Display embedded metadata:

```bash
./build/release/fbs-interlock-gateway-linux-amd64 -version
```

Output format:

```text
fbs-interlock-gateway version=<version> commit=<commit> date=<UTC-build-time>
```

The executable also provides the internal updater-verification command:

```bash
fbs-interlock-gateway verify-update-checksum \
  --checksum <asset>.sha256 \
  --signature <asset>.sha256.sig \
  --asset <asset>
```

On success, the command prints only the authenticated lowercase SHA-256 digest and exits with status `0`.

# Service Templates and Installed Layouts

## Linux Templates

```text
services/linux/
├── app.service.in
├── install-linux.sh.in
├── install-linux-dev.sh.in
├── uninstall-linux.sh.in
├── update-linux.sh.in
├── update.service.in
└── update.timer.in
```

Installed layout:

```text
/opt/fbs-interlock-gateway/
├── fbs-interlock-gateway
├── uninstall.sh
└── update.sh                 # production mode

/etc/fbs-interlock-gateway/
├── config.yaml               # first-run seed / generated rollback mirror
├── config.yaml.bak
└── tls/
    ├── server-ca.crt
    ├── gateway-client.crt
    └── gateway-client.key

/var/lib/fbs-interlock-gateway/
└── gateway.sqlite3           # authoritative configuration

/etc/systemd/system/
├── fbs-interlock-gateway.service
├── fbs-interlock-gateway-update.service
└── fbs-interlock-gateway-update.timer
```

The production Linux installer:

- verifies or installs `lsof`, `curl`, `ca-certificates`, and `ufw`
- configures UFW default-deny inbound behavior and the authorized FBS source/range rule
- verifies all packaged runtime TLS files and validates the gateway binary
- creates the service user/group and the writable `/var/lib/fbs-interlock-gateway` state directory
- stops the existing service/update units before backing up replacement-sensitive state
- backs up the installed application/service files and `gateway.sqlite3` for installation rollback
- preserves the existing YAML rollback source/mirror and installed TLS identity
- installs a systemd service that passes both `-config` and the authoritative `/var/lib/.../gateway.sqlite3` `-db` path
- verifies that the service account can read the YAML/TLS files and can create state files in the database directory
- starts the gateway, waits for the Admin API, and requires a non-empty service-account-readable/writable SQLite database
- enables the hourly update timer only after the production gateway has passed health/database validation
- restores the previous application, database, units, and service state when installation validation fails

The development Linux installer is a thin wrapper around the same installer with `--development`. It applies the same dependency, firewall, binary, SQLite, YAML, TLS, and service setup while disabling/removing managed updater units so a local build is not replaced.

The systemd service runs from `/etc/fbs-interlock-gateway`, uses `StateDirectory=fbs-interlock-gateway` for mutable SQLite state under `/var/lib`, writes to journald, restarts after exits with bounded rapid-restart behavior, and applies `NoNewPrivileges=true`.

The standard uninstaller removes the executable, updater, systemd units, and gateway-specific UFW rule while preserving `/var/lib/fbs-interlock-gateway/gateway.sqlite3`, `/etc/fbs-interlock-gateway/config.yaml`, the installed `tls/` directory, and the service account. `--purge` removes both the configuration and state directories while still preserving the service account and normal journal history.

## Windows Templates

```text
services/windows/
├── install.bat.in
├── install-dev.bat.in
├── install.ps1.in
├── start.bat.in
├── update.bat.in
├── update.ps1.in
├── uninstall.bat.in
└── uninstall.ps1.in
```

Installed layout:

```text
C:\FBS\fbs-interlock-gateway\
├── fbs-interlock-gateway.exe
├── gateway.sqlite3           # authoritative configuration
├── config.yaml               # first-run seed / generated rollback mirror
├── config.yaml.bak
├── start.bat
├── update.bat                # production mode only
├── update.ps1                # production mode only
├── tls\
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── logs\
    ├── gateway.log
    ├── gateway-error.log
    ├── update.log
    └── update-error.log
```

The Windows installer:

- elevates through User Account Control and validates a runnable AMD64 PE binary
- backs up existing executable files, Task Scheduler definitions, and `gateway.sqlite3` for rollback
- preserves the existing YAML rollback source/mirror and installed TLS identity
- applies restricted NTFS access to Administrators, `SYSTEM`, and the runtime account
- gives `NT AUTHORITY\LOCAL SERVICE` the modification rights required for SQLite and atomic compatibility-mirror replacement while keeping TLS private material restricted
- runs the gateway task as `NT AUTHORITY\LOCAL SERVICE`
- runs the production update task as `SYSTEM`
- uses `start.bat` as a two-second restart supervisor and passes explicit `-config` and `-db` paths
- sets default inbound firewall behavior to block and installs a source-, port-, and executable-restricted allow rule
- starts the gateway, validates the Admin API, and requires a non-empty SQLite database
- restores the previous database/files/task definitions when installation validation fails
- installs the hourly updater only in production mode

Development installation removes the managed update task and scripts while preserving the normal gateway task, SQLite configuration, YAML rollback mirror, TLS files, permissions, and firewall controls.

The standard uninstaller removes tasks, running processes, application/updater files, executable backups, and gateway firewall rules while preserving `gateway.sqlite3`, `config.yaml`, the installed `tls\` directory, and `logs\`. Purge mode removes the preserved SQLite configuration, YAML configuration, TLS files, and logs.

## macOS Templates

```text
services/macos/
├── com.williamveith.fbs-interlock-gateway.plist.in
├── com.williamveith.fbs-interlock-gateway-update.plist.in
├── fbs-interlock-gateway.pf.in
├── install-macos.sh.in
├── install-macos-dev.sh.in
├── start.sh.in
├── update-macos.sh.in
└── uninstall-macos.sh.in
```

Installed layout:

```text
/usr/local/libexec/fbs-interlock-gateway/
├── fbs-interlock-gateway
├── start.sh
└── update.sh                 # production mode only

/Library/Application Support/fbs-interlock-gateway/
├── gateway.sqlite3           # authoritative configuration
├── config.yaml               # first-run seed / generated rollback mirror
├── config.yaml.bak
└── tls/
    ├── server-ca.crt
    ├── gateway-client.crt
    └── gateway-client.key

/Library/LaunchDaemons/
├── com.williamveith.fbs-interlock-gateway.plist
└── com.williamveith.fbs-interlock-gateway-update.plist  # production mode only

/etc/pf.anchors/com.williamveith.fbs-interlock-gateway
/Library/Logs/fbs-interlock-gateway/
├── gateway.log
├── gateway-error.log
├── update.log
└── update-error.log
```

The macOS installer:

- validates the operating system, current architecture, packaged Mach-O executable, scripts, property lists, Packet Filter anchor, and TLS files before replacement
- creates the hidden non-login `_fbs-gateway` account/group when needed
- stops the existing LaunchDaemons before backing up mutable SQLite state
- backs up `gateway.sqlite3` together with the executable, wrappers, plists, and Packet Filter state for installation rollback
- preserves the existing YAML rollback source/mirror and installed TLS identity
- verifies that `_fbs-gateway` can create/remove files in the Application Support directory
- installs a startup wrapper that passes explicit `-config` and `-db` paths
- enables the gateway LaunchDaemon before bootstrapping it, then starts it
- waits for the Admin API and requires a non-empty database that `_fbs-gateway` can read and write
- installs the main LaunchDaemon and the hourly update LaunchDaemon in production mode
- registers the executable with the Application Firewall
- installs and validates a managed `pf` anchor and managed block in `/etc/pf.conf`
- creates separate gateway and updater stdout/stderr logs
- rolls back the executable, wrappers, plists, database, Packet Filter files, and prior service state when installation validation fails

Development installation preserves all production runtime security controls but removes/disables the updater and update LaunchDaemon.

Standard uninstall removes executables, LaunchDaemons, updater, Packet Filter anchor, and the managed `pf.conf` block while preserving `gateway.sqlite3`, the YAML rollback/compatibility files, TLS data, logs, and the service account. Purge mode removes persistent configuration/database state and logs as documented in the macOS deployment guide.

Detailed procedures are maintained in:

```text
docs/deployment guides/Linux Install Instructions.md
docs/deployment guides/Windows Install Instructions.md
docs/deployment guides/macOS Install Instructions.md
```

Deployment packages contain rendered PDF copies of the matching guides.

# Automatic Updates and Log Maintenance

Production installers enable managed hourly release checks. Development installers keep the locally built gateway and disable managed release replacement. The updater never intentionally replaces the authoritative configuration with release content: release assets contain the application executable, while persistent SQLite/YAML/TLS state remains host-local.

The update protocol is deliberately lightweight. Every normal hourly check downloads only the architecture-specific `.sha256` file and its **64-byte Ed25519 detached signature**. The multi-megabyte executable is downloaded only when the authenticated SHA differs from the installed executable.

```text
hourly check
    |
    +-> download <asset>.sha256
    +-> download <asset>.sha256.sig
    |
    v
CURRENT installed gateway
verify-update-checksum
    |
    +-> invalid signature / asset / SHA -> fail closed
    |
    v
compare installed SHA
    |
    +-> same -> no binary download
    |
    v
download candidate
    |
    +-> SHA must match authenticated checksum
    +-> candidate version must be stable SemVer
    +-> authenticated downgrade is rejected
    |
    v
backup -> install/stage -> verify installed SHA
    |
    +-> restart/health failure -> rollback
```

The updaters do not require GitHub CLI, Cosign, GPG, OpenSSL, or another signature-verification package on deployed hosts. Ed25519 verification is performed by the already-installed gateway executable using Go's standard-library implementation and the embedded update-trust public key.

Because the current executable is the trust anchor, the signed-update updater requires an existing trusted gateway binary that implements `verify-update-checksum`. A fresh installation or the documented one-time bootstrap is required before switching an older deployment to the hardened updater.

Equal semantic versions are permitted when their bytes differ, allowing an authenticated release to repair a locally modified or corrupted binary. Lower semantic versions are rejected automatically.

## Linux

The generated systemd timer runs after boot and then hourly.

The Linux updater:

1. selects the matching Linux AMD64 or ARM64 release asset
2. downloads only the matching `.sha256` and `.sha256.sig` metadata for the normal hourly check
3. requires HTTPS/TLS 1.2 or newer for GitHub release downloads
4. invokes the currently installed gateway to authenticate the checksum signature and asset name
5. hashes the installed executable and exits without downloading or restarting when it already matches
6. downloads the candidate binary only when the authenticated SHA differs
7. verifies the candidate SHA against the authenticated checksum
8. reads current and candidate `-version` metadata and rejects stable-SemVer downgrades
9. stops the gateway and backs up both the installed executable and the authoritative `gateway.sqlite3` when present
10. installs and re-hashes the final executable
11. restarts the service, waits for the Admin API, and requires a non-empty SQLite database
12. restores the previous executable and database when installation or post-update health/database validation fails

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

## Windows

Production installation creates the `FBS Interlock Gateway Update` Task Scheduler task. It begins at minute `17` and repeats once per hour under `SYSTEM`.

Each run:

1. acquires an update lock and cleans a stale lock after the configured age
2. downloads the Windows AMD64 `.sha256` and 64-byte `.sha256.sig`
3. forces TLS 1.2 for Windows PowerShell 5.1 GitHub downloads
4. uses the currently installed `fbs-interlock-gateway.exe` to authenticate the checksum
5. compares the authenticated SHA with `Get-FileHash` of the installed executable
6. skips the executable download when the SHA already matches
7. verifies the candidate SHA, Windows PE format, and AMD64 machine type
8. reads current and candidate version metadata and rejects authenticated downgrades
9. preserves the existing executable ACL, creates a timestamped executable backup, and backs up `gateway.sqlite3` plus its ACL when the database exists
10. stages the replacement and verifies the staged/final executable hashes
11. restarts the gateway task and waits for the Admin API and database health checks
12. restores the previous executable/ACL and SQLite database/ACL when post-update validation fails
13. rotates `gateway.log` and `gateway-error.log` when either reaches 10 MiB

Windows retains up to 30 numbered ZIP archives per gateway log.

Run the update task manually:

```powershell
Start-ScheduledTask -TaskName "FBS Interlock Gateway Update"
```

## macOS

Production installation creates `system/com.williamveith.fbs-interlock-gateway-update`. The LaunchDaemon runs at minute `17` of every hour with low-priority I/O.

Each run:

1. acquires an update lock
2. detects Apple Silicon or Intel architecture
3. downloads the matching `.sha256` and 64-byte `.sha256.sig`
4. requires HTTPS/TLS 1.2 or newer for GitHub release downloads
5. uses the currently installed gateway to authenticate the checksum
6. hashes the installed binary and skips the candidate download when it already matches and no log maintenance is required
7. verifies the candidate SHA and Mach-O architecture
8. reads current and candidate version metadata and rejects authenticated downgrades
9. creates a timestamped executable backup and stages the replacement
10. verifies the staged and final installed binary hashes
11. restarts the gateway only when needed and waits for the Admin API health check
12. restores the previous binary when installation or health validation fails
13. rotates `gateway.log` and `gateway-error.log` when either reaches 10 MiB

The macOS updater does not intentionally replace `gateway.sqlite3`, `config.yaml`, or TLS identity files. The macOS installer separately includes `gateway.sqlite3` in its installation rollback snapshot.

macOS retains up to 30 numbered gzip archives per gateway log.

Run maintenance manually:

```bash
sudo /usr/local/libexec/fbs-interlock-gateway/update.sh
```

Use the packaged development installer on any platform when testing an unpublished local build, then run the normal production installer to restore managed updates.

# Continuous Integration

The CI workflow runs for pull requests and pushes to `main`.

It:

1. checks out the repository with `actions/checkout` pinned to a full immutable commit SHA
2. installs the Go version declared in `go.mod` with `actions/setup-go` pinned to a full commit SHA
3. runs the unified `make verify` gate, including Staticcheck and ShellCheck
4. executes the Linux AMD64 binary with `-version`
5. verifies the generated formats for:
   - Linux AMD64
   - Linux ARM64
   - Windows AMD64
   - macOS ARM64
   - macOS AMD64

The macOS and Windows binaries are cross-compiled and format-checked on the Linux runner. They are not executed by that runner.

GitHub Actions dependencies are pinned to full commit SHAs rather than mutable major-version tags. Dependabot is configured to check:

- Go modules every Monday at 06:00 America/Chicago, grouping minor/patch updates
- GitHub Actions every Monday at 06:15 America/Chicago, grouping minor/patch updates

Dependabot pull requests still pass through the normal repository validation/review path.

# Release Workflow

Releases are created through the manually triggered **Validate, Attest, Sign, Tag, and Release** GitHub Actions workflow. The caller uses least-privilege job permissions and delegates build/provenance generation to the separately maintained reusable workflow in `williamveith/fbs-interlock-gateway-builder`, pinned to an exact full commit SHA.

The workflow is split into three trust stages.

## Preflight

The preflight job:

1. requires `refs/heads/main`
2. checks out complete history without persisting checkout credentials
3. validates the requested release version
4. rejects a tag that already exists

## Trusted build and provenance

The caller invokes the pinned reusable trusted builder with only the permissions required for artifact/provenance work. The trusted builder performs the full repository validation and release build, validates the expected assets and embedded metadata, and generates SLSA v1 provenance for the release binaries and checksum files using GitHub-hosted infrastructure and GitHub OIDC/Sigstore.

The build/provenance stage does not receive the long-lived GPG release-signing key or Ed25519 update-signing private key.

## Verify, authorize, sign, and publish

The protected `release` environment publish job:

1. downloads the exact artifacts produced by the trusted builder
2. re-verifies every SHA-256 checksum
3. verifies SLSA provenance for every binary and checksum subject
4. requires the expected repository, exact source commit, `refs/heads/main`, exact reusable-builder workflow and builder commit, SLSA v1 predicate, and GitHub-hosted runner
5. loads the Ed25519 update-signing private key from the protected release environment
6. signs each authenticated `.sha256` file and verifies every generated 64-byte signature
7. executes the built Linux AMD64 binary's `verify-update-checksum` command to prove that the embedded public key matches the configured private signing key
8. imports the protected GPG key only after build/provenance/update-authorization checks have succeeded
9. creates and locally verifies a GPG-signed annotated tag at the exact source commit
10. pushes the signed tag
11. creates the GitHub Release with all five binaries, five checksum files, and five detached Ed25519 signatures
12. waits for and verifies the immutable GitHub release attestation
13. verifies every published release asset against the immutable-release attestation

Required `release` environment secrets:

```text
GPG_PRIVATE_KEY
GPG_PASSPHRASE
UPDATE_SIGNING_PRIVATE_KEY
```

Required `release` environment variables used by GPG import/tagging:

```text
GPG_FINGERPRINT
GIT_COMMITTER_NAME
GIT_COMMITTER_EMAIL
```

The update-signing private key must correspond exactly to the public key committed at:

```text
internal/updateauth/update-signing-public.pem
```

The release workflow fails before publication when those keys do not match.

Start the workflow with GitHub CLI:

```bash
gh workflow run release.yml \
  --ref main \
  -f version=<version>
```

After it succeeds, verify the signed Git tag and immutable release:

```bash
git fetch origin --tags
git tag -v <version>

gh release verify <version> \
  --repo williamveith/fbs-interlock-gateway
```

Verify a downloaded release asset against the immutable release attestation:

```bash
gh release verify-asset <version> \
  <asset> \
  --repo williamveith/fbs-interlock-gateway
```

For a stronger build-origin check, verify SLSA provenance with the exact trusted-builder workflow revision and source commit recorded for that release:

```bash
gh attestation verify <asset> \
  --repo williamveith/fbs-interlock-gateway \
  --signer-workflow williamveith/fbs-interlock-gateway-builder/.github/workflows/build-release.yml \
  --signer-digest <trusted-builder-commit-sha> \
  --source-ref refs/heads/main \
  --source-digest <release-source-commit-sha> \
  --predicate-type https://slsa.dev/provenance/v1 \
  --deny-self-hosted-runners
```

This release architecture is designed around a separated trusted-builder/provenance boundary; it should be described as a SLSA-hardened or SLSA Build Level 3-oriented design rather than as an external certification.

# Branch and Pull-Request Workflow

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

# Local Testing

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
  -config ./config.yaml \
  -db ./gateway.sqlite3
```

On the first run, an uninitialized `gateway.sqlite3` imports the YAML. After that, SQLite is authoritative; editing `config.yaml` alone does not change the running configuration. Remove the local test database only when you intentionally want to exercise first-run migration again.

Run with an explicit Admin address:

```bash
go run ./cmd/fbs-interlock-gateway \
  -config ./config.yaml \
  -db ./gateway.sqlite3 \
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

Export the authoritative local test database:

```bash
go run ./cmd/fbs-interlock-gateway config export   -db ./gateway.sqlite3   -output ./config-export.yaml
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

# Runtime Behavior

On startup, the gateway:

1. parses `-db`, `-config`, `-admin`, and `-version`
2. resolves the SQLite path from explicit `-db`, then `FBS_GATEWAY_DB_PATH`, then `gateway.sqlite3` beside the YAML path
3. opens/configures SQLite, applies supported schema migrations, and performs `PRAGMA quick_check(1)`
4. loads configuration from SQLite when initialized; otherwise loads, normalizes, defaults, validates, and transactionally imports the legacy YAML source
5. reloads the accepted configuration from SQLite and treats the database as authoritative thereafter
6. deep-clones the accepted runtime configuration
7. initializes the shared status store with safe-output placeholders
8. loads the configured Shelly server CA and gateway client identity when TLS is configured
9. validates enabled tools, protocols, listener ports, and required TLS fields
10. starts the Admin server unless disabled
11. starts one FBS-facing HTTP listener per enabled tool
12. maps each gateway port to one configured interlock
13. logs inbound FBS requests and outbound responses
14. uses HTTP or HTTPS, optional mutual TLS, and optional Digest Authentication per tool
15. records every completed Shelly result in the shared Admin status store
16. reports the configured safe state and records the error when a device request fails
17. shuts down cleanly on interrupt, termination, Admin restart request, or server error

During operation:

- ordinary FBS/Shelly requests use the in-memory configuration; SQLite is not queried on every request
- ordinary Admin status polling reads memory only
- an explicit Admin refresh queries enabled tools with up to 32 workers
- refresh results are published independently as each device completes
- completed partial refresh results remain stored when the overall scan reaches its context deadline
- FBS requests passively update only the affected tool row
- FBS status and set requests take priority over Admin status probes for the same Shelly
- an Admin probe never queues behind another device request and can be canceled by arriving FBS traffic
- Admin deferral preserves the current row while the FBS operation supplies the authoritative update
- per-device revisions prevent stale scan results from replacing newer FBS results
- requests to one Shelly are serialized and may reuse an idle connection, TLS session, and Digest session
- transient status failures may receive one retry
- persistent Shelly HTTP `423` or `429` conditions may schedule a cooldown-protected reboot request
- configuration snapshots are independent deep copies rather than aliases of gateway-owned slices or credential pointers

Disabled tools do not receive FBS listeners and are not contacted by explicit status refreshes.

## Port Ownership Warning

Before starting an enabled listener, the gateway may clear a process already using that configured port. Use dedicated gateway ports and confirm that unrelated services do not use the configured range.

## Configuration Reload Behavior

A successful Admin UI save validates the complete configuration and commits it through the configured persistence store. Production runtime uses the SQLite store, so the authoritative database is replaced transactionally and the generated YAML compatibility mirror is refreshed afterward. The Admin save then requests a process restart.

The installed platform supervisor rebuilds runtime listeners, the shared status store, the Shelly transport, TLS trust, Digest state, and per-device scheduling state by starting the process again. Manual `config import` also updates the authoritative database transactionally, but the operator must restart the supervised gateway afterward.

# Logging

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

They also report whether the HTTP connection was reused and whether the TLS session resumed. FBS status and set failures are logged before the configured safe state is returned.

Platform logs:

```text
Linux gateway:
  sudo journalctl -u fbs-interlock-gateway.service -f

Linux updater:
  sudo journalctl -u fbs-interlock-gateway-update.service

Windows gateway stdout:
  C:\FBS\fbs-interlock-gateway\logs\gateway.log

Windows gateway stderr:
  C:\FBS\fbs-interlock-gateway\logs\gateway-error.log

Windows updater stdout/stderr:
  C:\FBS\fbs-interlock-gateway\logs\update.log
  C:\FBS\fbs-interlock-gateway\logs\update-error.log

macOS gateway stdout/stderr:
  /Library/Logs/fbs-interlock-gateway/gateway.log
  /Library/Logs/fbs-interlock-gateway/gateway-error.log

macOS updater stdout/stderr:
  /Library/Logs/fbs-interlock-gateway/update.log
  /Library/Logs/fbs-interlock-gateway/update-error.log
```

The Windows and macOS production updaters rotate `gateway.log` and `gateway-error.log` at 10 MiB and retain up to 30 compressed archives per file. Linux runtime retention remains controlled by the host's journald configuration.

# Repository Safety

Ignored local artifacts currently include:

```gitignore
.DS_Store
build
pki
config.yaml
config.yaml.bak
*.db-journal
*.db-wal
*.db-shm
```

`pki/` contains private CA material, gateway certificate requests/identity material, and per-device Shelly keys. `build/` contains generated binaries, deployment scripts, service definitions, copied runtime TLS material, generated update files, and deployment-guide PDFs.

`gateway.sqlite3` is authoritative production configuration and can contain Shelly credentials. Production databases, non-redacted configuration exports, generated compatibility YAML containing credentials, and SQLite sidecar files must not be committed to source control.

There is no repository-root generated `tls/` staging directory. Platform deployment builds copy the required runtime trust/identity files directly from `pki/ca/` and `pki/gateway/` into their generated `build/.../tls/` directories.

Committed security material is limited to non-secret trust/configuration source such as certificate templates and `internal/updateauth/update-signing-public.pem`. The Ed25519 update-signing **private** key, GPG private key, CA private keys, production configuration/database state, active credentials, generated certificates/CSRs, and production mappings must remain outside version control.

`SECURITY.md` directs vulnerability reports to GitHub Private Vulnerability Reporting and defines operational-safety restrictions for testing against a system that can affect physical interlocks.

# License

This project is available under the
[FBS Interlock Gateway Public Benefit Source License 1.0](LICENSE.md).

Eligible individuals and organizations may use the project without charge.
Current and former employers of the Licensor require a separate written
license.

See [Commercial and Employer Licensing](COMMERCIAL-LICENSING.md) for details.
