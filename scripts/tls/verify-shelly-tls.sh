#!/usr/bin/env sh
set -eu

# =========================
# Shelly mTLS Verification
# =========================
#
# This script verifies that:
#   1. The generated Shelly certificate chains to server-ca.crt.
#   2. The Shelly rejects HTTPS clients without a client certificate.
#   3. The gateway client certificate is accepted.
#   4. Shelly Digest authentication still works over HTTPS.

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

TLS_DIR="${PROJECT_DIR}/tls"
PKI_DIR="${PROJECT_DIR}/pki"

SERVER_CA_CERT="${TLS_DIR}/server-ca.crt"
GATEWAY_CERT="${TLS_DIR}/gateway-client.crt"
GATEWAY_KEY="${TLS_DIR}/gateway-client.key"

# =========================
# VALIDATION
# =========================

if ! command -v curl >/dev/null 2>&1; then
  echo "ERROR: curl is required." >&2
  exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
  echo "ERROR: openssl is required." >&2
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

LOCAL_SHELLY_CERT="${PKI_DIR}/shellys/${INTERLOCK_NAME}/shelly-server.crt"

for required_file in \
  "$SERVER_CA_CERT" \
  "$GATEWAY_CERT" \
  "$GATEWAY_KEY" \
  "$LOCAL_SHELLY_CERT"
do
  if [ ! -f "$required_file" ]; then
    echo "ERROR: required file does not exist:" >&2
    echo "  $required_file" >&2
    exit 1
  fi
done

TMP_DIR=$(mktemp -d)
URL="https://${SHELLY_HOST}/rpc/Shelly.GetDeviceInfo"

# =========================
# VERIFY LOCAL CERTIFICATE
# =========================

echo
echo "Verifying the generated Shelly certificate..."

openssl verify \
  -purpose sslserver \
  -CAfile "$SERVER_CA_CERT" \
  "$LOCAL_SHELLY_CERT"

# =========================
# VERIFY CLIENT CERT REQUIRED
# =========================

echo
echo "Testing HTTPS without a client certificate..."

NO_CERT_ERROR="${TMP_DIR}/without-client-cert.txt"

if curl \
  --silent \
  --show-error \
  --cacert "$SERVER_CA_CERT" \
  --output /dev/null \
  "$URL" \
  2> "$NO_CERT_ERROR"
then
  echo "ERROR: HTTPS succeeded without a client certificate." >&2
  echo "The Shelly may not have loaded client-ca.crt." >&2
  exit 1
fi

echo "Connection was rejected as expected."
echo
echo "TLS error returned without a client certificate:"
sed 's/^/  /' "$NO_CERT_ERROR"

# =========================
# VERIFY GATEWAY IDENTITY
# =========================

echo
echo "Testing HTTPS with the gateway client certificate..."

curl \
  --fail-with-body \
  --silent \
  --show-error \
  --anyauth \
  --user "admin:${SHELLY_PASSWORD}" \
  --cacert "$SERVER_CA_CERT" \
  --cert "$GATEWAY_CERT" \
  --key "$GATEWAY_KEY" \
  "$URL"

# =========================
# RESULT
# =========================

echo
echo
echo "mTLS verification succeeded."
echo
echo "Interlock name:"
echo "  $INTERLOCK_NAME"
echo
echo "Shelly hostname:"
echo "  $SHELLY_HOST"
echo
echo "Verified:"
echo "  Shelly certificate is trusted by server-ca.crt."
echo "  Client certificates are required."
echo "  gateway-client.crt and gateway-client.key are accepted."
echo "  Shelly authentication works over HTTPS."
