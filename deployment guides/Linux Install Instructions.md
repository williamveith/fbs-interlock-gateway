\begin{titlepage}
\thispagestyle{empty}
\centering
\vspace*{\fill}

{\Huge\bfseries FBS Interlock Gateway\par}
\vspace{1em}
{\LARGE Linux Installation Instructions\par}

\vspace*{\fill}
\end{titlepage}

\pagenumbering{roman}
\renewcommand{\contentsname}{Table of Contents}
\setcounter{tocdepth}{2}
\tableofcontents
\clearpage

\pagenumbering{arabic}

# Set Up the Gateway Machine

## Install the Operating System

Install [**Debian GNU/Linux 12 (Bookworm)** with **GNOME 43.9**](https://www.debian.org/releases/bookworm/debian-installer/).

## Create the User Account

During installation, create the user account:

```text
fbs-gateway
```

After installation, open a root shell using PolicyKit and add `fbs-gateway` to the `sudo` group:

```bash
pkexec bash
usermod -aG sudo fbs-gateway
exit
```

Reboot the gateway machine:

```bash
sudo reboot
```

\newpage

# Deploy the Software

## Prepare the Gateway TLS Files

Before building the Linux deployment directory, generate the certificate authorities and gateway client identity:

```bash
make ca
make gateway-cert
```

The gateway certificate command populates the repository's runtime TLS directory:

```text
tls/
├── server-ca.crt
├── gateway-client.crt
└── gateway-client.key
```

Only these three runtime files are packaged for the Linux gateway.

The following files are **not** copied to the gateway:

- The complete `pki/` directory
- CA private keys
- Shelly private keys and certificates
- Certificate signing requests
- `client-ca.crt`

> **Security note:** `gateway-client.key` is a private key. Keep the development machine, deployment directory, and USB drive secure. Remove temporary copies after installation when they are no longer needed.

The installed configuration uses the following relative paths:

```yaml
defaults:
  shelly_tls:
    server_ca_file: "./tls/server-ca.crt"
    client_cert_file: "./tls/gateway-client.crt"
    client_key_file: "./tls/gateway-client.key"
```

The systemd service uses the following working directory:

```text
/etc/fbs-interlock-gateway
```

Therefore, the relative TLS paths resolve to:

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

Do **not** copy only the application binary. The complete deployment directory is required because it contains:

- The application binary
- The standard and development installers
- The uninstaller
- The systemd service files
- The updater service and timer
- The update script
- The configuration file
- The gateway runtime TLS directory
- These installation instructions

## Copy the Deployment Directory to the Gateway Machine

Insert the USB flash drive into the gateway machine.

Copy the `linux` directory from the USB drive into the current user's `Downloads` directory.

The resulting directory should look similar to:

```text
~/Downloads/linux/
├── fbs-interlock-gateway
├── install.sh
├── install-dev.sh
├── uninstall.sh
├── update.sh
├── config.yaml
├── tls/
│   ├── server-ca.crt
│   ├── gateway-client.crt
│   └── gateway-client.key
└── ...
```

## Install the Gateway

Choose either the standard installation or the development installation.

### Standard Installation

```bash
cd ~/Downloads/linux
chmod +x install.sh uninstall.sh update.sh fbs-interlock-gateway
sudo ./install.sh
```

### Development Installation

```bash
cd ~/Downloads/linux
chmod +x install-dev.sh uninstall.sh update.sh fbs-interlock-gateway
sudo ./install-dev.sh
```

## What the Installer Does

The installer performs the following actions:

| Component | Installed location or action |
|---|---|
| Application binary | `/opt/fbs-interlock-gateway/fbs-interlock-gateway` |
| Uninstaller | `/opt/fbs-interlock-gateway/uninstall.sh` |
| Configuration file | `/etc/fbs-interlock-gateway/config.yaml` |
| Gateway TLS files | `/etc/fbs-interlock-gateway/tls/` |
| Service account | Creates the gateway service account when required |
| Gateway service | Installs, enables, and starts the systemd service |
| Automatic updater | Installs the updater service and timer when their files are present |

During reinstallation:

- An existing production configuration file is preserved.
- Existing gateway TLS files are preserved.
- The hourly updater replaces only the application binary.
- The updater does not modify the configuration file or TLS files.

## Verify the Installed TLS Files

Confirm that the TLS directory contains all three required files:

```bash
sudo ls -l /etc/fbs-interlock-gateway/tls
```

Expected files:

```text
server-ca.crt
gateway-client.crt
gateway-client.key
```

Confirm that the `fbs-gateway` service account can read each file:

```bash
sudo -u fbs-gateway test -r /etc/fbs-interlock-gateway/tls/server-ca.crt &&
sudo -u fbs-gateway test -r /etc/fbs-interlock-gateway/tls/gateway-client.crt &&
sudo -u fbs-gateway test -r /etc/fbs-interlock-gateway/tls/gateway-client.key &&
echo "All gateway TLS files are readable."
```

If all files are readable, the command prints:

```text
All gateway TLS files are readable.
```

## Check the Service Status

### View the Complete Service Status

```bash
sudo systemctl status fbs-interlock-gateway.service \
  --no-pager \
  --full
```

### Confirm That the Service Is Active

```bash
sudo systemctl is-active fbs-interlock-gateway.service
```

Expected result:

```text
active
```

### Confirm That the Service Is Enabled

```bash
sudo systemctl is-enabled fbs-interlock-gateway.service
```

Expected result:

```text
enabled
```

## View Live Logs

Follow the gateway service logs:

```bash
sudo journalctl \
  -u fbs-interlock-gateway.service \
  -f
```

Press **Ctrl+C** to stop following the logs.

## Restart the Service Manually

After manually editing the configuration file, restart the service:

```bash
sudo systemctl restart fbs-interlock-gateway.service
```

Verify that the service restarted successfully:

```bash
sudo systemctl status fbs-interlock-gateway.service \
  --no-pager \
  --full
```

## View the Admin Panel

On the gateway machine, open the following address in a web browser:

<http://127.0.0.1:18090>

The admin panel is bound to the local loopback interface and is therefore accessible only from the gateway machine.

## Uninstall the Gateway

The installed uninstaller removes the gateway executable, updater, systemd units, and the gateway-specific UFW rule. It does not disable UFW or reverse system-wide default firewall policies.

### Standard Uninstall

Run:

```bash
sudo /opt/fbs-interlock-gateway/uninstall.sh
```

The standard uninstall preserves:

```text
/etc/fbs-interlock-gateway/config.yaml
/etc/fbs-interlock-gateway/tls/
```

It also preserves the `fbs-gateway` service account and group so that a later reinstallation can reuse them safely.

### Purge Configuration and TLS Files

To remove the installed configuration and gateway TLS files as well, run:

```bash
sudo /opt/fbs-interlock-gateway/uninstall.sh --purge
```

The `--purge` option removes the complete directory:

```text
/etc/fbs-interlock-gateway/
```

The service account and group are still preserved. Existing gateway entries in `journald` are not explicitly deleted; they remain subject to the machine's normal journal retention policy.

### Run the Deployment-Copy Uninstaller

The deployment directory also contains `uninstall.sh`, so it can be used if the installed copy is unavailable:

```bash
cd ~/Downloads/linux
sudo ./uninstall.sh
```

Add `--purge` to that command when the configuration and TLS files must also be removed.
