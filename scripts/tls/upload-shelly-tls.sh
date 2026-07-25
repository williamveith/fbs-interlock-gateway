#!/usr/bin/env sh
set -eu

# =========================
# Shelly TLS Upload Script
# =========================
#
# This script uploads one Shelly's HTTPS server certificate,
# its matching private key, and the shared gateway client CA.
#
# It then reboots the Shelly so the new HTTPS configuration is loaded.

TMP_DIR=""

cleanup() {
  stty echo 2>/dev/null || true

  if [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
    rm -rf "$TMP_DIR"
  fi
}

trap cleanup EXIT INT TERM HUP

# =========================
# FUNCTIONS
# =========================

prompt_required() {
  label="$1"
  value=""

  while [ -z "$value" ]; do
    printf "%s: " "$label" >&2
    IFS= read -r value

    if [ -z "$value" ]; then
      echo "ERROR: value cannot be blank." >&2
    fi
  done

  printf "%s" "$value"
}

prompt_secret_optional() {
  label="$1"

  printf "%s: " "$label" >&2
  stty -echo
  IFS= read -r value
  stty echo
  echo >&2

  printf "%s" "$value"
}

validate_interlock_name() {
  value="$1"

  case "$value" in
    *[!A-Za-z0-9._-]*)
      echo "ERROR: interlock name may contain only letters, numbers," >&2
      echo "       periods, underscores, and hyphens." >&2
      exit 1
      ;;
  esac
}

normalize_and_validate_ddns_host() {
  value=$(printf "%s" "$1" | tr '[:upper:]' '[:lower:]')

  case "$value" in
    *://*|*/*|*:*|*[!a-z0-9.-]*)
      echo "ERROR: enter only the DDNS hostname." >&2
      echo "Do not include http://, https://, a path, or a port." >&2
      exit 1
      ;;
  esac

  case "$value" in
    *.dynamic.utexas.edu)
      ;;
    *)
      echo "ERROR: expected a UT Austin Dynamic DNS hostname such as:" >&2
      echo "  2c41389b0d77.dynamic.utexas.edu" >&2
      exit 1
      ;;
  esac

  printf "%s" "$value"
}

create_rpc_payload() {
  method="$1"
  input_file="$2"
  output_file="$3"

  python3 - "$method" "$input_file" > "$output_file" <<'PY'
import json
import sys

method = sys.argv[1]
path = sys.argv[2]

with open(path, "r", encoding="utf-8") as handle:
    data = handle.read()

json.dump(
    {
        "id": 1,
        "method": method,
        "params": {
            "data": data,
            "append": False,
        },
    },
    sys.stdout,
)
PY
}

validate_rpc_response() {
  response_file="$1"

  python3 - "$response_file" <<'PY'
import json
import sys

path = sys.argv[1]

with open(path, "r", encoding="utf-8") as handle:
    response = json.load(handle)

if "error" in response:
    print(json.dumps(response, indent=2))
    raise SystemExit(1)

print(json.dumps(response, indent=2))
PY
}

upload_file() {
  method="$1"
  input_file="$2"
  description="$3"

  payload_file="${TMP_DIR}/payload.json"
  response_file="${TMP_DIR}/response.json"

  create_rpc_payload "$method" "$input_file" "$payload_file"

  echo "$description..."

  curl \
    --fail-with-body \
    --silent \
    --show-error \
    --anyauth \
    --user "admin:${SHELLY_PASSWORD}" \
    --request POST \
    --header "Content-Type: application/json" \
    --data-binary "@${payload_file}" \
    --output "$response_file" \
    "http://${SHELLY_HOST}/rpc"

  validate_rpc_response "$response_file"
}

reboot_shelly() {
  payload_file="${TMP_DIR}/reboot.json"
  response_file="${TMP_DIR}/reboot-response.json"

  cat > "$payload_file" <<'JSON'
{"id":1,"method":"Shelly.Reboot","params":{"delay_ms":500}}
JSON

  curl \
    --fail-with-body \
    --silent \
    --show-error \
    --anyauth \
    --user "admin:${SHELLY_PASSWORD}" \
    --request POST \
    --header "Content-Type: application/json" \
    --data-binary "@${payload_file}" \
    --output "$response_file" \
    "http://${SHELLY_HOST}/rpc"

  validate_rpc_response "$response_file"
}

# =========================
# PATHS
# =========================

SCRIPT_DIR=$(
  CDPATH= cd -- "$(dirname -- "$0")" &&
    pwd
)

PROJECT_DIR=$(
  CDPATH= cd -- "${SCRIPT_DIR}/../.." &&
    pwd
)

PKI_DIR="${PROJECT_DIR}/pki"
CLIENT_CA_CERT="${PKI_DIR}/ca/client-ca.crt"

# =========================
# VALIDATION
# =========================

if ! command -v curl >/dev/null 2>&1; then
  echo "ERROR: curl is required." >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "ERROR: python3 is required." >&2
  exit 1
fi

# =========================
# USER INPUT
# =========================

INTERLOCK_NAME=$(prompt_required "Interlock name")
SHELLY_HOST=$(prompt_required "Shelly DDNS hostname")
SHELLY_PASSWORD=$(prompt_secret_optional "Shelly auth password, leave blank if authentication is disabled")

validate_interlock_name "$INTERLOCK_NAME"
SHELLY_HOST=$(normalize_and_validate_ddns_host "$SHELLY_HOST")

DEVICE_DIR="${PKI_DIR}/shellys/${INTERLOCK_NAME}"
SERVER_CERT="${DEVICE_DIR}/shelly-server.crt"
SERVER_KEY="${DEVICE_DIR}/shelly-server.key"

for required_file in \
  "$SERVER_CERT" \
  "$SERVER_KEY" \
  "$CLIENT_CA_CERT"
do
  if [ ! -f "$required_file" ]; then
    echo "ERROR: required file does not exist:" >&2
    echo "  $required_file" >&2
    exit 1
  fi
done

TMP_DIR=$(mktemp -d)

# =========================
# UPLOAD TLS FILES
# =========================

echo
echo "Uploading TLS configuration to:"
echo "  $SHELLY_HOST"
echo

upload_file \
  "Shelly.PutHTTPServerCert" \
  "$SERVER_CERT" \
  "Uploading Shelly server certificate"

echo

upload_file \
  "Shelly.PutHTTPServerKey" \
  "$SERVER_KEY" \
  "Uploading Shelly server private key"

echo

upload_file \
  "Shelly.PutHTTPServerCABundle" \
  "$CLIENT_CA_CERT" \
  "Uploading gateway client CA bundle"

# =========================
# REBOOT
# =========================

echo
echo "Rebooting Shelly..."
reboot_shelly

# =========================
# RESULT
# =========================

echo
echo "TLS files uploaded successfully."
echo
echo "The Shelly is rebooting. After it returns, run:"
echo "  ./scripts/tls/verify-shelly-tls.sh"
