# Linux Install Instructions

## Table of Contents

* [Set Up the Gateway Machine](#set-up-the-gateway-machine)

  * [Install Dependencies](#install-dependencies)
  * [Configure the Firewall](#configure-the-firewall)
* [Deploy the Software](#deploy-the-software)

  * [Build the Deployment Assets](#build-the-deployment-assets)
  * [Copy the Deployment Directory to a USB Drive](#copy-the-deployment-directory-to-a-usb-drive)
  * [Copy the Deployment Directory to the Gateway Machine](#copy-the-deployment-directory-to-the-gateway-machine)
  * [Install the Gateway](#install-the-gateway)
  * [What the Installer Does](#what-the-installer-does)
  * [Check the Service Status](#check-the-service-status)
  * [View Live Logs](#view-live-logs)
  * [Restart the Service Manually](#restart-the-service-manually)

## Set Up the Gateway Machine

Install Debian GNU/Linux 12 (Bookworm) with GNOME 43.9:

https://www.debian.org/releases/bookworm/debian-installer/

### Install Dependencies

Update the package index and install the required dependencies:

```bash
sudo apt update

sudo apt install --no-install-recommends \
  lsof \
  curl \
  ca-certificates \
  ufw
```

### Configure the Firewall

Configure UFW to deny incoming traffic by default, allow outgoing traffic, and permit gateway connections only from the authorized FBS server IP address:

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow from 146.6.76.61 to any port 8081:8981 proto tcp
sudo ufw enable
sudo ufw status verbose
```

Confirm that UFW is active and that TCP ports `8081` through `8981` are accessible only from `146.6.76.61`.

## Deploy the Software

### Build the Deployment Assets

On the development machine, run:

```bash
make clean
make build-linux-amd64
```

This generates the Linux deployment directory:

```text
build/linux/
```

### Copy the Deployment Directory to a USB Drive

Copy the entire `build/linux/` directory to a USB flash drive.

Do not copy only the application binary. The complete directory is required because it contains:

* The application binary
* The installer
* The systemd service files
* The updater and update timer files
* The configuration file
* The deployment instructions

### Copy the Deployment Directory to the Gateway Machine

Insert the USB flash drive into the gateway machine.

Copy the `linux` directory from the USB flash drive into the current user's `Downloads` directory.

The resulting directory should be similar to:

```text
~/Downloads/linux/
```

### Install the Gateway

Move to the `Downloads` folder, make the deployment files executable, and run the installer:

```bash
cd ~/Downloads/linux
chmod +x install.sh update.sh fbs-interlock-gateway
sudo ./install.sh
```

### What the Installer Does

The installer performs the following actions:

* Installs application binary in: /opt/fbs-interlock-gateway/
* Installs configuration file at: /etc/fbs-interlock-gateway/config.yaml

* Creates the gateway service account when needed
* Installs the systemd service
* Enables and starts the gateway service
* Installs the updater and update timer when their files are present

An existing production configuration file is preserved during reinstallation.

### Check the Service Status

Verify that the service is running:

```bash
sudo systemctl status fbs-interlock-gateway.service --no-pager --full
```

Verify that systemd reports the service as active:

```bash
sudo systemctl is-active fbs-interlock-gateway.service
```

### View Live Logs

To follow the gateway service logs:

```bash
sudo journalctl -u fbs-interlock-gateway.service -f
```

Press `Ctrl+C` to stop following the logs.

### Restart the Service Manually

After manually editing the configuration file, restart the service and verify that it restarted successfully:

```bash
sudo systemctl restart fbs-interlock-gateway.service
sudo systemctl status fbs-interlock-gateway.service --no-pager --full
```
