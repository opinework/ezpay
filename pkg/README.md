# EzPay 打包指南

本目录包含为不同 Linux 发行版构建 EzPay 安装包的脚本和配置文件。**所有构建脚本都使用本地源码**，不会从 GitHub 或其他远程仓库下载代码。

## 目录结构

```
pkg/
├── archlinux/          # Arch Linux 打包文件
│   ├── PKGBUILD       # Arch 打包配置
│   ├── build.sh       # 自动化构建脚本
│   ├── ezpay.install  # 安装/卸载钩子
│   └── ezpay.service  # systemd 服务文件
├── debian/            # Debian/Ubuntu 打包文件
│   ├── build.sh       # 自动化构建脚本
│   ├── control        # 包控制文件
│   ├── rules          # 构建规则
│   ├── changelog      # 变更日志
│   ├── postinst       # 安装后脚本
│   ├── prerm          # 卸载前脚本
│   ├── postrm         # 卸载后脚本
│   └── ezpay.service  # systemd 服务文件
└── rpm/               # RPM 打包文件 (待实现)
```

## Arch Linux 打包

### 前置要求

```bash
# 安装必要的构建工具
sudo pacman -S base-devel go
```

### 构建包

```bash
# 方法 1: 使用自动化脚本（推荐）
cd pkg/archlinux
./build.sh

# 方法 2: 手动构建
cd pkg/archlinux
makepkg -f --clean --noextract
```

### 安装包

```bash
# 安装构建好的包
sudo pacman -U release/dist/archlinux/ezpay-*.pkg.tar.zst

# 配置服务
sudo vim /etc/ezpay/config.yaml
sudo systemctl enable ezpay
sudo systemctl start ezpay
```

### 测试构建（推荐用于发布）

使用干净的 chroot 环境测试构建：

```bash
# 创建 chroot 环境
mkdir -p ~/chroot
mkarchroot ~/chroot/root base-devel

# 在 chroot 中构建
cd pkg/archlinux
makechrootpkg -c -r ~/chroot
```

## Debian/Ubuntu 打包

### 前置要求

```bash
# Debian/Ubuntu
sudo apt install build-essential devscripts debhelper golang-go

# 确保 Go 版本 >= 1.21
go version
```

### 构建包

```bash
# 方法 1: 使用自动化脚本（推荐）
cd pkg/debian
./build.sh

# 方法 2: 从项目根目录构建
cd /path/to/ezpay
dpkg-buildpackage -us -uc -b
```

### 安装包

```bash
# 安装构建好的包
sudo dpkg -i release/dist/debian/ezpay_*.deb

# 安装依赖（如果有缺失）
sudo apt-get install -f

# 配置服务
sudo nano /etc/ezpay/config.yaml
sudo systemctl enable ezpay
sudo systemctl start ezpay
sudo systemctl status ezpay
```

### 卸载

```bash
# 保留配置文件
sudo apt-get remove ezpay

# 完全删除（包括配置）
sudo apt-get purge ezpay
```

## 构建特性

### 本地源码构建

所有构建脚本都配置为使用项目根目录的源代码，**不会从任何远程仓库下载**：

- **Arch Linux**: `PKGBUILD` 中 `source=()` 为空，直接使用 `$startdir/../..` 访问项目目录
- **Debian**: `rules` 文件直接在 `$(CURDIR)` 构建，不执行任何下载操作

### 构建选项

两种构建系统都使用以下 Go 构建选项：

```bash
CGO_ENABLED=0          # 静态编译，无 C 依赖
GOOS=linux             # 目标操作系统
-trimpath              # 去除文件系统路径
-ldflags="-s -w"       # 压缩二进制文件
-X main.Version=...    # 嵌入版本信息
-X main.BuildDate=...  # 嵌入构建日期
```

### 包含的文件

打包后的安装包包含：

- **二进制文件**: `/usr/bin/ezpay`
- **配置文件**: `/etc/ezpay/config.yaml`
- **Web 资源**: `/usr/share/ezpay/web/{templates,static}`
- **systemd 服务**: `/usr/lib/systemd/system/ezpay.service` (Arch)
  `/lib/systemd/system/ezpay.service` (Debian)
- **数据目录**: `/var/lib/ezpay`
- **日志目录**: `/var/log/ezpay`
- **文档**: `/usr/share/doc/ezpay/README.md`
- **许可证**: `/usr/share/licenses/ezpay/LICENSE` (Arch)

### 安装后操作

包安装后会自动：

1. 创建系统用户和组 `ezpay`
2. 设置正确的文件权限
3. 将配置文件权限设为 `600` (仅 ezpay 用户可读写)
4. 重新加载 systemd 配置

## 构建输出位置

- **Arch Linux**: `release/dist/archlinux/ezpay-*.pkg.tar.zst`
- **Debian**: `release/dist/debian/ezpay_*.deb`

## 版本管理

在发布新版本前，需要更新：

### Arch Linux
编辑 `pkg/archlinux/PKGBUILD`:
```bash
pkgver=1.0.1  # 新版本号
pkgrel=1      # 包发布号（同一版本的不同构建）
```

### Debian
编辑 `pkg/debian/changelog`:
```bash
dch -v 1.0.1-1 "Release version 1.0.1"
# 或手动编辑 changelog 文件
```

## 故障排除

### Go 版本过低

```bash
# Arch Linux
sudo pacman -S go

# Debian/Ubuntu - 可能需要添加官方 Go PPA
sudo add-apt-repository ppa:longsleep/golang-backports
sudo apt update
sudo apt install golang-go
```

### 构建权限错误

确保你有权限读取项目源码：

```bash
# 检查权限
ls -la /path/to/ezpay

# 如果需要，修改权限
sudo chown -R $USER:$USER /path/to/ezpay
```

### makepkg 找不到文件

确保从正确的目录运行：

```bash
# 对于 Arch，必须在 pkg/archlinux 目录
cd pkg/archlinux
./build.sh
```

### dpkg-buildpackage 错误

```bash
# 清理之前的构建
rm -rf debian/ezpay debian/.debhelper

# 确保 debian 符号链接正确
ln -sf pkg/debian debian

# 重新构建
./pkg/debian/build.sh
```

## 开发者注意事项

### 添加新文件到包

如果你向项目添加了新文件需要包含在安装包中：

1. **Arch Linux**: 编辑 `pkg/archlinux/PKGBUILD` 的 `package()` 函数
2. **Debian**: 编辑 `pkg/debian/rules` 的 `override_dh_auto_install` 部分

### 修改 systemd 服务

编辑对应的 `ezpay.service` 文件后重新构建包。

### 测试包

在虚拟机或容器中测试安装包是最佳实践：

```bash
# 使用 Docker 测试 Debian 包
docker run -it --rm -v $(pwd):/build debian:12
cd /build
apt update && apt install -y build-essential devscripts debhelper golang-go
./pkg/debian/build.sh

# 使用 Docker 测试 Arch 包
docker run -it --rm -v $(pwd):/build archlinux:latest
cd /build
pacman -Syu --noconfirm base-devel go
cd pkg/archlinux && makepkg -f --clean --noextract
```

## 贡献

欢迎提交改进建议和 bug 报告！在修改打包脚本时，请确保：

1. 保持本地源码构建特性
2. 测试在干净环境中的构建
3. 更新相关文档
4. 遵循各发行版的打包规范

## 许可证

与 EzPay 主项目相同的许可证。
