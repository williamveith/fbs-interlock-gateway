# Linux Install Instructions

## Build the Deployment Assets

On the development machine, run:

```bash
make clean
make build-linux-amd64
```

This will generate the Linux deployment folder at:

```text
build/linux/
```

## Copy to USB Drive

Copy the entire directory below onto a USB flash drive:

```text
build/linux/
```

Do not copy only the binary. The full directory is needed because it contains the installer, service files, update files, config file, and deployment instructions.

## Install on the Linux Computer

Plug the USB flash drive into the target Linux computer.

Open a terminal in the `linux` directory on the USB flash drive, then run:

```bash
chmod +x install.sh update.sh fbs-interlock-gateway
sudo ./install.sh
```

## What the Installer Does

The installer will:

* Install the application binary to:

```text
/opt/fbs-interlock-gateway/
```

* Install `config.yaml` to:

```text
/etc/fbs-interlock-gateway/config.yaml
```

* Install the systemd service
* Enable and restart the service
* Install the updater and update timer, if present

## Check Service Status

After installation, check that the service is running:

```bash
sudo systemctl status fbs-interlock-gateway
```

## View Live Logs

To view live service logs, run:

```bash
sudo journalctl -u fbs-interlock-gateway -f
```

## Restart the Service Manually

If the config file is edited later, restart the service with:

```bash
sudo systemctl restart fbs-interlock-gateway
```
