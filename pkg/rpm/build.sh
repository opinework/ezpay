#!/bin/bash
# Build script for creating RPM packages
# Run this from the project root directory

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
VERSION="1.0.0"

cd "$PROJECT_DIR"

# Check if we have necessary tools
if ! command -v rpmbuild &> /dev/null; then
    echo "Error: rpmbuild not found. Install rpm-build package"
    echo "  dnf install rpm-build rpmdevtools"
    echo "  # or"
    echo "  yum install rpm-build rpmdevtools"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "Error: go not found. Install golang >= 1.21"
    exit 1
fi

# Setup RPM build directories
mkdir -p ~/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Create source tarball
TARBALL_NAME="ezpay-${VERSION}"
mkdir -p "/tmp/${TARBALL_NAME}"
cp -r . "/tmp/${TARBALL_NAME}/"
tar -C /tmp -czf ~/rpmbuild/SOURCES/${TARBALL_NAME}.tar.gz "${TARBALL_NAME}"
rm -rf "/tmp/${TARBALL_NAME}"

# Copy spec file
cp "$SCRIPT_DIR/ezpay.spec" ~/rpmbuild/SPECS/

# Build RPM
echo "Building RPM package..."
rpmbuild -bb ~/rpmbuild/SPECS/ezpay.spec

# Move packages to dist directory
mkdir -p "$PROJECT_DIR/release/dist"
find ~/rpmbuild/RPMS -name "*.rpm" -exec cp {} "$PROJECT_DIR/release/dist/" \;

echo "Done! Packages are in release/dist/"
ls -la "$PROJECT_DIR/release/dist/"*.rpm 2>/dev/null || echo "No .rpm files found"
