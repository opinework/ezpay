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
│   │   ├── blockchain.go     # 区块链监控
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

管理 CNY/USDT/TRX 汇率转换。

**USDT 汇率模式**:
- `auto`: 自动从交易所获取实时汇率
- `manual`: 使用手动设置的固定汇率
- `hybrid`: 优先自动获取，失败时回退到手动汇率

**TRX 汇率**:
- 强制使用自动模式，从 Binance API 实时获取 TRX/USDT 价格
- TRX/CNY = TRX/USDT × USDT/CNY

**汇率来源**:
- Binance API (主要)
- OKX API (备用)

**缓存机制**:
- 汇率缓存5分钟 (可配置)
- 请求失败时使用上次有效汇率

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
| balance | DECIMAL(18,2) | 余额 |
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
| money | DECIMAL(10,2) | CNY金额 |
| usdt_amount | DECIMAL(18,6) | USDT金额 |
| actual_amount | DECIMAL(18,6) | 实际收到USDT |
| rate | DECIMAL(10,4) | 汇率 |
| chain | VARCHAR(20) | 链类型 |
| to_address | VARCHAR(100) | 收款地址 |
| from_address | VARCHAR(100) | 付款地址 |
| tx_hash | VARCHAR(100) | 交易哈希 |
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
| tx_hash | VARCHAR(100) | 交易哈希 |
| from_address | VARCHAR(100) | 发送地址 |
| to_address | VARCHAR(100) | 接收地址 |
| amount | VARCHAR(50) | 金额 |
| block_number | BIGINT | 区块号 |
| matched | BOOL | 是否已匹配订单 |
| order_id | INT | 关联订单ID |
| created_at | DATETIME | 创建时间 |

## 安全设计

### 签名验证
- 彩虹易支付: 参数排序 + 拼接 + MD5
- V免签: 固定顺序拼接 + MD5

### JWT认证
- 管理后台和商户后台API使用JWT Token认证
- Token有效期24小时
- 密钥在 `config.yaml` 的 `jwt.secret` 配置

### 地址验证
- TRC20地址以 T 开头
- ERC20/BEP20/Polygon地址以 0x 开头

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
