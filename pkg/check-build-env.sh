#!/bin/bash
# 检查打包构建环境是否准备就绪

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SUCCESS=0
WARNINGS=0
ERRORS=0

echo -e "${BLUE}=== EzPay 构建环境检查 ===${NC}"
echo ""

# Function to check command
check_command() {
    local cmd=$1
    local name=$2
    local install_hint=$3

    if command -v "$cmd" &> /dev/null; then
        local version=$($cmd --version 2>&1 | head -n1)
        echo -e "${GREEN}✓${NC} $name: ${GREEN}已安装${NC}"
        echo -e "  版本: $version"
        return 0
    else
        echo -e "${RED}✗${NC} $name: ${RED}未安装${NC}"
        if [ -n "$install_hint" ]; then
            echo -e "  ${YELLOW}安装命令: $install_hint${NC}"
        fi
        ((ERRORS++))
        return 1
    fi
}

# Function to check file
check_file() {
    local file=$1
    local desc=$2

    if [ -f "$file" ]; then
        echo -e "${GREEN}✓${NC} $desc: ${GREEN}存在${NC}"
        echo -e "  路径: $file"
        return 0
    else
        echo -e "${YELLOW}⚠${NC} $desc: ${YELLOW}不存在${NC}"
        echo -e "  预期路径: $file"
        ((WARNINGS++))
        return 1
    fi
}

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo -e "${BLUE}项目信息:${NC}"
echo -e "  项目目录: $PROJECT_DIR"
echo -e "  pkg 目录: $SCRIPT_DIR"
echo ""

# Detect OS
echo -e "${BLUE}系统信息:${NC}"
if [ -f /etc/os-release ]; then
    . /etc/os-release
    echo -e "  发行版: $NAME"
    echo -e "  版本: $VERSION"
    OS_ID=$ID
else
    echo -e "${YELLOW}  无法检测操作系统${NC}"
    OS_ID="unknown"
fi
echo -e "  内核: $(uname -r)"
echo -e "  架构: $(uname -m)"
echo ""

# Check common tools
echo -e "${BLUE}通用构建工具:${NC}"
check_command "git" "Git" "请访问 https://git-scm.com/"
check_command "go" "Go 编译器" "请访问 https://golang.org/dl/"

if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    GO_MAJOR=$(echo $GO_VERSION | cut -d. -f1)
    GO_MINOR=$(echo $GO_VERSION | cut -d. -f2)

    if [ "$GO_MAJOR" -ge 1 ] && [ "$GO_MINOR" -ge 21 ]; then
        echo -e "  ${GREEN}Go 版本满足要求 (>= 1.21)${NC}"
    else
        echo -e "  ${RED}Go 版本过低，需要 >= 1.21${NC}"
        ((ERRORS++))
    fi
fi
echo ""

# Check Arch Linux tools
if [ "$OS_ID" = "arch" ] || [ "$OS_ID" = "manjaro" ]; then
    echo -e "${BLUE}Arch Linux 打包工具:${NC}"
    check_command "makepkg" "makepkg" "sudo pacman -S base-devel"
    check_command "pacman" "pacman" "系统包管理器"
    echo ""
fi

# Check Debian/Ubuntu tools
if [ "$OS_ID" = "debian" ] || [ "$OS_ID" = "ubuntu" ]; then
    echo -e "${BLUE}Debian/Ubuntu 打包工具:${NC}"
    check_command "dpkg-buildpackage" "dpkg-buildpackage" "sudo apt install build-essential devscripts debhelper"
    check_command "dpkg" "dpkg" "系统包管理器"
    check_command "dh" "debhelper" "sudo apt install debhelper"
    echo ""
fi

# Check project structure
echo -e "${BLUE}项目文件检查:${NC}"
check_file "$PROJECT_DIR/main.go" "主程序文件"
check_file "$PROJECT_DIR/go.mod" "Go 模块文件"
check_file "$PROJECT_DIR/config.yaml" "配置文件模板"
check_file "$PROJECT_DIR/web/templates" "Web 模板目录" || true
check_file "$PROJECT_DIR/web/static" "静态文件目录" || true
echo ""

# Check package files
echo -e "${BLUE}打包文件检查:${NC}"
check_file "$SCRIPT_DIR/archlinux/PKGBUILD" "Arch Linux PKGBUILD"
check_file "$SCRIPT_DIR/archlinux/build.sh" "Arch Linux 构建脚本"
check_file "$SCRIPT_DIR/debian/control" "Debian control 文件"
check_file "$SCRIPT_DIR/debian/rules" "Debian rules 文件"
check_file "$SCRIPT_DIR/debian/build.sh" "Debian 构建脚本"
echo ""

# Check version variables in main.go
echo -e "${BLUE}版本变量检查:${NC}"
if grep -q "var.*Version.*=" "$PROJECT_DIR/main.go" 2>/dev/null; then
    echo -e "${GREEN}✓${NC} main.go 包含 Version 变量"
else
    echo -e "${YELLOW}⚠${NC} main.go 未找到 Version 变量"
    echo -e "  ${YELLOW}建议: 查看 pkg/VERSION_VARS.md 了解如何添加版本变量${NC}"
    ((WARNINGS++))
fi

if grep -q "var.*BuildDate.*=" "$PROJECT_DIR/main.go" 2>/dev/null; then
    echo -e "${GREEN}✓${NC} main.go 包含 BuildDate 变量"
else
    echo -e "${YELLOW}⚠${NC} main.go 未找到 BuildDate 变量"
    echo -e "  ${YELLOW}建议: 查看 pkg/VERSION_VARS.md 了解如何添加版本变量${NC}"
    ((WARNINGS++))
fi
echo ""

# Check Go dependencies
echo -e "${BLUE}Go 依赖检查:${NC}"
cd "$PROJECT_DIR"
if go mod verify &> /dev/null; then
    echo -e "${GREEN}✓${NC} Go 模块验证通过"
else
    echo -e "${YELLOW}⚠${NC} Go 模块验证失败，可能需要运行 'go mod tidy'"
    ((WARNINGS++))
fi

if go mod download &> /dev/null; then
    echo -e "${GREEN}✓${NC} 依赖下载成功"
else
    echo -e "${RED}✗${NC} 依赖下载失败"
    ((ERRORS++))
fi
echo ""

# Summary
echo -e "${BLUE}=== 检查结果汇总 ===${NC}"
if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
    echo -e "${GREEN}✓ 环境检查完全通过！${NC}"
    echo -e "  可以开始构建打包了"
elif [ $ERRORS -eq 0 ]; then
    echo -e "${YELLOW}⚠ 环境检查通过，但有 $WARNINGS 个警告${NC}"
    echo -e "  可以尝试构建，但建议先处理警告"
else
    echo -e "${RED}✗ 发现 $ERRORS 个错误和 $WARNINGS 个警告${NC}"
    echo -e "  请先解决错误再进行构建"
fi
echo ""

# Build suggestions
if [ $ERRORS -eq 0 ]; then
    echo -e "${BLUE}=== 构建建议 ===${NC}"

    if [ "$OS_ID" = "arch" ] || [ "$OS_ID" = "manjaro" ]; then
        echo -e "${GREEN}Arch Linux 用户:${NC}"
        echo -e "  cd $SCRIPT_DIR/archlinux"
        echo -e "  ./build.sh"
    fi

    if [ "$OS_ID" = "debian" ] || [ "$OS_ID" = "ubuntu" ]; then
        echo -e "${GREEN}Debian/Ubuntu 用户:${NC}"
        echo -e "  cd $SCRIPT_DIR/debian"
        echo -e "  ./build.sh"
    fi

    echo ""
    echo -e "${BLUE}详细文档:${NC}"
    echo -e "  查看 pkg/README.md 获取完整的打包指南"
fi

exit $ERRORS
