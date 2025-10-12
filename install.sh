#!/bin/bash

set -e

INSTALL_PATH="${HOME}/.local/bin"

echo "Installing SSH Dashboard..."
echo ""

echo "Building..."
make build

mkdir -p "${INSTALL_PATH}"

echo "Installing to ${INSTALL_PATH}..."
cp ssh-dashboard "${INSTALL_PATH}/"
chmod +x "${INSTALL_PATH}/ssh-dashboard"

echo ""
echo "✓ SSH Dashboard installed successfully!"
echo ""

if [[ ":$PATH:" == *":${INSTALL_PATH}:"* ]]; then
    echo "Run 'ssh-dashboard' to start"
else
    echo "⚠️  ${INSTALL_PATH} is not in your PATH"
    echo ""
    echo "Add this to your ~/.zshrc or ~/.bashrc:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    echo "Then run: source ~/.zshrc"
    echo "After that, run 'ssh-dashboard' to start"
fi

