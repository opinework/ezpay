# EzPay 构建指南

本文档介绍如何构建 EzPay 项目，包括开发环境配置和生产版本编译。

## 环境要求

- Go 1.21+
- Make (可选，用于简化构建命令)
- Git

## 项目结构

```
ezpay/
├── bin/                    # 开发运行目录
│   ├── ezpay              # 开发版二进制
│   ├── config.yaml        # 开发配置文件
│   ├── ezpay_data/        # 运行时上传目录
│   └── web -> ../web      # 符号链接
├── release/               # 生产发布目录
│   ├── ezpay-linux-amd64
│   ├── ezpay-linux-arm64
│   ├── ezpay-windows-amd64.exe
│   ├── ezpay-darwin-amd64
│   ├── ezpay-darwin-arm64
│   └── dist/              # 打包文件
├── web/
│   ├── templates/         # HTML模板 (生产版嵌入)
│   ├── static/            # 静态资源 (生产版嵌入)
│   ├── embed.go           # 生产模式资源嵌入
│   └── embed_dev.go       # 开发模式文件系统加载
├── Makefile               # 构建脚本
└── config.yaml            # 配置文件模板
```

## 存储目录

上传的文件（收款码、APK等）存储在 `storage.data_dir` 配置的目录中：

| 平台 | 默认路径 |
|------|---------|
| Linux | `/var/lib/ezpay` |
| Windows | `{可执行文件目录}/ezpay_data` |
| macOS | `{可执行文件目录}/ezpay_data` |

目录结构：
```
{data_dir}/
├── qrcode/     # 收款码图片
└── apk/        # APP安装包
```

## 构建模式

EzPay 支持两种构建模式：

| 模式 | 资源加载 | 适用场景 |
|------|---------|---------|
| 开发模式 | 从文件系统加载 | 本地开发，支持热更新 |
| 生产模式 | 嵌入到二进制 | 部署发布，单文件运行 |

### 开发模式特点

- 模板和静态文件从文件系统读取
- 修改 HTML/CSS/JS 后刷新浏览器即可生效
- 配置文件从可执行文件所在目录读取
- 使用 `-tags dev` 构建标签

### 生产模式特点

- 模板和静态文件嵌入到二进制中
- 单文件部署，无需额外资源文件
- 上传目录仍从文件系统读写
- 二进制约 18MB

## 快速开始

### 使用 build-all.sh (推荐)

```bash
# 编译所有平台
./build-all.sh

# 单独编译指定平台
./build-all.sh linux        # Linux x64
./build-all.sh linux-arm64  # Linux ARM64
./build-all.sh windows      # Windows x64
./build-all.sh macos        # macOS Intel
./build-all.sh macos-arm64  # macOS Apple Silicon

# 开发版本
./build-all.sh dev

# 清理编译产物
./build-all.sh clean
```

### 使用 Makefile

```bash
# 查看所有命令
make help

# 开发模式
make dev        # 编译到 bin/
make run-dev    # 编译并运行

# 生产模式
make release    # 编译当前平台
make release-all # 编译所有平台
make dist       # 编译并打包
```

### 手动构建

```bash
# 安装依赖
go mod tidy

# 开发版本 (从文件系统加载资源)
go build -tags dev -o bin/ezpay .

# 生产版本 (嵌入资源)
CGO_ENABLED=0 go build -ldflags="-s -w" -o release/ezpay .
```

## 开发环境配置

### 1. 初始化开发环境

```bash
make setup-dev
```

这会创建以下结构：
- `bin/ezpay` - 开发版可执行文件
- `bin/config.yaml` - 开发配置文件
- `bin/ezpay_data/` - 上传目录 (macOS/Windows 默认)
- `bin/web` - 指向源码 web 目录的符号链接

### 2. 配置数据库

编辑 `bin/config.yaml`：

```yaml
database:
  host: "127.0.0.1"
  port: 3306
  user: "ezpay"
  password: "your_password"
  dbname: "ezpay"
```

### 3. 运行开发版本

```bash
make run-dev
# 或
cd bin && ./ezpay
```

### 4. 开发工作流

1. 修改 Go 代码 → `make dev` 重新编译
2. 修改 HTML/CSS/JS → 直接刷新浏览器
3. 修改配置 → 编辑 `bin/config.yaml` 并重启

## GoLand 配置

### 方式一：使用 Makefile

1. **Run** → **Edit Configurations** → **+** → **Shell Script**
2. 配置：
   - Name: `EzPay Dev`
   - Script path: `/usr/bin/make`
   - Script options: `run-dev`
   - Working directory: `项目根目录`

### 方式二：Go Build 配置

1. **Run** → **Edit Configurations** → **+** → **Go Build**
2. 配置：
   - Name: `EzPay Dev`
   - Run kind: `Package`
   - Package path: `ezpay`
   - Output directory: `项目根目录/bin`
   - Working directory: `项目根目录/bin`
   - Go tool arguments: `-tags dev`

## 生产版本编译

### 单平台编译

```bash
# 当前平台
make release

# 指定平台
make release-linux        # Linux x64
make release-linux-arm64  # Linux ARM64
make release-windows      # Windows x64
make release-macos        # macOS Intel
make release-macos-arm64  # macOS Apple Silicon
```

### 全平台编译

```bash
make release-all
```

输出文件：
```
release/
├── ezpay-linux-amd64      # Linux x64 服务器
├── ezpay-linux-arm64      # Linux ARM64 (树莓派等)
├── ezpay-windows-amd64.exe # Windows
├── ezpay-darwin-amd64     # macOS Intel
└── ezpay-darwin-arm64     # macOS M1/M2/M3
```

### 打包分发

```bash
make dist
```

创建压缩包 (包含可执行文件 + 配置模板)：
```
release/dist/
├── ezpay-{version}-linux-amd64.tar.gz
├── ezpay-{version}-linux-arm64.tar.gz
├── ezpay-{version}-windows-amd64.zip
├── ezpay-{version}-darwin-amd64.tar.gz
└── ezpay-{version}-darwin-arm64.tar.gz
```

## 交叉编译说明

Go 原生支持交叉编译，无需安装额外工具链：

| 环境变量 | 说明 | 示例值 |
|---------|------|--------|
| `GOOS` | 目标操作系统 | linux, windows, darwin |
| `GOARCH` | 目标架构 | amd64, arm64 |
| `CGO_ENABLED` | 是否启用 CGO | 0 (禁用，支持静态链接) |

手动交叉编译示例：

```bash
# Linux AMD64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ezpay-linux-amd64 .

# Windows AMD64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ezpay-windows-amd64.exe .

# macOS ARM64 (Apple Silicon)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o ezpay-darwin-arm64 .
```

## 编译优化

### 减小二进制体积

Makefile 默认使用以下优化参数：

```bash
-ldflags="-s -w"
```

- `-s`: 去除符号表
- `-w`: 去除 DWARF 调试信息

### 版本信息注入

编译时自动注入版本信息：

```bash
-ldflags="-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
```

## Docker 构建

### 多阶段构建 Dockerfile

```dockerfile
# 构建阶段
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o ezpay .

# 运行阶段
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/ezpay .
EXPOSE 6088
CMD ["./ezpay"]
```

### 构建镜像

```bash
docker build -t ezpay:latest .
```

## 常见问题

### 1. 开发模式找不到模板

确保在正确目录运行：
```bash
cd bin && ./ezpay
# 或
make run-dev
```

### 2. 生产版本静态文件 404

生产版本的 CSS/JS/字体等静态资源已嵌入二进制。如果出现 404：
- 检查是否使用了正确的构建命令 (不带 `-tags dev`)
- 检查 `web/static/` 目录下的文件是否完整

### 3. 上传文件找不到

上传目录始终从文件系统读取，确保数据目录存在且有写入权限：

```bash
# Linux
mkdir -p /var/lib/ezpay/qrcode /var/lib/ezpay/apk
chmod 755 /var/lib/ezpay

# macOS/Windows (在可执行文件目录下)
mkdir -p ezpay_data/qrcode ezpay_data/apk
```

也可以在 `config.yaml` 中自定义路径：
```yaml
storage:
  data_dir: "/custom/path/to/data"
```

### 4. 交叉编译失败

确保 Go 版本 >= 1.21，且没有使用 CGO 依赖：
```bash
CGO_ENABLED=0 go build ...
```

## Linux 系统包构建

EzPay 支持构建 Arch Linux、Debian 和 RPM 格式的系统安装包。

### 打包文件位置

```
pkg/
├── archlinux/
│   ├── PKGBUILD           # Arch Linux 打包配置
│   ├── ezpay.install      # 安装钩子脚本
│   └── ezpay.service      # systemd 服务文件
├── debian/
│   ├── build.sh           # 构建脚本
│   ├── changelog          # 版本日志
│   ├── compat             # debhelper 兼容级别
│   ├── conffiles          # 配置文件列表
│   ├── control            # 包元数据
│   ├── ezpay.service      # systemd 服务文件
│   ├── postinst           # 安装后脚本
│   ├── postrm             # 卸载后脚本
│   ├── prerm              # 卸载前脚本
│   └── rules              # 构建规则
└── rpm/
    ├── build.sh           # 构建脚本
    ├── ezpay.service      # systemd 服务文件
    └── ezpay.spec         # RPM spec 文件
```

### 平台要求

| 包格式 | 需要的系统 | 依赖工具 |
|--------|-----------|----------|
| .pkg.tar.zst | Arch Linux | `makepkg`, `pacman` |
| .deb | Debian/Ubuntu | `dpkg-buildpackage`, `debhelper` |
| .rpm | RHEL/CentOS/Fedora | `rpmbuild`, `rpm-build` |

### 原生平台构建

#### Arch Linux

```bash
# 安装构建依赖
sudo pacman -S base-devel go

# 构建包
cd pkg/archlinux
makepkg -s

# 安装
sudo pacman -U ezpay-1.0.0-1-x86_64.pkg.tar.zst
```

#### Debian/Ubuntu

```bash
# 安装构建依赖
sudo apt install build-essential devscripts debhelper golang-go

# 构建包
./pkg/debian/build.sh

# 安装
sudo dpkg -i release/dist/ezpay_1.0.0-1_amd64.deb
```

#### RHEL/CentOS/Fedora

```bash
# 安装构建依赖 (Fedora/RHEL 8+)
sudo dnf install rpm-build rpmdevtools golang

# 或 CentOS 7
sudo yum install rpm-build rpmdevtools golang

# 构建包
./pkg/rpm/build.sh

# 安装
sudo rpm -ivh release/dist/ezpay-1.0.0-1.x86_64.rpm
# 或
sudo dnf install release/dist/ezpay-1.0.0-1.x86_64.rpm
```

### Docker 跨平台构建

在任何平台（macOS、Windows、Linux）上使用 Docker 容器构建系统包：

#### 构建 Debian 包

```bash
docker run --rm -v $(pwd):/src -w /src debian:12 bash -c "
    apt update &&
    apt install -y build-essential devscripts debhelper golang-go &&
    ./pkg/debian/build.sh
"
```

#### 构建 RPM 包

```bash
docker run --rm -v $(pwd):/src -w /src fedora:39 bash -c "
    dnf install -y rpm-build golang &&
    ./pkg/rpm/build.sh
"
```

#### 构建 Arch Linux 包

```bash
docker run --rm -v $(pwd):/src -w /src archlinux:base-devel bash -c "
    pacman -Sy --noconfirm go &&
    useradd -m builder &&
    chown -R builder:builder /src &&
    su builder -c 'cd /src/pkg/archlinux && makepkg -s --noconfirm'
"
```

#### 一键构建所有平台

```bash
# 创建输出目录
mkdir -p release/dist

# 并行构建所有包
docker run --rm -v $(pwd):/src -w /src debian:12 bash -c \
    "apt update && apt install -y build-essential devscripts debhelper golang-go && ./pkg/debian/build.sh" &

docker run --rm -v $(pwd):/src -w /src fedora:39 bash -c \
    "dnf install -y rpm-build golang && ./pkg/rpm/build.sh" &

wait
echo "All packages built in release/dist/"
```

### 安装后的文件位置

| 路径 | 说明 |
|------|------|
| `/usr/bin/ezpay` | 可执行文件 |
| `/etc/ezpay/config.yaml` | 配置文件 |
| `/usr/share/ezpay/web/` | Web 资源文件 |
| `/var/lib/ezpay/` | 数据目录 |
| `/var/log/ezpay/` | 日志目录 |
| `/usr/lib/systemd/system/ezpay.service` | systemd 服务 |

### 服务管理

```bash
# 编辑配置文件
sudo vim /etc/ezpay/config.yaml

# 启动服务
sudo systemctl start ezpay

# 开机自启
sudo systemctl enable ezpay

# 查看状态
sudo systemctl status ezpay

# 查看日志
sudo journalctl -u ezpay -f
```

### 卸载

```bash
# Arch Linux
sudo pacman -R ezpay

# Debian/Ubuntu
sudo apt remove ezpay
sudo apt purge ezpay  # 同时删除配置和数据

# RHEL/CentOS/Fedora
sudo dnf remove ezpay
```

## 清理

```bash
# 清理所有构建产物
make clean

# 仅清理发布目录
make clean-release
```
