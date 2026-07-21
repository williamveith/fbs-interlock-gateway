# Windows Install Instructions

## Build the Deployment Assets

On the development machine, run:

```bash
make clean
make build-windows-amd64
```

This will generate the Windows deployment folder at:

```text
build/windows/
```

The folder should contain:

```text
fbs-interlock-gateway.exe
config.yaml
start.bat
```

## Copy to USB Drive

Copy the entire directory below onto a USB flash drive:

```text
build/windows/
```

Do not copy only the `.exe` file. The full directory is needed because it contains the application, config file, and startup script.

## Install on the Windows Computer

Plug the USB flash drive into the target Windows computer.

Create the install directory:

```text
C:\FBS\fbs-interlock-gateway
```

Copy the full contents of the USB `windows` folder into:

```text
C:\FBS\fbs-interlock-gateway
```

The final folder should look like:

```text
C:\FBS\fbs-interlock-gateway\
├── fbs-interlock-gateway.exe
├── config.yaml
└── start.bat
```

## Start the Gateway

Open the install folder:

```text
C:\FBS\fbs-interlock-gateway
```

Double-click:

```text
start.bat
```

This will start the FBS Interlock Gateway from the install directory.

## Verify That It Is Running

Open a browser and go to:

```text
http://127.0.0.1:18090/api/status
```

Or test from PowerShell:

```powershell
Invoke-RestMethod http://127.0.0.1:18090/api/status
```

To test a tool port, replace `8081` with the configured port for that tool:

```powershell
Invoke-RestMethod http://127.0.0.1:8081/status
Invoke-RestMethod http://127.0.0.1:8081/on
Invoke-RestMethod http://127.0.0.1:8081/off
```

## Edit the Config

The active config file is:

```text
C:\FBS\fbs-interlock-gateway\config.yaml
```

After editing `config.yaml`, stop and restart the gateway.

## Start Automatically on Login

To start the gateway automatically when the Windows user logs in:

1. Press `Win + R`
2. Type:

```text
shell:startup
```

3. Press Enter
4. Create a shortcut to:

```text
C:\FBS\fbs-interlock-gateway\start.bat
```

Place the shortcut in the Startup folder.

The gateway will now start automatically when that Windows user logs in.

## Notes

This Windows deployment runs the gateway as a normal user process.

For a more permanent unattended setup, use Windows Task Scheduler to start:

```text
C:\FBS\fbs-interlock-gateway\start.bat
```

at system startup or user login.
