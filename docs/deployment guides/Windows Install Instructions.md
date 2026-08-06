---
title: "FBS Interlock Gateway"
subtitle: "Windows Installation and Operations Guide"
author: "William Veith"
date: "2026-08-06"
lang: en-US
---

> **Purpose**
>
> This guide covers building, transferring, installing, validating, operating, updating, and uninstalling `fbs-interlock-gateway` on a dedicated 64-bit Windows gateway computer.

> **Security boundary**
>
> The gateway controls software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. The installer restricts the FBS listener range to the authorized FBS source address through Windows Defender Firewall. The Admin UI must remain bound to loopback unless remote access is intentionally secured.

# Table of Contents

- [Deployment Overview](#deployment-overview)
- [Supported Windows Architecture](#supported-windows-architecture)
- [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Prepare Gateway TLS Material](#prepare-gateway-tls-material)
- [Build the Deployment Assets](#build-the-deployment-assets)
- [Transfer the Deployment Directory](#transfer-the-deployment-directory)
- [Choose an Installation Mode](#choose-an-installation-mode)
- [Install the Gateway](#install-the-gateway)
- [What the Installer Does](#what-the-installer-does)
- [Installed Layout](#installed-layout)
- [Runtime Accounts and File Permissions](#runtime-accounts-and-file-permissions)
- [Verify the Installation](#verify-the-installation)
- [Verify Tool Communication](#verify-tool-communication)
- [View the Admin Panel](#view-the-admin-panel)
- [Automatic Updates and Log Maintenance](#automatic-updates-and-log-maintenance)
- [View Gateway Logs](#view-gateway-logs)
- [Edit the Configuration](#edit-the-configuration)
- [Restart the Gateway](#restart-the-gateway)
- [Firewall Behavior](#firewall-behavior)
- [Troubleshooting](#troubleshooting)
- [Uninstall the Gateway](#uninstall-the-gateway)
- [Command Reference](#command-reference)

<div class="page-break"></div>

# Deployment Overview

The Windows deployment uses two system-wide Task Scheduler tasks:

1. **FBS Interlock Gateway** starts at system boot and keeps the gateway process running.
2. **FBS Interlock Gateway Update** checks the latest GitHub release once per hour and performs verified binary updates and gateway log maintenance.

The gateway task runs as the restricted built-in `LOCAL SERVICE` account. The update task runs as `SYSTEM` because replacing the installed executable and restarting the gateway require administrative access.

```text
FBS server
    -> Windows Defender Firewall source restriction
    -> Windows gateway listener port
    -> fbs-interlock-gateway.exe
    -> Shelly RPC over HTTP or HTTPS
    -> tool interlock circuit
```

The installed Admin UI remains local by default:

```text
http://127.0.0.1:18090
```

Normal FBS `/status`, `/on`, and `/off` requests update the Admin status cache for the affected tool. The browser reads that in-memory cache without generating recurring Shelly requests. The **Refresh Status** action performs an explicit fleet-wide verification.

## Deployment sequence

1. Generate the required gateway runtime TLS files.
2. Build the Windows amd64 deployment directory.
3. Copy the complete deployment directory to the gateway computer.
4. Choose production or development installation mode.
5. Run the selected installer as an administrator.
6. Verify the gateway task, update task, Admin API, TLS files, firewall rule, listener ports, and logs.

# Supported Windows Architecture

The supplied Windows deployment target is:

```text
GOOS=windows
GOARCH=amd64
```

Use it on a 64-bit Windows installation capable of running amd64 executables.

The installer validates all of the following before replacing installed files:

- Windows is 64-bit.
- The deployment executable is a valid PE executable.
- The PE machine type is amd64 (`0x8664`).
- The executable successfully responds to `-version`.

The build target is:

```bash
make build-windows-amd64
```

The generated directory is:

```text
build/windows/
```

# Pre-Deployment Checklist

Before building or installing, confirm the following:

- The repository is on the intended commit or release.
- `make verify` completes successfully.
- `config.yaml` contains the intended non-production or production configuration.
- `make ca` and `make gateway-cert` have populated the repository `tls/` directory.
- The Windows gateway is running a 64-bit version of Windows.
- Windows PowerShell 5.1 is available.
- The `ScheduledTasks` and `NetSecurity` PowerShell modules are available.
- The configured FBS listener ports do not conflict with other services.
- `FBS_SOURCE_IP` and `FBS_PORT_RANGE` in the Makefile are correct before building.
- The Admin address remains `127.0.0.1:18090` unless a reviewed remote-access design is being used.
- The gateway computer can resolve every configured Shelly hostname.
- The gateway computer can reach GitHub Releases when managed automatic updates are required.
- The deployment directory will be transferred and stored as a complete unit.

Current generated firewall defaults are:

```text
Authorized FBS source: 146.6.76.61
Gateway listener range: TCP 8081-8981
```

# Prepare Gateway TLS Material

All Linux, macOS, and Windows deployment builds require the gateway runtime TLS files. This keeps each deployment package ready for HTTPS or mutual-TLS Shelly communication.

Generate the certificate authorities and gateway client identity on the controlled development machine:

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

The Windows build copies these files into:

```text
build/windows/tls/
```

Do not copy the complete `pki/` directory into the deployment package.

> **Private-key handling**
>
> Never place CA private keys, certificate requests, or Shelly server private keys on the Windows gateway. Only the gateway runtime server CA certificate, gateway client certificate, and gateway client private key are packaged.

The build fails before producing the Windows deployment directory when any required runtime TLS file is missing.

The default configuration uses relative TLS paths:

```yaml
defaults:
  shelly_tls:
    server_ca_file: "./tls/server-ca.crt"
    client_cert_file: "./tls/gateway-client.crt"
    client_key_file: "./tls/gateway-client.key"
```

The gateway task uses the configured Windows configuration directory as its working directory. With the default Makefile settings, these paths resolve under:

```text
C:\FBS\fbs-interlock-gateway\tls\
```

# Build the Deployment Assets

On the development machine, run:

```bash
make clean
make build-windows-amd64
```

The build performs the following Windows-specific work:

- Verifies the three runtime TLS files exist.
- Renders all Windows deployment templates.
- Copies all matching Windows deployment guides as PDFs.
- Copies `config.yaml`.
- Copies the runtime TLS files.
- Builds the amd64 Windows executable.

The generated directory contains:

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

The exact guide PDF name follows the source Markdown filename.

# Transfer the Deployment Directory

Copy the entire `build/windows/` directory to a USB flash drive or another approved transfer medium.

Do not copy only the executable. The complete directory is required because it contains:

- The application executable
- The production and development launchers
- The PowerShell installer
- The restart supervisor
- The automatic updater
- The uninstaller
- The configuration file
- The gateway runtime TLS files
- The deployment guide

On the Windows gateway computer, copy the complete directory to a local location such as:

```text
C:\Users\<username>\Downloads\windows
```

Run the installer from the local copy. Do not install directly from removable media.

# Choose an Installation Mode

## Production installation

Use `install.bat` for normal production deployment.

Production mode:

- Installs and starts the gateway task.
- Installs the checksum-aware updater.
- Registers the hourly update task.
- Preserves an existing configuration and TLS identity.
- Enables automatic gateway log rotation during update checks.

## Development installation

Use `install-dev.bat` when testing a locally built executable.

Development mode:

- Installs the local executable and normal gateway task.
- Removes installed updater scripts.
- Removes the managed update task.
- Prevents an hourly release check from replacing the local development build.
- Preserves the production configuration and TLS files.

Run the normal `install.bat` later to restore managed production updates.

# Install the Gateway

## Production installation

Open the copied Windows deployment directory.

Right-click:

```text
install.bat
```

Select:

```text
Run as administrator
```

Approve the Windows User Account Control prompt.

## Development installation

Right-click:

```text
install-dev.bat
```

Select:

```text
Run as administrator
```

Approve the Windows User Account Control prompt.

## PowerShell invocation

The batch files are convenience launchers. The equivalent PowerShell commands are:

```powershell
Set-Location C:\Users\<username>\Downloads\windows
PowerShell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

Development mode:

```powershell
PowerShell.exe `
    -NoProfile `
    -ExecutionPolicy Bypass `
    -File .\install.ps1 `
    -Development
```

# What the Installer Does

The installer performs the following actions:

1. Requires administrator privileges.
2. Verifies every required deployment file.
3. Validates that the executable is a runnable amd64 PE binary.
4. Captures the existing task definitions and installed executable files for rollback.
5. Stops the existing gateway and updater tasks.
6. Stops any remaining gateway process.
7. Creates the installation, TLS, and log directories.
8. Installs the executable and startup supervisor.
9. Preserves an existing `config.yaml`.
10. Preserves existing gateway TLS files.
11. Installs missing TLS files from the deployment directory.
12. Installs or removes the managed updater according to the selected mode.
13. Applies restricted NTFS permissions.
14. Sets Windows Defender Firewall default inbound behavior to block.
15. Removes the older unrestricted gateway firewall rule when present.
16. Adds a source- and port-restricted inbound firewall rule.
17. Registers the gateway startup task under `LOCAL SERVICE`.
18. Registers the hourly update task under `SYSTEM` in production mode.
19. Starts the gateway.
20. Waits for the Admin API to become ready.
21. Rolls back the installed executable, scripts, and task definitions when installation fails after replacement.

# Installed Layout

With the default Makefile settings, the permanent installation is:

```text
C:\FBS\fbs-interlock-gateway\
├── fbs-interlock-gateway.exe
├── config.yaml
├── start.bat
├── update.bat              # production mode only
├── update.ps1              # production mode only
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

Timestamped executable backups created by successful update attempts are stored beside the executable:

```text
fbs-interlock-gateway.exe.backup.YYYYMMDDTHHMMSSZ
```

# Runtime Accounts and File Permissions

## Gateway task

The main gateway task runs as:

```text
NT AUTHORITY\LOCAL SERVICE
```

The installer grants this account only the access needed for operation:

- Read and execute access to the executable and startup script
- Read access to `config.yaml`
- Read access to the TLS certificate and private-key files
- Modify access to the log directory

## Update task

The update task runs as:

```text
SYSTEM
```

Administrative execution is limited to the update workflow because it must:

- Download and verify a release executable
- Stop and start the gateway task
- Replace the protected installed executable
- Restore the previous executable after a failed health check
- Rotate protected log files

## Administrative access

The installer restricts the installation tree to:

- Local Administrators: full control
- `SYSTEM`: full control
- `LOCAL SERVICE`: the minimum runtime access described above

Other ordinary local users are not granted direct access to the gateway configuration or client private key.

# Verify the Installation

Open PowerShell as an administrator.

## Verify the gateway task

```powershell
Get-ScheduledTask -TaskName "FBS Interlock Gateway"
```

Expected state while the gateway is running:

```text
Running
```

View detailed task information:

```powershell
Get-ScheduledTaskInfo -TaskName "FBS Interlock Gateway"
```

Confirm the runtime account:

```powershell
(Get-ScheduledTask -TaskName "FBS Interlock Gateway").Principal
```

The user ID should resolve to `LOCAL SERVICE`.

## Verify the update task

Production installation:

```powershell
Get-ScheduledTask -TaskName "FBS Interlock Gateway Update"
```

Development installation should not have this task:

```powershell
Get-ScheduledTask `
    -TaskName "FBS Interlock Gateway Update" `
    -ErrorAction SilentlyContinue
```

## Verify the executable version

```powershell
& "C:\FBS\fbs-interlock-gateway\fbs-interlock-gateway.exe" -version
```

## Verify the Admin API

```powershell
Invoke-RestMethod http://127.0.0.1:18090/api/status
```

## Verify installed TLS files

```powershell
Get-ChildItem C:\FBS\fbs-interlock-gateway\tls
```

Expected files:

```text
server-ca.crt
gateway-client.crt
gateway-client.key
```

Inspect the private-key permissions:

```powershell
Get-Acl `
    C:\FBS\fbs-interlock-gateway\tls\gateway-client.key |
    Format-List
```

## Verify listening ports

```powershell
Get-NetTCPConnection `
    -State Listen `
    -ErrorAction SilentlyContinue |
    Where-Object {
        $_.LocalPort -eq 18090 -or
        ($_.LocalPort -ge 8081 -and $_.LocalPort -le 8981)
    } |
    Sort-Object LocalPort
```

# Verify Tool Communication

Replace `8081` with the configured listener port for the tool being tested.

Check status:

```powershell
Invoke-RestMethod http://127.0.0.1:8081/status
```

Turn the interlock on:

```powershell
Invoke-RestMethod http://127.0.0.1:8081/on
```

Turn the interlock off:

```powershell
Invoke-RestMethod http://127.0.0.1:8081/off
```

Use `/on` and `/off` only when it is operationally safe to change the tool state.

For an HTTPS or mutual-TLS Shelly configuration, review `gateway-error.log` for certificate, hostname, or trust-chain failures.

# View the Admin Panel

On the gateway machine, open the following address in a web browser:

<http://127.0.0.1:18090>

The Admin status API is:

```text
http://127.0.0.1:18090/api/status
```

The Admin UI is intentionally local. Do not change it to a non-loopback bind address without a reviewed authentication, authorization, firewall, and transport-security design.

# Automatic Updates and Log Maintenance

The production installer creates:

```text
FBS Interlock Gateway Update
```

The task starts at minute 17 and repeats once per hour.

## Update sequence

Each run performs the following sequence:

1. Requires administrative execution.
2. Acquires an update lock to prevent overlapping update operations.
3. Downloads the latest published SHA-256 checksum first.
4. Calculates the installed executable SHA-256 checksum.
5. Skips the executable download when the installed checksum already matches.
6. Checks whether gateway logs require rotation.
7. Downloads the release executable only when the checksum differs.
8. Verifies the downloaded SHA-256 checksum.
9. Verifies that the download is a valid amd64 PE executable.
10. Runs the downloaded executable with `-version`.
11. Backs up the current executable.
12. Stops the gateway task.
13. Rotates oversized gateway logs.
14. Stages and verifies the replacement executable.
15. Starts the gateway task.
16. Waits for the Admin API health check.
17. Restores the previous executable when the health check fails.

The updater modifies only the application executable and gateway logs. It does not replace:

- `config.yaml`
- `server-ca.crt`
- `gateway-client.crt`
- `gateway-client.key`

## Run an update check manually

Open PowerShell as an administrator:

```powershell
Start-ScheduledTask -TaskName "FBS Interlock Gateway Update"
```

Follow the updater logs:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\update.log `
    -Wait
```

## Log rotation

During each hourly maintenance check, the updater examines:

```text
gateway.log
gateway-error.log
```

A log is rotated after it reaches 10 MiB.

The updater retains up to 30 compressed archives:

```text
gateway.log.1.zip
gateway.log.2.zip
...
gateway.log.30.zip
```

The same pattern applies to `gateway-error.log`.

# View Gateway Logs

## Standard output

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\gateway.log `
    -Wait
```

## Standard error

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\gateway-error.log `
    -Wait
```

## Update output

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\update.log `
    -Wait
```

## Update errors

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\update-error.log `
    -Wait
```

Press `Ctrl+C` to stop following a log.

# Edit the Configuration

The active configuration file is:

```text
C:\FBS\fbs-interlock-gateway\config.yaml
```

Run the editor as an administrator because ordinary users do not have write access to the protected configuration.

For example:

```powershell
Start-Process notepad.exe `
    -ArgumentList "C:\FBS\fbs-interlock-gateway\config.yaml" `
    -Verb RunAs
```

An existing production configuration is preserved when either installer mode is run again.

After editing `config.yaml`, restart the gateway task.

# Restart the Gateway

Open PowerShell as an administrator:

```powershell
Stop-ScheduledTask -TaskName "FBS Interlock Gateway"
Start-ScheduledTask -TaskName "FBS Interlock Gateway"
```

Verify the Admin API:

```powershell
Invoke-RestMethod http://127.0.0.1:18090/api/status
```

The `start.bat` supervisor provides behavior equivalent to the Linux and macOS restart policy:

- Restart delay: 2 seconds
- Restart window: 60 seconds
- Maximum starts within the window: 10
- Continuous restart after normal or abnormal executable exit until the rate limit is exceeded

Task Scheduler also provides a slower fallback restart policy if the supervisor itself exits.

# Firewall Behavior

The installer sets the default inbound action to `Block` for the Domain, Private, and Public firewall profiles.

It then creates this application-specific allow rule:

```text
Display name: FBS Interlock Gateway - Authorized FBS Source
Direction:    Inbound
Protocol:     TCP
Program:      C:\FBS\fbs-interlock-gateway\fbs-interlock-gateway.exe
Local ports:  8081-8981
Remote IP:    146.6.76.61
Profiles:     Any
```

The installer removes the older broad rule named:

```text
FBS Interlock Gateway
```

Inspect the active rule:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Format-List
```

Inspect its port filter:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallPortFilter |
    Format-List
```

Inspect its remote-address filter:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallAddressFilter |
    Format-List
```

Local loopback access remains available for Admin UI and local tool-port testing.

# Troubleshooting

## Installer reports a missing TLS file

Confirm the development repository contains:

```text
tls/server-ca.crt
tls/gateway-client.crt
tls/gateway-client.key
```

Then rebuild:

```bash
make clean
make build-windows-amd64
```

Do not manually bypass the TLS build check by copying an incomplete deployment directory.

## Installer reports the executable is not amd64

Rebuild with:

```bash
make build-windows-amd64
```

Confirm the deployment directory was not mixed with a Linux or macOS binary.

## The gateway task is Ready instead of Running

Inspect task history and logs:

```powershell
Get-ScheduledTaskInfo -TaskName "FBS Interlock Gateway"
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\gateway-error.log `
    -Tail 100
```

Start it manually:

```powershell
Start-ScheduledTask -TaskName "FBS Interlock Gateway"
```

## The Admin API does not respond

Check whether the gateway task is running:

```powershell
Get-ScheduledTask -TaskName "FBS Interlock Gateway"
```

Check the logs:

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\gateway-error.log `
    -Tail 100
```

Confirm port `18090` is listening:

```powershell
Get-NetTCPConnection -LocalPort 18090 -State Listen
```

## A tool listener does not respond

Confirm the configured listener port is open locally:

```powershell
Test-NetConnection 127.0.0.1 -Port 8081
```

Check the exact tool entry in `config.yaml` and review gateway logs for hostname resolution, authentication, timeout, TLS, or certificate errors.

## The gateway cannot read TLS files

Inspect the ACLs:

```powershell
Get-Acl C:\FBS\fbs-interlock-gateway\tls\gateway-client.key |
    Format-List
```

Re-run `install.bat` as administrator to restore the intended permissions without replacing an existing configuration or TLS identity.

## The update task reports a checksum mismatch

Do not install the downloaded file manually.

Review:

```text
C:\FBS\fbs-interlock-gateway\logs\update-error.log
```

A checksum mismatch causes the updater to stop before replacing the installed executable.

## The update is rolled back

The updater rolls back when the Admin API does not become healthy after replacement.

Review both:

```text
logs\update-error.log
logs\gateway-error.log
```

The timestamped backup executable remains in the installation directory for inspection.

## Windows Firewall blocks expected FBS traffic

Confirm the source address used by FBS matches the generated rule:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallAddressFilter
```

Confirm the local port is within the generated range:

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source" |
    Get-NetFirewallPortFilter
```

Rebuild and reinstall after changing `FBS_SOURCE_IP` or `FBS_PORT_RANGE` in the Makefile.

# Uninstall the Gateway

The normal uninstaller removes managed executable files and operating-system integration while preserving persistent gateway data.

## Standard uninstall

Open the original Windows deployment directory.

Right-click:

```text
uninstall.bat
```

Select:

```text
Run as administrator
```

The standard uninstaller removes:

- Gateway task
- Update task
- Running gateway process
- Windows Defender Firewall rules
- Application executable
- Startup supervisor
- Updater scripts
- Executable backups

It preserves:

- `config.yaml`
- `tls\`
- `logs\`

## Purge uninstall

A purge also removes the configuration, TLS files, and logs.

Open an elevated Command Prompt in the deployment directory:

```bat
./uninstall.bat --purge
```

Equivalent PowerShell command:

```powershell
PowerShell.exe `
    -NoProfile `
    -ExecutionPolicy Bypass `
    -File .\uninstall.ps1 `
    -Purge
```

> **Warning**
>
> A purge deletes the installed gateway client private key and local logs. Confirm that the required certificate material is backed up before using it.

# Command Reference

## Build

```bash
make clean
make build-windows-amd64
```

## Production install

```text
Right-click install.bat -> Run as administrator
```

## Development install

```text
Right-click install-dev.bat -> Run as administrator
```

## Gateway task status

```powershell
Get-ScheduledTask -TaskName "FBS Interlock Gateway"
Get-ScheduledTaskInfo -TaskName "FBS Interlock Gateway"
```

## Update task status

```powershell
Get-ScheduledTask -TaskName "FBS Interlock Gateway Update"
Get-ScheduledTaskInfo -TaskName "FBS Interlock Gateway Update"
```

## Restart gateway

```powershell
Stop-ScheduledTask -TaskName "FBS Interlock Gateway"
Start-ScheduledTask -TaskName "FBS Interlock Gateway"
```

## Run update check

```powershell
Start-ScheduledTask -TaskName "FBS Interlock Gateway Update"
```

## Admin API

```powershell
Invoke-RestMethod http://127.0.0.1:18090/api/status
```

## Tool status

```powershell
Invoke-RestMethod http://127.0.0.1:8081/status
```

## Follow gateway logs

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\gateway.log `
    -Wait
```

## Follow gateway errors

```powershell
Get-Content `
    C:\FBS\fbs-interlock-gateway\logs\gateway-error.log `
    -Wait
```

## View firewall rule

```powershell
Get-NetFirewallRule `
    -DisplayName "FBS Interlock Gateway - Authorized FBS Source"
```

## Standard uninstall

```text
Right-click uninstall.bat -> Run as administrator
```

## Purge uninstall

```bat
uninstall.bat --purge
```
