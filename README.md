# EzPay

多通道聚合收款平台，支持 USDT/TRX 加密货币和微信/支付宝法币收款，采用 **USD 统一结算**，兼容彩虹易支付和 V免签接口协议。

## 核心特性

### 💰 统一结算系统
- **USD 统一结算**: 所有订单统一按 USD 结算，支持多币种支付
- **智能汇率管理**: 自动获取实时汇率，支持手动/自动/混合模式
- **买卖价差**: 可配置买入卖出浮动，实现利润空间
- **多数据源**: Binance、OKX、自定义 API

### 🔗 多链支持
- **加密货币**: TRC20、ERC20、BEP20、Polygon、Optimism、Arbitrum、Base、TRX
- **法币收款**: 微信支付、支付宝
- **自动匹配**: 唯一金额标识，精确匹配订单

### 🔔 完整通知系统
#### 商户通知 (Telegram Bot)
- 订单创建/支付/过期
- 余额变动通知
- 提现状态通知
- 登录成功/失败警告
- 密钥重置警告
- 钱包添加/移除
- 回调失败提醒

#### 管理员通知
- 提现地址待审核
- IP 封禁事件
- 系统异常警告

#### 账号状态管理
- 自动检测 Telegram 账号封禁
- 封禁后自动停止推送
- 避免无效通知消耗

### 🔒 安全防护
- **IP 黑名单/白名单**: 自动封禁异常IP，缓存机制提升性能
- **签名验证**: 所有 API 请求支持签名校验
- **JWT 认证**: 管理员和商户独立认证
- **交易防重**: 防止重复处理同一笔交易
- **并发保护**: 乐观锁机制防止订单重复入账
- **限流保护**: API 和登录接口限流

### 🌍 国际化
- 支持 **7 种语言**: 英语、简中、繁中、俄语、波斯语、越南语、缅甸语
- RTL 布局支持（波斯语）
- 自动语言检测

### 🔧 商户管理
- **独立后台**: 商户可自助管理钱包、订单、提现
- **余额提现**: 支持 TRC20/BEP20 USDT 提现
- **API 密钥**: 可重置密钥
- **提现地址管理**: 审核机制保障安全
- **钱包限制**: 可配置钱包数量上限

### 📊 接口兼容
- **彩虹易支付**: 完全兼容 submit.php、mapi.php、api.php
- **V免签**: 兼容 createOrder、appHeart、appPush 等接口
- **标准化**: 统一的响应格式和错误码

## 快速开始

### 环境要求

- Go 1.21+
- MySQL/MariaDB 8.0+

### 安装运行

```bash
# 下载发布版本或编译
make release

# 编辑配置文件
cp config.yaml.example config.yaml
vim config.yaml

# 初始化数据库（自动执行迁移）
# 首次运行会自动创建表结构

# 运行
./ezpay
```

### 访问地址

默认端口: **6088**

| 页面 | 地址 | 说明 |
|------|------|------|
| 管理后台 | http://localhost:6088/admin | 系统管理、商户管理、汇率管理 |
| 商户后台 | http://localhost:6088/merchant | 订单查询、钱包管理、提现申请 |
| 收银台 | http://localhost:6088/cashier/{trade_no} | 用户支付页面 |
| 健康检查 | http://localhost:6088/health | 服务状态检查 |
| 详细健康检查 | http://localhost:6088/health/detail | 数据库连接状态 |

### 默认账号

- **管理员**: `admin` / `admin123`
- **测试商户**: `10001` / `123456`

> ⚠️ 首次登录后请立即修改密码！

## 编译构建

### 使用 build-all.sh (推荐)

```bash
./build-all.sh              # 编译所有平台
./build-all.sh dev          # 开发版本
./build-all.sh linux        # 仅 Linux AMD64
./build-all.sh linux-arm64  # 仅 Linux ARM64
./build-all.sh windows      # 仅 Windows
./build-all.sh macos        # 仅 macOS Intel
./build-all.sh macos-arm64  # 仅 macOS Apple Silicon
./build-all.sh clean        # 清理编译产物
```

### 使用 Makefile

```bash
make dev         # 开发版本
make release     # 生产版本
make release-all # 全平台编译
make dist        # 打包发布
```

### 编译产物

全平台编译后的文件位于 `release/` 目录：

```
release/
├── ezpay-linux-amd64       # Linux x64
├── ezpay-linux-arm64       # Linux ARM64
├── ezpay-windows-amd64.exe # Windows
├── ezpay-darwin-amd64      # macOS Intel
└── ezpay-darwin-arm64      # macOS Apple Silicon
```

## 配置说明

所有配置都在 `config.yaml` 中，无需 `.env` 文件：

```yaml
# 服务器
server:
  host: "0.0.0.0"
  port: 6088

# 数据库
database:
  host: "127.0.0.1"
  port: 3306
  user: "ezpay"
  password: "your_password"
  dbname: "ezpay"

# JWT认证
jwt:
  secret: "change-this-secret-key-in-production"
  expire_hour: 24

# 数据存储
storage:
  data_dir: "/var/lib/ezpay"  # Linux 默认

# 订单配置
order:
  expire_minutes: 30          # 订单过期时间
  cleanup_hours: 24           # 自动清理无效订单

# 汇率配置
rate:
  auto_update_enabled: true   # 启用自动更新
  update_interval: 60         # 更新间隔(分钟)
  source: "binance"           # 主数据源
  fallback_source: "okx"      # 备用数据源
  cny_api: "https://api.exchangerate-api.com/v4/latest/USD"  # CNY汇率API
  cache_seconds: 300          # 缓存时间

# 区块链监控
blockchain:
  trx:
    enabled: true
    rpc: "https://api.trongrid.io"
    confirmations: 19
    scan_interval: 15
  trc20:
    enabled: true
    rpc: "https://api.trongrid.io"
    contract_address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
    confirmations: 19
    scan_interval: 15
  # ... 其他链配置
```

> 💡 管理员账号、Telegram Bot、买卖浮动等在管理后台"系统设置"中配置

## 汇率系统

### USD 统一结算

所有订单统一按 USD 结算，支持多币种支付：

```
用户支付: 100 CNY
  ↓ 使用买入汇率(1 CNY = 0.14 USD × 1.02)
结算金额: 100 × 0.14 × 1.02 = 14.28 USD
  ↓ 计入商户余额
商户余额: +14.28 USD
```

### 买卖价差

- **买入浮动**: 用户支付时，汇率上浮（如 +2%），平台多收
- **卖出浮动**: 商户提现时，汇率下浮（如 -2%），平台少给
- **利润空间**: 买卖价差 = 4%

### 支持的汇率

| 汇率对 | 说明 | 自动更新 |
|--------|------|---------|
| EUR → USD | 欧元转美元 | ✅ Binance |
| CNY → USD | 人民币转美元 | ✅ exchangerate-api |
| USD → CNY | 美元转人民币 | ✅ exchangerate-api |
| USD → TRX | 美元转波场 | ✅ Binance |
| USD → USDT | 美元转USDT | ✅ 固定1:1 |

## 支持的链路

### 区块链加密货币

| 链 | 代币 | 说明 | 确认数 |
|----|------|------|-------|
| TRX | TRX | Tron 原生币 | 19 |
| TRC20 | USDT | Tron USDT | 19 |
| ERC20 | USDT | Ethereum USDT | 12 |
| BEP20 | USDT | BSC USDT | 15 |
| Polygon | USDT | Polygon USDT | 128 |
| Optimism | USDT | Optimism USDT | 10 |
| Arbitrum | USDT | Arbitrum USDT | 10 |
| Base | USDT | Base USDT | 10 |
| Avalanche | USDT | Avalanche USDT | 12 |

### 传统支付

| 类型 | 说明 |
|------|------|
| WeChat | 微信支付（需上传收款码） |
| Alipay | 支付宝（需上传收款码） |

## Telegram 通知

### 商户绑定

商户可通过 Telegram Bot 接收实时通知：

```
1. 在 Telegram 搜索你的 Bot (@YourBot)
2. 发送命令: /bind 商户号 密钥
3. 绑定成功后自动接收通知
```

### 通知类型

**订单通知**
- 📦 新订单创建
- 💰 订单支付成功
- ⏰ 订单过期

**资金通知**
- 💵 余额变动（订单入账）
- 📤 提现申请已提交
- ✅ 提现审批通过
- ❌ 提现被拒绝
- 💸 提现已打款

**安全通知**
- 🔐 登录成功
- ⚠️ 登录失败
- 🔑 密钥重置

**系统通知**
- 💳 钱包添加
- 🗑️ 钱包移除
- 📞 回调失败警告

### 管理员通知

管理员通过系统配置的 Telegram 群组接收：
- 📬 新增提现地址待审核
- 🚫 IP 被封禁事件
- 🔔 系统警告

## 存储目录

上传文件存储在 `storage.data_dir` 配置的目录：

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

## 数据库表

| 表名 | 说明 |
|------|------|
| merchants | 商户表 |
| wallets | 钱包地址表 |
| orders | 订单表（USD结算） |
| transaction_logs | 交易日志表 |
| exchange_rates | 汇率配置表 |
| exchange_rate_history | 汇率历史表 |
| withdrawals | 提现表 |
| withdraw_addresses | 提现地址表 |
| system_configs | 系统配置表 |
| admins | 管理员表 |
| api_logs | API 日志表 |
| ip_blacklist | IP 黑名单表 |
| app_versions | APP版本表 |

## 安全机制

### 交易安全
- ✅ **交易防重**: 通过 tx_hash 唯一索引防止重复处理
- ✅ **并发保护**: 乐观锁机制防止订单重复入账
- ✅ **金额精确匹配**: 使用 unique_amount 精确匹配订单

### 访问控制
- ✅ **IP 黑名单**: 自动封禁异常IP，带缓存提升性能
- ✅ **IP 白名单**: 商户可配置 IP 白名单
- ✅ **Referer 白名单**: 限制来源域名
- ✅ **签名验证**: 所有 API 请求签名校验

### 通知安全
- ✅ **异步回调**: 带签名的异步通知，失败自动重试
- ✅ **同步跳转**: 带签名的同步跳转URL
- ✅ **Telegram 封禁检测**: 自动停止向被封账号推送

## API 测试

### 获取管理员 Token

```bash
curl -X POST "http://localhost:6088/admin/api/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

### 使用 Token 调用 API

```bash
TOKEN="your_token_here"
curl -s "http://localhost:6088/admin/api/dashboard" \
  -H "Authorization: Bearer $TOKEN"
```

### 创建订单（彩虹易支付格式）

```bash
# 参数签名: MD5(参数排序&key=商户密钥)
curl "http://localhost:6088/submit.php" \
  -d "pid=10001" \
  -d "type=trc20" \
  -d "out_trade_no=ORDER123" \
  -d "notify_url=https://your-site.com/notify" \
  -d "return_url=https://your-site.com/return" \
  -d "name=测试商品" \
  -d "money=100" \
  -d "sign=MD5签名"
```

### 健康检查

```bash
# 简单健康检查
curl -s "http://localhost:6088/health"

# 详细健康检查 (含数据库状态)
curl -s "http://localhost:6088/health/detail"
```

## 文档

详细文档请参阅 [docs](./docs/) 目录：

| 文档 | 说明 |
|------|------|
| [用户手册](./docs/USER_GUIDE.md) | 管理员和商户操作指南 |
| [构建指南](./docs/BUILD.md) | 开发/生产模式，跨平台编译 |
| [部署指南](./docs/DEPLOY.md) | Systemd、Nginx、Docker |
| [API文档](./docs/API.md) | 彩虹易支付/V免签兼容接口 |
| [架构设计](./docs/ARCHITECTURE.md) | 项目结构、核心模块 |
| [国际化指南](./docs/I18N.md) | 多语言支持、翻译开发 |

## 系统要求

### 最低配置
- CPU: 1 核
- 内存: 512MB
- 硬盘: 10GB
- 网络: 公网 IP（用于接收区块链回调）

### 推荐配置
- CPU: 2 核
- 内存: 2GB
- 硬盘: 50GB SSD
- 网络: 10Mbps+ 带宽

## 性能指标

- **订单处理**: >1000 TPS
- **API 响应**: <100ms (p99)
- **区块链扫描**: 15秒间隔
- **数据库连接池**: 100 连接
- **并发支持**: 支持高并发场景

## 交流群组

- Telegram 交流群: https://t.me/OpineWorkOfficial
- Telegram 频道: https://t.me/OpineWorkPublish

## 贡献

欢迎提交 Issue 和 Pull Request！

## License

MIT License

## 致谢

感谢所有贡献者和使用者的支持！
