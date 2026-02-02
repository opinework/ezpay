#!/bin/bash
# Multi-architecture build script for Debian packages
# Builds packages for both amd64 and arm64 architectures

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}=== EzPay Debian Multi-Architecture Package Builder ===${NC}"
echo -e "${BLUE}Project directory: $PROJECT_DIR${NC}"
echo -e "${BLUE}Package directory: $SCRIPT_DIR${NC}"
echo ""

# Check tools
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: go not found${NC}"
    exit 1
fi

if ! command -v fakeroot &> /dev/null; then
    echo -e "${RED}Error: fakeroot not found. Install it with:${NC}"
    echo "  sudo apt install fakeroot"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo -e "${BLUE}Using Go version: $GO_VERSION${NC}"
echo ""

# Get version from changelog
if [ -f "$SCRIPT_DIR/changelog" ]; then
    VERSION=$(head -n1 "$SCRIPT_DIR/changelog" | sed 's/.*(\(.*\)).*/\1/')
else
    VERSION="1.0.0-1"
fi

echo -e "${BLUE}Package version: $VERSION${NC}"
echo ""

cd "$PROJECT_DIR"

# Define architectures
ARCHS=("amd64" "arm64")
BUILD_SUCCESS=()
BUILD_FAILED=()

# Clean previous builds
echo -e "${YELLOW}Cleaning previous builds...${NC}"
rm -f ../*.deb ../*.changes ../*.buildinfo 2>/dev/null || true
rm -rf debian/ezpay debian/.debhelper debian/files debian/debhelper* debian/*.log debian/*.substvars 2>/dev/null || true

# Create debian symlink if needed
if [ ! -d "$PROJECT_DIR/debian" ]; then
    ln -sf pkg/debian debian
fi

RELEASE_DIR="$PROJECT_DIR/release"
mkdir -p "$RELEASE_DIR"

# Build for each architecture
for ARCH in "${ARCHS[@]}"; do
    echo -e "${GREEN}=== Building for $ARCH ===${NC}"

    # Set GOARCH
    export CGO_ENABLED=0
    export GOOS=linux
    export GOARCH="$ARCH"

    BUILDDATE=$(date -u '+%Y-%m-%d %H:%M:%S UTC')

    echo -e "${YELLOW}Compiling binary for $ARCH...${NC}"

    # Build binary
    if go build \
        -buildvcs=false \
        -trimpath \
        -ldflags="-s -w -X main.Version=${VERSION%-*} -X 'main.BuildDate=$BUILDDATE'" \
        -o "ezpay-$ARCH" \
        .; then

        echo -e "${GREEN}✓ Binary compiled successfully${NC}"

        # Create package structure
        PKG_DIR="debian/ezpay-$ARCH"
        rm -rf "$PKG_DIR"
        mkdir -p "$PKG_DIR"

        echo -e "${YELLOW}Creating package structure...${NC}"

        # Install binary
        install -Dm755 "ezpay-$ARCH" "$PKG_DIR/usr/bin/ezpay"

        # Install config
        install -Dm644 config.yaml "$PKG_DIR/etc/ezpay/config.yaml"

        # Install web files
        install -dm755 "$PKG_DIR/usr/share/ezpay/web"
        cp -r web/templates "$PKG_DIR/usr/share/ezpay/web/" 2>/dev/null || echo "  Warning: templates not found"
        cp -r web/static "$PKG_DIR/usr/share/ezpay/web/" 2>/dev/null || echo "  Warning: static not found"

        # Install database migration scripts
        install -dm755 "$PKG_DIR/usr/share/ezpay/db"
        cp -r db/* "$PKG_DIR/usr/share/ezpay/db/" 2>/dev/null || echo "  Warning: db directory not found"

        # Install systemd service
        install -Dm644 "$SCRIPT_DIR/ezpay.service" "$PKG_DIR/lib/systemd/system/ezpay.service"

        # Create directories
        install -dm750 "$PKG_DIR/var/lib/ezpay"
        install -dm750 "$PKG_DIR/var/lib/ezpay/uploads"
        install -dm750 "$PKG_DIR/var/lib/ezpay/qrcode"
        install -dm750 "$PKG_DIR/var/log/ezpay"

        # Install docs
        mkdir -p "$PKG_DIR/usr/share/doc/ezpay"
        [ -f README.md ] && install -Dm644 README.md "$PKG_DIR/usr/share/doc/ezpay/README.md"

        # Create DEBIAN directory
        mkdir -p "$PKG_DIR/DEBIAN"

        # Calculate installed size (in KB)
        INSTALLED_SIZE=$(du -sk "$PKG_DIR" | cut -f1)

        # Create control file
        cat > "$PKG_DIR/DEBIAN/control" << EOF
Package: ezpay
Version: $VERSION
Section: net
Priority: optional
Architecture: $ARCH
Maintainer: Your Name <your.email@example.com>
Installed-Size: $INSTALLED_SIZE
Depends: libc6 (>= 2.17)
Description: EzPay - A lightweight payment gateway
 A multi-chain cryptocurrency payment gateway supporting USDT (TRC20/ERC20/BEP20),
 TRX, WeChat Pay and Alipay. Features include automatic exchange rate updates,
 wallet rotation, Telegram notifications and more.
EOF

        # Copy postinst script
        if [ -f "$SCRIPT_DIR/postinst" ]; then
            cp "$SCRIPT_DIR/postinst" "$PKG_DIR/DEBIAN/postinst"
            chmod 755 "$PKG_DIR/DEBIAN/postinst"
        fi

        # Copy prerm script
        if [ -f "$SCRIPT_DIR/prerm" ]; then
            cp "$SCRIPT_DIR/prerm" "$PKG_DIR/DEBIAN/prerm"
            chmod 755 "$PKG_DIR/DEBIAN/prerm"
        fi

        # Copy postrm script
        if [ -f "$SCRIPT_DIR/postrm" ]; then
            cp "$SCRIPT_DIR/postrm" "$PKG_DIR/DEBIAN/postrm"
            chmod 755 "$PKG_DIR/DEBIAN/postrm"
        fi

        # Copy conffiles
        if [ -f "$SCRIPT_DIR/conffiles" ]; then
            cp "$SCRIPT_DIR/conffiles" "$PKG_DIR/DEBIAN/conffiles"
        else
            echo "/etc/ezpay/config.yaml" > "$PKG_DIR/DEBIAN/conffiles"
        fi

        # Build package
        echo -e "${YELLOW}Building .deb package...${NC}"
        PACKAGE_NAME="ezpay_${VERSION}_${ARCH}.deb"

        if fakeroot dpkg-deb --build "$PKG_DIR" "$RELEASE_DIR/$PACKAGE_NAME" > /dev/null 2>&1; then
            echo -e "${GREEN}✓ Package created: $PACKAGE_NAME${NC}"
            BUILD_SUCCESS+=("$ARCH")

            # Cleanup
            rm -f "ezpay-$ARCH"
            rm -rf "$PKG_DIR"
        else
            echo -e "${RED}✗ Failed to create package for $ARCH${NC}"
            BUILD_FAILED+=("$ARCH")
        fi
    else
        echo -e "${RED}✗ Failed to compile binary for $ARCH${NC}"
        BUILD_FAILED+=("$ARCH")
    fi

    echo ""
done

# Cleanup
rm -f debian 2>/dev/null || true

# Summary
echo -e "${GREEN}=== Build Summary ===${NC}"
echo -e "${BLUE}Build directory: $RELEASE_DIR${NC}"
echo ""

if [ ${#BUILD_SUCCESS[@]} -gt 0 ]; then
    echo -e "${GREEN}Successfully built packages for:${NC}"
    for arch in "${BUILD_SUCCESS[@]}"; do
        echo -e "  ${GREEN}✓${NC} $arch"
    done
    echo ""
fi

if [ ${#BUILD_FAILED[@]} -gt 0 ]; then
    echo -e "${RED}Failed to build packages for:${NC}"
    for arch in "${BUILD_FAILED[@]}"; do
        echo -e "  ${RED}✗${NC} $arch"
    done
    echo ""
fi

# List created packages
echo -e "${BLUE}Created packages:${NC}"
ls -lh "$RELEASE_DIR"/*.deb 2>/dev/null || echo "No packages found"

echo ""
echo -e "${YELLOW}=== Installation Instructions ===${NC}"
echo -e "${BLUE}To install (amd64):${NC}"
echo "  sudo dpkg -i $RELEASE_DIR/ezpay_${VERSION}_amd64.deb"
echo "  sudo apt-get install -f"
echo ""
echo -e "${BLUE}To install (arm64):${NC}"
echo "  sudo dpkg -i $RELEASE_DIR/ezpay_${VERSION}_arm64.deb"
echo "  sudo apt-get install -f"
echo ""
echo -e "${YELLOW}Note: Cross-compiled packages should be tested on target architecture${NC}"
echo -e "${YELLOW}before distribution.${NC}"

# Exit with error if any builds failed
[ ${#BUILD_FAILED[@]} -eq 0 ]
