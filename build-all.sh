#!/bin/bash
# Master build script - Build all package formats
# Supports: Arch Linux (.pkg.tar.zst), Debian (.deb), and Generic (.tar.gz)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Print banner
cat << 'EOF'
╔═══════════════════════════════════════════════════════╗
║       EzPay - Multi-Format Package Builder            ║
║                                                       ║
║  Builds packages for multiple distributions:          ║
║  • Arch Linux (.pkg.tar.zst)                          ║
║  • Debian/Ubuntu (.deb)                               ║
║  • Generic Linux (.tar.gz)                            ║
╚═══════════════════════════════════════════════════════╝
EOF

echo ""

# Parse command line arguments
BUILD_ARCH=true
BUILD_DEB=true
BUILD_TAR=true

if [ "$#" -gt 0 ]; then
    BUILD_ARCH=false
    BUILD_DEB=false
    BUILD_TAR=false

    for arg in "$@"; do
        case "$arg" in
            --arch|--archlinux)
                BUILD_ARCH=true
                ;;
            --deb|--debian)
                BUILD_DEB=true
                ;;
            --tar|--tarball)
                BUILD_TAR=true
                ;;
            --all)
                BUILD_ARCH=true
                BUILD_DEB=true
                BUILD_TAR=true
                ;;
            --help|-h)
                echo "Usage: $0 [options]"
                echo ""
                echo "Options:"
                echo "  --arch, --archlinux    Build Arch Linux packages only"
                echo "  --deb, --debian        Build Debian packages only"
                echo "  --tar, --tarball       Build tarball packages only"
                echo "  --all                  Build all formats (default)"
                echo "  --help, -h             Show this help message"
                echo ""
                echo "Examples:"
                echo "  $0                     # Build all formats"
                echo "  $0 --arch              # Build only Arch Linux packages"
                echo "  $0 --deb --tar         # Build Debian and tarball packages"
                exit 0
                ;;
            *)
                echo -e "${RED}Error: Unknown option '$arg'${NC}"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
fi

# Build summary
echo -e "${CYAN}Build Configuration:${NC}"
[ "$BUILD_ARCH" = true ] && echo -e "  ${GREEN}✓${NC} Arch Linux (.pkg.tar.zst)" || echo -e "  ${YELLOW}✗${NC} Arch Linux (skipped)"
[ "$BUILD_DEB" = true ] && echo -e "  ${GREEN}✓${NC} Debian (.deb)" || echo -e "  ${YELLOW}✗${NC} Debian (skipped)"
[ "$BUILD_TAR" = true ] && echo -e "  ${GREEN}✓${NC} Generic Tarball (.tar.gz)" || echo -e "  ${YELLOW}✗${NC} Generic Tarball (skipped)"
echo ""

# Create release directory
mkdir -p "$PROJECT_DIR/release"

# Track build status
BUILDS_SUCCEEDED=()
BUILDS_FAILED=()

# Build Arch Linux packages
if [ "$BUILD_ARCH" = true ]; then
    echo -e "${BLUE}╔═══════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║    Building Arch Linux Packages            ║${NC}"
    echo -e "${BLUE}╚═══════════════════════════════════════════╝${NC}"
    echo ""

    if bash "$PROJECT_DIR/pkg/archlinux/build-multiarch.sh"; then
        BUILDS_SUCCEEDED+=("Arch Linux")
    else
        BUILDS_FAILED+=("Arch Linux")
    fi
    echo ""
fi

# Build Debian packages
if [ "$BUILD_DEB" = true ]; then
    echo -e "${BLUE}╔═══════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║    Building Debian Packages                ║${NC}"
    echo -e "${BLUE}╚═══════════════════════════════════════════╝${NC}"
    echo ""

    if bash "$PROJECT_DIR/pkg/debian/build-multiarch.sh"; then
        BUILDS_SUCCEEDED+=("Debian")
    else
        BUILDS_FAILED+=("Debian")
    fi
    echo ""
fi

# Build Generic tarballs
if [ "$BUILD_TAR" = true ]; then
    echo -e "${BLUE}╔═══════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║    Building Generic Tarballs               ║${NC}"
    echo -e "${BLUE}╚═══════════════════════════════════════════╝${NC}"
    echo ""

    if bash "$PROJECT_DIR/pkg/tarball/build-multiarch.sh"; then
        BUILDS_SUCCEEDED+=("Tarball")
    else
        BUILDS_FAILED+=("Tarball")
    fi
    echo ""
fi

# Final summary
echo ""
echo -e "${BLUE}╔═══════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║              Final Build Summary                      ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════════════════════╝${NC}"
echo ""

if [ ${#BUILDS_SUCCEEDED[@]} -gt 0 ]; then
    echo -e "${GREEN}Successfully built:${NC}"
    for build in "${BUILDS_SUCCEEDED[@]}"; do
        echo -e "  ${GREEN}✓${NC} $build"
    done
    echo ""
fi

if [ ${#BUILDS_FAILED[@]} -gt 0 ]; then
    echo -e "${RED}Failed builds:${NC}"
    for build in "${BUILDS_FAILED[@]}"; do
        echo -e "  ${RED}✗${NC} $build"
    done
    echo ""
fi

# Generate combined checksums
echo -e "${CYAN}Generating checksums...${NC}"
cd "$PROJECT_DIR/release"
find . -name "*.pkg.tar.zst" -o -name "*.deb" -o -name "*.tar.gz" -o -name "*.zip" 2>/dev/null | \
    xargs -r sha256sum > checksums.txt 2>/dev/null || true
echo -e "  ${GREEN}✓${NC} Checksums generated"
echo ""

# List all created packages
echo -e "${CYAN}All packages in: $PROJECT_DIR/release${NC}"
echo ""
echo -e "${BLUE}Arch Linux:${NC}"
ls -lh "$PROJECT_DIR/release/archlinux/"*.pkg.tar.zst 2>/dev/null || echo "  (none)"
echo -e "${BLUE}Debian:${NC}"
ls -lh "$PROJECT_DIR/release/debian/"*.deb 2>/dev/null || echo "  (none)"
echo -e "${BLUE}Tarball:${NC}"
ls -lh "$PROJECT_DIR/release/tarball/"*.tar.gz "$PROJECT_DIR/release/tarball/"*.zip 2>/dev/null || echo "  (none)"
echo ""

echo -e "${YELLOW}═══════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}Build process completed!${NC}"
echo -e "${YELLOW}═══════════════════════════════════════════════════════${NC}"

# Exit with error if any builds failed
[ ${#BUILDS_FAILED[@]} -eq 0 ]
