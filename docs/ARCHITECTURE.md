# EzPay 架构设计文档

## 技术栈

- **语言**: Go 1.21+
- **Web框架**: Gin
- **ORM**: GORM
- **数据库**: MySQL 8.0+
- **前端**: 原生 HTML/CSS/JS

## 项目结构

```
ezpay/
├── main.go                     # 程序入口
├── Makefile                    # 构建脚本
├── config.yaml                 # 配置文件 (自动生成)
├── go.mod                      # Go模块定义
│
├── bin/                        # 开发运行目录
│   ├── ezpay                  # 开发版二进制
│   ├── config.yaml            # 开发配置
│   ├── ezpay_data/            # 上传目录
│   └── web -> ../web          # 符号链接
│
├── release/                    # 生产发布目录
│   ├── ezpay-linux-amd64      # Linux x64
│   ├── ezpay-linux-arm64      # Linux ARM64
│   ├── ezpay-windows-amd64.exe # Windows
│   ├── ezpay-darwin-amd64     # macOS Intel
│   ├── ezpay-darwin-arm64     # macOS Apple Silicon
│   └── dist/                  # 打包文件
│
├── config/
│   └── config.go              # 配置管理
│
├── internal/
│   ├── model/                 # 数据模型
│   │   ├── db.go             # 数据库初始化
│   │   ├── merchant.go       # 商户模型
│   │   ├── order.go          # 订单模型
│   │   ├── wallet.go         # 钱包模型
│   │   ├── app_version.go    # APP版本模型
│   │   └── config.go         # 系统配置模型
│   ├── handler/               # HTTP处理器
│   │   ├── epay.go           # 彩虹易支付兼容接口
│   │   ├── vmq.go            # V免签兼容接口
│   │   ├── admin.go          # 管理后台API
│   │   ├── merchant.go       # 商户后台API
│   │   ├── cashier.go        # 收银台
│   │   └── channel.go        # 上游通道
│   ├── service/               # 业务逻辑
│   │   ├── order.go          # 订单服务
│   │   ├── blockchain.go     # 区块链监控主服务
│   │   ├── blockchain_rpc.go # RPC 客户端（重试+故障转移）
│   │   ├── blockchain_scanner.go # 改进的链扫描器
│   │   ├── blockchain_utils.go   # 区块链工具函数
│   │   ├── blockchain_metrics.go # 监控指标收集
│   │   ├── rate.go           # 汇率服务
│   │   ├── notify.go         # 回调通知
│   │   ├── bot.go            # 机器人通知
│   │   └── telegram.go       # Telegram服务
│   ├── middleware/            # 中间件
│   │   └── auth.go           # JWT认证
│   └── util/                  # 工具函数
│       ├── sign.go           # 签名验证
│       ├── crypto.go         # 加密工具
│       ├── qrcode.go         # 二维码生成/解析
│       └── helper.go         # 辅助函数
│
├── web/
│   ├── embed.go              # 生产模式资源嵌入
│   ├── embed_dev.go          # 开发模式文件系统加载
│   ├── templates/            # HTML模板 (生产版嵌入)
│   │   ├── cashier.html     # 收银台页面
│   │   ├── admin.html       # 管理后台
│   │   ├── merchant.html    # 商户后台
│   │   ├── error.html       # 错误页面
│   │   └── success.html     # 成功页面
│   └── static/               # 静态资源 (生产版嵌入)
│       ├── css/
│       ├── js/
│       ├── fonts/
│       └── webfonts/
│
├── {data_dir}/                # 运行时数据目录 (不嵌入)
│   ├── qrcode/               # 收款码图片
│   └── apk/                  # APP安装包
│   # Linux: /var/lib/ezpay
│   # Windows/macOS: {exe_dir}/ezpay_data
│
└── docs/                      # 文档
    ├── README.md             # 项目概述
    ├── BUILD.md              # 构建指南
    ├── DEPLOY.md             # 部署指南
    ├── API.md                # API文档
    ├── USER_GUIDE.md         # 用户手册
    └── ARCHITECTURE.md       # 架构设计
```

## 构建模式

EzPay 支持两种构建模式，通过 Go build tags 切换：

### 开发模式 (`-tags dev`)

```go
// web/embed_dev.go
//go:build dev

// 从文件系统加载资源
r.LoadHTMLGlob("web/templates/*")
r.Static("/static", "web/static")
```

特点：
- 模板和静态文件从文件系统读取
- 修改后刷新浏览器即可生效
- 适合本地开发调试

### 生产模式 (默认)

```go
// web/embed.go
//go:build !dev

//go:embed templates/*
var templateFS embed.FS

//go:embed static/css static/js static/fonts static/webfonts
var staticFS embed.FS
```

特点：
- 使用 Go 1.16+ embed 功能嵌入资源
- 单文件部署，无需额外资源目录
- uploads 目录仍从文件系统读写

## 核心模块

### 1. 区块链监控服务 (blockchain.go)

负责监控多条链上的 USDT 转账交易。

**支持的链**:
- TRC20 (Tron): 通过 TronGrid API
- ERC20 (Ethereum): 通过 eth_getLogs RPC
- BEP20 (BSC): 通过 eth_getLogs RPC
- Polygon: 通过 eth_getLogs RPC
- Optimism: 通过 eth_getLogs RPC (Layer2)
- Arbitrum: 通过 eth_getLogs RPC (Layer2)
- Avalanche: 通过 eth_getLogs RPC (C-Chain)
- Base: 通过 eth_getLogs RPC (Coinbase Layer2)

**监控流程**:
1. 定时扫描区块链交易 (可配置间隔)
2. 筛选发送到收款地址的 USDT 转账
3. 等待足够的区块确认数
4. 根据金额匹配待支付订单
5. 更新订单状态并触发回调

**金额匹配机制**:
- 创建订单时生成唯一金额 (基础金额 + 随机小数位)
- 支付时精确匹配6位小数
- 支持0.01%误差的模糊匹配

### 2. 汇率服务 (rate.go)

管理多币种汇率转换，支持 **USD 统一结算**。

**核心概念**:
- 所有订单统一按 **USD** 结算
- 支持多币种商品标价 (USD, EUR, CNY)
- 支持多币种支付 (USDT, TRX, CNY)
- 买入/卖出浮动实现汇差利润

**汇率模式**:
- `auto`: 自动从交易所获取实时汇率
- `manual`: 使用手动设置的固定汇率
- `hybrid`: 混合模式，自动优先，失败回退手动

**支持的汇率对**:
- EUR → USD (欧元转美元)
- CNY → USD (人民币转美元)
- USD → CNY (美元转人民币)
- USD → TRX (美元转波场)
- USD → USDT (固定 1:1)

**汇率数据源**:
- Binance API: EUR/USD (EURUSDT), TRX/USDT (TRXUSDT)
- exchangerate-api.com: USD/CNY 转换
- OKX API (备用数据源)

**买卖浮动机制**:
- **买入浮动** (rate_buy_float): 用户支付时汇率上浮 (如 +2%)
- **卖出浮动** (rate_sell_float): 商户提现时汇率下浮 (如 -2%)
- **利润空间**: 买入浮动 + 卖出浮动 (如 2% + 2% = 4%)

**转换流程**:
1. 商品价格 (EUR/CNY) → **买入汇率** → USD 结算金额
2. USD 结算金额 → **买入浮动** → 实际支付金额 (USDT/TRX)
3. USD 余额 → **卖出浮动** → 提现打款金额 (USDT/TRX)

**缓存机制**:
- 汇率缓存 5 分钟 (可配置)
- 请求失败时使用上次有效汇率
- 自动更新间隔可配置 (默认 60 分钟)

### 3. 订单服务 (order.go)

管理订单生命周期。

**订单状态**:
- 0: 待支付 (pending)
- 1: 已支付 (paid)
- 2: 已过期 (expired)
- 3: 已取消 (cancelled)

**功能**:
- 创建订单 (CNY转USDT、生成唯一金额、分配收款地址)
- 查询订单
- 订单过期处理 (后台定时任务)
- 手动标记已支付 (管理员功能)

### 4. 回调通知服务 (notify.go)

处理支付成功后的商户回调。

**流程**:
1. 支付成功后触发回调
2. 构建通知参数 (含签名)
3. 发送 GET 请求到商户 notify_url
4. 检查响应是否为 "success"
5. 失败时按指数退避重试 (最多5次)

### 5. 机器人通知服务 (bot.go, telegram.go)

推送订单通知到 Telegram 和 Discord。

**通知类型**:
- 新订单创建
- 订单支付成功
- 订单过期
- 大额支付警报
- 每日报告 (每天早上9点)

**Telegram 配置** (在管理后台系统设置中):
- `telegram_enabled`: 是否启用 (true/false)
- `telegram_bot_token`: Telegram Bot Token

商户通过与 Bot 对话绑定账号，接收个人通知。

**Discord 配置** (在管理后台系统设置中):
- `discord_webhook`: Discord Webhook URL

## 数据库设计

### merchants (商户表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| pid | VARCHAR(32) | 商户ID (唯一) |
| name | VARCHAR(100) | 商户名称 |
| key | VARCHAR(64) | 商户密钥 |
| notify_url | VARCHAR(500) | 默认异步通知地址 |
| return_url | VARCHAR(500) | 默认同步返回地址 |
| status | TINYINT | 状态: 1正常 0禁用 |
| balance | DECIMAL(18,2) | 余额 (USD) |
| frozen_balance | DECIMAL(18,2) | 冻结余额 (USD) |
| telegram_notify | BOOLEAN | 是否启用Telegram通知 |
| telegram_chat_id | BIGINT | Telegram Chat ID |
| telegram_status | VARCHAR(20) | Telegram状态: normal/blocked/unbound |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

### orders (订单表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键 |
| trade_no | VARCHAR(64) | 平台订单号 (唯一) |
| out_trade_no | VARCHAR(64) | 商户订单号 |
| merchant_id | INT | 商户ID |
| type | VARCHAR(20) | 支付类型 |
| name | VARCHAR(200) | 商品名称 |
| currency | VARCHAR(10) | 原始货币类型 (USD/EUR/CNY) |
| money | DECIMAL(10,2) | 原始金额 |
| settlement_amount | DECIMAL(18,6) | 结算金额 (USD) |
| pay_currency | VARCHAR(10) | 实际支付货币 (USDT/TRX/CNY) |
| pay_amount | DECIMAL(18,6) | 需支付金额 |
| actual_amount | DECIMAL(18,6) | 实际收到金额 |
| rate | DECIMAL(10,8) | 使用的汇率 |
| chain | VARCHAR(20) | 链类型 |
| to_address | VARCHAR(100) | 收款地址 |
| from_address | VARCHAR(100) | 付款地址 |
| tx_hash | VARCHAR(100) | 交易哈希 (唯一索引) |
| status | TINYINT | 订单状态 |
| notify_url | VARCHAR(500) | 异步通知地址 |
| return_url | VARCHAR(500) | 同步返回地址 |
| notify_count | INT | 通知次数 |
| notify_status | TINYINT | 通知状态 |
| param | TEXT | 附加参数 |
| client_ip | VARCHAR(50) | 客户端IP |
| created_at | DATETIME | 创建时间 |
| paid_at | DATETIME | 支付时间 |
| expired_at | DATETIME | 过期时间 |

### wallets (钱包表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| chain | VARCHAR(20) | 链类型 |
| address | VARCHAR(100) | 钱包地址 |
| label | VARCHAR(50) | 标签 |
| status | TINYINT | 状态: 1启用 0禁用 |
| created_at | DATETIME | 创建时间 |

### system_configs (系统配置表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| key | VARCHAR(50) | 配置键 |
| value | TEXT | 配置值 |
| description | VARCHAR(200) | 描述 |
| updated_at | DATETIME | 更新时间 |

### transaction_logs (交易日志表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| chain | VARCHAR(20) | 链类型 |
| tx_hash | VARCHAR(100) | 交易哈希 (唯一索引) |
| from_address | VARCHAR(100) | 发送地址 |
| to_address | VARCHAR(100) | 接收地址 |
| amount | VARCHAR(50) | 金额 |
| block_number | BIGINT | 区块号 |
| matched | BOOL | 是否已匹配订单 |
| order_id | INT | 关联订单ID |
| created_at | DATETIME | 创建时间 |

### exchange_rates (汇率配置表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| from_currency | VARCHAR(10) | 源货币 (EUR/CNY/USD) |
| to_currency | VARCHAR(10) | 目标货币 (USD/CNY/TRX/USDT) |
| rate | DECIMAL(18,8) | 基础汇率 |
| mode | VARCHAR(10) | 模式: auto/manual/hybrid |
| enabled | BOOLEAN | 是否启用 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

### exchange_rate_history (汇率历史表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键 |
| from_currency | VARCHAR(10) | 源货币 |
| to_currency | VARCHAR(10) | 目标货币 |
| rate | DECIMAL(18,8) | 汇率 |
| source | VARCHAR(20) | 数据源 (binance/okx/manual) |
| created_at | DATETIME | 创建时间 |

### withdrawals (提现表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| merchant_id | INT | 商户ID |
| amount | DECIMAL(18,2) | 提现金额 (USD) |
| fee | DECIMAL(18,2) | 手续费 (USD) |
| real_amount | DECIMAL(18,2) | 实际金额 (USD) |
| payout_amount | DECIMAL(18,6) | 打款金额 (USDT/TRX) |
| payout_currency | VARCHAR(10) | 打款货币 |
| payout_rate | DECIMAL(18,8) | 打款汇率 (卖出汇率) |
| pay_method | VARCHAR(20) | 提现方式 |
| account | VARCHAR(100) | 收款账号 |
| status | TINYINT | 状态: 0待审核 1通过 2拒绝 3已打款 |
| created_at | DATETIME | 创建时间 |
| processed_at | DATETIME | 处理时间 |

### withdraw_addresses (提现地址表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| merchant_id | INT | 商户ID |
| chain | VARCHAR(20) | 链类型 |
| address | VARCHAR(100) | 提现地址 |
| label | VARCHAR(50) | 标签 |
| status | TINYINT | 状态: 0待审核 1通过 2拒绝 |
| created_at | DATETIME | 创建时间 |

### ip_blacklist (IP黑名单表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| ip | VARCHAR(50) | IP地址 |
| reason | VARCHAR(200) | 封禁原因 |
| created_at | DATETIME | 创建时间 |

### api_logs (API日志表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键 |
| merchant_id | INT | 商户ID |
| ip | VARCHAR(50) | 请求IP |
| method | VARCHAR(10) | 请求方法 |
| path | VARCHAR(200) | 请求路径 |
| params | TEXT | 请求参数 |
| created_at | DATETIME | 创建时间 |

## 安全设计

### 签名验证
- 彩虹易支付: 参数排序 + RFC 3986 URL编码 + 拼接 + MD5
- V免签: 固定顺序拼接 + MD5
- 所有API请求支持签名校验

### JWT认证
- 管理后台和商户后台API使用JWT Token认证
- Token有效期24小时
- 密钥在 `config.yaml` 的 `jwt.secret` 配置

### 交易安全
#### 1. 交易防重
```go
// 检查交易哈希是否已存在
var existingLog model.TransactionLog
if err := model.GetDB().Where("tx_hash = ?", transfer.TxHash).First(&existingLog).Error; err == nil {
    return // 跳过已处理的交易
}
```

**机制**: 通过 tx_hash 唯一索引防止同一笔交易重复处理

#### 2. 并发保护 (乐观锁)
```go
// 仅更新待支付订单
result := model.GetDB().Model(&order).
    Where("status = ?", model.OrderStatusPending).
    Updates(updates)

// 检查是否实际更新
if result.RowsAffected == 0 {
    log.Printf("Order already processed by another process")
    return
}
```

**机制**: 使用 WHERE status=pending 条件 + RowsAffected 检查，防止订单重复入账

#### 3. 金额精确匹配
- 使用 unique_amount 生成唯一金额 (6位小数)
- 区块链监听时精确匹配金额
- 支持 0.01% 误差的模糊匹配

### 访问控制
#### IP黑名单
- 自动封禁异常IP
- 缓存机制提升性能 (TTL 30秒)
- 手动添加/移除后立即刷新缓存
- 商户可配置IP白名单

#### Referer白名单
- 限制来源域名
- 防止跨域恶意调用

### 通知安全
#### 异步回调
- 带签名的POST通知
- 失败自动重试 (最多5次)
- 指数退避策略

#### Telegram账号管理
- 自动检测账号封禁状态
- 封禁后停止推送
- 状态管理：normal/blocked/unbound

### 地址验证
- TRC20地址以 T 开头，包含 checksum 验证
- ERC20/BEP20/Polygon地址以 0x 开头
- 完整的 hex ↔ base58 转换（Tron）

## 部署建议

### 单机部署
```bash
# 编译
go build -o ezpay main.go

# 运行
./ezpay
```

### Docker部署
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o ezpay .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/ezpay .
RUN mkdir -p /var/lib/ezpay
EXPOSE 6088
CMD ["./ezpay"]
```

> 生产模式静态资源已嵌入二进制，无需复制 web/ 目录

### 反向代理 (Nginx)
```nginx
server {
    listen 80;
    server_name pay.example.com;

    location / {
        proxy_pass http://127.0.0.1:6088;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## 监控与运维

### 健康检查
```
GET /health
```

### 日志
- 标准输出日志
- 建议使用 systemd 管理进程
- 建议配置日志轮转

### 区块链节点
- TRC20: 默认使用 TronGrid 公共API
- ERC20: 需要自行配置 Infura 或其他节点
- BEP20: 默认使用 Binance 公共节点
- Polygon: 默认使用 Polygon 公共节点
- Optimism: 默认使用 Optimism 公共节点
- Arbitrum: 默认使用 Arbitrum 公共节点
- Avalanche: 默认使用 Avalanche 公共节点
- Base: 默认使用 Base 公共节点

## 区块链监控架构（最新改进）

### 核心组件

#### 1. RPCClient - 高可用 RPC 客户端
```go
type RPCClient struct {
    endpoints       []string          // 多个 RPC 端点
    currentIndex    int               // 当前使用的端点
    maxRetries      int               // 最大重试次数（3次）
    retryDelay      time.Duration     // 基础延迟（1秒）
    failureCount    map[string]int    // 失败计数
    healthCheckInt  time.Duration     // 健康检查间隔
}
```

**功能特性:**
- ✅ **指数退避重试**: 1s → 2s → 4s
- ✅ **故障自动切换**: RPC 节点故障时自动切换到备用节点
- ✅ **健康检查**: 定期检查失败节点，自动恢复使用
- ✅ **批量请求**: 支持 JSON-RPC 批量调用优化性能

#### 2. BlockchainMetrics - 监控指标系统
```go
type BlockchainMetrics struct {
    // 扫描统计
    ScanCount       map[string]int64      // 扫描次数
    ScanSuccess     map[string]int64      // 成功次数
    ScanFailure     map[string]int64      // 失败次数
    ScanDuration    map[string]time.Duration // 扫描耗时
    
    // 交易统计
    TransferFound   map[string]int64      // 发现的转账
    OrderMatched    map[string]int64      // 匹配的订单
    DuplicateTx     map[string]int64      // 重复交易
    
    // 错误统计
    ErrorCount      map[string]int64      // 错误计数
    LastError       map[string]string     // 最后错误
    
    // RPC 统计
    RPCCallCount    map[string]int64      // RPC 调用
    RPCFailCount    map[string]int64      // RPC 失败
    RPCRetryCount   map[string]int64      // 重试次数
    
    // 区块统计
    CurrentBlock    map[string]uint64     // 当前区块
    LastBlock       map[string]uint64     // 最后扫描区块
    BlocksBehind    map[string]uint64     // 落后区块数
}
```

**自动告警条件:**
- 扫描失败率 > 50%
- RPC 失败率 > 30%
- 区块落后 > 100
- 超过 5 分钟未扫描

#### 3. ChainListener - 增强链监听器
```go
type ChainListener struct {
    // 基础配置
    chain           string
    rpc             string
    rpcBackups      []string           // 备用 RPC 节点
    contractAddress string
    confirmations   int
    
    // 动态扫描
    scanInterval    int                // 当前扫描间隔
    baseScanInterval int               // 基础间隔
    
    // 重组检测
    reorgDepth      int                // 检测深度
    blockHistory    []uint64           // 区块历史
}
```

**动态扫描间隔:**
- 发现交易时: 间隔减半（最小 5 秒）
- 无交易时: 恢复基础间隔

### 改进功能详解

#### 1. Tron 地址转换
实现了完整的 hex ↔ base58 转换：
```go
// 支持的格式转换
"41..." (hex) → "T..." (base58)
"T..." (base58) → "41..." (hex)
```

包含 checksum 验证，防止地址错误。

#### 2. RPC 重试机制
```go
retry 0: 立即执行
retry 1: 延迟 1s (2^0)
retry 2: 延迟 2s (2^1)
retry 3: 延迟 4s (2^2)
失败后: 尝试切换 RPC 节点
```

#### 3. 故障转移流程
```
1. RPC 调用失败
2. 记录失败次数
3. 连续失败 3 次标记节点不健康
4. 自动切换到下一个节点
5. 1 分钟后自动恢复尝试失败节点
```

#### 4. 区块重组检测
```go
// EVM 链重组处理
if currentBlock <= lastBlock {
    // 检测到重组
    // 回退 reorgDepth 个区块重新扫描
    listener.lastBlock = currentBlock - reorgDepth
}
```

#### 5. 批量地址查询优化
```go
// 传统方式: 逐个地址查询
for addr := range addresses {
    resp := rpc.Get(addr)  // N 次 RPC 调用
}

// 优化方式: 批量请求
batchRequests := []BatchRequest{}
for addr := range addresses {
    batchRequests = append(batchRequests, ...)
}
responses := rpc.BatchPostJSON(batchRequests) // 1 次 RPC 调用
```

**性能提升**: 100 个地址从 100 次请求减少到 1 次。

#### 6. Gas 价格监控
```go
// 每 5 分钟更新一次各链 Gas 价格
func (s *BlockchainService) monitorGasPrices() {
    ticker := time.NewTicker(5 * time.Minute)
    for {
        s.updateGasPrices() // ERC20, BEP20, Polygon, etc.
    }
}
```

返回单位: Gwei

### API 接口

#### 获取监控指标
```http
GET /admin/api/blockchain/metrics
GET /admin/api/blockchain/metrics/:chain
```

#### 获取 Gas 价格
```http
GET /admin/api/blockchain/gas-price/:chain
```

返回示例:
```json
{
  "chain": "erc20",
  "gas_price": 25.5,  // Gwei
  "updated_at": "2024-01-30T12:00:00Z"
}
```

### 监控面板数据

```json
{
  "scan_count": {"trx": 1000, "erc20": 950},
  "scan_success": {"trx": 995, "erc20": 940},
  "success_rate": {"trx": 99.5, "erc20": 98.9},
  "transfer_found": {"trx": 120, "erc20": 85},
  "order_matched": {"trx": 118, "erc20": 84},
  "duplicate_tx": {"trx": 2, "erc20": 1},
  "rpc_call_count": {"trx": 2000, "erc20": 1900},
  "rpc_fail_count": {"trx": 5, "erc20": 10},
  "blocks_behind": {"trx": 0, "erc20": 3}
}
```

### 性能指标

| 指标 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| RPC 成功率 | ~85% | ~99% | +14% |
| 地址查询耗时 (100地址) | ~15s | ~1s | 15x |
| 区块落后数 | 不确定 | 实时监控 | ✓ |
| 重组处理 | 无 | 自动 | ✓ |
| 扫描间隔 | 固定 | 动态 | 2x |

### 日志增强

```log
# 原日志
Error scanning trx: network error

# 新日志
[trx] Scan error: network timeout after 15s (attempt 3/3)
[trx] Failed to get transactions for TBxxx: HTTP 503
[trx] RPC POST failed (attempt 2/4): context deadline exceeded, retrying in 2s
⚠️  ALERT: Chain trx scan failure rate: 55.2%
[trx] Potential reorg detected: current=12345, last=12346
[trx] Gas price updated: 25.50 Gwei
[trx] Found 5 transfers
[trx] Order ORDER_123 matched with tx ABC123, amount: 100.5 USDT
```

### 配置示例

```yaml
blockchain:
  trx:
    enabled: true
    rpc: https://api.trongrid.io
    # 备用 RPC 节点（未来支持）
    rpc_backups:
      - https://api.tronstack.io
      - https://api.shasta.trongrid.io
    confirmations: 19
    scan_interval: 15
    
  erc20:
    enabled: true
    rpc: https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY
    rpc_backups:
      - https://mainnet.infura.io/v3/YOUR_KEY
    confirmations: 12
    scan_interval: 15
```

### 故障排查

#### 1. 查看监控指标
```bash
curl http://localhost:6088/admin/api/blockchain/metrics/trx
```

#### 2. 检查日志
```bash
tail -f /var/log/ezpay/ezpay.log | grep "\[trx\]"
```

#### 3. 常见问题

**Q: RPC 调用失败率高？**
A: 检查网络连接，考虑添加备用 RPC 节点

**Q: 区块落后很多？**
A: 降低 scan_interval 或检查服务器性能

**Q: 交易遗漏？**
A: 检查地址格式、确认数设置、查看 duplicate_tx

**Q: 重组频繁？**
A: 增加 confirmations，使用主网而非测试网

