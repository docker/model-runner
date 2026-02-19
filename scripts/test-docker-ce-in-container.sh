#!/bin/bash

set -eux -o pipefail

echo "Installing curl..."
apt-get update -qq 1>/dev/null
apt-get install -y curl 1>/dev/null

echo "Installing Docker CE..."
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh 1>/dev/null

echo "Testing docker model version..."
version_output=$(docker model version 2>&1 || true)
echo "Output: $version_output"

# Extract client version from the "Client:" section (first "Version:" after "Client:")
client_version=$(echo "$version_output" | awk '/^Client:/{found=1} found && /Version:/{print $2; exit}')

# Extract server version from the "Server:" section (first "Version:" after "Server:")
server_version=$(echo "$version_output" | awk '/^Server:/{found=1} found && /Version:/{print $2; exit}')

errors=0

if [ "$client_version" = "$EXPECTED_VERSION" ]; then
  echo "✓ Client version matches expected $EXPECTED_VERSION"
else
  echo "✗ Error: Expected client version $EXPECTED_VERSION, got '$client_version'"
  errors=$((errors + 1))
fi

if [ "$server_version" = "$EXPECTED_VERSION" ]; then
  echo "✓ Server version matches expected $EXPECTED_VERSION"
else
  echo "✗ Error: Expected server version $EXPECTED_VERSION, got '$server_version'"
  errors=$((errors + 1))
fi

if [ "$errors" -gt 0 ]; then
  exit 1
fi

echo "✓ All version checks passed!"
