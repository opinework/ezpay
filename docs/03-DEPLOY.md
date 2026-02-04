# EzPay 部署指南

## 环境要求

- Go 1.21+ (仅编译时需要)
- MySQL 8.0+
- (可选) Nginx 用于反向代理

## 安装步骤

### 1. 创建数据库

```sql
CREATE DATABASE ezpay CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'ezpay'@'localhost' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON ezpay.* TO 'ezpay'@'localhost';
FLUSH PRIVILEGES;
```

### 2. 初始化数据库结构

EzPay 使用版本化数据库迁移系统管理数据库结构。

#### 首次部署

```bash
# 执行数据库迁移（自动创建所有表）
./db/migrate.sh migrate
```

迁移工具会：
- 自动创建版本管理表
- 应用 V001 初始化数据库结构
- 自动备份（可回滚）
- 记录迁移历史

#### 升级现有数据库

```bash
# 查看待执行的迁移
./db/migrate.sh status

# 执行迁移更新
./db/migrate.sh migrate
```

#### 查看迁移历史

```bash
./db/migrate.sh history
```

**注意**：
- 数据库结构由迁移系统管理，**不要**手动修改表结构
- 新增字段请创建新的迁移文件（参考 `db/migrations/README.md`）
- 生产环境迁移前请先在测试环境验证

### 3. 获取可执行文件

#### 方式一：下载预编译版本

从 Release 页面下载对应平台的版本：
- `ezpay-linux-amd64` - Linux x64 服务器
- `ezpay-linux-arm64` - Linux ARM64 (树莓派等)
- `ezpay-windows-amd64.exe` - Windows
- `ezpay-darwin-amd64` - macOS Intel
- `ezpay-darwin-arm64` - macOS Apple Silicon

#### 方式二：源码编译

```bash
cd /path/to/ezpay

# 安装依赖
go mod tidy

# 使用 build-all.sh (推荐)
./build-all.sh            # 编译所有平台
./build-all.sh linux      # 仅 Linux x64
./build-all.sh linux-arm64 # 仅 Linux ARM64
./build-all.sh windows    # 仅 Windows

# 或使用 Makefile
make release              # 编译当前平台
make release-all          # 编译所有平台
```

生产版本特点：
- 静态资源已嵌入二进制，单文件部署
- 无需 web/ 目录
- 约 18MB 大小

### 3. 配置

首次运行会自动生成 `config.yaml`，编辑配置文件：

```yaml
server:
  host: "0.0.0.0"
  port: 6088

database:
  host: "127.0.0.1"
  port: 3306
  user: "root"
  password: "your_password"
  dbname: "ezpay"
  max_open_conns: 100      # 最大连接数
  max_idle_conns: 10       # 最大空闲连接
  conn_max_lifetime: 60    # 连接生命周期(分钟)

jwt:
  secret: "change-this-to-random-string"  # 请修改!
  expire_hour: 24

storage:
  data_dir: "/var/lib/ezpay"  # Linux 默认

# 安全配置
security:
  rate_limit_api: 20         # API每秒请求限制
  rate_limit_api_burst: 50
  rate_limit_login: 2        # 登录限流
  rate_limit_login_burst: 5
  cors_allow_origins: []     # CORS白名单，留空允许所有
  ip_blacklist_cache_ttl: 30 # IP黑名单缓存(秒)
  http_timeout: 15           # HTTP超时(秒)

# 订单配置
order:
  expire_minutes: 30         # 订单过期时间
  cleanup_hours: 24          # 清理无效订单间隔
  wallet_cache_ttl: 30       # 钱包缓存(秒)

# 通知配置
notify:
  retry_count: 5             # 重试次数
  retry_interval: 60         # 重试间隔(秒)
  timeout: 10                # 通知超时(秒)

# 日志配置
log:
  level: "info"              # debug, info, warn, error
  db_log_level: "warn"
  api_log_days: 30           # API日志保留天数

# 汇率配置
rate:
  auto_update_enabled: true  # 启用自动更新
  update_interval: 60        # 更新间隔(分钟)
  source: "binance"          # 主数据源: binance/okx
  fallback_source: "okx"     # 备用数据源
  cny_api: "https://api.exchangerate-api.com/v4/latest/USD"  # CNY汇率API
  cache_seconds: 300         # 缓存时间
  buy_float: 0.02            # 买入浮动 2%
  sell_float: 0.02           # 卖出浮动 2%

# 区块链监控 (完整配置见 config.yaml)
blockchain:
  trc20:
    enabled: true
    rpc: "https://api.trongrid.io"
    contract_address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
    confirmations: 19
    scan_interval: 5
  bep20:
    enabled: true
    rpc: "https://bsc-dataseed.binance.org"
    contract_address: "0x55d398326f99059fF775485246999027B3197955"
    confirmations: 15
    scan_interval: 3
```

> 管理员账号和 Telegram 配置在管理后台"系统设置"中配置，不在配置文件中。

### 4. 启动服务

**重要**: 启动前请确保已执行数据库迁移（参考步骤2）

```bash
# 前台运行（测试）
./ezpay

# 或后台运行
nohup ./ezpay > ezpay.log 2>&1 &
```

**说明**：
- 服务**不会**自动创建数据库表
- 数据库结构由迁移系统管理（`./db/migrate.sh`）
- 首次部署必须先执行数据库迁移

### 5. 配置管理后台

访问 http://your-server:6088/admin

默认账号: admin / admin123

**首次使用请务必修改密码！**

### 6. 添加收款钱包

在管理后台 > 钱包管理 > 添加钱包

选择对应的链类型，填入您的 USDT 收款地址。

## 使用 Systemd 管理

创建服务文件 `/etc/systemd/system/ezpay.service`:

```ini
[Unit]
Description=EzPay USDT Payment Platform
After=network.target mysql.service

[Service]
Type=simple
User=www-data
WorkingDirectory=/path/to/ezpay
ExecStart=/path/to/ezpay/ezpay
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# 启动服务
sudo systemctl start ezpay

# 设置开机启动
sudo systemctl enable ezpay

# 查看状态
sudo systemctl status ezpay

# 查看日志
sudo journalctl -u ezpay -f
```

## Nginx 反向代理

```nginx
server {
    listen 80;
    server_name pay.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name pay.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:6088;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

## Docker 部署

项目已包含完整的 Docker 配置文件，可直接使用。

### 快速启动

```bash
# 1. 复制配置文件
cp config.docker.yaml config.yaml

# 2. 修改配置 (可选)
vim config.yaml

# 3. 启动服务
docker-compose up -d

# 4. 查看日志
docker-compose logs -f ezpay
```

访问 http://localhost:6088/admin，默认账号: admin / admin123

### 配置说明

项目包含以下 Docker 相关文件：

| 文件 | 说明 |
|------|------|
| `Dockerfile` | 多阶段构建镜像 |
| `docker-compose.yml` | 完整服务编排 (含 MySQL) |
| `config.docker.yaml` | Docker 环境配置模板 |
| `.dockerignore` | 构建排除文件 |

### docker-compose.yml

```yaml
version: '3.8'

services:
  ezpay:
    build: .
    image: ezpay:latest
    container_name: ezpay
    restart: unless-stopped
    ports:
      - "6088:6088"
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ezpay_data:/var/lib/ezpay
    environment:
      - TZ=Asia/Shanghai
    depends_on:
      mysql:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:6088/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  mysql:
    image: mysql:8.0
    container_name: ezpay-mysql
    restart: unless-stopped
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
      - ./db/init.sql:/docker-entrypoint-initdb.d/init.sql:ro
    environment:
      - MYSQL_ROOT_PASSWORD=root123456
      - MYSQL_DATABASE=ezpay
      - MYSQL_USER=ezpay
      - MYSQL_PASSWORD=ezpay123456
    command:
      - --character-set-server=utf8mb4
      - --collation-server=utf8mb4_unicode_ci
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  ezpay_data:
  mysql_data:
```

### 常用命令

```bash
# 启动服务
docker-compose up -d

# 停止服务
docker-compose down

# 查看状态
docker-compose ps

# 查看日志
docker-compose logs -f ezpay

# 重新构建
docker-compose build --no-cache

# 进入容器
docker exec -it ezpay sh

# 仅启动 EzPay (使用外部数据库)
docker-compose up -d ezpay
```

### 使用外部数据库

如果已有 MySQL 服务器，修改 `config.yaml`:

```yaml
database:
  host: "your-mysql-host"
  port: 3306
  user: "ezpay"
  password: "your-password"
  dbname: "ezpay"
```

然后仅启动 ezpay 服务：

```bash
docker-compose up -d ezpay
```

### 使用预编译镜像

如果已有编译好的二进制文件，可使用简化的 Dockerfile：

```dockerfile
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY ezpay-linux-amd64 ./ezpay
RUN chmod +x ./ezpay && mkdir -p /var/lib/ezpay/qrcode /var/lib/ezpay/apk

EXPOSE 6088
HEALTHCHECK --interval=30s --timeout=10s CMD wget --spider http://localhost:6088/health || exit 1
CMD ["./ezpay"]
```

### 生产环境建议

1. **修改默认密码**
   ```yaml
   # docker-compose.yml
   environment:
     - MYSQL_ROOT_PASSWORD=strong-random-password
     - MYSQL_PASSWORD=strong-random-password
   ```

2. **修改 JWT 密钥**
   ```yaml
   # config.yaml
   jwt:
     secret: "your-random-secret-key-here"
   ```

3. **使用 Nginx 反向代理** (见上方 Nginx 配置)

4. **定期备份数据**
   ```bash
   # 备份 MySQL
   docker exec ezpay-mysql mysqldump -uroot -pPASSWORD ezpay > backup.sql

   # 备份上传文件
   docker cp ezpay:/var/lib/ezpay ./ezpay_data_backup
   ```

## 机器人通知配置

### Telegram

1. 创建机器人: 与 @BotFather 对话，发送 `/newbot`
2. 获取 Bot Token
3. 在管理后台 > 系统设置 中配置:
   - `telegram_enabled`: true
   - `telegram_bot_token`: 机器人 Token
4. 商户在 Telegram 中与 Bot 对话绑定账号

详细配置说明请参考 [07-TELEGRAM.md](./07-TELEGRAM.md)

### Discord

1. 在 Discord 服务器设置中创建 Webhook
2. 复制 Webhook URL
3. 在管理后台 > 系统设置 添加:
   - `discord_webhook`: Webhook URL

## 常见问题

### 1. 区块链监控不工作

- 检查 RPC 节点是否可访问
- 检查配置中对应链的 `enabled` 是否为 `true`
- 查看日志是否有错误信息

### 2. 订单无法匹配

- 确保付款金额与订单金额完全一致 (6位小数)
- 确保付款地址正确
- 等待足够的区块确认数

### 3. 回调通知失败

- 确保商户服务器可访问
- 确保 notify_url 返回 `success` 字符串
- 检查签名验证逻辑

### 4. 汇率获取失败

- 检查网络是否可访问 Binance/OKX API
- 可切换为手动汇率模式

## 区块链监控优化配置

### RPC 节点配置（推荐）

为了提高稳定性，建议配置多个 RPC 节点：

#### 以太坊 (ERC20)
```yaml
blockchain:
  erc20:
    enabled: true
    rpc: https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY
    # 未来版本将支持多节点配置
    confirmations: 12
    scan_interval: 15
```

**推荐 RPC 提供商:**
- Alchemy: https://www.alchemy.com/
- Infura: https://www.infura.io/
- QuickNode: https://www.quicknode.com/

#### Tron (TRX/TRC20)
```yaml
blockchain:
  trx:
    enabled: true
    rpc: https://api.trongrid.io
    confirmations: 19
    scan_interval: 15

  trc20:
    enabled: true
    rpc: https://api.trongrid.io
    contract_address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
    confirmations: 19
    scan_interval: 15
```

**备用 TronGrid 节点:**
- https://api.tronstack.io
- https://api.shasta.trongrid.io (测试网)

### 监控指标接入

#### 1. 日志监控

```bash
# 实时监控扫描日志
tail -f /var/log/ezpay/ezpay.log | grep -E "\[(trx|erc20|bep20)\]"

# 监控告警
tail -f /var/log/ezpay/ezpay.log | grep "ALERT"

# 监控 RPC 重试
tail -f /var/log/ezpay/ezpay.log | grep "retrying"
```

#### 2. API 监控

创建监控脚本 `/usr/local/bin/ezpay-monitor.sh`:

```bash
#!/bin/bash

# 获取监控指标
METRICS=$(curl -s http://localhost:6088/admin/api/blockchain/metrics)

# 解析成功率
SUCCESS_RATE=$(echo $METRICS | jq -r '.success_rate.trx')

# 检查告警
if (( $(echo "$SUCCESS_RATE < 90" | bc -l) )); then
    echo "ALERT: TRX scan success rate: $SUCCESS_RATE%"
    # 发送告警通知
fi
```

添加到 crontab:
```bash
# 每 5 分钟检查一次
*/5 * * * * /usr/local/bin/ezpay-monitor.sh
```

#### 3. Prometheus 集成（未来）

监控端点（计划中）:
```
GET /metrics
```

### 性能调优

#### 1. 扫描间隔优化

根据交易量调整：

```yaml
# 高频交易（如交易所）
scan_interval: 5

# 中等频率（如商户）
scan_interval: 15

# 低频率（如个人）
scan_interval: 30
```

#### 2. 确认数调整

根据安全需求调整：

```yaml
# 高安全（大额交易）
confirmations: 20

# 平衡（推荐）
confirmations: 12

# 快速（小额交易）
confirmations: 3
```

**警告**: 确认数过低可能导致重组风险

#### 3. 数据库优化

```sql
-- 为交易日志添加索引
CREATE INDEX idx_tx_hash ON transaction_logs(tx_hash);
CREATE INDEX idx_chain_status ON transaction_logs(chain, matched);
CREATE INDEX idx_created_at ON transaction_logs(created_at);

-- 定期清理旧日志（可选）
DELETE FROM transaction_logs WHERE created_at < DATE_SUB(NOW(), INTERVAL 30 DAY);
```

### 故障排查

#### 问题 1: RPC 调用失败率高

**症状:**
```log
[trx] RPC POST failed (attempt 3/3): context deadline exceeded
ALERT: Chain trx RPC failure rate: 35.2%
```

**排查步骤:**
1. 检查网络连接: `ping api.trongrid.io`
2. 测试 RPC: `curl https://api.trongrid.io/wallet/getnowblock`
3. 检查防火墙规则
4. 考虑更换 RPC 提供商

**解决方案:**
- 配置备用 RPC 节点（未来版本）
- 增加超时时间（修改 httpClient.Timeout）
- 使用付费 RPC 服务

#### 问题 2: 区块落后严重

**症状:**
```log
ALERT: Chain erc20 is 150 blocks behind
```

**排查步骤:**
1. 检查服务器性能: `top`, `htop`
2. 检查数据库性能: `SHOW PROCESSLIST;`
3. 查看监控指标: 扫描耗时

**解决方案:**
- 减小 scan_interval
- 增加服务器资源
- 优化数据库索引

#### 问题 3: 交易遗漏

**症状:** 用户支付后订单长时间未更新

**排查步骤:**
1. 检查链是否启用: `/admin/api/chains`
2. 检查地址格式: 特别是 Tron 地址
3. 查看重复交易: `duplicate_tx` 指标
4. 检查确认数设置

**解决方案:**
- 核对收款地址格式（Tron 需要 T 开头）
- 降低确认数（谨慎操作）
- 检查钱包是否启用

#### 问题 4: 重组频繁

**症状:**
```log
[polygon] Potential reorg detected: current=45678, last=45679
```

**排查步骤:**
1. 确认使用的是主网还是测试网
2. 检查 RPC 节点是否稳定
3. 查看网络区块重组历史

**解决方案:**
- 增加 confirmations
- 使用更稳定的 RPC 节点
- 考虑切换到主网

### 监控仪表板

推荐使用 Grafana + Prometheus 构建监控仪表板（未来版本将内置支持）

**关键指标:**
- 扫描成功率（目标 > 95%）
- RPC 成功率（目标 > 98%）
- 平均扫描延迟（目标 < 2s）
- 区块落后数（目标 < 10）
- 订单匹配率（目标 > 99%）

### 升级指南

从旧版本升级到新版本：

```bash
# 1. 备份数据库
mysqldump -u root -p ezpay > ezpay_backup_$(date +%Y%m%d).sql

# 2. 停止服务
systemctl stop ezpay

# 3. 备份旧版本
cp /usr/bin/ezpay /usr/bin/ezpay.bak

# 4. 部署新版本
cp ezpay-improved /usr/bin/ezpay
chmod +x /usr/bin/ezpay

# 5. 启动服务
systemctl start ezpay

# 6. 查看日志确认
journalctl -u ezpay -f
```

查看新功能是否正常:
```bash
# 检查监控指标
curl http://localhost:6088/admin/api/blockchain/metrics

# 应该看到新的指标字段
```
