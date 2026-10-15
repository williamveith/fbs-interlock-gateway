---

title: "FBS Interlock Gateway"
subtitle: "Shelly Interlock Hardware Guide"
author: "William Veith"
date: "2026-09-18"
lang: en-US
-----------

> **Purpose**
>
> This guide documents the supported wiring configurations, junction-box labeling, material selections, device-identification format, fabrication notes, assembly workflow, and verification requirements for the Shelly-based interlock hardware used with `fbs-interlock-gateway`.

> **Security and safety boundary**
>
> The gateway and Shelly relay control software access signals. Hardware interlocks and fail-safe circuitry remain authoritative. This guide does not replace qualified electrical review, applicable electrical codes, equipment ratings, lockout/tagout procedures, or facility safety requirements.
>
> Verify the terminal layout, conductor requirements, supply requirements, and ratings printed on the exact Shelly model and other installed components before wiring or energizing an assembly.

# Table of Contents

* [Hardware Overview](#hardware-overview)
* [Repository Hardware Assets](#repository-hardware-assets)
* [Supported Power Configurations](#supported-power-configurations)
* [Pre-Fabrication Checklist](#pre-fabrication-checklist)
* [General Materials](#general-materials)
* [Bill of Materials](#bill-of-materials)
* [Wiring Configuration: External 12 VDC Supply](#wiring-configuration-external-12-vdc-supply)
* [Wiring Configuration: Tool 110–240 VAC Supply](#wiring-configuration-tool-110240-vac-supply)
* [Junction-Box Labeling](#junction-box-labeling)
* [QR-Code Device Identity](#qr-code-device-identity)
* [Fiber-Laser Marking Parameters](#fiber-laser-marking-parameters)
* [Assembly and Documentation Workflow](#assembly-and-documentation-workflow)
* [Verification Checklist](#verification-checklist)
* [Replacement and Maintenance](#replacement-and-maintenance)
* [File Reference](#file-reference)

<div class="page-break"></div>

# Hardware Overview

The interlock hardware consists of a Shelly 1 relay installed in a labeled junction box and wired between the facility gateway system and the tool's interlock or monitor circuit.

Two power arrangements are supported:

1. an interlock box powered from an external 12 VDC supply
2. an interlock box powered from the tool's 110–240 VAC supply

The logical control path is:

```text
FBS
    -> fbs-interlock-gateway
    -> Shelly RPC over HTTP or HTTPS
    -> Shelly relay
    -> tool interlock / monitor circuit
```

The software, enclosure, wiring, serialized device identity, and physical labeling together form one auditable interlock assembly.

The hardware design is intended to provide a repeatable physical interface to the existing tool interlock or monitor circuit. It does not replace the tool's authoritative hardware safety systems.

# Repository Hardware Assets

The authoritative hardware documentation and artwork are maintained under:

```text
docs/hardware/
├── Shelly Interlock Hardware Guide.md
└── assets
    ├── Junction Box Label - Front.svg
    ├── Junction Box Label - Side.svg
    ├── Wiring Powered From 12VDC.svg
    └── Wiring Powered From Tool (110-240VAC).svg
```

The assets are divided into two categories:

### Wiring diagrams

```text
assets/Wiring Powered From 12VDC.svg
assets/Wiring Powered From Tool (110-240VAC).svg
```

These define the two supported electrical configurations.

### Junction-box labeling

```text
assets/Junction Box Label - Front.svg
assets/Junction Box Label - Side.svg
```

The current enclosure-labeling system uses separate front and side artwork.

The front and side artwork should be treated as a matched labeling set for the completed enclosure. Do not substitute retired or locally modified label files without updating the repository documentation and build record.

# Supported Power Configurations

## External 12 VDC supply

The Shelly is powered from a separate 12 VDC source. The relay's isolated dry contacts switch the tool interlock or monitor circuit.

Use:

```text
docs/hardware/assets/Wiring Powered From 12VDC.svg
```

## Tool 110–240 VAC supply

The Shelly is powered from the tool's 110–240 VAC supply. The relay output is used for the tool interlock connection.

Use:

```text
docs/hardware/assets/Wiring Powered From Tool (110-240VAC).svg
```

The current 110–240 VAC assembly also includes the manual key switch and green indicator identified in the bill of materials. Their connections and installation must follow the current repository wiring drawing.

> **Important**
>
> Do not assume that terminal names, current ratings, voltage ratings, power-input options, or wiring arrangements are identical across Shelly generations or regional variants.
>
> Confirm the markings and documentation for the exact installed device before applying power.

# Pre-Fabrication Checklist

Before constructing or labeling an interlock box, confirm the following:

* The required 12 VDC or 110–240 VAC power configuration has been selected.
* The current repository wiring drawing has been selected for that configuration.
* The exact Shelly generation and model number are known.
* The Shelly terminal arrangement has been checked against the selected wiring diagram.
* The tool interlock or monitor circuit requirements are understood.
* The enclosure has adequate internal clearance for the relay, connectors, wiring, cable glands, and configuration-specific components.
* For the 110–240 VAC configuration, adequate clearance exists for the manual key switch and indicator.
* The cable glands match the intended cable diameters and environmental requirements.
* The current front and side label artwork has been selected.
* The QR data identifies the Shelly installed in that specific enclosure.
* No password, certificate, private key, network credential, or other secret is present in the QR data.
* The enclosure material and coating have been test-marked before production engraving.
* Electrical work will be performed under the facility's required review and lockout/tagout procedures.

# General Materials

Materials vary by power configuration. The configuration-specific BOM and current wiring drawing are authoritative for component quantities and conductor requirements.

| Component           | General specification                                                                          |
| ------------------- | ---------------------------------------------------------------------------------------------- |
| Junction box        | Black ABS; nominal size 4.5" × 3.5" × 2.2"; IP65                                               |
| Cable glands        | PG7; black PA66; IP68                                                                          |
| Wireless relay      | Shelly 1 Gen4 or supported equivalent documented by the applicable build                       |
| Splicing connectors | WAGO 221-series lever-nut connectors as specified by the applicable BOM and drawing            |
| Conductors          | Configuration-specific conductor type and gauge as specified by the applicable BOM and drawing |
| Ferrules            | Sized for the installed stranded conductor and compatible terminal                             |
| Labeling            | Repository-controlled front and side SVG artwork                                               |
| Device identity     | Serialized QR record corresponding to the Shelly installed in the enclosure                    |

The enclosure dimensions are recorded as supplied for the selected box. Confirm the manufacturer's internal dimensions, lid thickness, cable-entry clearances, switch/indicator clearance where applicable, and available mounting space before fabrication.

> **Material substitution**
>
> When substituting an enclosure, cable gland, connector, conductor, relay, switch, indicator, power supply, or other component, verify that the replacement remains suitable for the intended voltage, conductor type, environment, installation method, and applicable electrical requirements.
>
> Update the corresponding drawing, BOM, or build record whenever the substitution changes the assembly.

# Bill of Materials

The following bills of materials specify quantities and estimated material costs per completed junction-box assembly.

> **Pricing note**
>
> Unit prices are procurement references, not fixed quotations. Prices, package quantities, shipping, taxes, and availability may change.
>
> Line costs and assembly totals use the recorded procurement prices and are rounded to the nearest cent.

## External 12 VDC supply

| Component          | Specification                                       | Qty. |                                                                                                                                                                                                                                      Unit Price | Line Cost |
| ------------------ | --------------------------------------------------- | ---: | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------: | --------: |
| Junction Box       | 4.5" × 3.5" × 2.2" IP65 ABS, black                  |    1 | [$7.25](https://www.amazon.com/Zulkit-Dustproof-Waterproof-Universal-Electrical/dp/B07Z42YCXF?refinements=p_n_g-101013999996111%3A107518270011%2Cp_n_g-101013998791111%3A107245770011%2Cp_n_g-101013998994111%3A107245762011&rnid=107245654011) |     $7.25 |
| Cable Gland        | PG7, IP68, PA66, black                              |    2 |                                                                                                                                                   [$0.45](https://www.amazon.com/AMPELE-Plastic-Waterproof-Adjustable-Connectors/dp/B08TCFM4C7) |     $0.90 |
| Wireless Relay     | Shelly 1 Gen4 (S4SW-001X16EU)                       |    1 |                                                                                                                                                                                          [$24.99](https://us.shelly.com/products/shelly-1-gen4) |    $24.99 |
| Power Supply       | 12 VDC, 10' cable                                   |    1 |                                                                                                                                              [$10.36](https://www.amazon.com/Adapter-Extension-5-5x2-5mm-Replacement-Transformer/dp/B0D4TBSZ4B) |    $10.36 |
| Splicing Connector | WAGO 221-412 lever-nut                              |    1 |                                                                                               [$0.34](https://www.homedepot.com/pep/WAGO-221-412-Wire-Lever-Nuts-2-Wire-Conductor-Compact-Splicing-Connectors-50-Pack-0221412K00-012/326254029) |     $0.34 |
| Ferrule            | 12 AWG insulated bootlace ferrule, single-conductor |    4 |                                                                                                                                                  [$0.01](https://www.amazon.com/Fidioto-Terminals-Connector-Insulated-Industrial/dp/B0BFFPGFYD) |     $0.04 |

**Estimated material cost per 12 VDC assembly: $43.88**

## Tool 110–240 VAC supply

| Component               | Specification                             |   Qty. |                                                                                                                                                                                                                                      Unit Price | Line Cost |
| ----------------------- | ----------------------------------------- | -----: | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------: | --------: |
| Junction Box            | 4.5" × 3.5" × 2.2" IP65 ABS, black        |      1 | [$7.25](https://www.amazon.com/Zulkit-Dustproof-Waterproof-Universal-Electrical/dp/B07Z42YCXF?refinements=p_n_g-101013999996111%3A107518270011%2Cp_n_g-101013998791111%3A107245770011%2Cp_n_g-101013998994111%3A107245762011&rnid=107245654011) |     $7.25 |
| Cable Gland             | PG7, IP68, PA66, black                    |      2 |                                                                                                                                                   [$0.45](https://www.amazon.com/AMPELE-Plastic-Waterproof-Adjustable-Connectors/dp/B08TCFM4C7) |     $0.90 |
| Wireless Relay          | Shelly 1 Gen4 (S4SW-001X16EU)             |      1 |                                                                                                                                         [$23.75](https://www.amazon.com/Shelly-Zigbee-Matter-Switch-Contacts/dp/B0F9LJF6BV/140-2731126-2577851) |    $23.75 |
| Splicing Connector      | WAGO 221-415 Lever-Nut, 5-conductor       |      3 |                                                                                                                [$0.77](https://www.amazon.com/Wago-221-415-LEVER-NUTS-Conductor-Connectors/dp/B01M6Y2UEK?cv_ct_cx=wago+221-415&aref=iBsbkkef8W) |     $2.31 |
| Splicing Connector      | WAGO 221-413 Lever-Nut, 3-conductor       |      1 |                                                                                                                                                    [$0.49](https://www.amazon.com/WAGO-221-413-Lever-Nuts-3-Conductor-Connectors/dp/B07W7W91FX) |     $0.49 |
| Wire                    | 14 AWG THHN, black                        | 1.5 ft |                                                                                                                                                                     [$0.39/ft](https://www.amazon.com/gp/aw/d/B0GBWVJN85?i=aps&aref=wlEfuK8006) |     $0.59 |
| Wire                    | 14 AWG THHN, white                        | 1.5 ft |                                                                                                                                                                     [$0.39/ft](https://www.amazon.com/gp/aw/d/B0GBWHWCVH?i=aps&aref=wlEfuK8006) |     $0.59 |
| Ferrule                 | 14 AWG insulated wire ferrule             |      4 |                                                                                                                                                  [$0.01](https://www.amazon.com/Fidioto-Terminals-Connector-Insulated-Industrial/dp/B0BH8H827S) |     $0.05 |
| Manual Key Switch       | 12 mm On/Off metal key-lock switch, 250 V |      1 |                                                                                                                                                                                                   [$2.50](https://www.amazon.com/dp/B0BFGGK856) |     $2.50 |
| Indicator Light (Green) | 12 mm, 110–220 V metal LED indicator      |      1 |                                                                                                                                                                                                   [$2.20](https://www.amazon.com/dp/B0GQ8MPG2H) |     $2.20 |

**Estimated material cost per 110–240 VAC assembly: $40.63**

<div class="page-break"></div>

# Wiring Configuration: External 12 VDC Supply

This configuration powers the Shelly from a separate 12 VDC source while using the relay's dry contacts to switch the tool interlock or monitor circuit.

<img src="assets/Wiring Powered From 12VDC.svg" width="450">

## Design intent

* The external 12 VDC supply powers the Shelly.
* The Shelly relay contacts remain electrically separate from the Shelly power input.
* The relay contact pair is connected to the tool interlock or monitor circuit according to the equipment requirements.
* The completed enclosure receives the current front and side labeling.
* The serialized device identity corresponds to the Shelly physically installed in that enclosure.

## Required drawing

```text
docs/hardware/assets/Wiring Powered From 12VDC.svg
```

## Required label artwork

```text
docs/hardware/assets/Junction Box Label - Front.svg
docs/hardware/assets/Junction Box Label - Side.svg
```

> **Verification**
>
> Confirm polarity, conductor routing, terminal identification, power-supply requirements, and the exact dry-contact terminals on the installed Shelly before energizing the assembly.

# Wiring Configuration: Tool 110–240 VAC Supply

This configuration powers the Shelly from the tool's 110–240 VAC supply while using the relay output for the tool interlock connection.

<img src="assets/Wiring Powered From Tool (110-240VAC).svg" width="450">

## Design intent

* The Shelly receives operating power from the tool's AC supply.
* The relay output connects to the tool interlock or monitor circuit according to the equipment requirements.
* The assembly includes the manual key switch specified in the BOM.
* The assembly includes the green AC indicator specified in the BOM.
* The completed enclosure receives the current front and side labeling.
* The serialized device identity corresponds to the Shelly physically installed in that enclosure.

## Required drawing

```text
docs/hardware/assets/Wiring Powered From Tool (110-240VAC).svg
```

## Required label artwork

```text
docs/hardware/assets/Junction Box Label - Front.svg
docs/hardware/assets/Junction Box Label - Side.svg
```

> **Line-voltage caution**
>
> The 110–240 VAC configuration includes line-voltage wiring. Construction, installation, inspection, energization, and maintenance must follow applicable facility electrical requirements and lockout/tagout procedures.

# Junction-Box Labeling

The enclosure uses separate front and side artwork.

The authoritative files are:

```text
docs/hardware/assets/Junction Box Label - Front.svg
docs/hardware/assets/Junction Box Label - Side.svg
```

The front and side labels together make up the current enclosure-labeling system.

## Front label

<img src="assets/Junction Box Label - Front.svg" width="450">

Source file:

```text
docs/hardware/assets/Junction Box Label - Front.svg
```

## Side label

<img src="assets/Junction Box Label - Side.svg" width="450">

Source file:

```text
docs/hardware/assets/Junction Box Label - Side.svg
```

## Label-control rules

* Use the current repository-controlled front and side artwork.
* Treat the front and side labels as a matched set.
* Confirm that all markings correspond to the completed physical assembly.
* Generate the serialized device identity for the Shelly installed in that specific enclosure.
* Verify the QR code after final marking.
* Do not encode passwords, certificates, private keys, Wi-Fi credentials, Digest credentials, or other secrets.
* Replace or remark affected enclosure surfaces when the installed Shelly identity changes.
* Do not reuse serialized labeling on an enclosure containing a different Shelly.
* Do not use superseded label artwork simply because an older enclosure was produced with it.
* Treat any change to the authoritative artwork as a documented hardware-configuration change.

The previous power-configuration-specific label file is no longer part of the current repository asset set. Power configuration is controlled by the wiring design and build record, while the current enclosure marking uses the front and side artwork listed above.

<div class="page-break"></div>

# QR-Code Device Identity

The junction-box labeling includes a QR code containing serialized JSON that identifies the Shelly relay installed in the enclosure.

The identity record uses three fields:

| Field    | Purpose                            |
| -------- | ---------------------------------- |
| `device` | Human-readable Shelly product name |
| `model`  | Shelly hardware model number       |
| `id`     | Unique Shelly device ID            |

Example:

```json
{"device": "Shelly 1 Gen4", "model": "S4SW-001X16EU", "id": "A085E3B5325C"}
```

## Audit purpose

Scanning the enclosure QR code provides a fast, auditable method for collecting the installed device identity during:

* initial installation
* inspection
* preventive maintenance
* inventory reconciliation
* troubleshooting
* certificate or hostname verification
* hardware replacement

The scanned JSON can be copied into an inventory, service, or installation record without manually transcribing the product name, model number, and unique device ID.

## Data rules

Keep the field names stable:

```text
device
model
id
```

Stable field names allow the QR data to be parsed consistently by future inventory, maintenance, or audit tools.

The QR record must:

* describe the Shelly installed in that enclosure
* use the exact hardware model number
* use the unique Shelly device ID
* be regenerated whenever the installed Shelly is replaced
* match the final physical assembly before the box enters service

The QR record must not contain:

* passwords
* certificates
* private keys
* Wi-Fi credentials
* Digest credentials
* network secrets
* other sensitive configuration data

## Replacement rule

When the Shelly is replaced:

1. Scan and record the outgoing device identity.
2. Apply the required electrical isolation and lockout/tagout controls.
3. Install and configure the replacement Shelly.
4. Confirm the replacement model and unique Shelly ID.
5. Generate a new JSON identity record.
6. Generate a new QR code.
7. Update or replace the affected enclosure labeling.
8. Scan the finished QR mark.
9. Confirm the decoded record matches the installed device.
10. Update the applicable inventory, maintenance, or installation record.

# Fiber-Laser Marking Parameters

The example enclosure labels were produced with a 30 W fiber laser using the following recorded settings:

| Parameter | Setting |
| --------- | ------: |
| Speed     |     60% |
| Power     |     30% |
| Frequency |      80 |

These values are recorded machine settings, not universal material-processing specifications.

The appropriate settings depend on the laser, focus, enclosure material, coating, geometry, and artwork.

Before marking a production enclosure:

* verify laser focus
* verify the marking area
* confirm enclosure-material response
* confirm ventilation and fume-control requirements
* test the current artwork on a spare or representative surface
* verify front-label placement
* verify side-label placement
* verify that all text remains readable
* verify that the QR code remains readable
* scan the finished QR mark
* confirm that the decoded JSON exactly matches the intended Shelly identity
* confirm that laser marking has not damaged the enclosure or impaired its mechanical use

> **QR-code verification**
>
> A visually acceptable QR code is not sufficient.
>
> Scan the completed mark with a phone or other intended scanner and confirm that the decoded JSON exactly matches the Shelly installed in the enclosure before placing the assembly into service.

# Assembly and Documentation Workflow

Use the following sequence for each interlock box.

1. Select the required 12 VDC or 110–240 VAC power configuration.
2. Select the matching repository wiring diagram.
3. Confirm the exact Shelly device name, model number, and unique device ID.
4. Verify the installed Shelly terminal arrangement against the selected drawing.
5. Confirm enclosure and cable-gland fit.
6. Confirm clearance for all configuration-specific internal and panel-mounted components.
7. For the 110–240 VAC version, confirm placement and clearance for the manual key switch and green indicator.
8. Install the cable glands and required components.
9. Complete the wiring under the applicable electrical controls.
10. Inspect the completed wiring against the current repository drawing.
11. Generate the serialized JSON identity record.
12. Insert or generate the corresponding QR code in the current label artwork as required by the artwork workflow.
13. Prepare both current label assets:

    * `Junction Box Label - Front.svg`
    * `Junction Box Label - Side.svg`
14. Mark the front enclosure surface.
15. Mark the side enclosure surface.
16. Inspect both markings for legibility and correct placement.
17. Scan and verify the finished QR code.
18. Confirm that the QR identity matches the installed Shelly.
19. Record the device identity and installation information in the applicable inventory or service record.
20. Complete the required mechanical and electrical inspection.
21. Perform authorized functional verification before placing the interlock into service.

# Verification Checklist

## Mechanical inspection

* The enclosure and lid are undamaged.
* Cable glands are installed securely.
* Conductors are routed without excessive strain.
* Lever connectors are fully closed.
* No exposed conductor extends beyond the intended terminal or connector.
* Internal parts do not interfere with the enclosure or lid.
* Panel-mounted components are mechanically secure.
* The manual key switch is mechanically secure where applicable.
* The indicator is mechanically secure where applicable.
* The front marking is legible.
* The side marking is legible.
* The labeling corresponds to the completed assembly.
* The QR code is undamaged and readable.

## Electrical inspection

* The selected repository drawing matches the completed wiring.
* The exact Shelly terminal markings have been verified.
* The installed Shelly model is appropriate for the selected power configuration.
* The power source matches the selected configuration.
* Conductors match the required type and size.
* Ferrules and terminals are correctly matched to the installed conductors.
* Splicing connectors are appropriate for the installed conductors.
* The tool interlock conductors are connected to the intended relay contacts.
* Configuration-specific components are wired according to the current drawing.
* Required facility electrical inspection has been completed.
* Lockout/tagout controls were used as required.
* Energization and functional testing are authorized.

## Label inspection

* Both current label assets were used.
* The front label is positioned correctly.
* The side label is positioned correctly.
* Text is legible.
* The QR mark is readable.
* Labeling corresponds to the completed hardware.
* No superseded configuration-specific label was used in place of the current front/side artwork.

## Identity and audit inspection

* The QR code scans successfully.
* The decoded data is valid JSON.
* The `device` value matches the installed product name.
* The `model` value matches the installed hardware model.
* The `id` value matches the unique Shelly ID.
* The identity record has been entered into the applicable inventory or service record.
* No secrets are encoded in the QR data.

## Functional verification

Before releasing the assembly for service:

* verify that the Shelly powers correctly
* verify expected network communication
* verify the relay changes state only when authorized
* verify the tool receives the expected interlock or monitor state
* verify configuration-specific controls according to the current wiring drawing
* verify the final physical device identity against the gateway or inventory record

Do not bypass authoritative hardware safety interlocks as part of gateway functional testing.

# Replacement and Maintenance

When servicing an existing interlock box:

* scan and record the installed Shelly identity before disassembly
* confirm the existing power and wiring configuration
* select the current repository drawing
* apply the required lockout/tagout procedure
* inspect the enclosure, glands, connectors, conductors, switch, indicator, and relay as applicable
* replace damaged or unsuitable components
* verify replacement parts against the current BOM and drawing
* verify wiring against the current repository drawing
* inspect both front and side labeling
* regenerate the QR identity whenever the Shelly changes
* replace or remark any label surface that contains obsolete device identity
* scan the completed QR mark before returning the box to service
* update the maintenance, installation, or inventory record
* perform authorized functional verification

A wiring, component, device-identity, or labeling change should be treated as a documented configuration change rather than an undocumented field modification.

## Shelly replacement

Replacing the Shelly does not permit retention of stale serialized identity information.

After replacement:

```text
physical Shelly
        ↓
device / model / id
        ↓
QR identity
        ↓
physical enclosure marking
        ↓
inventory / service record
```

All representations of device identity must agree before the assembly returns to service.

## Label replacement

If either enclosure marking becomes damaged, unreadable, inaccurate, or obsolete:

1. verify the installed hardware identity
2. retrieve the current repository artwork
3. regenerate any serialized content
4. mark the replacement surface
5. inspect the completed mark
6. scan the QR code
7. confirm the decoded identity
8. update the applicable service record when required

Do not reproduce obsolete artwork from an older physical enclosure when the repository contains a newer authoritative revision.

# File Reference

The authoritative editable hardware assets are:

```text
docs/hardware/
├── Shelly Interlock Hardware Guide.md
└── assets
    ├── Junction Box Label - Front.svg
    ├── Junction Box Label - Side.svg
    ├── Wiring Powered From 12VDC.svg
    └── Wiring Powered From Tool (110-240VAC).svg
```

| File                                        | Purpose                                                                     |
| ------------------------------------------- | --------------------------------------------------------------------------- |
| `Junction Box Label - Front.svg`            | Current front enclosure label artwork                                       |
| `Junction Box Label - Side.svg`             | Current side enclosure label artwork                                        |
| `Wiring Powered From 12VDC.svg`             | Wiring diagram for an externally powered 12 VDC Shelly interlock box        |
| `Wiring Powered From Tool (110-240VAC).svg` | Wiring diagram for a Shelly interlock box powered from the tool's AC supply |

## Asset-control rule

The files in `docs/hardware/assets/` are the repository-controlled hardware references.

When a wiring diagram or label design changes:

1. update the appropriate SVG asset
2. update this guide if the change affects construction, materials, labeling, identity, or verification
3. review any affected BOM entries
4. verify the revised artwork or wiring before production use
5. preserve the change through normal repository version control

The repository version of the hardware documentation should remain sufficient to identify what drawing and artwork were authoritative for a given released revision of the interlock system.
