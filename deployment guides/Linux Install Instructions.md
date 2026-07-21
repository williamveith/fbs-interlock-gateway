# Linux Installation Instructions

## Table of Contents

- [Set Up the Gateway Machine](#set-up-the-gateway-machine)
  - [Install the Operating System](#install-the-operating-system)
  - [Create the User Account](#create-the-user-account)

- [Deploy the Software](#deploy-the-software)
  - [Build the Deployment Assets](#build-the-deployment-assets)
  - [Copy the Deployment Directory to a USB Drive](#copy-the-deployment-directory-to-a-usb-drive)
  - [Copy the Deployment Directory to the Gateway Machine](#copy-the-deployment-directory-to-the-gateway-machine)
  - [Install the Gateway](#install-the-gateway)
  - [What the Installer Does](#what-the-installer-does)
  - [Check the Service Status](#check-the-service-status)
  - [View Live Logs](#view-live-logs)
  - [Restart the Service Manually](#restart-the-service-manually)
  - [View the Admin Panel](#view-the-admin-panel)

---

# Set Up the Gateway Machine

## Install the Operating System

Install **Debian GNU/Linux 12 (Bookworm)** with **GNOME 43.9**:

https://www.debian.org/releases/bookworm/debian-installer/

## Create the User Account

Create the user **`fbs-gateway`** during installation and add it to the `sudo` group.

Open a root shell using PolicyKit:

```bash
pkexec bash
```

Add `fbs-gateway` to the `sudo` group:

```bash
usermod -aG sudo fbs-gateway
```

Reboot the machine.

---

# Deploy the Software

## Build the Deployment Assets

On the development machine, run:

```bash
make clean
make build-linux-amd64
```

This generates the Linux deployment directory:

```text
build/linux/
```

## Copy the Deployment Directory to a USB Drive

Copy the entire `build/linux/` directory to a USB flash drive.

Do **not** copy only the application binary. The complete directory is required because it contains:

- The application binary
- The installer
- The systemd service files
- The updater and update timer files
- The configuration file
- These deployment instructions

## Copy the Deployment Directory to the Gateway Machine

Insert the USB flash drive into the gateway machine.

Copy the `linux` directory from the USB flash drive into the current user's `Downloads` directory.

The resulting directory should look similar to:

```text
~/Downloads/linux/
```

## Install the Gateway

Move to the deployment directory, make the deployment files executable, and run the installer:

```bash
cd ~/Downloads/linux
chmod +x install.sh update.sh fbs-interlock-gateway
sudo ./install.sh
```

## What the Installer Does

The installer performs the following actions:

- Installs the application binary in:
  ```
  /opt/fbs-interlock-gateway/
  ```

- Installs the configuration file at:
  ```
  /etc/fbs-interlock-gateway/config.yaml
  ```

- Creates the gateway service account when needed
- Installs the systemd service
- Enables and starts the gateway service
- Installs the updater and update timer when their files are present

An existing production configuration file is preserved during reinstallation.

## Check the Service Status

Verify that the service is running:

```bash
sudo systemctl status fbs-interlock-gateway.service --no-pager --full
```

Verify that systemd reports the service as active:

```bash
sudo systemctl is-active fbs-interlock-gateway.service
```

## View Live Logs

To follow the gateway service logs:

```bash
sudo journalctl -u fbs-interlock-gateway.service -f
```

Press **Ctrl+C** to stop following the logs.

## Restart the Service Manually

After manually editing the configuration file, restart the service and verify that it restarted successfully:

```bash
sudo systemctl restart fbs-interlock-gateway.service
sudo systemctl status fbs-interlock-gateway.service --no-pager --full
```

## View the Admin Panel

The admin panel is available at:

```
http://127.0.0.1:18090
```