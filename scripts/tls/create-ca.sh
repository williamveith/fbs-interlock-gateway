#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"

PKI_DIR="${PROJECT_DIR}/pki"
CA_DIR="${PKI_DIR}/ca"
TEMPLATE_DIR="${SCRIPT_DIR}/templates"

SERVER_CA_CONFIG="${TEMPLATE_DIR}/server-ca.cnf"
CLIENT_CA_CONFIG="${TEMPLATE_DIR}/client-ca.cnf"
SERVER_CA_KEY="${CA_DIR}/server-ca.key"
SERVER_CA_CERT="${CA_DIR}/server-ca.crt"
CLIENT_CA_KEY="${CA_DIR}/client-ca.key"
CLIENT_CA_CERT="${CA_DIR}/client-ca.crt"
CA_VALID_DAYS="${CA_VALID_DAYS:-3650}"

command -v openssl >/dev/null 2>&1 || { echo "ERROR: openssl is required."; exit 1; }

for required_file in "$SERVER_CA_CONFIG" "$CLIENT_CA_CONFIG"; do
  [[ -f "$required_file" ]] || { echo "ERROR: Missing OpenSSL template: $required_file"; exit 1; }
done

mkdir -p "$CA_DIR"
chmod 0700 "$PKI_DIR" "$CA_DIR" 2>/dev/null || true

for output_file in "$SERVER_CA_KEY" "$SERVER_CA_CERT" "$CLIENT_CA_KEY" "$CLIENT_CA_CERT"; do
  if [[ -e "$output_file" ]]; then
    echo "ERROR: CA output already exists:"
    echo "  $output_file"
    echo "The script refuses to overwrite an existing CA."
    exit 1
  fi
done

echo "Creating Shelly server CA private key..."
openssl ecparam -name prime256v1 -genkey -noout -out "$SERVER_CA_KEY"

echo "Creating Shelly server CA certificate..."
openssl req -new -x509 -sha256 -days "$CA_VALID_DAYS" \
  -key "$SERVER_CA_KEY" -out "$SERVER_CA_CERT" -config "$SERVER_CA_CONFIG"

echo "Creating gateway client CA private key..."
openssl ecparam -name prime256v1 -genkey -noout -out "$CLIENT_CA_KEY"

echo "Creating gateway client CA certificate..."
openssl req -new -x509 -sha256 -days "$CA_VALID_DAYS" \
  -key "$CLIENT_CA_KEY" -out "$CLIENT_CA_CERT" -config "$CLIENT_CA_CONFIG"

chmod 0600 "$SERVER_CA_KEY" "$CLIENT_CA_KEY"
chmod 0644 "$SERVER_CA_CERT" "$CLIENT_CA_CERT"

echo
echo "Created certificate authorities:"
echo "  $SERVER_CA_CERT"
echo "  $SERVER_CA_KEY"
echo "  $CLIENT_CA_CERT"
echo "  $CLIENT_CA_KEY"
echo
echo "Next: ./scripts/tls/create-gateway-client.sh"
