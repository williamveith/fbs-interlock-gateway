# FBS Interlock Gateway

`fbs-interlock-gateway` is a small Go service that lets an FBS interlock server control networked interlocks through a local gateway.

The gateway sits between the FBS server and the physical interlock device. FBS talks to the gateway over HTTP. The gateway translates those requests into Shelly HTTP RPC calls and returns a simple JSON state response back to FBS.

Real deployment configuration must stay out of Git. Do not commit `config.yaml`.

## System architecture

```text
+------------+        HTTP         +-------------------------+        HTTP RPC        +----------------------+
|            |  /status /on /off   |                         |  Switch.GetStatus     |                      |
| FBS Server | <-----------------> |  fbs-interlock-gateway  | <-------------------> | Network Interlock    |
|            |                     |                         |  Switch.Set           | Relay / Control Box  |
+------------+                     +-------------------------+                     +----------------------+
                                                                                        |
                                                                                        v
                                                                                Tool enable / monitor
                                                                                circuit changes state
```

## What this service does

The gateway exposes one HTTP listener per configured tool/interlock. Each listener port represents one interlock target.

Example layout using placeholders only:

```text
FBS Server
  -> http://<gateway-host>:<tool-1-port>/status
  -> fbs-interlock-gateway
  -> http://<interlock-host-1>/rpc/Switch.GetStatus?id=<switch-id>

FBS Server
  -> http://<gateway-host>:<tool-2-port>/on
  -> fbs-interlock-gateway
  -> http://<interlock-host-2>/rpc/Switch.Set?id=<switch-id>&on=true
```

The gateway is the only service FBS needs to know about. The individual interlock hostnames/IPs live only in the local `config.yaml` on the deployment machine.

## Communication path

### 1. FBS Server → Gateway

FBS sends HTTP requests to the gateway:

```text
http://<gateway-host>:<port>/status
http://<gateway-host>:<port>/on
http://<gateway-host>:<port>/off
```

The gateway accepts several common request formats for on/off commands, including path-based and query-based values such as:

```text
/on
/off
?turn=on
?turn=off
?state=1
?state=0
?value=1
?value=0
```

The gateway responds to FBS with:

```json
{"Success":1,"State":1}
```

or:

```json
{"Success":1,"State":0}
```

`State: 1` means the interlock output is on.  
`State: 0` means the interlock output is off.

### 2. Gateway → Network Interlock

For status requests, the gateway asks the interlock for its current output state:

```text
http://<interlock-host>/rpc/Switch.GetStatus?id=<switch-id>
```

For command requests, the gateway sets the output state:

```text
http://<interlock-host>/rpc/Switch.Set?id=<switch-id>&on=true
http://<interlock-host>/rpc/Switch.Set?id=<switch-id>&on=false
```

### 3. Interlock → Tool

The network interlock changes the state of the configured relay/control output. The physical wiring determines what the relay controls, such as a monitor circuit, enable circuit, or another non-destructive control line.

The software path is:

```text
FBS event
  -> FBS HTTP request
  -> gateway listener for that tool
  -> interlock HTTP RPC command
  -> relay output changes state
  -> tool control circuit changes state
```

## Configuration

The service loads `config.yaml` from the same directory as the executable.

`config.yaml` is intentionally not tracked by Git because it contains deployment-specific information such as interlock hostnames, addresses, ports, and future credentials.

Use `config-sample.yaml` as the public template and keep all real values local.

Safe example:

```yaml
bind: "0.0.0.0"

defaults:
  timeout_ms: 800
  safe_state_on_error: "off"

tools:
  - interlock_name: "EQU-EXAMPLE-TOOL-01"
    ip: "interlock-01.example.local"
    port: 8001
    switch_id: 0
    username: null
    password: null
    enabled: true

  - interlock_name: "EQU-EXAMPLE-TOOL-02"
    ip: "interlock-02.example.local"
    port: 8002
    switch_id: 0
    username: null
    password: null
    enabled: true
```

## Config fields

| Field | Purpose |
|---|---|
| `bind` | Address the gateway listens on. Use `0.0.0.0` to listen on all interfaces. |
| `defaults.timeout_ms` | HTTP timeout for interlock requests. |
| `defaults.safe_state_on_error` | State reported back to FBS if the interlock cannot be reached. Usually `off`. |
| `tools[].interlock_name` | Human-readable tool/interlock name used in logs. |
| `tools[].ip` | Hostname or IP address of the network interlock. Keep real values out of Git. |
| `tools[].port` | Gateway listener port for that FBS tool/interlock. |
| `tools[].switch_id` | Interlock switch/relay ID. For Shelly 1 Mini Gen3 this is usually `0`. |
| `tools[].username` | Reserved for future authenticated interlocks. |
| `tools[].password` | Reserved for future authenticated interlocks. Do not commit real passwords. |
| `tools[].enabled` | Whether this gateway listener should start. |

## Building

Build for the local machine:

```bash
make build-mac
```

Build for Linux:

```bash
make build-linux
```

Format the code:

```bash
make fmt
```

Clean build outputs:

```bash
make clean
```

Use the exact targets available in the included `Makefile`.

## Local testing

Test the interlock directly first:

```bash
curl "http://<interlock-host>/rpc/Switch.GetStatus?id=<switch-id>"
curl "http://<interlock-host>/rpc/Switch.Set?id=<switch-id>&on=true"
curl "http://<interlock-host>/rpc/Switch.Set?id=<switch-id>&on=false"
```

Then test through the gateway:

```bash
curl "http://<gateway-host>:<port>/status"
curl "http://<gateway-host>:<port>/on"
curl "http://<gateway-host>:<port>/off"
```

Expected gateway response:

```json
{"Success":1,"State":1}
```

or:

```json
{"Success":1,"State":0}
```

## Deployment on Debian/Linux

Create the install directory:

```bash
sudo mkdir -p /opt/fbs-interlock-gateway
```

Copy the binary and local config:

```bash
sudo cp <built-binary> /opt/fbs-interlock-gateway/fbs-interlock-gateway
sudo cp config.yaml /opt/fbs-interlock-gateway/config.yaml
sudo chmod +x /opt/fbs-interlock-gateway/fbs-interlock-gateway
```

Install the systemd service:

```bash
sudo cp fbs-interlock-gateway.service /etc/systemd/system/fbs-interlock-gateway.service
sudo systemctl daemon-reload
sudo systemctl enable fbs-interlock-gateway.service
sudo systemctl restart fbs-interlock-gateway.service
```

Check status:

```bash
systemctl status fbs-interlock-gateway.service
journalctl -u fbs-interlock-gateway.service -f
```

## Runtime behavior

On startup, the gateway:

1. Loads `config.yaml` from the executable directory.
2. Applies default values when fields are omitted.
3. Starts one HTTP server per enabled tool.
4. Maps each gateway port to one configured interlock.
5. Logs inbound FBS requests.
6. Logs outbound FBS responses.
7. Shuts down cleanly on interrupt or systemd stop.

If a tool is disabled, the gateway skips that listener.

If an interlock cannot be reached, the gateway logs the error and reports the configured safe state back to FBS.

## Logging

Incoming FBS requests are logged with:

```text
FBS_IN
```

Outgoing FBS responses are logged with:

```text
FBS_OUT
```

When running under systemd:

```bash
journalctl -u fbs-interlock-gateway.service -f
```

## Git safety

`config.yaml` must stay ignored:

```gitignore
config.yaml
```

Only commit safe examples:

```text
config-sample.yaml
README.md
Makefile
fbs-interlock-gateway.service
main.go
```

Do not commit real hostnames, IP addresses, usernames, passwords, internal network names, or deployment-specific interlock mappings.

## Notes

- One gateway process can manage multiple interlocks.
- Each interlock gets its own gateway listener port.
- FBS communicates only with the gateway.
- The gateway communicates with each interlock using HTTP RPC.
- Real deployment mappings live only in local `config.yaml`.
- The current implementation uses unauthenticated Shelly-style HTTP RPC.
- The current implementation loads config from the executable directory.
