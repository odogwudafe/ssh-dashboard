#!/bin/bash

set -e

BINARY_NAME="ssh-dashboard"
VERSION=${VERSION:-"dev"}
LDFLAGS="-s -w -X main.version=${VERSION}"

echo "Building SSH Dashboard..."

go build -ldflags="${LDFLAGS}" -o "${BINARY_NAME}" ./cmd/ssh_dashboard

echo "✓ Built ${BINARY_NAME}"
echo ""
echo "To install: make install"
echo "To run: ./${BINARY_NAME}"

