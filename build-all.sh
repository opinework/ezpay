#!/bin/bash
#
# EzPay 全平台编译脚本
# 使用方法: ./build-all.sh
#

set -e

# =============================================================================
# 配置
# =============================================================================

# Go 编译器路径
GO="/Volumes/mindata/Library/go/bin/go"

# 项目信息
APP_NAME="ezpay"
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date +%Y%m%d%H%M%S)

# 输出目录
RELEASE_DIR="release"

# 编译参数
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# =============================================================================
# 函数定义
# =============================================================================

print_header() {
    echo -e "${BLUE}=============================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}=============================================${NC}"
}

print_step() {
    echo -e "${YELLOW}>>> $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# 检查 Go 编译器
check_go() {
    if [ ! -f "$GO" ]; then
        print_error "Go compiler not found at: $GO"
        echo "Please update the GO variable in this script."
        exit 1
    fi

    GO_VERSION=$($GO version)
    print_success "Go found: $GO_VERSION"
}

# 编译单个平台
build_platform() {
    local GOOS=$1
    local GOARCH=$2
    local OUTPUT=$3
    local DESC=$4

    print_step "Building $DESC..."

    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH $GO build \
        -ldflags="$LDFLAGS" \
        -o "$RELEASE_DIR/$OUTPUT" .

    if [ $? -eq 0 ]; then
        local SIZE=$(ls -lh "$RELEASE_DIR/$OUTPUT" | awk '{print $5}')
        print_success "$OUTPUT ($SIZE)"
    else
        print_error "Failed to build $OUTPUT"
        return 1
    fi
}

# =============================================================================
# 主流程
# =============================================================================

main() {
    print_header "EzPay Cross-Platform Build Script"
    echo ""
    echo "Version: $VERSION"
    echo "Build Time: $BUILD_TIME"
    echo ""

    # 检查 Go
    check_go
    echo ""

    # 清理并创建输出目录
    print_step "Preparing build directory..."
    rm -rf "$RELEASE_DIR"
    mkdir -p "$RELEASE_DIR"
    print_success "Output directory: $RELEASE_DIR"
    echo ""

    # 开始编译
    print_header "Building for all platforms"
    echo ""

    # Linux AMD64 (x86_64 服务器)
    build_platform "linux" "amd64" "${APP_NAME}-linux-amd64" "Linux AMD64 (x86_64)"

    # Linux ARM64 (ARM 服务器, 树莓派4等)
    build_platform "linux" "arm64" "${APP_NAME}-linux-arm64" "Linux ARM64"

    # Windows AMD64
    build_platform "windows" "amd64" "${APP_NAME}-windows-amd64.exe" "Windows AMD64"

    # macOS AMD64 (Intel Mac)
    build_platform "darwin" "amd64" "${APP_NAME}-darwin-amd64" "macOS AMD64 (Intel)"

    # macOS ARM64 (Apple Silicon M1/M2/M3/M4)
    build_platform "darwin" "arm64" "${APP_NAME}-darwin-arm64" "macOS ARM64 (Apple Silicon)"

    echo ""
    print_header "Build Complete!"
    echo ""
    echo "Output files:"
    ls -lh "$RELEASE_DIR/"
    echo ""

    # 询问是否创建发布包
    read -p "Create distribution packages (.tar.gz/.zip)? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        create_dist
    fi
}

# 创建发布包
create_dist() {
    print_header "Creating Distribution Packages"
    echo ""

    mkdir -p "$RELEASE_DIR/dist"

    # 复制配置文件模板
    if [ -f "config.yaml" ]; then
        cp config.yaml "$RELEASE_DIR/config.yaml.example"
    fi

    print_step "Creating Linux AMD64 package..."
    cd "$RELEASE_DIR"
    tar -czf "dist/${APP_NAME}-${VERSION}-linux-amd64.tar.gz" "${APP_NAME}-linux-amd64" config.yaml.example 2>/dev/null || \
    tar -czf "dist/${APP_NAME}-${VERSION}-linux-amd64.tar.gz" "${APP_NAME}-linux-amd64"
    cd - > /dev/null

    print_step "Creating Linux ARM64 package..."
    cd "$RELEASE_DIR"
    tar -czf "dist/${APP_NAME}-${VERSION}-linux-arm64.tar.gz" "${APP_NAME}-linux-arm64" config.yaml.example 2>/dev/null || \
    tar -czf "dist/${APP_NAME}-${VERSION}-linux-arm64.tar.gz" "${APP_NAME}-linux-arm64"
    cd - > /dev/null

    print_step "Creating Windows package..."
    cd "$RELEASE_DIR"
    zip -q "dist/${APP_NAME}-${VERSION}-windows-amd64.zip" "${APP_NAME}-windows-amd64.exe" config.yaml.example 2>/dev/null || \
    zip -q "dist/${APP_NAME}-${VERSION}-windows-amd64.zip" "${APP_NAME}-windows-amd64.exe"
    cd - > /dev/null

    print_step "Creating macOS Intel package..."
    cd "$RELEASE_DIR"
    tar -czf "dist/${APP_NAME}-${VERSION}-darwin-amd64.tar.gz" "${APP_NAME}-darwin-amd64" config.yaml.example 2>/dev/null || \
    tar -czf "dist/${APP_NAME}-${VERSION}-darwin-amd64.tar.gz" "${APP_NAME}-darwin-amd64"
    cd - > /dev/null

    print_step "Creating macOS Apple Silicon package..."
    cd "$RELEASE_DIR"
    tar -czf "dist/${APP_NAME}-${VERSION}-darwin-arm64.tar.gz" "${APP_NAME}-darwin-arm64" config.yaml.example 2>/dev/null || \
    tar -czf "dist/${APP_NAME}-${VERSION}-darwin-arm64.tar.gz" "${APP_NAME}-darwin-arm64"
    cd - > /dev/null

    echo ""
    print_success "Distribution packages created:"
    ls -lh "$RELEASE_DIR/dist/"
}

# =============================================================================
# 单平台编译选项
# =============================================================================

show_help() {
    echo "EzPay Build Script"
    echo ""
    echo "Usage: $0 [option]"
    echo ""
    echo "Options:"
    echo "  (no option)    Build all platforms"
    echo "  linux          Build Linux AMD64 only"
    echo "  linux-arm64    Build Linux ARM64 only"
    echo "  windows        Build Windows AMD64 only"
    echo "  macos          Build macOS Intel only"
    echo "  macos-arm64    Build macOS Apple Silicon only"
    echo "  dev            Build development version"
    echo "  clean          Clean build artifacts"
    echo "  help           Show this help message"
}

# =============================================================================
# 入口
# =============================================================================

case "${1:-all}" in
    "all"|"")
        main
        ;;
    "linux")
        check_go
        mkdir -p "$RELEASE_DIR"
        build_platform "linux" "amd64" "${APP_NAME}-linux-amd64" "Linux AMD64"
        ;;
    "linux-arm64")
        check_go
        mkdir -p "$RELEASE_DIR"
        build_platform "linux" "arm64" "${APP_NAME}-linux-arm64" "Linux ARM64"
        ;;
    "windows")
        check_go
        mkdir -p "$RELEASE_DIR"
        build_platform "windows" "amd64" "${APP_NAME}-windows-amd64.exe" "Windows AMD64"
        ;;
    "macos")
        check_go
        mkdir -p "$RELEASE_DIR"
        build_platform "darwin" "amd64" "${APP_NAME}-darwin-amd64" "macOS Intel"
        ;;
    "macos-arm64")
        check_go
        mkdir -p "$RELEASE_DIR"
        build_platform "darwin" "arm64" "${APP_NAME}-darwin-arm64" "macOS Apple Silicon"
        ;;
    "dev")
        check_go
        mkdir -p "bin"
        print_step "Building development version..."
        $GO build -tags dev -o "bin/${APP_NAME}" .
        print_success "Development build: bin/${APP_NAME}"
        ;;
    "clean")
        print_step "Cleaning build artifacts..."
        rm -rf "$RELEASE_DIR"
        rm -f "bin/${APP_NAME}"
        print_success "Clean complete"
        ;;
    "help"|"-h"|"--help")
        show_help
        ;;
    *)
        print_error "Unknown option: $1"
        echo ""
        show_help
        exit 1
        ;;
esac
