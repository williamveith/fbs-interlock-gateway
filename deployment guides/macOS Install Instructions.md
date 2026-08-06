---
title: "FBS Interlock Gateway"
subtitle: "macOS Installation and Operations Guide"
author: "Deployment Guide"
date: ""
lang: en-US
---

> **Purpose**
>
> This guide covers building, transferring, installing, validating, operating, and uninstalling `fbs-interlock-gateway` on a dedicated macOS gateway computer.

> **Security boundary**
>
> The gateway controls software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. Keep the Admin UI bound to loopback unless remote access is intentionally secured.

## Table of Contents

- [Deployment Overview](#deployment-overview)
- [Supported Mac Architectures](#supported-mac-architectures)
- [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Build the Deployment Assets](#build-the-deployment-assets)
- [Prepare TLS Files for HTTPS or mTLS](#prepare-tls-files-for-https-or-mtls)
- [Transfer the Deployment Directory](#transfer-the-deployment-directory)
- [Install the Gateway](#install-the-gateway)
- [What the Installer Does](#what-the-installer-does)
- [Verify That the Gateway Is Running](#verify-that-the-gateway-is-running)
- [Verify Tool Communication](#verify-tool-communication)
- [View the Admin Panel](#view-the-admin-panel)
- [View Gateway Logs](#view-gateway-logs)
- [Edit the Configuration](#edit-the-configuration)
- [Restart the Gateway](#restart-the-gateway)
- [Firewall Behavior](#firewall-behavior)
- [Gatekeeper and Quarantine](#gatekeeper-and-quarantine)
- [Troubleshooting](#troubleshooting)
- [Uninstall the Gateway](#uninstall-the-gateway)
- [Command Reference](#command-reference)

<div class="page-break"></div>

# Deployment Overview

The macOS deployment uses a system-wide `launchd` service so the gateway starts before a user signs in and restarts automatically if the process exits.

```text
FBS server
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
2. Build the matching deployment directory.
3. Prepare certificate files when using HTTPS or mutual TLS.
4. Copy the complete deployment directory to the gateway Mac.
5. Run the installer with administrator privileges.
6. Verify the LaunchDaemon, Admin API, tool ports, and logs.
7. Apply source-network restrictions appropriate for production.

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
> Use the deployment directory that matches the gateway Mac. An Intel binary will not run natively on Apple Silicon without translation, and an Apple Silicon binary will not run on Intel hardware.

# Pre-Deployment Checklist

Before building or installing, confirm the following:

- The repository is on the intended commit or release.
- `make verify` completes successfully.
- A non-production `config.yaml` is available in the repository root.
- The selected build target matches the gateway Mac architecture.
- The configured FBS listener ports do not conflict with other services.
- The Admin address remains `127.0.0.1:18090` unless a reviewed remote-access design is being used.
- HTTPS tools have valid gateway trust and client-identity files.
- The gateway Mac can resolve each configured Shelly hostname.
- A production source-IP restriction will be provided by the network, `pf`, or another reviewed control.

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

The selected deployment directory should contain:

```text
fbs-interlock-gateway
config.yaml
install.sh
start.sh
uninstall.sh
com.williamveith.fbs-interlock-gateway.plist
macOS Install Instructions.md
```

> **Do not edit generated deployment files directly.**
>
> Update the source templates or Makefile variables and rebuild instead.

# Prepare TLS Files for HTTPS or mTLS

The macOS deployment build does not automatically package gateway runtime TLS files. This section is required only when one or more tools use:

```yaml
protocol: "https"
```

Generate the private certificate authorities and gateway client identity on the controlled development machine:

```bash
make ca
make gateway-cert
```

The gateway runtime files are staged under:

```text
tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

Copy only those three runtime files into a `tls/` directory inside the architecture-specific macOS deployment directory:

```text
build/darwin/arm64/tls/
```

or:

```text
build/darwin/amd64/tls/
```

The complete deployment directory should then resemble:

```text
build/darwin/arm64/
├── fbs-interlock-gateway
├── config.yaml
├── install.sh
├── start.sh
├── uninstall.sh
├── com.williamveith.fbs-interlock-gateway.plist
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── macOS Install Instructions.md
```

> **Private-key handling**
>
> Do not copy `pki/`, CA private keys, certificate requests, or Shelly private keys to the gateway Mac. Only the gateway runtime CA certificate, client certificate, and client private key belong on the gateway.

After installation, place the runtime files in a service-readable location such as:

```text
/Library/Application Support/fbs-interlock-gateway/tls/
```

Then configure absolute paths:

```yaml
defaults:
  shelly_tls:
    server_ca_file: "/Library/Application Support/fbs-interlock-gateway/tls/server-ca.crt"
    client_cert_file: "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.crt"
    client_key_file: "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.key"
```

The certificate files must be readable by the dedicated gateway service account. Keep the private key inaccessible to ordinary users.

<div class="page-break"></div>

# Transfer the Deployment Directory

## Copy to a USB drive

Copy the complete architecture-specific deployment directory to a USB flash drive.

Do not copy only the executable. The complete directory contains:

- The application executable
- The installer
- The startup wrapper
- The LaunchDaemon property list
- The uninstaller
- The configuration file
- Optional runtime TLS files
- This deployment guide

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

Confirm the files are present before installing:

```bash
ls -la ~/Downloads/darwin/arm64
```

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
  start.sh \
  uninstall.sh \
  fbs-interlock-gateway
```

Run the installer:

```bash
sudo ./install.sh
```

Enter the administrator password when prompted.

> **Reinstallation behavior**
>
> The installer preserves an existing production configuration. Review the active configuration after reinstalling to confirm that it still contains the intended tool mappings, credentials, protocols, and TLS paths.

# What the Installer Does

The installer performs the following actions.

## Application files

Installs the executable and startup wrapper in:

```text
/usr/local/libexec/fbs-interlock-gateway/
```

Expected files:

```text
/usr/local/libexec/fbs-interlock-gateway/
├── fbs-interlock-gateway
└── start.sh
```

## Configuration

Creates the configuration directory and installs or preserves:

```text
/Library/Application Support/fbs-interlock-gateway/config.yaml
```

The configuration is owned by the dedicated gateway service account and installed with restrictive permissions.

## Service account and logs

- Creates a hidden, non-login service account when needed
- Creates the gateway log directory
- Creates separate standard-output and standard-error logs

## LaunchDaemon

Installs the system-wide property list at:

```text
/Library/LaunchDaemons/com.williamveith.fbs-interlock-gateway.plist
```

The LaunchDaemon:

- Starts before a user signs in
- Runs under the dedicated service account
- Keeps the gateway running
- Throttles rapid restart attempts
- Writes standard output and standard error to separate log files

## Firewall and startup

- Registers the executable with the macOS Application Firewall
- Loads and starts the LaunchDaemon immediately
- Checks whether the local Admin API responds

# Verify That the Gateway Is Running

## Check the LaunchDaemon

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway
```

Look for a running state and process identifier:

```text
state = running
pid = <process-id>
```

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

Check the Admin port:

```bash
sudo lsof -nP -iTCP:18090 -sTCP:LISTEN
```

Check a configured FBS listener port:

```bash
sudo lsof -nP -iTCP:8081 -sTCP:LISTEN
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
> `/on` and `/off` operate the configured interlock. Perform command testing only when the tool is in a condition where changing the interlock state is authorized and safe.

## Verify HTTPS or mTLS directly

From a trusted administrative shell with access to the gateway runtime certificate files:

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

<div class="page-break"></div>

# View the Admin Panel

Open a browser on the gateway Mac and navigate to:

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

Keep the Admin UI bound to loopback whenever possible. For temporary remote access, use an SSH tunnel from an authorized computer:

```bash
ssh -L 18090:127.0.0.1:18090 \
  <mac-user>@<gateway-host>
```

Then open locally:

```text
http://127.0.0.1:18090
```

# View Gateway Logs

Standard output:

```text
/Library/Logs/fbs-interlock-gateway/gateway.log
```

Standard error:

```text
/Library/Logs/fbs-interlock-gateway/gateway-error.log
```

Follow both logs:

```bash
sudo tail -F \
  "/Library/Logs/fbs-interlock-gateway/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway/gateway-error.log"
```

Press `Ctrl+C` to stop following the logs.

Useful log markers include:

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

The active configuration file is:

```text
/Library/Application Support/fbs-interlock-gateway/config.yaml
```

## Preferred method: Admin UI

Use the local Admin UI when possible:

```text
http://127.0.0.1:18090
```

The Admin UI validates fields, preserves stored passwords unless explicitly replaced or cleared, writes the configuration atomically, and requests a clean gateway restart.

## Manual method

Edit the file with an administrator-capable editor:

```bash
sudo nano \
  "/Library/Application Support/fbs-interlock-gateway/config.yaml"
```

Example structure:

```yaml
bind: "0.0.0.0"

defaults:
  timeout_ms: 10000
  safe_state_on_error: "off"
  shelly_tls:
    server_ca_file: "/Library/Application Support/fbs-interlock-gateway/tls/server-ca.crt"
    client_cert_file: "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.crt"
    client_key_file: "/Library/Application Support/fbs-interlock-gateway/tls/gateway-client.key"

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

Restart the gateway after manually editing the configuration.

> **Configuration rule**
>
> `tools[].ip` must contain only the hostname or IP address. Do not include `http://`, `https://`, a path, or a port.

# Restart the Gateway

Restart the LaunchDaemon:

```bash
sudo launchctl kickstart -k \
  system/com.williamveith.fbs-interlock-gateway
```

Verify that it restarted:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway
```

Then check the logs:

```bash
sudo tail -n 100 \
  "/Library/Logs/fbs-interlock-gateway/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway/gateway-error.log"
```

# Firewall Behavior

The installer adds the executable to the macOS Application Firewall allow list.

The Application Firewall controls incoming access by application. It does **not** reproduce the Linux UFW rule that permits only one source IP across a specific port range.

For production deployment, preserve the FBS-only restriction using one of the following:

- A campus or network firewall rule
- A separately reviewed macOS Packet Filter (`pf`) rule
- An application-level source-IP allowlist
- A dedicated protected network segment

Do not treat the Application Firewall entry as equivalent to:

```text
allow from the authorized FBS source IP
to TCP ports 8081:8981
```

## Confirm Application Firewall registration

```bash
sudo /usr/libexec/ApplicationFirewall/socketfilterfw \
  --listapps | grep -A 3 fbs-interlock-gateway
```

> **Production requirement**
>
> Confirm the actual source-network control before placing the gateway into service. The Admin UI should remain local-only even when FBS listener ports are remotely reachable.

# Gatekeeper and Quarantine

A locally built binary copied by USB normally does not require additional steps.

If macOS reports that a deployment file cannot be opened because it is quarantined, inspect the attributes first:

```bash
xattr -l fbs-interlock-gateway
```

Remove the quarantine attribute only when the files came from the trusted build process:

```bash
xattr -dr com.apple.quarantine .
```

Then rerun the installer:

```bash
sudo ./install.sh
```

<div class="page-break"></div>

# Troubleshooting

## LaunchDaemon is not running

Inspect the service state:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway
```

Review both log files:

```bash
sudo tail -n 200 \
  "/Library/Logs/fbs-interlock-gateway/gateway.log" \
  "/Library/Logs/fbs-interlock-gateway/gateway-error.log"
```

Attempt a restart:

```bash
sudo launchctl kickstart -k \
  system/com.williamveith.fbs-interlock-gateway
```

## Admin API does not respond

Confirm the process is listening:

```bash
sudo lsof -nP -iTCP:18090 -sTCP:LISTEN
```

Check whether the Admin UI was disabled with an empty `-admin` value in the LaunchDaemon or startup wrapper.

## Tool listener does not respond

Confirm the configured port is listening:

```bash
sudo lsof -nP -iTCP:8081 -sTCP:LISTEN
```

Review the active configuration:

```bash
sudo cat \
  "/Library/Application Support/fbs-interlock-gateway/config.yaml"
```

Confirm that the tool is enabled and that no unrelated process owns the listener port.

## Shelly hostname does not resolve

```bash
dig +short <shelly-hostname>
```

or:

```bash
nslookup <shelly-hostname>
```

The gateway Mac must be able to resolve the same hostname present in `tools[].ip` and in the Shelly server certificate.

## HTTPS or mTLS fails

Confirm all configured files exist:

```bash
sudo ls -l \
  "/Library/Application Support/fbs-interlock-gateway/tls"
```

Confirm the service account can read the files. Also verify:

- The Shelly certificate hostname matches `tools[].ip`
- The Shelly trusts `client-ca.crt`
- The gateway trusts `server-ca.crt`
- The gateway client certificate and key match
- The configured paths are absolute and correct
- The system clock is accurate

Look for:

```text
phase=tls_handshake
```

in the gateway logs.

## Admin status appears stale

Ordinary Admin polling reads memory only. A row changes when:

- FBS sends `/status`, `/on`, or `/off` for that tool
- An administrator selects **Refresh Status**

Use the explicit refresh endpoint when independent verification is needed:

```bash
curl -i \
  "http://127.0.0.1:18090/api/status?refresh=1"
```

# Uninstall the Gateway

From the original macOS deployment directory, run:

```bash
sudo ./uninstall.sh
```

The uninstaller will:

- Stop and unload the LaunchDaemon
- Remove the LaunchDaemon property list
- Remove the executable and startup wrapper
- Remove the executable from the Application Firewall allow list
- Preserve the production `config.yaml`
- Preserve existing gateway logs

After uninstalling, verify that the LaunchDaemon is gone:

```bash
sudo launchctl print \
  system/com.williamveith.fbs-interlock-gateway
```

A missing-service error is expected after successful removal.

> **Preserved data**
>
> Configuration, logs, and any manually installed TLS files may remain. Remove them separately only when they are no longer needed and retention requirements permit deletion.

# Command Reference

| Task | Command |
| --- | --- |
| Detect architecture | `uname -m` |
| Build Apple Silicon package | `make clean && make build-darwin-arm64` |
| Build Intel package | `make clean && make build-darwin-amd64` |
| Install | `sudo ./install.sh` |
| Check service | `sudo launchctl print system/com.williamveith.fbs-interlock-gateway` |
| Restart service | `sudo launchctl kickstart -k system/com.williamveith.fbs-interlock-gateway` |
| Read Admin cache | `curl -i http://127.0.0.1:18090/api/status` |
| Refresh all tools | `curl -i "http://127.0.0.1:18090/api/status?refresh=1"` |
| Follow logs | `sudo tail -F "/Library/Logs/fbs-interlock-gateway/gateway.log" "/Library/Logs/fbs-interlock-gateway/gateway-error.log"` |
| Edit config | `sudo nano "/Library/Application Support/fbs-interlock-gateway/config.yaml"` |
| Uninstall | `sudo ./uninstall.sh` |

---

**FBS Interlock Gateway - macOS Installation and Operations Guide**
