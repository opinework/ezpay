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
- MySQL/MariaDB 8.0+ 或 SQLite
- Git

### 1. 准备数据库

SQLite 适合单机轻量部署或开发测试，只需在 `config.yaml` 中设置：

```yaml
database:
  driver: "sqlite"
storage:
  data_dir: "/var/lib/ezpay"  # SQLite 文件会保存为 {data_dir}/ezpay.db
```

MySQL/MariaDB 适合生产高并发部署：

```sql
CREATE DATABASE ezpay CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'ezpay'@'localhost' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON ezpay.* TO 'ezpay'@'localhost';
FLUSH PRIVILEGES;
```

### 2. 构建和配置

```bash
# 编译当前平台生产版本
make release

# 准备运行目录配置文件
cp config.yaml release/config.yaml
vim release/config.yaml
```

至少需要修改：

- `database.*`: 数据库连接信息
- `jwt.secret`: 生产环境必须改为随机长字符串
- `storage.data_dir`: Linux 生产环境建议使用 `/var/lib/ezpay`
- `blockchain.*.rpc`: 生产环境建议替换为稳定 RPC 节点

### 3. 初始化并运行

```bash
# MySQL/MariaDB 推荐显式执行迁移
# 先按实际数据库修改 db/migrate.sh 顶部 DB_HOST/DB_USER/DB_PASS/DB_NAME
./db/migrate.sh migrate

# 运行
cd release && ./ezpay
```

> SQLite 模式不使用 MySQL 迁移脚本，程序启动时会通过 GORM `AutoMigrate` 初始化表结构。MySQL/MariaDB 生产环境仍建议使用 `db/migrate.sh` 管理数据库版本，便于备份、审计和回滚。执行迁移脚本时请确保脚本中的数据库连接与 `release/config.yaml` 一致。

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
- **测试商户**:
  - 通过 `db/migrate.sh` / `db/init.sql` 初始化：`10001` / `merchant123`
  - 空库直接启动程序自动初始化：`1001` / `merchant123`
- **默认 API Key**: `test_key_123456`

> ⚠️ 首次登录后请立即修改密码！

## 编译构建

### 使用 Makefile

适合本地开发、单平台构建和普通二进制分发：

```bash
make setup-dev    # 初始化开发目录和 web 符号链接
make dev          # 开发版本，输出 bin/ezpay
make run-dev      # 编译并运行开发版本
make release      # 当前平台生产版本，输出 release/ezpay
make release-all  # 全平台二进制
make dist         # 全平台 tar.gz/zip 分发包
make clean        # 清理构建产物
```

> 如果 `make` 找不到 Go，可执行 `make GO=$(which go) dev`，或直接使用 `go build -tags dev -o bin/ezpay .`。

### 使用 build-all.sh

适合发布 Linux 系统包和通用 tarball：

```bash
./build-all.sh          # 构建所有格式 (Arch Linux + Debian + Tarball)
./build-all.sh --arch   # 仅 Arch Linux .pkg.tar.zst
./build-all.sh --deb    # 仅 Debian/Ubuntu .deb
./build-all.sh --tar    # 仅通用 tar.gz/zip
./build-all.sh --help   # 查看帮助
```

### 编译产物

`make release-all` 的二进制位于 `release/` 目录：

```
release/
├── ezpay-linux-amd64       # Linux x64
├── ezpay-linux-arm64       # Linux ARM64
├── ezpay-windows-amd64.exe # Windows
├── ezpay-darwin-amd64      # macOS Intel
└── ezpay-darwin-arm64      # macOS Apple Silicon
```

`./build-all.sh` 的系统包和通用包位于：

```
release/
├── archlinux/
├── debian/
├── tarball/
└── checksums.txt
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
  driver: "mysql"  # mysql 或 sqlite
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
    scan_interval: 30
  trc20:
    enabled: true
    rpc: "https://api.trongrid.io"
    contract_address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
    confirmations: 19
    scan_interval: 30
  bep20:
    enabled: true
    rpc: "https://bsc-rpc.publicnode.com"
    rpc_backups:
      - "https://binance.llamarpc.com"
    contract_address: "0x55d398326f99059fF775485246999027B3197955"
    confirmations: 15
    scan_interval: 30
    max_block_range: 500       # 单次查询最大区块范围
    max_batch_size: 1          # 批量请求数量
    batch_delay_ms: 3000       # 请求间延迟(毫秒)
    rate_limit: 1.0            # 每秒最大请求数
  # ... 其他链配置 (polygon, optimism, arbitrum, base 等)
```

> 💡 管理员账号、Telegram Bot、买卖浮动等在管理后台"系统设置"中配置

## 汇率系统

### USD 统一结算（v1.3.8 起）

所有订单统一按 USD 结算，支持多币种支付。**结算金额 = 市场汇率折算的真实 USD 价值**（不含浮动），浮动只作用于跨币种的用户支付腿。

```
跨币种示例: 商品 100 CNY → 用 USDT 支付
  ↓ Step 1: 结算金额（市场汇率）
    100 CNY × 0.139 = 13.89 USD
  ↓ Step 2: 用户应付（跨币种应用买入浮动）
    13.89 / (1 - 0.02) = 14.17 USDT
  ↓
  商户余额 +13.89 USD，平台浮动收入 ≈ 0.28 USDT (~2%)

同币种示例: 商品 100 CNY → 用微信支付（CNY 通道）
  ↓ Step 1: 结算金额
    100 CNY × 0.139 = 13.89 USD
  ↓ Step 2: 同币种直通，不应用浮动
    pay_amount = 100 CNY
  ↓
  用户付 100 CNY，商户余额 +13.89 USD，平台仅收 FeeRate
```

### 商户级默认货币

每个商户可在后台配置 `default_currency`（USD/CNY/USDT/EUR）。订单创建时按 **请求 currency → 商户默认 → USD 兜底** 顺序解析。

> **老 PHP 易支付 / V免签 客户端零代码迁移**：把商户默认货币设为 `CNY`，老代码请求的 `money` 或 `price` 自动按 CNY 计价。

### 买卖价差

- **买入浮动** (`rate_buy_float`): 跨币种支付时平台多收（如 +2%）。同币种 / 同稳定锚定 (USD↔USDT) 不应用
- **卖出浮动** (`rate_sell_float`): 商户提现时平台少给（如 -2%）
- **利润空间**: 跨币种订单 ~ 买入浮动 + 卖出浮动；同币种订单仅 FeeRate

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
| block_scan_progress | 区块扫描进度表 |
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
- ✅ **异步回调**: 带签名的 GET 通知，失败自动重试
- ✅ **同步跳转**: 带签名的同步跳转URL
- ✅ **Telegram 封禁检测**: 自动停止向被封账号推送
- ✅ **SSRF 防护**: 回调地址会拦截 localhost、内网和保留地址

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
# 参数签名: 参数按键名排序后拼接，再追加商户密钥并 MD5
curl "http://localhost:6088/submit.php" \
  -d "pid=10001" \
  -d "type=usdt_trc20" \
  -d "out_trade_no=ORDER123" \
  -d "notify_url=https://your-site.com/notify" \
  -d "return_url=https://your-site.com/return" \
  -d "name=测试商品" \
  -d "money=100" \
  -d "currency=USD" \
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
| [文档中心](./docs/README.md) | 接口文档和用户手册索引 |
| [API文档](./docs/04-API.md) | 彩虹易支付/V免签兼容接口 |
| [用户手册](./docs/05-USER_GUIDE.md) | 管理员和商户操作指南 |

### GitHub Actions 构建

- 推送分支或创建 Pull Request 时，`CI` 工作流执行 Go 测试和生产版本编译。
- 在 Actions 的 `Release` 工作流中选择 `Run workflow`，可手动打包全部格式，或单独选择 `archlinux`、`debian`、`tarball`。安装包和 `CHECKSUMS.txt` 可从该次运行的 Artifacts 下载。
- 推送 `v*` 标签时，自动构建全格式安装包并创建 GitHub Release。手动构建仅上传 Artifacts。

构建使用 Ubuntu 执行器和 Go 1.25，调用仓库内的 `build-all.sh` 和 `pkg/` 打包脚本。

## 系统要求

### 最低配置
- CPU: 1 核
- 内存: 512MB
- 硬盘: 10GB
- 网络: 可访问 RPC 节点和汇率 API；生产环境建议具备公网 HTTPS 域名

### 推荐配置
- CPU: 2 核
- 内存: 2GB
- 硬盘: 50GB SSD
- 网络: 10Mbps+ 带宽，稳定公网 HTTPS 域名（用于收银台、商户回调、Telegram Webhook）

## 性能指标

- **订单处理**: >1000 TPS
- **API 响应**: <100ms (p99)
- **区块链扫描**: 30秒基础间隔（无订单时自动延长至4倍）
- **扫描进度持久化**: 重启后从上次位置继续扫描
- **数据库连接池**: 100 连接
- **并发支持**: 支持高并发场景
- **RPC故障转移**: 多节点自动切换，指数退避重试
- **链级限流**: 每条链独立配置请求速率

> 性能指标会受数据库、RPC 节点、链确认数、钱包数量和日志量影响，生产环境上线前请做压测和回归测试。

## 商业节点推荐

使用免费公共节点可能受到速率限制，生产环境建议使用商业节点服务：

| 提供商 | 支持链 | 特点 | 官网 |
|--------|--------|------|------|
| Alchemy | ETH, Polygon, Optimism, Arbitrum, Base | 免费额度大，稳定性好 | alchemy.com |
| Infura | ETH, Polygon, Optimism, Arbitrum | MetaMask 默认节点 | infura.io |
| QuickNode | 全链支持 | 速度快，API丰富 | quicknode.com |
| Ankr | 全链支持 | 价格实惠，支持BSC | ankr.com |
| TronGrid Pro | TRX, TRC20 | Tron官方，需申请API Key | trongrid.io |
| GetBlock | 全链支持 | 按请求数计费 | getblock.io |

配置商业节点示例：
```yaml
blockchain:
  bep20:
    rpc: "https://your-quicknode-endpoint.com"
    api_key: "your-api-key"
    rate_limit: 25.0            # 商业节点可设更高
    max_batch_size: 5           # 支持批量请求
    batch_delay_ms: 200         # 更短延迟
```

## 交流群组

- 加入 Telegram 交流群：[@OpineWorkOfficial](https://t.me/OpineWorkOfficial)
- Telegram 频道: https://t.me/OpineWorkPublish
- QQ群

## 贡献

欢迎提交 Issue 和 Pull Request！

## License

MIT License

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=opinework/ezpay&type=Date)](https://star-history.com/#opinework/ezpay&Date)

## 致谢

感谢所有贡献者和使用者的支持！
