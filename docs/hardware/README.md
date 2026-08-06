# Shelly Interlock Hardware

This directory contains the wiring diagrams, junction-box label artwork, and general construction notes for the Shelly-based interlock hardware used with `fbs-interlock-gateway`.

The repository contains two power configurations:

1. an interlock box powered from an external 12 VDC supply
2. an interlock box powered from the tool's 110–240 VAC supply

These documents describe the associated hardware design. They do not replace qualified electrical review, applicable electrical codes, equipment ratings, lockout/tagout procedures, or facility safety requirements. Verify the terminal layout and ratings printed on the exact Shelly model before wiring it.

## General materials

| Component | Specification |
| --- | --- |
| Junction box | Black ABS; nominal size 3.5 W × 4.5 H × 2.2 D; IP65 |
| Cable glands | Black nylon PA66; IP68 |
| Wireless relay | Shelly 1 Gen4 or Shelly 1 Gen3 |
| Two-conductor connector | WAGO 221-412 lever-nut splicing connector |
| Three-conductor connector | WAGO 221-413 lever-nut splicing connector |

The enclosure size is recorded as supplied for the selected box; confirm the manufacturer's dimensional units and internal clearances before fabrication.

## Wiring configurations

### Powered from an external 12 VDC supply

This configuration powers the Shelly from a separate 12 VDC source while using the relay's dry contacts to switch the tool interlock circuit.

<img src="Wiring Powered From 12VDC.svg" width="300">

### Powered from the tool's 110–240 VAC supply

This configuration powers the Shelly from the tool's 110–240 VAC supply while using the relay output for the tool interlock connection.

<img src="Wiring Powered From Tool (110-240VAC).svg" width="300">

## Junction-box labels

The label artwork identifies the power configuration and external connections on the lid of each interlock box.

### 12 VDC label

<img src="12VDC Junction Box Label.svg" width="300">

### 110–240 VAC label

<img src="110-240VAC Junction Box Label.svg" width="300">

## QR-code device identity

Each junction-box label includes a QR code containing serialized JSON that identifies the installed Shelly relay. The record uses three fields:

| Field | Purpose |
| --- | --- |
| `device` | Human-readable Shelly product name |
| `model` | Shelly hardware model number |
| `id` | Unique Shelly device ID |

Example:

```json
{"device": "Shelly 1 Gen4", "model": "S4SW-001X16EU", "id": "A085E3B5325C"}
```

Scanning the label with a phone provides a fast, auditable way to collect the device identity during installation, inspection, maintenance, inventory reconciliation, or troubleshooting. The scanned JSON can be copied directly into an inventory or service record without manually transcribing the product name, model number, and device ID.

Keep the field names stable so the QR data can be parsed consistently. Update the QR code whenever the installed Shelly is replaced. Do not encode passwords, certificates, private keys, network credentials, or other secrets in the label.

## Fiber-laser marking parameters

The example lid labels were produced with a 30 W fiber laser using these recorded settings:

| Parameter | Setting |
| --- | ---: |
| Speed | 60% |
| Power | 30% |
| Frequency | 80 |

These values are machine settings rather than universal material-processing specifications. Verify focus, marking area, coating response, ventilation, and test results on a spare lid before marking a production enclosure.
