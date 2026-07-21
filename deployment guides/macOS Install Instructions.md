# macOS Installation Instructions

## Table of Contents

- [Supported Mac Architectures](#supported-mac-architectures)
- [Build the Deployment Assets](#build-the-deployment-assets)
- [Copy the Deployment Directory to a USB Drive](#copy-the-deployment-directory-to-a-usb-drive)
- [Copy the Deployment Directory to the Gateway Mac](#copy-the-deployment-directory-to-the-gateway-mac)
- [Install the Gateway](#install-the-gateway)
- [What the Installer Does](#what-the-installer-does)
- [Verify That the Gateway Is Running](#verify-that-the-gateway-is-running)
- [View Gateway Logs](#view-gateway-logs)
- [Edit the Configuration](#edit-the-configuration)
- [Restart the Gateway](#restart-the-gateway)
- [View the Admin Panel](#view-the-admin-panel)
- [Firewall Behavior](#firewall-behavior)
- [Gatekeeper and Quarantine](#gatekeeper-and-quarantine)
- [Uninstall the Gateway](#uninstall-the-gateway)

## Supported Mac Architectures

Determine the Mac architecture by running:

```bash
uname -m
```

The expected output is:

- `arm64` for Apple Silicon Macs
- `x86_64` for Intel Macs

Use the deployment directory that matches the gateway Mac.

## Build the Deployment Assets

On the development Mac, run one of the following commands.

For Apple Silicon:

```bash
make clean
make build-darwin-arm64
```

For Intel:

```bash
make clean
make build-darwin-amd64
```

The proposed Makefile rules generate one of these deployment directories:

```text
build/darwin/arm64/
build/darwin/amd64/
```

The selected directory should contain:

```text
fbs-interlock-gateway
config.yaml
install.sh
start.sh
uninstall.sh
com.williamveith.fbs-interlock-gateway.plist
macOS Install Instructions.md
```

## Copy the Deployment Directory to a USB Drive

Copy the complete architecture-specific deployment directory to a USB flash
 drive.

Do not copy only the executable. The complete directory is required because it
contains:

- The application executable
- The installer
- The startup wrapper
- The LaunchDaemon property list
- The uninstaller
- The configuration file
- These deployment instructions

## Copy the Deployment Directory to the Gateway Mac

Insert the USB flash drive into the gateway Mac.

Copy the complete deployment directory into the current user's `Downloads`
directory. For example:

```text
~/Downloads/darwin/arm64/
```

## Install the Gateway

Open Terminal and move to the copied deployment directory:

```bash
cd ~/Downloads/darwin/arm64
```

Make the deployment files executable and run the installer:

```bash
chmod +x install.sh start.sh uninstall.sh fbs-interlock-gateway
sudo ./install.sh
```

Enter the administrator password when prompted.

## What the Installer Does

The installer performs the following actions:

- Installs the executable and startup wrapper in:

  ```text
  /usr/local/libexec/fbs-interlock-gateway/
  ```

- Installs the active configuration file at:

  ```text
  /Library/Application Support/fbs-interlock-gateway/config.yaml
  ```

- Preserves an existing production configuration during reinstallation
- Creates a hidden, non-login service account
- Creates the gateway log directory
- Installs a system-wide LaunchDaemon in:

  ```text
  /Library/LaunchDaemons/com.williamveith.fbs-interlock-gateway.plist
  ```

- Starts the gateway during boot, before any user signs in
- Uses `launchd` to restart the gateway if it exits
- Registers the executable as allowed by the macOS Application Firewall
- Starts the gateway immediately after installation
- Checks whether the admin API is responding

## Verify That the Gateway Is Running

View the LaunchDaemon state:

```bash
sudo launchctl print system/com.williamveith.fbs-interlock-gateway
```

The output should include a running process identifier and a state such as:

```text
state = running
```

Test the admin API:

```bash
curl http://127.0.0.1:18090/api/status
```

To test a tool port, replace `8081` with the configured port for that tool:

```bash
curl http://127.0.0.1:8081/status
curl http://127.0.0.1:8081/on
curl http://127.0.0.1:8081/off
```

## View Gateway Logs

Standard output is written to:

```text
/Library/Logs/fbs-interlock-gateway/gateway.log
```

Standard error is written to:

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

## Edit the Configuration

The active configuration file is:

```text
/Library/Application Support/fbs-interlock-gateway/config.yaml
```

Edit it with an administrator-capable editor. For example:

```bash
sudo nano "/Library/Application Support/fbs-interlock-gateway/config.yaml"
```

An existing production configuration is preserved when the installer is run
again.

Restart the gateway after manually changing the configuration.

## Restart the Gateway

Restart the LaunchDaemon:

```bash
sudo launchctl kickstart -k \
  system/com.williamveith.fbs-interlock-gateway
```

Verify that it restarted:

```bash
sudo launchctl print system/com.williamveith.fbs-interlock-gateway
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

## Firewall Behavior

The installer adds the executable to the macOS Application Firewall allow
list. The built-in Application Firewall grants or blocks incoming access by
application; it does not reproduce the Linux UFW rule that permits only one
source IP across a port range.

For production deployment, preserve the existing FBS-only restriction using
one of these controls:

- A campus or network firewall rule
- A separately reviewed macOS Packet Filter (`pf`) rule
- A source-IP allowlist implemented by the gateway itself

Do not rely on the Application Firewall entry alone as an equivalent to:

```text
allow from the FBS source IP to TCP ports 8081:8981
```

## Gatekeeper and Quarantine

A locally built binary copied by USB normally does not require additional
steps. If macOS reports that a deployment file cannot be opened because it is
quarantined, inspect the quarantine attributes first:

```bash
xattr -l fbs-interlock-gateway
```

Remove the quarantine attribute only when the files came from your trusted
build process:

```bash
xattr -dr com.apple.quarantine .
```

Then rerun the installer.

## Uninstall the Gateway

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
- Preserve the existing gateway logs
