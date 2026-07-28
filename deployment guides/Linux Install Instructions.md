# Linux Installation Instructions

## Table of Contents

- [Set Up the Gateway Machine](#set-up-the-gateway-machine)
  - [Install the Operating System](#install-the-operating-system)
  - [Create the User Account](#create-the-user-account)

- [Deploy the Software](#deploy-the-software)
  - [Prepare the Gateway TLS Files](#prepare-the-gateway-tls-files)
- [Build the Deployment Assets](#build-the-deployment-assets)
  - [Copy the Deployment Directory to a USB Drive](#copy-the-deployment-directory-to-a-usb-drive)
  - [Copy the Deployment Directory to the Gateway Machine](#copy-the-deployment-directory-to-the-gateway-machine)
  - [Install the Gateway](#install-the-gateway)
  - [What the Installer Does](#what-the-installer-does)
  - [Verify the Installed TLS Files](#verify-the-installed-tls-files)
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


## Prepare the Gateway TLS Files

Before building the Linux deployment directory, generate the certificate
authorities and gateway client identity:

```bash
make ca
make gateway-cert
```

The gateway certificate command populates the repository runtime directory:

```text
tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

Only these three runtime files are packaged for the Linux gateway. The complete
`pki/` directory, CA private keys, Shelly certificates, CSRs, and
`client-ca.crt` are not copied to the gateway.

The installed configuration uses:

```yaml
defaults:
  shelly_tls:
    server_ca_file: "./tls/server-ca.crt"
    client_cert_file: "./tls/gateway-client.crt"
    client_key_file: "./tls/gateway-client.key"
```

The systemd service runs with `/etc/fbs-interlock-gateway` as its working
directory, so these relative paths point to:

```text
/etc/fbs-interlock-gateway/tls/
```

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

The build fails if any required runtime TLS file is missing.

## Copy the Deployment Directory to a USB Drive

Copy the entire `build/linux/` directory to a USB flash drive.

Do **not** copy only the application binary. The complete directory is required because it contains:

- The application binary
- The installer
- The systemd service files
- The updater and update timer files
- The configuration file
- The gateway runtime TLS directory
- These deployment instructions

## Copy the Deployment Directory to the Gateway Machine

Insert the USB flash drive into the gateway machine.

Copy the `linux` directory from the USB flash drive into the current user's `Downloads` directory.

The resulting directory should look similar to:

```text
~/Downloads/linux/
├── fbs-interlock-gateway
├── install.sh
├── update.sh
├── config.yaml
├── tls
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── ...
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

- Installs the gateway runtime TLS files at:
  ```
  /etc/fbs-interlock-gateway/tls/
  ```

- Creates the gateway service account when needed
- Installs the systemd service
- Enables and starts the gateway service
- Installs the updater and update timer when their files are present

An existing production configuration file is preserved during reinstallation.
Existing gateway TLS files are also preserved. The hourly updater replaces only
the application binary and does not modify the configuration or TLS files.


## Verify the Installed TLS Files

Confirm that all three files are installed:

```bash
sudo ls -l /etc/fbs-interlock-gateway/tls
```

Expected files:

```text
server-ca.crt
gateway-client.crt
gateway-client.key
```

Confirm that the service account can read them:

```bash
sudo -u fbs-gateway test \
  -r /etc/fbs-interlock-gateway/tls/server-ca.crt

sudo -u fbs-gateway test \
  -r /etc/fbs-interlock-gateway/tls/gateway-client.crt

sudo -u fbs-gateway test \
  -r /etc/fbs-interlock-gateway/tls/gateway-client.key
```

Each command should exit without printing an error.

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