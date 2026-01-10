# EzPay - 多通道聚合收款平台

EzPay 是一个使用 Go 语言开发的多通道聚合收款平台，支持 USDT/TRX 加密货币和微信/支付宝法币收款，兼容彩虹易支付和 V免签接口协议。

## 技术支持

- Telegram 群组: https://t.me/OpineWorkPublish
- Telegram 频道: https://t.me/OpineWorkPublish

## 功能特性

### 支付通道
- **加密货币**: TRC20、ERC20、BEP20、Polygon、Optimism、Arbitrum、Avalanche、Base、TRX
- **法币收款**: 微信支付、支付宝
- **收款码上传**: 支持上传微信/支付宝收款码，自动解析二维码内容

### 钱包管理
- **系统钱包**: 管理员统一管理的收款地址
- **商户钱包**: 商户自行添加的个人收款地址
- **钱包模式**: 支持仅系统钱包、仅个人钱包、混合模式（优先个人）
- **轮询调度**: 同一链路多个钱包自动轮询使用，均衡负载
- **手续费差异化**: 系统钱包和个人钱包可设置不同手续费率

### 接口兼容
- **彩虹易支付**: 完全兼容 submit.php、mapi.php、api.php 接口
- **V免签**: 兼容 createOrder、checkOrder、closeOrder、appPush 接口
- **签名验证**: 支持 MD5 签名验证

### 订单管理
- **自动过期**: 订单超时自动标记过期，退还预扣手续费
- **一键清理**: 支持一键清理超过24小时的无效订单
- **测试支付**: 管理后台可发起测试支付订单
- **CSV导出**: 支持订单数据导出

### 商户功能
- **独立后台**: 商户可登录查看订单、管理钱包
- **余额提现**: 商户可申请提现到绑定的收款地址
- **API密钥**: 商户可自助重置API密钥
- **钱包限制**: 可限制商户添加钱包的数量

### 通知推送
- **Telegram Bot**: 支持商户绑定Telegram接收实时通知
- **异步回调**: 订单支付成功后自动回调商户服务器
- **回调重试**: 回调失败自动重试，支持手动重试

### 安全功能
- **IP白名单**: 支持配置商户API调用IP白名单
- **Referer白名单**: 支持配置允许的来源域名
- **IP黑名单**: 支持封禁恶意IP
- **API日志**: 记录所有API调用日志，支持按时间清理

### 国际化 (i18n)
- **多语言支持**: 英语、简体中文、繁体中文、俄语、波斯语、越南语、缅甸语
- **自动检测**: 根据浏览器语言自动切换
- **语言选择器**: 用户可随时切换语言
- **RTL支持**: 波斯语等从右到左语言的完整支持

## 快速开始

### 环境要求

- Go 1.21+
- MySQL 8.0+
- Make (可选)

### 安装

```bash
# 进入项目目录
cd /path/to/ezpay

# 安装依赖
go mod tidy

# 方式一: 使用 Makefile (推荐)
make release           # 编译当前平台生产版本
make release-all       # 编译所有平台

# 方式二: 手动编译
go build -o ezpay .

# 运行
./ezpay
```

### 开发模式

开发时推荐使用开发模式，支持模板热更新：

```bash
make dev        # 编译开发版本到 bin/
make run-dev    # 编译并运行

# 开发版本特点:
# - 模板从文件系统加载，修改后刷新浏览器即可
# - 配置文件在 bin/config.yaml
```

### 全平台编译

支持一键编译 Linux/Windows/macOS 全平台：

```bash
make release-all   # 编译所有平台
make dist          # 编译并打包成压缩包

# 输出:
# release/ezpay-linux-amd64
# release/ezpay-linux-arm64
# release/ezpay-windows-amd64.exe
# release/ezpay-darwin-amd64
# release/ezpay-darwin-arm64
```

详细构建说明请参阅 [BUILD.md](./BUILD.md)

### 配置

首次运行会自动生成 `config.yaml` 配置文件：

```yaml
server:
  host: "0.0.0.0"
  port: 6088

database:
  host: "127.0.0.1"
  port: 3306
  user: "ezpay"
  password: "your_password"
  dbname: "ezpay"

# JWT 签名密钥 (请修改为随机字符串)
jwt:
  secret: "change-this-secret-key-in-production"

# 数据存储目录 (上传文件、APK等)
# Linux 默认: /var/lib/ezpay
# Windows/macOS 默认: 可执行文件目录/ezpay_data
storage:
  data_dir: "/var/lib/ezpay"
```

> 管理员账号和 Telegram 配置在管理后台"系统设置"中配置，不在配置文件中。

### 访问地址

默认端口: **6088**

| 页面 | 地址 |
|------|------|
| 管理后台 | http://localhost:6088/admin |
| 商户后台 | http://localhost:6088/merchant |
| 收银台 | http://localhost:6088/cashier/{trade_no} |
| 健康检查 | http://localhost:6088/health |

### 默认账号

- 管理员: admin / admin123
- 测试商户: 10001 / 123456

## 系统设置

### 汇率配置

| 设置项 | 说明 |
|--------|------|
| 汇率模式 | auto(自动)/manual(手动)/hybrid(混合)，仅对USDT生效 |
| 手动汇率 | 手动模式下的USDT汇率 |

> TRX 汇率始终使用自动模式，从 Binance API 实时获取。

### 手续费配置

| 设置项 | 说明 |
|--------|------|
| 系统收款码手续费率 | 使用系统钱包时的手续费率，如0.02表示2% |
| 个人收款码手续费率 | 使用商户钱包时的手续费率，如0.01表示1% |

### 订单配置

| 设置项 | 说明 |
|--------|------|
| 订单过期时间 | 订单超时时间（分钟），默认30分钟 |

### 链路配置

在管理后台"链路管理"中可以启用/禁用各支付通道：

- trc20: TRC20 (Tron USDT)
- erc20: ERC20 (Ethereum USDT)
- bep20: BEP20 (BSC USDT)
- trx: TRX (Tron原生币)
- wechat: 微信支付
- alipay: 支付宝

## Telegram 通知

### 1. 创建 Bot

1. 在 Telegram 搜索 `@BotFather`
2. 发送 `/newbot` 创建新机器人
3. 按提示设置名称，获取 Bot Token

### 2. 配置 Bot Token

在管理后台 > 系统设置 中配置：

| 设置项 | 说明 |
|-------|------|
| telegram_enabled | 是否启用 (true/false) |
| telegram_bot_token | Bot Token |

### 3. 商户绑定

商户在 Telegram 中与 Bot 对话：

1. 发送 `/start` 开始
2. 发送 `/bind <商户PID> <商户密钥>` 绑定账号
3. 绑定成功后即可接收通知

### 4. 通知类型

| 通知类型 | 说明 |
|---------|------|
| 订单创建 | 新订单创建时通知 |
| 订单支付 | 订单收到支付时通知 |
| 订单过期 | 订单超时未支付时通知 |
| 提现申请 | 提现申请提交时通知 |
| 提现审核 | 提现审核通过/拒绝时通知 |
| 提现到账 | 提现完成打款时通知 |

### 5. Bot 命令

| 命令 | 说明 |
|------|------|
| `/start` | 开始使用 |
| `/bind <pid> <key>` | 绑定商户账号 |
| `/unbind` | 解除绑定 |
| `/status` | 查看绑定状态 |
| `/help` | 查看帮助 |

## 收款码上传

### 支持的格式

| 支付方式 | 支持的二维码格式 |
|---------|-----------------|
| 微信支付 | wxp://、weixin://、wechatpay.cn、wx.tenpay.com |
| 支付宝 | alipay://、qr.alipay.com |

### 上传流程

1. 在商户后台"钱包管理"点击"添加钱包"
2. 选择链类型（微信/支付宝）
3. 上传收款码图片
4. 系统自动解析二维码内容
5. 保存后即可用于收款

## 接口文档

详见 [API.md](./API.md)

## 项目结构

详见 [ARCHITECTURE.md](./ARCHITECTURE.md)

## 文档索引

| 文档 | 说明 |
|------|------|
| [USER_GUIDE.md](./USER_GUIDE.md) | 用户手册 - 管理员和商户操作指南 |
| [BUILD.md](./BUILD.md) | 构建指南 - 开发/生产模式，跨平台编译 |
| [DEPLOY.md](./DEPLOY.md) | 部署指南 - Systemd、Nginx、Docker |
| [API.md](./API.md) | API接口文档 - 彩虹易支付/V免签兼容接口 |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | 架构设计 - 项目结构、核心模块 |
| [I18N.md](./I18N.md) | 国际化指南 - 多语言支持、翻译开发 |
| [COMPARE_EPAY.md](./COMPARE_EPAY.md) | 彩虹易支付对比与迁移 |
| [COMPARE_VMQ.md](./COMPARE_VMQ.md) | V免签对比与迁移 |

## 更新日志

### v1.0.0 (2025-01-02)

**支付通道**
- 支持 USDT 多链收款 (TRC20, ERC20, BEP20, Polygon, Optimism, Arbitrum, Avalanche, Base)
- 支持 TRX 原生币支付
- 支持微信/支付宝收款码支付
- 商户手动确认收款功能 (法币订单)
- 兼容彩虹易支付和V免签接口协议

**钱包管理**
- 系统钱包和商户钱包双模式
- 钱包轮询调度，均衡负载
- 差异化手续费配置

**国际化**
- 支持 7 种语言：英语、简体中文、繁体中文、俄语、波斯语、越南语、缅甸语
- 自动语言检测：URL参数 > localStorage > 浏览器语言
- RTL 支持：波斯语等从右到左语言完整布局

**通知推送**
- Telegram Bot 实时通知
- 异步回调支持重试

**构建部署**
- 全平台交叉编译 (Linux/Windows/macOS)
- Docker 容器化部署
- Linux 打包支持 (Arch/Debian/RPM)
- 静态资源嵌入二进制
- 开发/生产双模式构建

**安全功能**
- IP 白名单/黑名单
- Referer 白名单
- API 调用日志
