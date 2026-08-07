---
title: "FBS Interlock Gateway"
subtitle: "Linux Installation and Operations Guide"
author: "William Veith"
date: "2026-08-06"
lang: en-US
---

> **Purpose**
>
> This guide covers preparing, building, transferring, installing, validating, operating, updating, and uninstalling `fbs-interlock-gateway` on a dedicated Linux gateway computer.

> **Security boundary**
>
> The gateway controls software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. The installer restricts the FBS listener range to the authorized FBS source address through UFW. The Admin UI must remain bound to loopback unless remote access is intentionally secured.

## Table of Contents

- [Deployment Overview](#deployment-overview)
- [Supported Linux Platforms and Architectures](#supported-linux-platforms-and-architectures)
- [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Set Up the Gateway Machine](#set-up-the-gateway-machine)
  - [Install the Operating System](#install-the-operating-system)
  - [Create the Administrative Account](#create-the-administrative-account)
- [Prepare Gateway TLS Material](#prepare-gateway-tls-material)
- [Build the Deployment Assets](#build-the-deployment-assets)
  - [AMD64](#amd64)
  - [ARM64](#arm64)
- [Transfer the Deployment Directory](#transfer-the-deployment-directory)
- [Choose an Installation Mode](#choose-an-installation-mode)
- [Install the Gateway](#install-the-gateway)
- [What the Installer Does](#what-the-installer-does)
- [Installed Layout](#installed-layout)
- [Verify the Installation](#verify-the-installation)
- [Verify Tool Communication](#verify-tool-communication)
- [View the Admin Panel](#view-the-admin-panel)
- [Automatic Updates](#automatic-updates)
- [View Gateway Logs](#view-gateway-logs)
- [Edit the Configuration](#edit-the-configuration)
- [Restart the Gateway](#restart-the-gateway)
- [Firewall Behavior](#firewall-behavior)
- [Troubleshooting](#troubleshooting)
- [Uninstall the Gateway](#uninstall-the-gateway)
- [Command Reference](#command-reference)

<div class="page-break"></div>

# Deployment Overview

The Linux deployment uses systemd for gateway supervision and managed production updates:

1. `fbs-interlock-gateway.service` starts the gateway and keeps it running.
2. `fbs-interlock-gateway-update.timer` periodically activates the checksum-aware update service in production mode.

```text
FBS server
    -> UFW source restriction
    -> Linux gateway listener port
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

1. Install the supported Linux operating system.
2. Confirm the gateway architecture.
3. Generate the required gateway runtime TLS files.
4. Build the matching Linux deployment directory.
5. Copy the complete deployment directory to the gateway computer.
6. Choose production or development installation mode.
7. Run the selected installer with administrator privileges.
8. Verify the gateway service, update timer, Admin API, TLS files, UFW rule, listener ports, tool communication, and logs.

# Supported Linux Platforms and Architectures

This guide documents deployment on:

```text
Debian GNU/Linux 12 (Bookworm)
GNOME 43.9
```

The project provides Linux builds for both common gateway architectures:

| Architecture output | Build target | Deployment directory |
| --- | --- | --- |
| `x86_64` | `make build-linux-amd64` | `build/linux/` |
| `aarch64` or `arm64` | `make build-linux-arm64` | `build/linux/` |

Determine the gateway architecture:

```bash
uname -m
```

> **Important**
>
> Both Linux build targets write to `build/linux/`. Run `make clean` first and build only the architecture required by the target gateway computer.

# Pre-Deployment Checklist

Before building or installing, confirm the following:

- The repository is on the intended commit or release.
- `make verify` completes successfully.
- `config.yaml` contains the intended non-production or production configuration.
- `make ca` and `make gateway-cert` have populated the required certificate material under `pki/ca/` and `pki/gateway/`.
- The build target matches the gateway CPU architecture.
- The configured FBS listener ports do not conflict with other services.
- `FBS_SOURCE_IP` and `FBS_PORT_RANGE` in the Makefile are correct before building.
- The Admin address remains `127.0.0.1:18090` unless a reviewed remote-access design is being used.
- The gateway can resolve every configured Shelly hostname.
- The gateway can reach GitHub Releases when managed automatic updates are required.
- The complete deployment directory will be transferred and stored as one unit.
- Any required remote-management access is allowed before the installer enables default-deny inbound UFW behavior.

Current generated firewall defaults are:

```text
Authorized FBS source: 146.6.76.61
Gateway listener range: TCP 8081:8981
```

> **Port ownership warning**
>
> The gateway may clear a process that already owns a configured listener port. Use dedicated gateway ports and confirm that unrelated services do not use the configured range.

# Set Up the Gateway Machine

## Install the Operating System

Install [**Debian GNU/Linux 12 (Bookworm)** with **GNOME 43.9**](https://www.debian.org/releases/bookworm/debian-installer/).

Apply current operating-system updates before deployment:

```bash
sudo apt update
sudo apt full-upgrade
```

Reboot when the operating-system update requires it:

```bash
sudo reboot
```

## Create the Administrative Account

During operating-system installation, create the local deployment account:

```text
fbs-gateway
```

If the account is not already permitted to use `sudo`, open a root shell through PolicyKit:

```bash
pkexec bash
```

Add the account to the `sudo` group:

```bash
usermod -aG sudo fbs-gateway
exit
```

Reboot so the new group membership is applied:

```bash
sudo reboot
```

The installer uses the configured gateway service user and group, creating them when required. The default generated deployment uses `fbs-gateway`.

# Prepare Gateway TLS Material

All Linux deployment builds require the gateway runtime TLS files. The canonical certificate material remains in the PKI directories; the Makefile copies only the runtime files required by the gateway into the deployment package.

Generate the certificate authorities and gateway client identity on the controlled development machine:

```bash
make ca
make gateway-cert
```

The required source files are:

```text
pki/
├── ca/
│   └── server-ca.crt
└── gateway/
    ├── gateway-client.crt
    └── gateway-client.key
```

The Linux build copies these files directly into:

```text
build/linux/tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

Do not copy the complete `pki/` directory into the deployment package.

> **Private-key handling**
>
> Never place CA private keys, certificate requests, `client-ca.crt`, or Shelly server private keys on the gateway. Only the gateway runtime server CA certificate, gateway client certificate, and gateway client private key are packaged.

The build fails before producing the Linux deployment directory when any required runtime TLS file is missing.

The default configuration uses relative TLS paths:

```yaml
defaults:
  shelly_tls:
    server_ca_file: "./tls/server-ca.crt"
    client_cert_file: "./tls/gateway-client.crt"
    client_key_file: "./tls/gateway-client.key"
```

The systemd service uses this working directory:

```text
/etc/fbs-interlock-gateway
```

The relative paths therefore resolve under:

```text
/etc/fbs-interlock-gateway/tls/
```

# Build the Deployment Assets

Run the build on the controlled development machine.

## AMD64

```bash
make clean
make build-linux-amd64
```

Use this target when `uname -m` reports:

```text
x86_64
```

## ARM64

```bash
make clean
make build-linux-arm64
```

Use this target when `uname -m` reports:

```text
aarch64
```

or:

```text
arm64
```

The selected build generates:

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

The exact guide PDF name follows the source Markdown filename.

The staged TLS modes are:

| File | Staged mode |
| --- | --- |
| `server-ca.crt` | `0644` |
| `gateway-client.crt` | `0644` |
| `gateway-client.key` | `0600` |

> **Do not edit generated deployment files directly.**
>
> Update the source templates, deployment guide, configuration source, or Makefile variables and rebuild instead.

# Transfer the Deployment Directory

Copy the entire `build/linux/` directory to a USB flash drive or another approved transfer medium.

Do not copy only the executable. The complete directory is required because it contains:

- The application executable
- The production and development installers
- The uninstaller
- The systemd gateway service
- The update service and timer
- The checksum-aware updater
- The configuration file
- The gateway runtime TLS files
- The deployment guide PDF

On the Linux gateway, copy the complete directory into the current user's `Downloads` directory.

Expected local location:

```text
~/Downloads/linux/
```

Confirm the package contents:

```bash
find ~/Downloads/linux -maxdepth 2 -print
```

Run the installer from the local copy rather than directly from removable media.

<div class="page-break"></div>

# Choose an Installation Mode

The deployment package supports two installation modes.

## Production installation

Use the normal installer for a managed production gateway:

```bash
sudo ./install.sh
```

Production mode:

- Installs and starts the gateway systemd service
- Installs `update.sh`
- Installs and enables the systemd update service and timer
- Performs checksum-aware updates from GitHub Releases
- Preserves an existing production configuration and installed TLS identity
- Restores managed updates after a previous development installation

## Development installation

Use the development installer when testing an unpublished local build:

```bash
sudo ./install-dev.sh
```

Development mode:

- Installs the local gateway executable and all normal security controls
- Preserves the production configuration and installed TLS files
- Stops and disables the managed update timer and service
- Removes installed updater units and the updater script
- Prevents the local development executable from being replaced by a published release

Run the normal production installer later to restore managed automatic updates.

# Install the Gateway

Open a terminal and move to the copied deployment directory:

```bash
cd ~/Downloads/linux
```

Make the deployment files executable:

```bash
chmod +x \
  install.sh \
  install-dev.sh \
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

> **Reinstallation behavior**
>
> Reinstallation preserves the active production `config.yaml` and installed TLS files. The installer corrects their ownership and modes but does not replace their contents.

# What the Installer Does

The installer performs the following operations.

## Preflight and dependencies

- Requires root privileges and elevates through `pkexec` or `sudo` when available
- Verifies the packaged executable, configuration, service file, uninstaller, and runtime TLS files
- Requires production updater files when production mode is selected
- Verifies or installs `lsof`, `curl`, `ca-certificates`, and `ufw`
- Stops existing gateway and updater units before replacement
- Checks the configured gateway listener range for conflicting processes

## Service account

- Creates the configured gateway service user and group when needed
- Uses the service identity for the long-running gateway process
- Preserves the service user and group during uninstall for safe reinstallation

## Application and configuration

- Installs the executable under `/opt/fbs-interlock-gateway/`
- Installs `uninstall.sh` under the application directory
- Installs `update.sh` in production mode
- Creates `/etc/fbs-interlock-gateway/`
- Installs a new `config.yaml` only when an active configuration does not already exist
- Preserves `config.yaml.bak` files created by atomic Admin UI configuration writes

## Gateway TLS files

- Creates `/etc/fbs-interlock-gateway/tls/`
- Installs missing runtime TLS files from the deployment package
- Preserves existing installed TLS files during reinstallation
- Sets installed TLS files to `root:<service-group>` with mode `0640`
- Verifies that the gateway service account can read the configuration and each TLS file

## systemd services

- Installs and enables `fbs-interlock-gateway.service`
- Runs the gateway with `/etc/fbs-interlock-gateway` as its working directory
- Writes standard output and standard error to journald
- Restarts the gateway after process exits with bounded rapid-restart behavior
- Applies `NoNewPrivileges=true`
- Installs and enables the update service and timer only in production mode
- Starts or restarts the gateway after installation

## Firewall controls

- Enables UFW when needed
- Sets the default incoming policy to deny
- Sets the default outgoing policy to allow
- Adds an inbound TCP rule restricted to the configured FBS source and gateway listener range
- Applies the same UFW controls in production and development modes

# Installed Layout

## Application directory

```text
/opt/fbs-interlock-gateway/
├── fbs-interlock-gateway
├── uninstall.sh
└── update.sh                 # production mode only
```

## Configuration and TLS

```text
/etc/fbs-interlock-gateway/
├── config.yaml
├── config.yaml.bak           # present after an atomic replacement
└── tls/
    ├── server-ca.crt
    ├── gateway-client.crt
    └── gateway-client.key
```

## systemd units

```text
/etc/systemd/system/
├── fbs-interlock-gateway.service
├── fbs-interlock-gateway-update.service    # production mode only
└── fbs-interlock-gateway-update.timer      # production mode only
```

Linux does not create a separate application log directory. Gateway and updater output is retained by the system journal according to the host's journald configuration.

# Verify the Installation

## Check the installed version

```bash
sudo /opt/fbs-interlock-gateway/fbs-interlock-gateway \
  -version
```

## Check the gateway service

View complete status:

```bash
sudo systemctl status \
  fbs-interlock-gateway.service \
  --no-pager \
  --full
```

Confirm the service is active:

```bash
sudo systemctl is-active \
  fbs-interlock-gateway.service
```

Expected result:

```text
active
```

Confirm the service is enabled:

```bash
sudo systemctl is-enabled \
  fbs-interlock-gateway.service
```

Expected result:

```text
enabled
```

## Check the update timer

Production installation:

```bash
sudo systemctl status \
  fbs-interlock-gateway-update.timer \
  --no-pager \
  --full
```

List the next scheduled activation:

```bash
sudo systemctl list-timers \
  fbs-interlock-gateway-update.timer
```

A development installation should not have the update timer enabled.

## Check the Admin API

Read the current in-memory status snapshot:

```bash
curl -i \
  http://127.0.0.1:18090/api/status
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
sudo lsof \
  -nP \
  -iTCP:18090 \
  -sTCP:LISTEN
```

Example FBS listener port:

```bash
sudo lsof \
  -nP \
  -iTCP:8081 \
  -sTCP:LISTEN
```

Alternative with `ss`:

```bash
sudo ss -ltnp |
  grep -E ':(18090|8081)\b'
```

## Verify configuration and TLS permissions

Confirm the service account can read the active configuration:

```bash
sudo -u fbs-gateway test -r \
  /etc/fbs-interlock-gateway/config.yaml
```

Confirm the service account can read all runtime TLS files:

```bash
sudo -u fbs-gateway test -r \
  /etc/fbs-interlock-gateway/tls/server-ca.crt

sudo -u fbs-gateway test -r \
  /etc/fbs-interlock-gateway/tls/gateway-client.crt

sudo -u fbs-gateway test -r \
  /etc/fbs-interlock-gateway/tls/gateway-client.key
```

Each command should exit silently with status `0`.

Inspect ownership and modes:

```bash
sudo ls -l \
  /etc/fbs-interlock-gateway/config.yaml \
  /etc/fbs-interlock-gateway/tls/
```

## Verify UFW

Display current UFW status and policies:

```bash
sudo ufw status verbose
```

Display the generated rules in command form:

```bash
sudo ufw show added
```

Confirm that an allow rule exists for:

```text
Source:   146.6.76.61
Protocol: TCP
Ports:    8081:8981
```

# Verify Tool Communication

Replace `8081` with the configured listener port for the tool being tested.

Read the tool status:

```bash
curl \
  http://127.0.0.1:8081/status
```

Turn the tool output on:

```bash
curl \
  http://127.0.0.1:8081/on
```

Turn the tool output off:

```bash
curl \
  http://127.0.0.1:8081/off
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
sudo curl \
  --cacert /etc/fbs-interlock-gateway/tls/server-ca.crt \
  --cert /etc/fbs-interlock-gateway/tls/gateway-client.crt \
  --key /etc/fbs-interlock-gateway/tls/gateway-client.key \
  "https://<shelly-ddns-host>/rpc/Switch.GetStatus?id=0"
```

Add Digest Authentication when the Shelly also requires it:

```bash
sudo curl \
  --anyauth \
  -u "admin:<password>" \
  --cacert /etc/fbs-interlock-gateway/tls/server-ca.crt \
  --cert /etc/fbs-interlock-gateway/tls/gateway-client.crt \
  --key /etc/fbs-interlock-gateway/tls/gateway-client.key \
  "https://<shelly-ddns-host>/rpc/Switch.GetStatus?id=0"
```

# View the Admin Panel

On the gateway machine, open:

<http://127.0.0.1:18090>

The Admin UI provides:

- Current cached status for every configured tool
- Connected, disconnected, output, protocol, and error information
- Passive row updates from normal FBS traffic
- A **Refresh Status** button for explicit fleet verification
- Configuration editing and validation
- Password replacement or clearing without exposing stored passwords
- A clean gateway restart after a successful configuration save

## Passive status behavior

The browser polls only the gateway's in-memory status cache every three seconds. These reads do not contact Shelly devices.

```text
FBS request
    -> Shelly operation
    -> shared status row updated
    -> Admin browser reads cache
```

Use **Refresh Status** when an independent `Switch.GetStatus` check is required.

An explicit refresh uses bounded worker concurrency, publishes rows as each device completes, and preserves completed partial results if the overall refresh reaches its deadline. FBS requests take priority over Admin probes for the same device.

## Remote Admin access

Keep the Admin UI bound to loopback. For temporary remote access, use an SSH tunnel from an authorized computer:

```bash
ssh -L 18090:127.0.0.1:18090 \
  fbs-gateway@<gateway-host>
```

Then open locally:

```text
http://127.0.0.1:18090
```

# Automatic Updates

Production installation enables:

```text
fbs-interlock-gateway-update.timer
```

The timer activates:

```text
fbs-interlock-gateway-update.service
```

which executes:

```text
/opt/fbs-interlock-gateway/update.sh
```

## Update behavior

The updater:

1. Acquires the update execution context through systemd.
2. Selects the release asset matching the installed Linux architecture.
3. Downloads the published SHA-256 checksum first.
4. Validates that the checksum contains a usable SHA-256 value.
5. Calculates the checksum of the installed executable.
6. Exits without downloading or restarting when the installed executable already matches.
7. Downloads the binary only when it differs or is missing.
8. Verifies the downloaded binary against the published checksum.
9. Backs up the installed executable.
10. Installs and verifies the replacement.
11. Restarts the gateway only when required.
12. Restores the previous executable when the installed checksum is wrong or the service fails to start.

The updater changes only the application executable. It does not replace:

- `config.yaml`
- `server-ca.crt`
- `gateway-client.crt`
- `gateway-client.key`

Linux log retention remains controlled by journald; the updater does not rotate separate gateway log files.

## Run an update manually

Run the installed updater directly:

```bash
sudo /opt/fbs-interlock-gateway/update.sh
```

Or start the update service through systemd:

```bash
sudo systemctl start \
  fbs-interlock-gateway-update.service
```

Review the result:

```bash
sudo systemctl status \
  fbs-interlock-gateway-update.service \
  --no-pager \
  --full
```

## Disable managed updates for development

Use the packaged development installer:

```bash
sudo ./install-dev.sh
```

Or disable the timer manually:

```bash
sudo systemctl disable --now \
  fbs-interlock-gateway-update.timer
```

## Restore managed updates

Run the normal production installer:

```bash
sudo ./install.sh
```

# View Gateway Logs

Gateway output is written to journald.

Follow the gateway service:

```bash
sudo journalctl \
  -u fbs-interlock-gateway.service \
  -f
```

Review recent gateway logs:

```bash
sudo journalctl \
  -u fbs-interlock-gateway.service \
  --since "1 hour ago" \
  --no-pager \
  -o short-iso
```

Review updater output:

```bash
sudo journalctl \
  -u fbs-interlock-gateway-update.service \
  --no-pager \
  -o short-iso
```

Follow both gateway and updater output:

```bash
sudo journalctl \
  -u fbs-interlock-gateway.service \
  -u fbs-interlock-gateway-update.service \
  -f
```

Useful gateway markers include:

```text
FBS_IN
FBS_OUT
shelly_status_retry
shelly_authentication_throttled
shelly_reboot_scheduled
shelly_reboot_suppressed
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

Search for common failures:

```bash
sudo journalctl \
  -u fbs-interlock-gateway.service \
  --since "24 hours ago" \
  --no-pager |
grep -Ei \
  'context deadline exceeded|shelly_status_error|shelly_set_error|phase=|reboot|panic|failed'
```

Existing log history remains subject to the host's journald retention settings.

# Edit the Configuration

The active configuration is:

```text
/etc/fbs-interlock-gateway/config.yaml
```

## Preferred method: Admin UI

Open:

```text
http://127.0.0.1:18090
```

The Admin UI validates fields, preserves stored passwords unless explicitly replaced or cleared, writes the configuration atomically, creates `config.yaml.bak` when possible, and requests a clean gateway restart.

## Manual method

```bash
sudo nano \
  /etc/fbs-interlock-gateway/config.yaml
```

Recommended TLS paths are relative to the configuration directory:

```yaml
bind: "0.0.0.0"

defaults:
  timeout_ms: 3000
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

The systemd service uses the configuration directory as its working directory. The gateway also resolves relative TLS paths against the directory containing the loaded configuration.

Restart the gateway after manually editing the configuration.

> **Configuration rule**
>
> `tools[].ip` must contain only the hostname or IP address. Do not include `http://`, `https://`, a path, or a port.

# Restart the Gateway

Restart the service:

```bash
sudo systemctl restart \
  fbs-interlock-gateway.service
```

Verify:

```bash
sudo systemctl status \
  fbs-interlock-gateway.service \
  --no-pager \
  --full
```

Verify the Admin API:

```bash
curl -i \
  http://127.0.0.1:18090/api/status
```

Review recent logs:

```bash
sudo journalctl \
  -u fbs-interlock-gateway.service \
  -n 100 \
  --no-pager
```

# Firewall Behavior

The installer configures UFW with these baseline policies:

```text
Default incoming: deny
Default outgoing: allow
```

It then adds a gateway-specific inbound rule equivalent to:

```bash
ufw allow \
  from 146.6.76.61 \
  to any \
  port 8081:8981 \
  proto tcp
```

Inspect UFW:

```bash
sudo ufw status verbose
sudo ufw show added
```

The rule protects the FBS listener range. The Admin UI remains available through local loopback and should remain bound to `127.0.0.1`.

> **Remote-management caution**
>
> Default-deny inbound behavior can affect SSH or other management services. Confirm any required management rules before relying on remote-only access to the gateway.

The standard and purge uninstall paths remove the gateway-specific UFW rule. They do not disable UFW or reverse system-wide firewall defaults.

# Troubleshooting

## Build fails because TLS files are missing

Generate the required runtime files:

```bash
make ca
make gateway-cert
```

Confirm the canonical runtime source files exist:

```bash
ls -l \
  pki/ca/server-ca.crt \
  pki/gateway/gateway-client.crt \
  pki/gateway/gateway-client.key
```

Then rebuild:

```bash
make clean
make build-linux-amd64
```

or:

```bash
make clean
make build-linux-arm64
```

Do not manually bypass the TLS build check by copying an incomplete deployment directory.

## The executable architecture does not match the gateway

Check the gateway architecture:

```bash
uname -m
```

Inspect the packaged executable:

```bash
file fbs-interlock-gateway
```

Rebuild with the matching target.

## The gateway service is not running

Inspect status:

```bash
sudo systemctl status \
  fbs-interlock-gateway.service \
  --no-pager \
  --full
```

Review logs:

```bash
sudo journalctl \
  -u fbs-interlock-gateway.service \
  -n 200 \
  --no-pager
```

Restart:

```bash
sudo systemctl restart \
  fbs-interlock-gateway.service
```

## The Admin API does not respond

Confirm the service is active:

```bash
sudo systemctl is-active \
  fbs-interlock-gateway.service
```

Confirm port `18090` is listening:

```bash
sudo lsof \
  -nP \
  -iTCP:18090 \
  -sTCP:LISTEN
```

Review the journal for startup, configuration, listener, or TLS errors.

Confirm the Admin UI was not disabled through an empty Admin address.

## A tool listener does not respond

Confirm the configured listener port is open locally:

```bash
sudo lsof \
  -nP \
  -iTCP:8081 \
  -sTCP:LISTEN
```

Check the exact tool entry in `config.yaml` and confirm the tool is enabled.

Review logs for hostname resolution, authentication, timeout, TLS, or certificate errors.

## The gateway cannot read the configuration or TLS files

Test access as the service account:

```bash
sudo -u fbs-gateway test -r \
  /etc/fbs-interlock-gateway/config.yaml

sudo -u fbs-gateway test -r \
  /etc/fbs-interlock-gateway/tls/gateway-client.key
```

Inspect ownership and modes:

```bash
sudo namei -l \
  /etc/fbs-interlock-gateway/tls/gateway-client.key
```

Re-run the selected installer to restore intended ownership and permissions without replacing the active configuration or installed TLS identity.

## The UFW rule is missing or incorrect

Inspect current rules:

```bash
sudo ufw status verbose
sudo ufw show added
```

Confirm the source address and port range match the values used when the deployment package was built.

After changing `FBS_SOURCE_IP` or `FBS_PORT_RANGE` in the Makefile, rebuild and reinstall the package.

## The update timer is absent

This is expected after a development installation.

Confirm:

```bash
sudo systemctl status \
  fbs-interlock-gateway-update.timer
```

Restore production updates by running:

```bash
sudo ./install.sh
```

## The updater reports a checksum or download failure

Review:

```bash
sudo journalctl \
  -u fbs-interlock-gateway-update.service \
  --no-pager \
  --full
```

Common causes include:

- GitHub Release access failure
- Missing or invalid published checksum
- Release architecture mismatch
- Downloaded checksum mismatch
- Installed executable verification failure

Do not manually install a binary that failed checksum validation.

## The update is rolled back

The updater restores the previous executable when the installed checksum is incorrect or the gateway service fails after replacement.

Review both units:

```bash
sudo journalctl \
  -u fbs-interlock-gateway-update.service \
  -u fbs-interlock-gateway.service \
  --since "1 hour ago" \
  --no-pager
```

Correct the reported problem before running another update.

## A Shelly hostname does not resolve

```bash
getent hosts \
  <shelly-hostname>
```

or:

```bash
nslookup \
  <shelly-hostname>
```

The hostname must match the value in `tools[].ip` and the Shelly server certificate.

## HTTPS or mutual TLS fails

Confirm the installed files:

```bash
sudo ls -l \
  /etc/fbs-interlock-gateway/tls/
```

Verify:

- The Shelly certificate hostname matches `tools[].ip`
- The Shelly trusts `client-ca.crt`
- The gateway trusts `server-ca.crt`
- The gateway client certificate and key match
- The service account can read every configured file
- The gateway system clock is accurate

Look for:

```text
phase=tls_handshake
```

in the gateway journal.

## A request reaches its context deadline

Search the journal:

```bash
sudo journalctl \
  -u fbs-interlock-gateway.service \
  --since "24 hours ago" \
  --no-pager |
grep -F \
  "context deadline exceeded"
```

Review the associated `phase=` value, elapsed time, connection-reuse state, and tool name before increasing `defaults.timeout_ms`.

## Admin status appears stale

Ordinary Admin polling reads memory only. A row changes when:

- FBS sends `/status`, `/on`, or `/off` for that tool
- An administrator selects **Refresh Status**

Start an explicit refresh:

```bash
curl -i \
  "http://127.0.0.1:18090/api/status?refresh=1"
```

# Uninstall the Gateway

The installed uninstaller removes the gateway executable, updater, systemd units, and gateway-specific UFW rule. It does not disable UFW or reverse system-wide default firewall policies.

## Standard uninstall

Run the installed copy:

```bash
sudo /opt/fbs-interlock-gateway/uninstall.sh
```

The standard uninstall preserves:

```text
/etc/fbs-interlock-gateway/config.yaml
/etc/fbs-interlock-gateway/tls/
```

It also preserves the configured gateway service user and group so a later installation can reuse stable ownership.

Existing journald entries remain subject to the host's normal journal retention policy.

## Purge persistent configuration and TLS

Run:

```bash
sudo /opt/fbs-interlock-gateway/uninstall.sh \
  --purge
```

Purge mode also removes:

```text
/etc/fbs-interlock-gateway/
```

The service user and group remain preserved.

## Use the deployment-copy uninstaller

The deployment directory also contains `uninstall.sh`. Use it when the installed copy is unavailable:

```bash
cd ~/Downloads/linux
sudo ./uninstall.sh
```

Purge through the deployment copy:

```bash
sudo ./uninstall.sh \
  --purge
```

# Command Reference

| Task | Command |
| --- | --- |
| Detect architecture | `uname -m` |
| Generate gateway TLS material | `make ca && make gateway-cert` |
| Build AMD64 package | `make clean && make build-linux-amd64` |
| Build ARM64 package | `make clean && make build-linux-arm64` |
| Production install | `sudo ./install.sh` |
| Development install | `sudo ./install-dev.sh` |
| Check gateway service | `sudo systemctl status fbs-interlock-gateway.service --no-pager --full` |
| Check update timer | `sudo systemctl status fbs-interlock-gateway-update.timer --no-pager --full` |
| Restart gateway | `sudo systemctl restart fbs-interlock-gateway.service` |
| Run updater manually | `sudo /opt/fbs-interlock-gateway/update.sh` |
| Read Admin cache | `curl -i http://127.0.0.1:18090/api/status` |
| Refresh all tools | `curl -i "http://127.0.0.1:18090/api/status?refresh=1"` |
| Show UFW status | `sudo ufw status verbose` |
| Follow gateway logs | `sudo journalctl -u fbs-interlock-gateway.service -f` |
| View updater logs | `sudo journalctl -u fbs-interlock-gateway-update.service --no-pager` |
| Edit config | `sudo nano /etc/fbs-interlock-gateway/config.yaml` |
| Standard uninstall | `sudo /opt/fbs-interlock-gateway/uninstall.sh` |
| Purge uninstall | `sudo /opt/fbs-interlock-gateway/uninstall.sh --purge` |

---

**FBS Interlock Gateway - Linux Installation and Operations Guide**
