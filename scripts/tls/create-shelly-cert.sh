#!/usr/bin/env sh
set -eu

# =========================
# Shelly Certificate Setup
# =========================
#
# This script creates a unique HTTPS server certificate and private key
# for one Shelly device.
#
# The certificate identity uses the device's UT Austin Dynamic DNS hostname.
# No IP address is embedded in the certificate.

umask 077

# Restore terminal echo if the script is interrupted during secret entry.
trap 'stty echo 2>/dev/null || true' EXIT INT TERM HUP

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

prompt_yes_no() {
  label="$1"

  printf "%s [y/N]: " "$label" >&2
  IFS= read -r answer

  case "$answer" in
    y|Y|yes|YES|Yes)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
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

PKI_DIR="${PROJECT_DIR}/pki"
CA_DIR="${PKI_DIR}/ca"
TEMPLATE="${SCRIPT_DIR}/templates/shelly-server.cnf.in"

SERVER_CA_CERT="${CA_DIR}/server-ca.crt"
SERVER_CA_KEY="${CA_DIR}/server-ca.key"

CERT_VALID_DAYS="${CERT_VALID_DAYS:-825}"

# =========================
# VALIDATION
# =========================

if ! command -v openssl >/dev/null 2>&1; then
  echo "ERROR: openssl is required." >&2
  exit 1
fi

for required_file in \
  "$TEMPLATE" \
  "$SERVER_CA_CERT" \
  "$SERVER_CA_KEY"
do
  if [ ! -f "$required_file" ]; then
    echo "ERROR: required file does not exist:" >&2
    echo "  $required_file" >&2
    echo >&2
    echo "Run ./scripts/tls/create-ca.sh first." >&2
    exit 1
  fi
done

# =========================
# USER INPUT
# =========================

INTERLOCK_NAME=$(prompt_required "Interlock name")
SHELLY_HOST=$(prompt_required "Shelly DDNS hostname")

validate_interlock_name "$INTERLOCK_NAME"
SHELLY_HOST=$(normalize_and_validate_ddns_host "$SHELLY_HOST")

# =========================
# OUTPUT PATHS
# =========================

OUTPUT_DIR="${PKI_DIR}/shellys/${INTERLOCK_NAME}"
CONFIG_FILE="${OUTPUT_DIR}/shelly-server.cnf"
KEY_FILE="${OUTPUT_DIR}/shelly-server.key"
CSR_FILE="${OUTPUT_DIR}/shelly-server.csr"
CERT_FILE="${OUTPUT_DIR}/shelly-server.crt"

if [ -e "$OUTPUT_DIR" ]; then
  echo
  echo "Certificate output already exists:"
  echo "  $OUTPUT_DIR"

  if ! prompt_yes_no "Replace the existing generated files?"; then
    echo "Cancelled."
    exit 0
  fi

  rm -rf "$OUTPUT_DIR"
fi

mkdir -p "$OUTPUT_DIR"

# =========================
# GENERATE CONFIG
# =========================

sed \
  -e "s|@SHELLY_HOST@|${SHELLY_HOST}|g" \
  "$TEMPLATE" > "$CONFIG_FILE"

# =========================
# GENERATE CERTIFICATE
# =========================

echo
echo "Generating Shelly server private key..."

openssl ecparam \
  -name prime256v1 \
  -genkey \
  -noout \
  -out "$KEY_FILE"

echo "Generating certificate signing request..."

openssl req \
  -new \
  -sha256 \
  -key "$KEY_FILE" \
  -out "$CSR_FILE" \
  -config "$CONFIG_FILE"

echo "Signing Shelly server certificate..."

openssl x509 \
  -req \
  -sha256 \
  -days "$CERT_VALID_DAYS" \
  -in "$CSR_FILE" \
  -CA "$SERVER_CA_CERT" \
  -CAkey "$SERVER_CA_KEY" \
  -CAcreateserial \
  -out "$CERT_FILE" \
  -extfile "$CONFIG_FILE" \
  -extensions server_ext

echo "Verifying Shelly server certificate..."

openssl verify \
  -purpose sslserver \
  -CAfile "$SERVER_CA_CERT" \
  "$CERT_FILE"

chmod 0600 "$KEY_FILE"
chmod 0644 "$CONFIG_FILE" "$CSR_FILE" "$CERT_FILE"

# =========================
# RESULT
# =========================

echo
echo "Shelly certificate created."
echo
echo "Interlock name:"
echo "  $INTERLOCK_NAME"
echo
echo "Certificate hostname:"
echo "  $SHELLY_HOST"
echo
echo "Generated files:"
echo "  $CERT_FILE"
echo "  $KEY_FILE"
echo "  $CSR_FILE"
echo "  $CONFIG_FILE"
echo
echo "Next:"
echo "  ./scripts/tls/upload-shelly-tls.sh"
