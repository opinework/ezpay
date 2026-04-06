#!/bin/bash
# Multi-architecture build script for Arch Linux
# Builds packages for both x86_64 and aarch64 architectures

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}=== EzPay Multi-Architecture Package Builder ===${NC}"
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
echo ""

# Define architectures to build
ARCHS=("x86_64" "aarch64")
BUILD_SUCCESS=()
BUILD_FAILED=()

# Build for each architecture
for ARCH in "${ARCHS[@]}"; do
    echo -e "${GREEN}=== Building for $ARCH ===${NC}"

    # Set GOARCH based on CARCH
    case "$ARCH" in
        x86_64)
            GOARCH="amd64"
            ;;
        aarch64)
            GOARCH="arm64"
            ;;
        armv7h)
            GOARCH="arm"
            ;;
        *)
            echo -e "${RED}Unknown architecture: $ARCH${NC}"
            BUILD_FAILED+=("$ARCH")
            continue
            ;;
    esac

    echo -e "${BLUE}Building binary for GOARCH=$GOARCH${NC}"

    # Build the binary with cross-compilation
    cd "$PROJECT_DIR"

    export CGO_ENABLED=0
    export GOOS=linux
    export GOARCH="$GOARCH"

    # Get version from git tags or generate from date
    # Add safe.directory if running as root
    if [ "$(id -u)" -eq 0 ] && ! git config --global --get-all safe.directory | grep -q "^$PROJECT_DIR$" 2>/dev/null; then
        git config --global --add safe.directory "$PROJECT_DIR"
    fi
    # Convert git describe format to Arch Linux compatible version (replace - with .)
    # Example: 1.0.1-3-gb04297a -> 1.0.1.3.gb04297a
    PKGVER=$(git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' | sed 's/-/./g' || echo "1.0.0.dev.$(date +%Y%m%d)")
    BUILDDATE=$(date -u '+%Y-%m-%d %H:%M:%S UTC')

    echo -e "${BLUE}Version: $PKGVER${NC}"
    echo -e "${YELLOW}Compiling binary for $ARCH ($GOARCH)...${NC}"

    if go build \
        -buildvcs=false \
        -trimpath \
        -ldflags="-s -w -X main.Version=${PKGVER} -X 'main.BuildDate=$BUILDDATE'" \
        -o "ezpay-$ARCH" \
        .; then

        echo -e "${GREEN}✓ Binary compiled successfully${NC}"

        # Create a package using fpm (if available) or manual packaging
        RELEASE_DIR="$PROJECT_DIR/release"
        mkdir -p "$RELEASE_DIR"

        # Create package directory structure
        PKG_DIR=$(mktemp -d)
        PKG_NAME="ezpay-${PKGVER}-1-${ARCH}.pkg.tar.zst"

        echo -e "${YELLOW}Creating package structure...${NC}"

        # Install files
        install -Dm755 "ezpay-$ARCH" "$PKG_DIR/usr/bin/ezpay"
        install -Dm644 config.yaml "$PKG_DIR/etc/ezpay/config.yaml"

        # Install web files
        install -dm755 "$PKG_DIR/usr/share/ezpay/web"
        cp -r web/templates "$PKG_DIR/usr/share/ezpay/web/" 2>/dev/null || echo "  Warning: templates not found"
        cp -r web/static "$PKG_DIR/usr/share/ezpay/web/" 2>/dev/null || echo "  Warning: static not found"

        # Install database migration scripts
        install -dm755 "$PKG_DIR/usr/share/ezpay/db"
        cp -r db/* "$PKG_DIR/usr/share/ezpay/db/" 2>/dev/null || echo "  Warning: db directory not found"

        # Install systemd service
        install -Dm644 "$SCRIPT_DIR/ezpay.service" "$PKG_DIR/usr/lib/systemd/system/ezpay.service"

        # Create directories
        install -dm750 "$PKG_DIR/var/lib/ezpay"
        install -dm750 "$PKG_DIR/var/lib/ezpay/uploads"
        install -dm750 "$PKG_DIR/var/lib/ezpay/qrcode"
        install -dm750 "$PKG_DIR/var/log/ezpay"

        # Install docs
        [ -f README.md ] && install -Dm644 README.md "$PKG_DIR/usr/share/doc/ezpay/README.md"
        [ -f LICENSE ] && install -Dm644 LICENSE "$PKG_DIR/usr/share/licenses/ezpay/LICENSE"

        # Create .PKGINFO
        cat > "$PKG_DIR/.PKGINFO" << EOF
pkgname = ezpay
pkgbase = ezpay
pkgver = ${PKGVER}-1
pkgdesc = EzPay - A lightweight payment gateway supporting USDT, WeChat and Alipay
url = https://github.com/yourusername/ezpay
builddate = $(date +%s)
packager = Unknown Packager
size = $(du -sb "$PKG_DIR" | awk '{print $1}')
arch = $ARCH
license = MIT
depend = glibc
backup = etc/ezpay/config.yaml
EOF

        # Create .INSTALL file (copy from ezpay.install)
        if [ -f "$SCRIPT_DIR/ezpay.install" ]; then
            cp "$SCRIPT_DIR/ezpay.install" "$PKG_DIR/.INSTALL"
        fi

        # Create package
        echo -e "${YELLOW}Compressing package...${NC}"
        cd "$PKG_DIR"

        # Create file list (all files, directories, and links under the package root)
        # Exclude .MTREE, .PKGINFO, .INSTALL from the * glob, handle them separately
        FILELIST=$(find . -mindepth 1 ! -name '.MTREE' ! -name '.PKGINFO' ! -name '.INSTALL' | sed 's|^\./||' | sort)

        # Create MTREE
        bsdtar -czf .MTREE --format=mtree \
            --options='!all,use-set,type,uid,gid,mode,time,size,md5,sha256,link' \
            .PKGINFO .INSTALL $FILELIST 2>/dev/null

        # Create final package
        bsdtar -cf - .MTREE .PKGINFO .INSTALL $FILELIST | \
            zstd -19 -T0 --ultra > "$RELEASE_DIR/$PKG_NAME"

        cd "$PROJECT_DIR"
        rm -rf "$PKG_DIR"
        rm -f "ezpay-$ARCH"

        if [ -f "$RELEASE_DIR/$PKG_NAME" ]; then
            echo -e "${GREEN}✓ Package created: $PKG_NAME${NC}"
            BUILD_SUCCESS+=("$ARCH")
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
ls -lh "$RELEASE_DIR/"*.pkg.tar.zst 2>/dev/null || echo "No packages found"

echo ""
echo -e "${YELLOW}Note: Cross-compiled packages should be tested on target architecture${NC}"
echo -e "${YELLOW}before distribution.${NC}"

# Exit with error if any builds failed
[ ${#BUILD_FAILED[@]} -eq 0 ]
