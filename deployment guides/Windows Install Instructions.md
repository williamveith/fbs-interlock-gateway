# Windows Installation Instructions

## Table of Contents

- [Build the Deployment Assets](#build-the-deployment-assets)
- [Copy the Deployment Directory to a USB Drive](#copy-the-deployment-directory-to-a-usb-drive)
- [Copy the Deployment Directory to the Gateway Machine](#copy-the-deployment-directory-to-the-gateway-machine)
- [Install the Gateway](#install-the-gateway)
- [What the Installer Does](#what-the-installer-does)
- [Verify That the Gateway Is Running](#verify-that-the-gateway-is-running)
- [View Gateway Logs](#view-gateway-logs)
- [Edit the Configuration](#edit-the-configuration)
- [Restart the Gateway](#restart-the-gateway)
- [View the Admin Panel](#view-the-admin-panel)
- [Uninstall the Gateway](#uninstall-the-gateway)

## Build the Deployment Assets

On the development machine, run:

```bash
make clean
make build-windows-amd64
```

This generates the Windows deployment directory:

```text
build/windows/
```

The directory should contain:

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

## Copy the Deployment Directory to a USB Drive

Copy the entire `build/windows/` directory to a USB flash drive.

Do not copy only the application executable. The complete directory is
required because it contains:

- The application executable
- The installer
- The startup script
- The uninstaller
- The configuration file
- These deployment instructions

## Copy the Deployment Directory to the Gateway Machine

Insert the USB flash drive into the Windows gateway machine.

Copy the complete `windows` directory from the USB flash drive to the
gateway machine. The deployment directory may be placed in the current
user's `Downloads` directory.

For example:

```text
C:\Users\<username>\Downloads\windows
```

The installer will copy the required files into the permanent installation
directory.

## Install the Gateway

Open the copied `windows` deployment directory.

Right-click:

```text
install.bat
```

Select:

```text
Run as administrator
```

Approve the Windows User Account Control prompt when it appears.

The installer will install and start the FBS Interlock Gateway.

## What the Installer Does

The installer performs the following actions:

- Installs the gateway in:

  ```text
  C:\FBS\fbs-interlock-gateway
  ```

- Installs the active configuration file at:

  ```text
  C:\FBS\fbs-interlock-gateway\config.yaml
  ```

- Preserves an existing production configuration during reinstallation
- Creates the gateway log directory
- Adds a Windows Firewall rule for the gateway executable
- Registers the gateway with Windows Task Scheduler
- Runs the gateway as the Windows `SYSTEM` account
- Starts the gateway automatically when Windows starts
- Configures Windows to restart the task following an unexpected failure
- Starts the gateway immediately after installation
- Checks whether the admin API is responding

## Verify That the Gateway Is Running

Open PowerShell as an administrator.

Check the scheduled task:

```powershell
Get-ScheduledTask -TaskName "FBS Interlock Gateway"
```

The task state should be:

```text
Running
```

View detailed task information:

```powershell
Get-ScheduledTaskInfo -TaskName "FBS Interlock Gateway"
```

Test the admin API:

```powershell
Invoke-RestMethod http://127.0.0.1:18090/api/status
```

To test a tool port, replace `8081` with the configured port for that tool:

```powershell
Invoke-RestMethod http://127.0.0.1:8081/status
Invoke-RestMethod http://127.0.0.1:8081/on
Invoke-RestMethod http://127.0.0.1:8081/off
```

## View Gateway Logs

The gateway log is located at:

```text
C:\FBS\fbs-interlock-gateway\logs\gateway.log
```

To follow the log from PowerShell:

```powershell
Get-Content `
    "C:\FBS\fbs-interlock-gateway\logs\gateway.log" `
    -Wait
```

Press `Ctrl+C` to stop following the log.

## Edit the Configuration

The active configuration file is:

```text
C:\FBS\fbs-interlock-gateway\config.yaml
```

An existing production configuration file is preserved when the installer is
run again.

After manually editing `config.yaml`, restart the gateway.

## Restart the Gateway

Open PowerShell as an administrator and run:

```powershell
Stop-ScheduledTask -TaskName "FBS Interlock Gateway"
Start-ScheduledTask -TaskName "FBS Interlock Gateway"
```

Verify that the gateway restarted:

```powershell
Get-ScheduledTask -TaskName "FBS Interlock Gateway"
```

## View the Admin Panel

The admin panel is available at:

```text
http://127.0.0.1:18090
```

The admin status API is available at:

```text
http://127.0.0.1:18090/api/status
```

## Uninstall the Gateway

Open the original Windows deployment directory.

Right-click:

```text
uninstall.bat
```

Select:

```text
Run as administrator
```

The uninstaller will:

- Stop the running gateway
- Remove the scheduled task
- Remove the Windows Firewall rule
- Remove the installed application files
- Preserve the production `config.yaml` file