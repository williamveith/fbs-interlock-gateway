#!/usr/bin/env sh
set -eu

# =========================
# Shelly Auth Setup Script
# =========================
#
# This script enables or updates local HTTP RPC authentication
# on a Shelly Gen2/Gen3 device.
#
# It does not store passwords in the script, so this file is safe
# to commit to git.
#
# Usage:
#   chmod +x scripts/set-shelly-auth.sh
#   ./scripts/set-shelly-auth.sh

# Restore terminal echo if script is interrupted during password entry
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

prompt_secret_required() {
  label="$1"
  value=""

  while [ -z "$value" ]; do
    printf "%s: " "$label" >&2
    stty -echo
    IFS= read -r value
    stty echo
    echo >&2

    if [ -z "$value" ]; then
      echo "ERROR: password cannot be blank." >&2
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

sha256_hex() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 | awk '{print $NF}'
  else
    echo "ERROR: Need shasum, sha256sum, or openssl for SHA-256." >&2
    exit 1
  fi
}

json_get_realm() {
  python3 -c '
import json, sys
d = json.load(sys.stdin)
print(d.get("auth_domain") or d.get("id") or "")
'
}

json_get_auth_enabled() {
  python3 -c '
import json, sys
d = json.load(sys.stdin)
print("true" if d.get("auth_en") else "false")
'
}

# =========================
# VALIDATION
# =========================

if ! command -v curl >/dev/null 2>&1; then
  echo "ERROR: curl is required." >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "ERROR: python3 is required to parse Shelly.GetDeviceInfo JSON." >&2
  exit 1
fi

# =========================
# USER INPUT
# =========================

SHELLY_HOST=$(prompt_required "Shelly host/IP")
SHELLY_PASSWORD=$(prompt_secret_required "New Shelly auth password")
CURRENT_PASSWORD=$(prompt_secret_optional "Current Shelly auth password, leave blank if auth is currently disabled or blank")

# =========================
# GET DEVICE REALM
# =========================

echo
echo "Reading Shelly device info from $SHELLY_HOST..."

DEVICE_INFO=$(curl -fsS "http://$SHELLY_HOST/rpc/Shelly.GetDeviceInfo")
REALM=$(printf "%s" "$DEVICE_INFO" | json_get_realm)
AUTH_ENABLED=$(printf "%s" "$DEVICE_INFO" | json_get_auth_enabled)

if [ -z "$REALM" ]; then
  echo "ERROR: Could not determine Shelly realm/device ID." >&2
  echo "$DEVICE_INFO" >&2
  exit 1
fi

echo "Shelly realm: $REALM"
echo "Auth currently enabled: $AUTH_ENABLED"

# =========================
# COMPUTE HA1
# =========================

HA1=$(printf "admin:%s:%s" "$REALM" "$SHELLY_PASSWORD" | sha256_hex)

echo "Computed HA1."

# =========================
# ENABLE / UPDATE AUTH
# =========================

echo "Setting Shelly authentication..."

curl --anyauth -u "admin:$CURRENT_PASSWORD" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"id\":1,\"method\":\"Shelly.SetAuth\",\"params\":{\"user\":\"admin\",\"realm\":\"$REALM\",\"ha1\":\"$HA1\"}}" \
  "http://$SHELLY_HOST/rpc"

echo
echo "Auth update sent."

# =========================
# VERIFY
# =========================

echo
echo "Verifying authenticated access..."

curl --anyauth -u "admin:$SHELLY_PASSWORD" \
  "http://$SHELLY_HOST/rpc/Switch.GetStatus?id=0"

echo
echo "Done. Shelly auth is enabled/updated for $SHELLY_HOST."