#!/bin/bash
# Multi-architecture build script for Tarball packages
# Builds deployment packages for all platforms

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}=== EzPay Tarball Multi-Platform Package Builder ===${NC}"
echo -e "${BLUE}Project directory: $PROJECT_DIR${NC}"
echo -e "${BLUE}Package directory: $SCRIPT_DIR${NC}"
echo ""

# Check Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: go not found${NC}"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo -e "${BLUE}Using Go version: $GO_VERSION${NC}"

# Get version
cd "$PROJECT_DIR"
# Add safe.directory if running as root
if [ "$(id -u)" -eq 0 ] && ! git config --global --get-all safe.directory | grep -q "^$PROJECT_DIR$" 2>/dev/null; then
    git config --global --add safe.directory "$PROJECT_DIR"
fi
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev-$(date +%Y%m%d-%H%M%S)")
BUILDDATE=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
LDFLAGS="-s -w -X main.Version=${VERSION} -X 'main.BuildDate=$BUILDDATE'"

echo -e "${BLUE}Version: $VERSION${NC}"
echo -e "${BLUE}Build date: $BUILDDATE${NC}"
echo ""

# Define platforms
declare -A PLATFORMS=(
    ["linux-amd64"]="linux amd64"
    ["linux-arm64"]="linux arm64"
    ["darwin-amd64"]="darwin amd64"
    ["darwin-arm64"]="darwin arm64"
    ["windows-amd64"]="windows amd64"
)

RELEASE_DIR="$PROJECT_DIR/release"
mkdir -p "$RELEASE_DIR"

BUILD_SUCCESS=()
BUILD_FAILED=()

# Build for each platform
for PLATFORM in "${!PLATFORMS[@]}"; do
    read -r GOOS GOARCH <<< "${PLATFORMS[$PLATFORM]}"

    echo -e "${GREEN}=== Building for $PLATFORM ===${NC}"

    BINARY_NAME="ezpay"
    if [ "$GOOS" = "windows" ]; then
        BINARY_NAME="ezpay.exe"
    fi

    # Build binary
    echo -e "${YELLOW}Compiling binary...${NC}"
    cd "$PROJECT_DIR"
    if CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
        -buildvcs=false \
        -trimpath \
        -ldflags="$LDFLAGS" \
        -o "$BINARY_NAME" \
        .; then

        echo -e "${GREEN}✓ Binary compiled successfully${NC}"

        # Create package directory
        PKG_NAME="ezpay-${VERSION}-${PLATFORM}"
        PKG_DIR="/tmp/$PKG_NAME"
        rm -rf "$PKG_DIR"
        mkdir -p "$PKG_DIR"

        echo -e "${YELLOW}Creating package structure...${NC}"

        # Copy binary
        cp "$BINARY_NAME" "$PKG_DIR/"
        chmod +x "$PKG_DIR/$BINARY_NAME"

        # Copy configuration
        cp config.yaml "$PKG_DIR/"

        # Copy database scripts
        mkdir -p "$PKG_DIR/db"
        cp -r db/* "$PKG_DIR/db/"

        # Copy documentation
        [ -f README.md ] && cp README.md "$PKG_DIR/"
        [ -f LICENSE ] && cp LICENSE "$PKG_DIR/"
        [ -f INSTALL.md ] && cp INSTALL.md "$PKG_DIR/"

        # Create installation guide for this platform
        cat > "$PKG_DIR/INSTALL-${PLATFORM}.txt" << EOF
# EzPay Installation Guide for $PLATFORM

## Quick Start

1. Extract the archive:
EOF

        if [ "$GOOS" = "windows" ]; then
            cat >> "$PKG_DIR/INSTALL-${PLATFORM}.txt" << EOF
   Extract the ZIP file to your preferred location.

2. Edit configuration:
   Edit config.yaml with your database and settings.

3. Initialize database:
   The service requires database to be initialized first.
   Use MySQL client to run: db\migrate.sh migrate
   (Or run migrations manually from your MySQL environment)

4. Run the application:
   Double-click ezpay.exe or run from Command Prompt:
   ezpay.exe

5. Access the admin panel:
   Open http://localhost:6088/admin in your browser
   Default credentials: admin / admin123
EOF
        else
            cat >> "$PKG_DIR/INSTALL-${PLATFORM}.txt" << EOF
   tar -xzf ezpay-${VERSION}-${PLATFORM}.tar.gz
   cd ezpay-${VERSION}-${PLATFORM}

2. Edit configuration:
   vi config.yaml
   # Update database connection settings

3. Initialize database:
   # First deployment:
   ./db/migrate.sh migrate

   # For upgrades (automatically applies new migrations):
   ./db/migrate.sh migrate

4. Run the application:
   ./ezpay

5. Access the admin panel:
   http://localhost:6088/admin
   Default: admin / admin123

For systemd service setup, see README.md
EOF
        fi

        # Create archive
        echo -e "${YELLOW}Creating archive...${NC}"

        if [ "$GOOS" = "windows" ]; then
            # Create ZIP for Windows
            cd /tmp
            zip -q -r "$RELEASE_DIR/${PKG_NAME}.zip" "$PKG_NAME"
            ARCHIVE_FILE="${PKG_NAME}.zip"
        else
            # Create tar.gz for Unix-like systems
            cd /tmp
            tar -czf "$RELEASE_DIR/${PKG_NAME}.tar.gz" "$PKG_NAME"
            ARCHIVE_FILE="${PKG_NAME}.tar.gz"
        fi

        # Cleanup
        rm -rf "$PKG_DIR" "$PROJECT_DIR/$BINARY_NAME"

        if [ -f "$RELEASE_DIR/$ARCHIVE_FILE" ]; then
            SIZE=$(ls -lh "$RELEASE_DIR/$ARCHIVE_FILE" | awk '{print $5}')
            echo -e "${GREEN}✓ Package created: $ARCHIVE_FILE ($SIZE)${NC}"
            BUILD_SUCCESS+=("$PLATFORM")
        else
            echo -e "${RED}✗ Failed to create package for $PLATFORM${NC}"
            BUILD_FAILED+=("$PLATFORM")
        fi
    else
        echo -e "${RED}✗ Failed to compile binary for $PLATFORM${NC}"
        BUILD_FAILED+=("$PLATFORM")
        rm -f "$PROJECT_DIR/$BINARY_NAME"
    fi

    echo ""
done

# Generate checksums
echo -e "${YELLOW}Generating checksums...${NC}"
cd "$RELEASE_DIR"
sha256sum *.tar.gz *.zip 2>/dev/null > checksums.txt || true
echo -e "${GREEN}✓ Checksums generated${NC}"
echo ""

# Summary
echo -e "${GREEN}=== Build Summary ===${NC}"
echo -e "${BLUE}Build directory: $RELEASE_DIR${NC}"
echo ""

if [ ${#BUILD_SUCCESS[@]} -gt 0 ]; then
    echo -e "${GREEN}Successfully built packages for:${NC}"
    for platform in "${BUILD_SUCCESS[@]}"; do
        echo -e "  ${GREEN}✓${NC} $platform"
    done
    echo ""
fi

if [ ${#BUILD_FAILED[@]} -gt 0 ]; then
    echo -e "${RED}Failed to build packages for:${NC}"
    for platform in "${BUILD_FAILED[@]}"; do
        echo -e "  ${RED}✗${NC} $platform"
    done
    echo ""
fi

# List created packages
echo -e "${BLUE}Created packages:${NC}"
ls -lh "$RELEASE_DIR"/*.tar.gz "$RELEASE_DIR"/*.zip 2>/dev/null || echo "No packages found"

echo ""
echo -e "${BLUE}Checksums:${NC}"
cat "$RELEASE_DIR/checksums.txt" 2>/dev/null || echo "No checksums file"

# Exit with error if any builds failed
[ ${#BUILD_FAILED[@]} -eq 0 ]
