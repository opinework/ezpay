# EzPay Makefile
# 开发模式: make dev      - 编译到 bin/ 目录，从文件系统加载资源
# 生产模式: make release  - 编译单文件，嵌入所有资源
# 全平台:   make all      - 编译所有平台版本

.PHONY: dev release release-all clean run run-dev setup-dev \
        release-linux release-linux-arm64 release-windows release-macos release-macos-arm64

# Go 编译器路径
GO := /Volumes/mindata/Library/go/bin/go

# 项目信息
APP_NAME := ezpay
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date +%Y%m%d%H%M%S)

# 目录
BIN_DIR := bin
RELEASE_DIR := release

# 通用编译参数
LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)

# =============================================================================
# 开发模式
# =============================================================================

# 初始化开发环境 (创建符号链接)
setup-dev:
	@echo "Setting up development environment..."
	@mkdir -p $(BIN_DIR)
	@mkdir -p $(BIN_DIR)/uploads
	@# 创建 web 目录的符号链接
	@rm -f $(BIN_DIR)/web
	@ln -sf ../web $(BIN_DIR)/web
	@# 复制配置文件模板 (如果不存在)
	@if [ ! -f $(BIN_DIR)/config.yaml ]; then \
		cp config.yaml $(BIN_DIR)/config.yaml 2>/dev/null || echo "No config.yaml template found"; \
	fi
	@echo "Development environment ready!"
	@echo "  - Binary output: $(BIN_DIR)/$(APP_NAME)"
	@echo "  - Config file: $(BIN_DIR)/config.yaml"
	@echo "  - Web resources: $(BIN_DIR)/web -> ../web (symlink)"

# 编译开发版本
dev: setup-dev
	@echo "Building development version..."
	$(GO) build -tags dev -o $(BIN_DIR)/$(APP_NAME) .
	@echo "Build complete: $(BIN_DIR)/$(APP_NAME)"

# 运行开发版本
run-dev: dev
	@echo "Starting development server..."
	cd $(BIN_DIR) && ./$(APP_NAME)

# =============================================================================
# 生产模式 - 单平台
# =============================================================================

# 编译当前平台版本
release:
	@echo "Building release version for current platform..."
	@mkdir -p $(RELEASE_DIR)
	CGO_ENABLED=0 $(GO) build -ldflags="$(LDFLAGS)" -o $(RELEASE_DIR)/$(APP_NAME) .
	@echo "Build complete: $(RELEASE_DIR)/$(APP_NAME)"
	@ls -lh $(RELEASE_DIR)/$(APP_NAME)

# =============================================================================
# 生产模式 - 跨平台编译
# =============================================================================

# Linux AMD64 (常用服务器)
release-linux:
	@echo "Building for Linux AMD64..."
	@mkdir -p $(RELEASE_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-linux-amd64 .
	@ls -lh $(RELEASE_DIR)/$(APP_NAME)-linux-amd64

# Linux ARM64 (树莓派, ARM服务器)
release-linux-arm64:
	@echo "Building for Linux ARM64..."
	@mkdir -p $(RELEASE_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-linux-arm64 .
	@ls -lh $(RELEASE_DIR)/$(APP_NAME)-linux-arm64

# Windows AMD64
release-windows:
	@echo "Building for Windows AMD64..."
	@mkdir -p $(RELEASE_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-windows-amd64.exe .
	@ls -lh $(RELEASE_DIR)/$(APP_NAME)-windows-amd64.exe

# macOS AMD64 (Intel Mac)
release-macos:
	@echo "Building for macOS AMD64 (Intel)..."
	@mkdir -p $(RELEASE_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-darwin-amd64 .
	@ls -lh $(RELEASE_DIR)/$(APP_NAME)-darwin-amd64

# macOS ARM64 (Apple Silicon M1/M2/M3)
release-macos-arm64:
	@echo "Building for macOS ARM64 (Apple Silicon)..."
	@mkdir -p $(RELEASE_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-darwin-arm64 .
	@ls -lh $(RELEASE_DIR)/$(APP_NAME)-darwin-arm64

# =============================================================================
# 全平台编译
# =============================================================================

# 编译所有平台
release-all: clean-release
	@echo "============================================="
	@echo "Building EzPay $(VERSION) for all platforms"
	@echo "============================================="
	@mkdir -p $(RELEASE_DIR)
	@# Linux AMD64
	@echo ""
	@echo "[1/5] Linux AMD64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-linux-amd64 .
	@# Linux ARM64
	@echo "[2/5] Linux ARM64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-linux-arm64 .
	@# Windows
	@echo "[3/5] Windows AMD64..."
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-windows-amd64.exe .
	@# macOS Intel
	@echo "[4/5] macOS AMD64 (Intel)..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-darwin-amd64 .
	@# macOS Apple Silicon
	@echo "[5/5] macOS ARM64 (Apple Silicon)..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" \
		-o $(RELEASE_DIR)/$(APP_NAME)-darwin-arm64 .
	@echo ""
	@echo "============================================="
	@echo "Build complete! Output files:"
	@echo "============================================="
	@ls -lh $(RELEASE_DIR)/
	@echo ""
	@echo "Version: $(VERSION)"
	@echo "Build time: $(BUILD_TIME)"

# 打包发布文件 (带配置文件模板)
dist: release-all
	@echo ""
	@echo "Creating distribution packages..."
	@mkdir -p $(RELEASE_DIR)/dist
	@# 复制配置文件模板
	@cp config.yaml $(RELEASE_DIR)/config.yaml.example
	@# 创建各平台压缩包
	@cd $(RELEASE_DIR) && tar -czf dist/$(APP_NAME)-$(VERSION)-linux-amd64.tar.gz $(APP_NAME)-linux-amd64 config.yaml.example
	@cd $(RELEASE_DIR) && tar -czf dist/$(APP_NAME)-$(VERSION)-linux-arm64.tar.gz $(APP_NAME)-linux-arm64 config.yaml.example
	@cd $(RELEASE_DIR) && zip -q dist/$(APP_NAME)-$(VERSION)-windows-amd64.zip $(APP_NAME)-windows-amd64.exe config.yaml.example
	@cd $(RELEASE_DIR) && tar -czf dist/$(APP_NAME)-$(VERSION)-darwin-amd64.tar.gz $(APP_NAME)-darwin-amd64 config.yaml.example
	@cd $(RELEASE_DIR) && tar -czf dist/$(APP_NAME)-$(VERSION)-darwin-arm64.tar.gz $(APP_NAME)-darwin-arm64 config.yaml.example
	@echo ""
	@echo "Distribution packages created:"
	@ls -lh $(RELEASE_DIR)/dist/

# =============================================================================
# 通用
# =============================================================================

# 直接运行 (使用当前目录的资源)
run:
	$(GO) run -tags dev .

# 清理发布目录
clean-release:
	@rm -rf $(RELEASE_DIR)

# 清理所有构建产物
clean:
	@rm -rf $(BIN_DIR)/$(APP_NAME)
	@rm -rf $(RELEASE_DIR)
	@echo "Cleaned build artifacts"

# 帮助
help:
	@echo "EzPay Build Commands:"
	@echo ""
	@echo "Development:"
	@echo "  make setup-dev       - Initialize development environment"
	@echo "  make dev             - Build development version to bin/"
	@echo "  make run-dev         - Build and run development version"
	@echo "  make run             - Run directly with 'go run'"
	@echo ""
	@echo "Production (single platform):"
	@echo "  make release         - Build for current platform"
	@echo "  make release-linux   - Build for Linux AMD64"
	@echo "  make release-linux-arm64  - Build for Linux ARM64"
	@echo "  make release-windows - Build for Windows AMD64"
	@echo "  make release-macos   - Build for macOS Intel"
	@echo "  make release-macos-arm64  - Build for macOS Apple Silicon"
	@echo ""
	@echo "Production (all platforms):"
	@echo "  make release-all     - Build all platforms"
	@echo "  make dist            - Build all + create .tar.gz/.zip packages"
	@echo ""
	@echo "Other:"
	@echo "  make clean           - Remove build artifacts"
	@echo "  make help            - Show this help message"
