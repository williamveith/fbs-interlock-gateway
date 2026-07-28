#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"

PKI_DIR="${PROJECT_DIR}/pki"
CA_DIR="${PKI_DIR}/ca"
GATEWAY_DIR="${PKI_DIR}/gateway"
RUNTIME_TLS_DIR="${PROJECT_DIR}/tls"

TEMPLATE="${SCRIPT_DIR}/templates/gateway-client.cnf"
CLIENT_CA_CERT="${CA_DIR}/client-ca.crt"
CLIENT_CA_KEY="${CA_DIR}/client-ca.key"
SERVER_CA_CERT="${CA_DIR}/server-ca.crt"

CONFIG_FILE="${GATEWAY_DIR}/gateway-client.cnf"
KEY_FILE="${GATEWAY_DIR}/gateway-client.key"
CSR_FILE="${GATEWAY_DIR}/gateway-client.csr"
CERT_FILE="${GATEWAY_DIR}/gateway-client.crt"
CERT_VALID_DAYS="${CERT_VALID_DAYS:-825}"

command -v openssl >/dev/null 2>&1 || { echo "ERROR: openssl is required."; exit 1; }

for required_file in "$TEMPLATE" "$CLIENT_CA_CERT" "$CLIENT_CA_KEY" "$SERVER_CA_CERT"; do
  if [[ ! -f "$required_file" ]]; then
    echo "ERROR: Required file does not exist: $required_file"
    echo "Run ./scripts/tls/create-ca.sh first."
    exit 1
  fi
done

mkdir -p "$GATEWAY_DIR" "$RUNTIME_TLS_DIR"
chmod 0700 "$GATEWAY_DIR" "$RUNTIME_TLS_DIR" 2>/dev/null || true

for output_file in "$KEY_FILE" "$CSR_FILE" "$CERT_FILE"; do
  [[ ! -e "$output_file" ]] || { echo "ERROR: Gateway output already exists: $output_file"; exit 1; }
done

cp "$TEMPLATE" "$CONFIG_FILE"

echo "Generating gateway client private key..."
openssl ecparam -name prime256v1 -genkey -noout -out "$KEY_FILE"

echo "Generating gateway client certificate request..."
openssl req -new -sha256 -key "$KEY_FILE" -out "$CSR_FILE" -config "$CONFIG_FILE"

echo "Signing gateway client certificate with the client CA..."
openssl x509 -req -sha256 -days "$CERT_VALID_DAYS" \
  -in "$CSR_FILE" -CA "$CLIENT_CA_CERT" -CAkey "$CLIENT_CA_KEY" \
  -CAcreateserial -out "$CERT_FILE" -extfile "$CONFIG_FILE" -extensions client_ext

echo "Verifying gateway client certificate..."
openssl verify -purpose sslclient -CAfile "$CLIENT_CA_CERT" "$CERT_FILE"

chmod 0600 "$KEY_FILE"
chmod 0644 "$CERT_FILE" "$CSR_FILE" "$CONFIG_FILE"

cp "$SERVER_CA_CERT" "${RUNTIME_TLS_DIR}/server-ca.crt"
cp "$CERT_FILE" "${RUNTIME_TLS_DIR}/gateway-client.crt"
cp "$KEY_FILE" "${RUNTIME_TLS_DIR}/gateway-client.key"
chmod 0644 "${RUNTIME_TLS_DIR}/server-ca.crt" "${RUNTIME_TLS_DIR}/gateway-client.crt"
chmod 0600 "${RUNTIME_TLS_DIR}/gateway-client.key"

echo
echo "Created and staged gateway runtime files:"
echo "  ${RUNTIME_TLS_DIR}/server-ca.crt"
echo "  ${RUNTIME_TLS_DIR}/gateway-client.crt"
echo "  ${RUNTIME_TLS_DIR}/gateway-client.key"
echo
echo "Upload this CA certificate to every Shelly:"
echo "  $CLIENT_CA_CERT"
