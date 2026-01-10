#!/bin/bash
# Build script for creating Debian packages
# Run this from the project root directory

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

cd "$PROJECT_DIR"

# Check if we have necessary tools
if ! command -v dpkg-buildpackage &> /dev/null; then
    echo "Error: dpkg-buildpackage not found. Install build-essential and devscripts"
    echo "  apt install build-essential devscripts debhelper"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "Error: go not found. Install golang-go >= 1.21"
    exit 1
fi

# Create debian directory symlink if not exists
if [ ! -d "$PROJECT_DIR/debian" ]; then
    ln -sf pkg/debian debian
fi

# Build package
echo "Building Debian package..."
dpkg-buildpackage -us -uc -b

# Move packages to dist directory
mkdir -p "$PROJECT_DIR/release/dist"
mv ../*.deb "$PROJECT_DIR/release/dist/" 2>/dev/null || true
mv ../*.changes "$PROJECT_DIR/release/dist/" 2>/dev/null || true
mv ../*.buildinfo "$PROJECT_DIR/release/dist/" 2>/dev/null || true

# Cleanup
rm -f debian 2>/dev/null || true

echo "Done! Packages are in release/dist/"
ls -la "$PROJECT_DIR/release/dist/"*.deb 2>/dev/null || echo "No .deb files found"
