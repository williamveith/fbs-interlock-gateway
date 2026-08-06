---
title: "FBS Interlock Gateway"
subtitle: "macOS Installation and Operations Guide"
author: "Deployment Guide"
date: ""
lang: en-US
---

> **Purpose**
>
> This guide covers building, transferring, installing, validating, operating, updating, and uninstalling `fbs-interlock-gateway` on a dedicated macOS gateway computer.

> **Security boundary**
>
> The gateway controls software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. The installer restricts the FBS listener range with a managed macOS Packet Filter (`pf`) anchor, but the Admin UI must remain bound to loopback unless remote access is intentionally secured.

## Table of Contents

- [Deployment Overview](#deployment-overview)
- [Supported Mac Architectures](#supported-mac-architectures)
- [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Prepare Gateway TLS Material](#prepare-gateway-tls-material)
- [Build the Deployment Assets](#build-the-deployment-assets)
- [Transfer the Deployment Directory](#transfer-the-deployment-directory)
- [Choose an Installation Mode](#choose-an-installation-mode)
- [Install the Gateway](#install-the-gateway)
- [What the Installer Does](#what-the-installer-does)
- [Installed Layout](#installed-layout)
- [Verify the Installation](#verify-the-installation)
- [Verify Tool Communication](#verify-tool-communication)
- [View the Admin Panel](#view-the-admin-panel)
- [Automatic Updates and Log Maintenance](#automatic-updates-and-log-maintenance)
- [View Gateway Logs](#view-gateway-logs)
- [Edit the Configuration](#edit-the-configuration)
- [Restart the Gateway](#restart-the-gateway)
- [Firewall and Packet Filter Behavior](#firewall-and-packet-filter-behavior)
- [Gatekeeper and Quarantine](#gatekeeper-and-quarantine)
- [Troubleshooting](#troubleshooting)
- [Uninstall the Gateway](#uninstall-the-gateway)
- [Command Reference](#command-reference)

<div class="page-break"></div>

# Deployment Overview

The macOS deployment uses two system-wide `launchd` jobs:

1. The gateway LaunchDaemon starts before a user signs in and keeps the gateway process running.
2. The production update LaunchDaemon checks the latest published release once per hour and performs binary and log maintenance when required.

```text
FBS server
    -> managed macOS pf source restriction
    -> macOS gateway listener port
    -> fbs-interlock-gateway
    -> Shelly RPC over HTTP or HTTPS
    -> tool interlock circuit
```

The installed Admin UI remains local by default:

```text
http://127.0.0.1:18090
```

Normal FBS `/status`, `/on`, and `/off` requests update the Admin status cache for the affected tool. The browser reads that in-memory cache without generating recurring Shelly requests. The **Refresh Status** action performs an explicit fleet-wide verification.

## Deployment sequence

1. Identify the Mac architecture.
2. Generate the required gateway runtime TLS files.
3. Build the matching deployment directory.
4. Copy the complete deployment directory to the gateway Mac.
5. Choose production or development installation mode.
6. Run the installer with administrator privileges.
7. Verify the gateway LaunchDaemon, update LaunchDaemon, Admin API, TLS access, `pf` rules, tool ports, and logs.

# Supported Mac Architectures

Determine the gateway Mac architecture:

```bash
uname -m
```

| Output | Mac type | Build target | Deployment directory |
| --- | --- | --- | --- |
| `arm64` | Apple Silicon | `make build-darwin-arm64` | `build/darwin/arm64/` |
| `x86_64` | Intel | `make build-darwin-amd64` | `build/darwin/amd64/` |

> **Important**
>
> Use the deployment directory that matches the gateway Mac. The installer validates the Mach-O architecture before replacing any installed files and exits when the binary does not match the machine.

# Pre-Deployment Checklist

Before building or installing, confirm the following:

- The repository is on the intended commit or release.
- `make verify` completes successfully.
- `config.yaml` contains the intended non-production or production configuration.
- `make ca` and `make gateway-cert` have populated the repository `tls/` directory.
- The selected build target matches the gateway Mac architecture.
- The configured FBS listener ports do not conflict with other services.
- `FBS_SOURCE_IP` and `FBS_PORT_RANGE` in the Makefile are correct before building.
- The Admin address remains `127.0.0.1:18090` unless a reviewed remote-access design is being used.
- The gateway Mac can resolve every configured Shelly hostname.
- The gateway Mac can reach GitHub Releases when managed automatic updates are required.
- The deployment directory will be transferred and stored as a complete unit.

Current generated firewall defaults are:

```text
Authorized FBS source: 146.6.76.61
Gateway listener range: TCP 8081:8981
```

Review the generated Packet Filter file before production installation:

```text
com.williamveith.fbs-interlock-gateway.pf
```

# Prepare Gateway TLS Material

All Linux and macOS deployment builds now require the gateway runtime TLS files. This keeps the deployment packages consistent and ensures they are ready for HTTPS or mutual-TLS Shelly communication.

Generate the private certificate authorities and gateway client identity on the controlled development machine:

```bash
make ca
make gateway-cert
```

The runtime files are staged under:

```text
tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

The macOS build automatically copies these three files into the architecture-specific deployment directory. Do not manually copy the complete `pki/` directory.

> **Private-key handling**
>
> Never place CA private keys, certificate requests, or Shelly server private keys on the gateway Mac. Only the gateway runtime server CA certificate, gateway client certificate, and gateway client private key are packaged.

The build fails before producing a macOS deployment package when any required runtime TLS file is missing.

# Build the Deployment Assets

On the development Mac, start from a clean build directory.

## Apple Silicon

```bash
make clean
make build-darwin-arm64
```

Generated directory:

```text
build/darwin/arm64/
```

## Intel

```bash
make clean
make build-darwin-amd64
```

Generated directory:

```text
build/darwin/amd64/
```

The selected deployment directory contains:

```text
build/darwin/<architecture>/
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
└── macOS Installation and Operations Guide.pdf
```

The staged TLS modes are:

| File | Staged mode |
| --- | --- |
| `server-ca.crt` | `0644` |
| `gateway-client.crt` | `0644` |
| `gateway-client.key` | `0600` |

> **Do not edit generated deployment files directly.**
>
> Update the source templates, deployment guide, or Makefile variables and rebuild instead.

# Transfer the Deployment Directory

## Copy to a USB drive

Copy the complete architecture-specific deployment directory to a USB flash drive.

Do not copy only the executable. The complete directory contains:

- The application executable
- Production and development installers
- The startup wrapper
- The gateway and update LaunchDaemon property lists
- The checksum-aware updater
- The Packet Filter anchor
- The uninstaller
- The configuration file
- The gateway runtime TLS files
- The deployment guide PDF

## Copy to the gateway Mac

Insert the USB drive into the gateway Mac and copy the complete directory into the current user's `Downloads` directory.

Apple Silicon example:

```text
~/Downloads/darwin/arm64/
```

Intel example:

```text
~/Downloads/darwin/amd64/
```

Confirm the complete package is present:

```bash
find ~/Downloads/darwin/arm64 -maxdepth 2 -print
```

<div class="page-break"></div>

# Choose an Installation Mode

The deployment package supports two installation modes.

## Production installation

Use the normal installer for a managed production gateway:

```bash
sudo ./install.sh
```

Production mode:

- Installs the local gateway binary
- Installs and enables the hourly update LaunchDaemon
- Installs `update.sh`
- Performs checksum-aware updates from GitHub Releases
- Rotates gateway logs when they reach the configured size limit
- Restores managed updates after a previous development installation

## Development installation

Use the development installer when testing an unpublished local build:

```bash
sudo ./install-dev.sh
```

This is equivalent to:

```bash
sudo ./install.sh --development
```

Development mode:

- Installs the local gateway binary and all production security controls
- Preserves the production configuration and installed TLS files
- Stops and disables the update LaunchDaemon
- Removes the installed updater script and update plist
- Prevents the development binary from being replaced by the latest published release

Run the normal production installer later to restore managed automatic updates.

# Install the Gateway

Open Terminal and move to the copied deployment directory.

Apple Silicon example:

```bash
cd ~/Downloads/darwin/arm64
```

Intel example:

```bash
cd ~/Downloads/darwin/amd64
```

Make the deployment files executable:

```bash
chmod +x \
  install.sh \
  install-dev.sh \
  start.sh \
  uninstall.sh \
  update.sh \
  fbs-interlock-gateway
```

Run the selected installer.

Production:

```bash
sudo ./install.sh
```

Development:

```bash
sudo ./install-dev.sh
```

Enter the administrator password when prompted.

The installer validates the packaged binary architecture and executes its `-version` command before stopping the existing gateway.

> **Reinstallation behavior**
>
> Reinstallation preserves the active production `config.yaml` and installed TLS files. It corrects their ownership and modes but does not replace their contents.

# What the Installer Does

The installer performs the following operations.

## Preflight validation

- Verifies that the installer is running on macOS
- Elevates through `sudo` when required
- Requires the binary, config, startup wrapper, gateway plist, Packet Filter anchor, and all three TLS files
- Requires the updater and update plist in production mode
- Validates generated property lists with `plutil`
- Confirms the binary is a Mach-O executable for the current architecture
- Executes the binary's `-version` command

## Service account

- Creates the hidden `_fbs-gateway` group and service account when needed
- Uses a non-login shell and `/var/empty` home directory
- Preserves the account during uninstall so a later reinstall keeps stable ownership

## Application and configuration

- Installs the binary and startup wrapper under `/usr/local/libexec/fbs-interlock-gateway/`
- Runs the gateway with `/Library/Application Support/fbs-interlock-gateway` as its working directory
- Installs or preserves `config.yaml` with service-account ownership and mode `0640`
- Removes the executable quarantine attribute when present

## Gateway TLS files

- Creates `/Library/Application Support/fbs-interlock-gateway/tls/`
- Installs new runtime TLS files with `root:_fbs-gateway` ownership and mode `0640`
- Preserves existing installed TLS files during reinstallation
- Verifies that `_fbs-gateway` can read the config and every TLS file

## LaunchDaemons

- Installs the main gateway LaunchDaemon
- Uses a ten-second restart throttle
- Uses a 30-second exit timeout and restrictive process umask
- Starts the gateway immediately and waits for the Admin API health check
- Installs the update LaunchDaemon only in production mode
- Schedules production updates at minute 17 of every hour

## Firewall controls

- Adds the executable to the macOS Application Firewall allow list
- Installs a named `pf` anchor
- Adds a managed block to `/etc/pf.conf`
- Validates the complete generated `pf` configuration before replacing `/etc/pf.conf`
- Reloads Packet Filter and enables it when necessary
- Allows loopback access to the listener range
- Allows the configured FBS source IP to the listener range
- Blocks every other inbound TCP connection to that range

## Logs

Creates:

```text
/Library/Logs/fbs-interlock-gateway/
├── gateway.log
├── gateway-error.log
├── update.log
└── update-error.log
```

Gateway logs are owned by the gateway service account. Update logs are owned by `root:wheel`.

## Rollback and health validation

Before replacing an existing installation, the installer backs up the currently installed binary, wrappers, plists, Packet Filter anchor, and `/etc/pf.conf` into a temporary rollback directory.

If Packet Filter validation, LaunchDaemon loading, or the Admin API health check fails, the installer restores the previous executable and service/network files and attempts to restart the previous installation.

The preserved production config and TLS files are not replaced during normal installation.

# Installed Layout

## Application directory

```text
/usr/local/libexec/fbs-interlock-gateway/
├── fbs-interlock-gateway
├── start.sh
└── update.sh                 # production mode only
```

## Configuration and TLS

```text
/Library/Application Support/fbs-interlock-gateway/
├── config.yaml
└── tls/
    ├── server-ca.crt
    ├── gateway-client.crt
    └── gateway-client.key
```

## LaunchDaemons

```text
/Library/LaunchDaemons/
├── com.williamveith.fbs-interlock-gateway.plist
└── com.williamveith.fbs-interlock-gateway-update.plist
```

The update plist is absent after a development installation.

## Packet Filter

```text
/etc/pf.anchors/com.williamveith.fbs-interlock-gateway
/etc/pf.conf
```

The installer places a clearly marked managed anchor block in `/etc/pf.conf` rather than replacing unrelated Packet Filter rules.

## Logs

```text
/Library/Logs/fbs-interlock-gateway/
├── gateway.log
├── gateway-error.log
├── update.log
└── update-error.log
```

<div class="page-break"></div>

# Verify the Installation

## Check the installed version

```bash
sudo "/usr/local/libexec/fbs-interlock-gateway/fbs-interlock-gateway" \
  -version
```

## Check the gateway LaunchDaemon

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway
```

Look for a running state and process identifier:

```text
state = running
pid = <process-id>
```

## Check the update LaunchDaemon

Production installation:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-update
```

A development installation should not have this job loaded.

## Check the Admin API

Read the current in-memory status snapshot:

```bash
curl -i http://127.0.0.1:18090/api/status
```

A successful response should include:

```text
HTTP/1.1 200 OK
X-Status-Refresh-In-Progress: false
```

Start an explicit fleet refresh:

```bash
curl -i \
  "http://127.0.0.1:18090/api/status?refresh=1"
```

During the scan, the response header reports:

```text
X-Status-Refresh-In-Progress: true
```

## Confirm listening ports

Admin port:

```bash
sudo lsof -nP -iTCP:18090 -sTCP:LISTEN
```

Example FBS listener port:

```bash
sudo lsof -nP -iTCP:8081 -sTCP:LISTEN
```

## Verify configuration and TLS permissions

```bash
sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway/config.yaml"

sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway/tls/server-ca.crt"

sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.crt"

sudo -u _fbs-gateway test -r \
  "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.key"
```

Each command should exit silently with status `0`.

## Verify Packet Filter

Display the installed anchor rules:

```bash
sudo pfctl \
  -a com.williamveith.fbs-interlock-gateway \
  -sr
```

Check Packet Filter status:

```bash
sudo pfctl -s info
```

Confirm the managed block exists:

```bash
sudo grep -A 4 -B 1 \
  "BEGIN fbs-interlock-gateway managed anchor" \
  /etc/pf.conf
```

## Confirm Application Firewall registration

```bash
sudo /usr/libexec/ApplicationFirewall/socketfilterfw \
  --listapps | grep -A 3 fbs-interlock-gateway
```

# Verify Tool Communication

Replace `8081` with the configured listener port for the tool being tested.

Read the tool status:

```bash
curl http://127.0.0.1:8081/status
```

Turn the tool output on:

```bash
curl http://127.0.0.1:8081/on
```

Turn the tool output off:

```bash
curl http://127.0.0.1:8081/off
```

Expected FBS-compatible responses:

```json
{"Success":1,"State":1}
```

```json
{"Success":1,"State":0}
```

> **Operational caution**
>
> `/on` and `/off` operate the configured interlock. Perform command testing only when changing the interlock state is authorized and safe.

## Verify HTTPS or mutual TLS directly

```bash
curl \
  --cacert "/Library/Application Support/fbs-interlock-gateway/tls/server-ca.crt" \
  --cert "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.crt" \
  --key "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.key" \
  "https://<shelly-ddns-host>/rpc/Switch.GetStatus?id=0"
```

Add Digest Authentication when the Shelly also requires it:

```bash
curl \
  --anyauth \
  -u "admin:<password>" \
  --cacert "/Library/Application Support/fbs-interlock-gateway/tls/server-ca.crt" \
  --cert "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.crt" \
  --key "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.key" \
  "https://<shelly-ddns-host>/rpc/Switch.GetStatus?id=0"
```

# View the Admin Panel

Open a browser on the gateway Mac:

```text
http://127.0.0.1:18090
```

The Admin UI provides:

- Current cached status for every configured tool
- Connected, disconnected, output, protocol, and error information
- Passive updates from normal FBS traffic
- A **Refresh Status** button for explicit fleet verification
- Configuration editing and validation
- Password replacement or clearing without exposing stored passwords
- Gateway restart after a successful configuration save

## Passive status behavior

The browser polls only the gateway's in-memory status cache every three seconds. These cache reads do not contact Shelly devices.

```text
FBS request
    -> Shelly operation
    -> shared status row updated
    -> Admin browser reads cache
```

Use **Refresh Status** when an independent `Switch.GetStatus` check is required.

## Remote Admin access

Keep the Admin UI bound to loopback. For temporary remote access, use an SSH tunnel from an authorized computer:

```bash
ssh -L 18090:127.0.0.1:18090 \
  <mac-user>@<gateway-host>
```

Then open locally:

```text
http://127.0.0.1:18090
```

# Automatic Updates and Log Maintenance

Production installation creates:

```text
system/com.williamveith.fbs-interlock-gateway-update
```

The update LaunchDaemon runs at minute `17` of every hour and executes:

```text
/usr/local/libexec/fbs-interlock-gateway/update.sh
```

## Update behavior

The updater:

1. Acquires a lock so concurrent update runs exit safely.
2. Detects Apple Silicon or Intel architecture.
3. Downloads the latest release checksum first.
4. Computes the installed binary SHA-256 with `shasum -a 256`.
5. Exits without downloading the binary when the checksum already matches and logs do not need rotation.
6. Downloads the matching `darwin-arm64` or `darwin-amd64` release only when needed.
7. Verifies the downloaded checksum and Mach-O architecture.
8. Creates a timestamped backup of the installed binary.
9. Stops the gateway only when it was loaded before maintenance.
10. Installs and verifies the new binary.
11. Restarts the gateway and waits for the Admin API.
12. Restores the previous binary when the post-update health check fails.

The updater changes only the application binary and gateway log files. It does not modify `config.yaml` or installed TLS files.

## Log rotation

The updater also checks:

```text
gateway.log
gateway-error.log
```

When either file reaches `10 MiB`, it:

- Stops the gateway when it is loaded
- Compresses the current log with `gzip`
- Shifts existing numbered archives
- Retains up to 30 compressed archives
- Creates a new service-owned log file
- Restarts the gateway and verifies the Admin API

## Run maintenance manually

```bash
sudo "/usr/local/libexec/fbs-interlock-gateway/update.sh"
```

## Disable managed updates for development

Use the packaged development installer:

```bash
sudo ./install-dev.sh
```

## Restore managed updates

Run the normal production installer:

```bash
sudo ./install.sh
```

# View Gateway Logs

Gateway standard output:

```text
/Library/Logs/fbs-interlock-gateway/gateway.log
```

Gateway standard error:

```text
/Library/Logs/fbs-interlock-gateway/gateway-error.log
```

Updater standard output:

```text
/Library/Logs/fbs-interlock-gateway/update.log
```

Updater standard error:

```text
/Library/Logs/fbs-interlock-gateway/update-error.log
```

Follow all logs:

```bash
sudo tail -F \
  "/Library/Logs/fbs-interlock-gateway/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway/gateway-error.log" \
  "/Library/Logs/fbs-interlock-gateway/update.log" \
  "/Library/Logs/fbs-interlock-gateway/update-error.log"
```

Useful gateway markers include:

```text
FBS_IN
FBS_OUT
shelly_status_retry
shelly_reboot_scheduled
shelly_reboot_requested
shelly_reboot_failed
```

Network failures may identify the observed phase:

```text
phase=dns_lookup
phase=tcp_connect
phase=tls_handshake
phase=response_headers
phase=response_body
```

# Edit the Configuration

The active configuration is:

```text
/Library/Application Support/fbs-interlock-gateway/config.yaml
```

## Preferred method: Admin UI

```text
http://127.0.0.1:18090
```

The Admin UI validates fields, preserves stored passwords unless explicitly replaced or cleared, writes the configuration atomically, and requests a clean gateway restart.

## Manual method

```bash
sudo nano \
  "/Library/Application Support/fbs-interlock-gateway/config.yaml"
```

Recommended TLS paths are relative to the configuration directory:

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
    ip: "2c41389b0d77.dynamic.utexas.edu"
    protocol: "https"
    port: 8081
    switch_id: 0
    username: "admin"
    password: "example-password"
    enabled: true
```

The LaunchDaemon and startup wrapper use the configuration directory as the working directory. The gateway also resolves relative TLS paths against the directory containing the loaded configuration.

Restart the gateway after manually editing the configuration.

> **Configuration rule**
>
> `tools[].ip` must contain only the hostname or IP address. Do not include `http://`, `https://`, a path, or a port.

# Restart the Gateway

```bash
sudo launchctl kickstart -k \
  system/com.williamveith.fbs-interlock-gateway
```

Verify:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway
```

Review recent logs:

```bash
sudo tail -n 100 \
  "/Library/Logs/fbs-interlock-gateway/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway/gateway-error.log"
```

# Firewall and Packet Filter Behavior

The installer applies two macOS firewall controls.

## Application Firewall

The executable is added to the macOS Application Firewall allow list. This control authorizes the application but does not restrict traffic by source IP.

## Managed Packet Filter anchor

The installer creates:

```text
/etc/pf.anchors/com.williamveith.fbs-interlock-gateway
```

and adds a managed block to:

```text
/etc/pf.conf
```

The generated anchor performs these actions in order:

1. Allows loopback TCP access to ports `8081:8981` for local testing.
2. Allows the configured FBS source IP to TCP ports `8081:8981`.
3. Blocks every other inbound TCP connection to that range.

The current generated values are:

```text
FBS source: 146.6.76.61
Ports:      8081:8981
```

The installer validates the complete candidate `/etc/pf.conf` with:

```bash
sudo pfctl -nf /etc/pf.conf
```

before installing and loading it.

> **Important**
>
> The managed anchor protects the gateway listener range. Keep the Admin UI on `127.0.0.1`; the anchor is not intended to expose or remotely protect the Admin port.

# Gatekeeper and Quarantine

A locally built binary copied by USB normally does not require additional steps. The installer removes the executable quarantine attribute after installing a trusted deployment binary.

Inspect quarantine attributes when troubleshooting:

```bash
xattr -l fbs-interlock-gateway
```

Remove quarantine recursively only when the files came from the trusted build process:

```bash
xattr -dr com.apple.quarantine .
```

Then rerun the installer.

<div class="page-break"></div>

# Troubleshooting

## Build fails because TLS files are missing

Generate the required runtime files:

```bash
make ca
make gateway-cert
```

Confirm:

```bash
ls -l tls/
```

Then rebuild the architecture-specific package.

## Installer reports a binary architecture mismatch

Confirm the gateway architecture:

```bash
uname -m
```

Inspect the packaged binary:

```bash
file fbs-interlock-gateway
```

Use `build/darwin/arm64/` for `arm64` and `build/darwin/amd64/` for `x86_64`.

## Gateway LaunchDaemon is not running

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway
```

Review logs:

```bash
sudo tail -n 200 \
  "/Library/Logs/fbs-interlock-gateway/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway/gateway-error.log"
```

Restart:

```bash
sudo launchctl kickstart -k \
  system/com.williamveith.fbs-interlock-gateway
```

## Installer rolls back

The installer rolls back when:

- The generated Packet Filter configuration is invalid
- The gateway LaunchDaemon cannot be loaded
- The Admin API does not become ready within the health-check window
- The update LaunchDaemon cannot be loaded in production mode

Review the installer output and the existing gateway logs. Correct the reported problem and rerun the installer.

## Admin API does not respond

```bash
sudo lsof -nP -iTCP:18090 -sTCP:LISTEN
```

Confirm the Admin UI was not disabled through an empty Admin address.

## Tool listener does not respond

```bash
sudo lsof -nP -iTCP:8081 -sTCP:LISTEN
```

Review the active configuration and confirm the tool is enabled.

## Packet Filter rule is not active

Validate the complete configuration:

```bash
sudo pfctl -nf /etc/pf.conf
```

Display the gateway anchor:

```bash
sudo pfctl \
  -a com.williamveith.fbs-interlock-gateway \
  -sr
```

Check status:

```bash
sudo pfctl -s info
```

Do not manually replace `/etc/pf.conf` without preserving unrelated system and site rules.

## Update LaunchDaemon is absent

This is expected after a development installation. Restore production updates with:

```bash
sudo ./install.sh
```

Then verify:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-update
```

## Updater reports another update is running

The updater uses:

```text
/usr/local/libexec/fbs-interlock-gateway/.update-lock
```

A concurrent updater exits safely. If no update process exists and a stale lock remains after an abnormal termination, inspect the directory before removing it.

## Update fails and rolls back

Review:

```text
/Library/Logs/fbs-interlock-gateway/update.log
/Library/Logs/fbs-interlock-gateway/update-error.log
```

Common causes include:

- GitHub Release access failure
- Invalid or missing release checksum
- Wrong release architecture
- Installed binary verification failure
- Gateway Admin API health-check failure after restart

## Shelly hostname does not resolve

```bash
dig +short <shelly-hostname>
```

or:

```bash
nslookup <shelly-hostname>
```

The hostname must match the value in `tools[].ip` and the Shelly server certificate.

## HTTPS or mutual TLS fails

Confirm installed files:

```bash
sudo ls -l \
  "/Library/Application Support/fbs-interlock-gateway/tls"
```

Verify:

- The Shelly certificate hostname matches `tools[].ip`
- The Shelly trusts `client-ca.crt`
- The gateway trusts `server-ca.crt`
- The gateway client certificate and key match
- The service account can read all configured files
- The system clock is accurate

Look for `phase=tls_handshake` in the gateway logs.

## Admin status appears stale

Ordinary Admin polling reads memory only. A row changes when:

- FBS sends `/status`, `/on`, or `/off` for that tool
- An administrator selects **Refresh Status**

Use explicit refresh for independent verification:

```bash
curl -i \
  "http://127.0.0.1:18090/api/status?refresh=1"
```

# Uninstall the Gateway

Run the uninstaller from the deployment directory.

## Standard uninstall

```bash
sudo ./uninstall.sh
```

Standard uninstall:

- Stops and disables the update LaunchDaemon
- Stops and disables the gateway LaunchDaemon
- Removes both LaunchDaemon property lists
- Removes the executable, startup wrapper, and updater
- Removes the executable from the Application Firewall
- Removes the managed `pf` anchor and managed `/etc/pf.conf` block
- Preserves `config.yaml`
- Preserves installed TLS files
- Preserves gateway and update logs
- Preserves the hidden service account

## Purge persistent data

```bash
sudo ./uninstall.sh --purge
```

Purge performs the standard uninstall and also removes:

```text
/Library/Application Support/fbs-interlock-gateway/
/Library/Logs/fbs-interlock-gateway/
```

This deletes the production configuration, installed TLS files, and logs. The hidden service account is still preserved for safe reinstallation.

> **Destructive operation**
>
> Use `--purge` only after confirming that configuration, certificate, and log retention requirements have been satisfied.

Verify removal:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway

sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway-update
```

Missing-service errors are expected after successful removal.

# Command Reference

| Task | Command |
| --- | --- |
| Detect architecture | `uname -m` |
| Generate gateway TLS material | `make ca && make gateway-cert` |
| Build Apple Silicon package | `make clean && make build-darwin-arm64` |
| Build Intel package | `make clean && make build-darwin-amd64` |
| Production install | `sudo ./install.sh` |
| Development install | `sudo ./install-dev.sh` |
| Check gateway service | `sudo launchctl print system/com.williamveith.fbs-interlock-gateway` |
| Check updater | `sudo launchctl print system/com.williamveith.fbs-interlock-gateway-update` |
| Restart gateway | `sudo launchctl kickstart -k system/com.williamveith.fbs-interlock-gateway` |
| Run updater manually | `sudo /usr/local/libexec/fbs-interlock-gateway/update.sh` |
| Read Admin cache | `curl -i http://127.0.0.1:18090/api/status` |
| Refresh all tools | `curl -i "http://127.0.0.1:18090/api/status?refresh=1"` |
| Show `pf` anchor | `sudo pfctl -a com.williamveith.fbs-interlock-gateway -sr` |
| Follow gateway logs | `sudo tail -F "/Library/Logs/fbs-interlock-gateway/gateway.log" "/Library/Logs/fbs-interlock-gateway/gateway-error.log"` |
| Follow update logs | `sudo tail -F "/Library/Logs/fbs-interlock-gateway/update.log" "/Library/Logs/fbs-interlock-gateway/update-error.log"` |
| Edit config | `sudo nano "/Library/Application Support/fbs-interlock-gateway/config.yaml"` |
| Standard uninstall | `sudo ./uninstall.sh` |
| Purge uninstall | `sudo ./uninstall.sh --purge` |

---

**FBS Interlock Gateway - macOS Installation and Operations Guide**
