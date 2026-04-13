# Telegram 配置与测试指南

本文档包含 Telegram 服务的完整配置说明和测试指南。

---

# 第一部分：配置说明

## 概述

EzPay 的 Telegram 服务支持以下功能：
1. **全局开关**：可以完全启用或禁用 Telegram 服务
2. **Webhook 模式**：支持轮询(polling)和 Webhook 两种接收模式
3. **自动 URL 生成**：Webhook URL 可以根据服务器配置自动生成

## 配置项说明

在系统配置表 `system_configs` 中新增以下配置项：

### 1. telegram_enabled
- **说明**：Telegram 服务总开关
- **可选值**：
  - `1`：启用 Telegram 服务
  - `0`：禁用 Telegram 服务（默认）
- **说明**：当设置为 `0` 时，即使配置了 Bot Token，服务也不会启动

### 2. telegram_mode
- **说明**：Telegram 消息接收模式
- **可选值**：
  - `polling`：轮询模式（默认）- 服务器主动每 2 秒询问 Telegram 是否有新消息
  - `webhook`：推送模式 - Telegram 有新消息时主动推送到服务器
- **默认值**：`polling`

### 3. telegram_webhook_url
- **说明**：Webhook 回调地址（仅在 webhook 模式下使用）
- **格式**：`https://yourdomain.com/telegram/webhook`
- **自动生成**：如果此配置为空且模式为 webhook，系统会根据服务器配置自动生成
  - 如果服务器地址是 localhost/127.0.0.1，使用 `http://`
  - 其他情况使用 `https://`
  - 如果端口不是 80/443，会包含端口号

### 4. telegram_bot_token
- **说明**：Telegram Bot Token（原有配置项）
- **格式**：从 BotFather 获取的 Token，格式类似 `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`

## 数据库迁移

执行以下 SQL 添加新配置项（已包含在 `db/migrations/V005__add_telegram_config.sql`）：

```sql
INSERT INTO `system_configs` (`key`, `value`, `description`) VALUES
('telegram_enabled', '0', 'Telegram服务总开关: 1启用 0禁用'),
('telegram_mode', 'polling', 'Telegram接收模式: polling轮询模式 webhook推送模式'),
('telegram_webhook_url', '', 'Telegram Webhook地址，例如: https://yourdomain.com/telegram/webhook')
ON DUPLICATE KEY UPDATE `description` = VALUES(`description`);
```

## 两种模式对比

### 轮询模式 (Polling)
**优点**：
- 配置简单，无需公网地址
- 适合开发环境和内网环境
- 不需要 HTTPS 证书

**缺点**：
- 有延迟（最长 2 秒）
- 持续占用服务器资源
- 网络带宽消耗较大

**适用场景**：
- 开发测试环境
- 没有公网域名的服务器
- 消息量不大的场景

### Webhook 模式
**优点**：
- 实时性好，无延迟
- 节省服务器资源
- 网络效率更高

**缺点**：
- 需要公网可访问的 HTTPS 地址
- 需要有效的 SSL 证书
- 配置相对复杂

**适用场景**：
- 生产环境
- 有公网域名和 SSL 证书
- 需要实时性的场景

## 使用方法

### 1. 在管理后台配置

访问管理后台 → 系统配置，添加或修改以下配置：

**启用 Telegram 服务（轮询模式）：**
```
telegram_enabled = 1
telegram_bot_token = 你的Bot Token
telegram_mode = polling
```

**启用 Telegram 服务（Webhook 模式）：**
```
telegram_enabled = 1
telegram_bot_token = 你的Bot Token
telegram_mode = webhook
telegram_webhook_url = (留空自动生成，或填写自定义地址)
```

### 2. 配置保存后自动生效

配置更新后，系统会自动：
1. 停止旧的服务（如果在运行）
2. 根据新配置重新启动服务
3. 如果是 webhook 模式，自动调用 Telegram API 设置 webhook
4. 如果是 polling 模式，自动删除可能存在的 webhook

### 3. 查看日志确认

启动日志示例：

**轮询模式：**
```
[Telegram] 服务启动 (轮询模式)
[Telegram] Webhook已删除
```

**Webhook 模式：**
```
[Telegram] 服务启动 (Webhook模式)
[Telegram] 自动生成 Webhook URL: https://yourdomain.com:8080/telegram/webhook
[Telegram] Webhook已设置: https://yourdomain.com:8080/telegram/webhook
```

## 技术实现说明

### 新增 API 端点

**Webhook 接收端点：**
```
POST /telegram/webhook
```

此端点接收来自 Telegram 服务器的消息推送，处理流程：
1. 检查服务是否启用，未启用直接返回 200
2. 验证请求头 `X-Telegram-Bot-Api-Secret-Token` 中的密钥，不匹配返回 403
3. 解析 JSON 请求体
4. 立即返回 `200 OK`（避免 Telegram 重试）
5. 异步处理消息内容

**Webhook 状态查询：**
```
GET /admin/api/telegram/webhook-info
```

返回当前 webhook 注册状态、待处理消息数、最后错误信息等，用于管理后台监控。

### Webhook 安全验证

系统启动时自动生成 32 字节的随机 `secret_token`，设置 webhook 时传给 Telegram。
之后 Telegram 每次推送请求都会在 `X-Telegram-Bot-Api-Secret-Token` 请求头中携带该密钥。
Webhook handler 会验证密钥是否匹配，**不匹配的请求直接返回 403 拒绝**，有效防止伪造请求。

密钥每次服务重启时自动重新生成，无需手动管理。

### 服务方法

新增以下服务方法：

```go
// 更新完整配置（包括模式和 webhook URL）
service.GetTelegramService().UpdateFullConfig(enabled, botToken, mode, webhookURL)

// 获取当前模式
service.GetTelegramService().GetMode()

// 获取 webhook URL
service.GetTelegramService().GetWebhookURL()

// 处理 webhook 请求
service.GetTelegramService().HandleWebhook(update)

// 验证 webhook 请求密钥
service.GetTelegramService().VerifyWebhookSecret(token)

// 查询 webhook 状态
service.GetTelegramService().GetWebhookInfo()
```

### 启动流程

1. 从数据库加载所有 Telegram 配置
2. 根据 `telegram_enabled` 决定是否启用服务
3. 如果启用：
   - Webhook 模式：调用 Telegram API 设置 webhook 地址
   - Polling 模式：删除可能存在的 webhook，启动轮询协程

### 模式切换

从轮询切换到 Webhook：
1. 停止轮询协程
2. 调用 `setWebhook` API 设置新地址
3. Telegram 开始推送消息到指定地址

从 Webhook 切换到轮询：
1. 调用 `deleteWebhook` API 删除 webhook
2. 启动轮询协程
3. 开始主动拉取消息

## 配置示例

### 示例 1：开发环境（轮询模式）
```
telegram_enabled = 1
telegram_bot_token = 123456789:ABCdefGHIjklMNOpqrsTUVwxyz
telegram_mode = polling
telegram_webhook_url = (留空)
```

### 示例 2：生产环境（Webhook 自动生成 URL）
```
telegram_enabled = 1
telegram_bot_token = 123456789:ABCdefGHIjklMNOpqrsTUVwxyz
telegram_mode = webhook
telegram_webhook_url = (留空，自动生成)
```

### 示例 3：生产环境（Webhook 自定义 URL）
```
telegram_enabled = 1
telegram_bot_token = 123456789:ABCdefGHIjklMNOpqrsTUVwxyz
telegram_mode = webhook
telegram_webhook_url = https://pay.example.com/telegram/webhook
```

### 示例 4：完全禁用 Telegram
```
telegram_enabled = 0
telegram_bot_token = (任意值或留空)
telegram_mode = (任意值)
telegram_webhook_url = (任意值)
```

## 安全机制

1. **Secret Token 验证**：Webhook 端点通过 `secret_token` 验证请求来源，密钥每次启动自动生成，伪造请求会被 403 拒绝
2. **启用状态检查**：服务禁用后 webhook 端点不处理任何请求
3. **Bot Token 保管**：Token 存储在数据库中，不要泄露到日志或前端
4. **HTTPS 必需**：Telegram 要求 Webhook 地址必须使用有效的 HTTPS 证书
5. **监控日志**：定期检查日志中的 `[Telegram Webhook] 验证失败` 记录，发现异常访问

## 常见问题

### Q1: Webhook 模式无法接收消息？

**检查清单：**
1. 确认服务器地址可以从公网访问
2. 确认使用的是 HTTPS（Telegram 要求）
3. 确认 SSL 证书有效（不能是自签名证书）
4. 查看日志是否显示 "Webhook已设置"
5. 使用 Telegram 的 `getWebhookInfo` API 检查状态

### Q2: 如何验证 Webhook 是否设置成功？

可以手动调用 Telegram API：
```bash
curl https://api.telegram.org/bot<YOUR_TOKEN>/getWebhookInfo
```

返回示例：
```json
{
  "ok": true,
  "result": {
    "url": "https://yourdomain.com/telegram/webhook",
    "has_custom_certificate": false,
    "pending_update_count": 0,
    "max_connections": 40
  }
}
```

### Q3: 本地开发环境可以使用 Webhook 模式吗？

不推荐，但可以通过以下方式实现：
1. 使用 ngrok 等工具创建公网隧道
2. 将 `telegram_webhook_url` 设置为 ngrok 提供的 HTTPS 地址
3. 开启 webhook 模式

**推荐做法**：开发环境使用轮询模式，生产环境使用 Webhook 模式

### Q4: 更改配置后多久生效？

立即生效。配置更新后会自动重启 Telegram 服务，通常在 1-2 秒内完成。

### Q5: 可以同时使用轮询和 Webhook 吗？

不可以。Telegram Bot 同一时间只能使用一种模式：
- 设置 webhook 后，`getUpdates` (轮询) 会返回错误
- 使用轮询模式时，需要先删除 webhook

---

# 第二部分：测试指南

## 测试前准备

### 1. 运行数据库迁移

确保执行了最新的迁移脚本：

```bash
# 方法1: 如果有自动迁移
./ezpay

# 方法2: 手动执行 SQL
mysql -u用户名 -p数据库名 < db/migrations/V005__add_telegram_config.sql
```

验证配置已添加：

```sql
SELECT * FROM system_configs WHERE `key` LIKE 'telegram_%';
```

预期结果：
```
telegram_enabled         | 0        | Telegram服务总开关: 1启用 0禁用
telegram_mode           | polling  | Telegram接收模式: polling轮询模式 webhook推送模式
telegram_webhook_url    |          | Telegram Webhook地址，例如: https://yourdomain.com/telegram/webhook
telegram_bot_token      | (原有)   | Bot Token
```

### 2. 重新编译项目

```bash
go build -o ezpay
```

### 3. 准备测试用的 Bot Token

从 Telegram BotFather 获取一个测试 Bot Token：
1. 在 Telegram 搜索 @BotFather
2. 发送 `/newbot` 创建新机器人
3. 按提示设置名称和用户名
4. 复制返回的 Token

## 测试场景

### 场景 1: 禁用状态测试

**目的**：验证全局开关功能

**步骤**：
1. 启动服务
2. 访问管理后台：`http://localhost:端口/admin`
3. 进入"系统配置"页面
4. 设置：
   - 启用 Telegram 服务：**禁用**
   - Bot Token：(填入有效 Token)
5. 点击"保存设置"
6. 查看控制台日志

**预期结果**：
- 日志中**不应该**出现 `[Telegram] 服务启动`
- Telegram 服务不会启动
- 商户无法收到通知

### 场景 2: 轮询模式测试

**目的**：验证轮询模式正常工作

**步骤**：
1. 在管理后台设置：
   - 启用 Telegram 服务：**启用**
   - 接收模式：**轮询模式 (Polling)**
   - Bot Token：(填入有效 Token)
2. 点击"保存设置"
3. 查看控制台日志
4. 在 Telegram 中搜索你的 Bot
5. 发送 `/start` 命令
6. 发送 `/help` 命令

**预期结果**：
- 日志显示：
  ```
  [Telegram] 服务启动 (轮询模式)
  [Telegram] Webhook已删除
  ```
- Bot 能够正常响应命令
- 延迟约 2 秒（轮询间隔）

**验证商户绑定**：
1. 在 Telegram 中发送：`/bind 商户号 密钥`
2. 应该收到绑定成功的消息
3. 发送 `/status` 查看商户状态

### 场景 3: Webhook 模式测试（本地开发）

**目的**：测试 Webhook URL 自动生成

**步骤**：
1. 确认 `config.yaml` 中的服务器配置：
   ```yaml
   server:
     host: localhost
     port: 8080
   ```
2. 在管理后台设置：
   - 启用 Telegram 服务：**启用**
   - 接收模式：**推送模式 (Webhook)**
   - Bot Token：(填入有效 Token)
   - Webhook URL：(留空)
3. 点击"保存设置"
4. 查看控制台日志

**预期结果**：
- 日志显示：
  ```
  [Telegram] 自动生成 Webhook URL: http://localhost:8080/telegram/webhook
  [Telegram] 服务启动 (Webhook模式)
  [Telegram] 设置Webhook失败: ...
  ```
- **注意**：本地环境 Telegram 无法访问，会失败（这是正常的）
- 可以验证 URL 格式正确

### 场景 4: Webhook 模式测试（生产环境）

**目的**：测试 Webhook 完整功能

**前提条件**：
- 有公网可访问的 HTTPS 域名
- SSL 证书有效

**步骤**：
1. 部署到生产服务器
2. 确认 `config.yaml` 配置：
   ```yaml
   server:
     host: yourdomain.com
     port: 443  # 或 80
   ```
3. 在管理后台设置：
   - 启用 Telegram 服务：**启用**
   - 接收模式：**推送模式 (Webhook)**
   - Bot Token：(填入有效 Token)
   - Webhook URL：(留空自动生成)
4. 点击"保存设置"
5. 查看日志

**预期结果**：
- 日志显示：
  ```
  [Telegram] 自动生成 Webhook URL: https://yourdomain.com/telegram/webhook
  [Telegram] 服务启动 (Webhook模式)
  [Telegram] Webhook已设置: https://yourdomain.com/telegram/webhook
  ```
- 在 Telegram 中发送消息，应该**立即**收到回复（无延迟）

**验证 Webhook 状态**：
```bash
curl https://api.telegram.org/bot<YOUR_TOKEN>/getWebhookInfo
```

返回示例：
```json
{
  "ok": true,
  "result": {
    "url": "https://yourdomain.com/telegram/webhook",
    "has_custom_certificate": false,
    "pending_update_count": 0,
    "max_connections": 40,
    "ip_address": "123.45.67.89"
  }
}
```

### 场景 5: 自定义 Webhook URL

**目的**：测试手动指定 Webhook URL

**步骤**：
1. 在管理后台设置：
   - 启用 Telegram 服务：**启用**
   - 接收模式：**推送模式 (Webhook)**
   - Bot Token：(填入有效 Token)
   - Webhook URL：`https://custom.domain.com/telegram/webhook`
2. 点击"保存设置"
3. 查看日志

**预期结果**：
- 日志显示使用的是自定义 URL：
  ```
  [Telegram] 服务启动 (Webhook模式)
  [Telegram] Webhook已设置: https://custom.domain.com/telegram/webhook
  ```

### 场景 6: 模式切换测试

**目的**：验证从轮询切换到 Webhook，反之亦然

**6.1 轮询 → Webhook**

1. 初始状态：轮询模式运行中
2. 修改配置为 Webhook 模式
3. 保存

**预期结果**：
- 日志显示：
  ```
  [Telegram] 服务停止
  [Telegram] 服务启动 (Webhook模式)
  [Telegram] Webhook已设置: ...
  ```

**6.2 Webhook → 轮询**

1. 初始状态：Webhook 模式运行中
2. 修改配置为轮询模式
3. 保存

**预期结果**：
- 日志显示：
  ```
  [Telegram] 服务停止
  [Telegram] 服务启动 (轮询模式)
  [Telegram] Webhook已删除
  ```

### 场景 7: 通知功能测试

**目的**：验证各类通知在不同模式下都能正常工作

**测试通知类型**：

1. **订单创建通知**
   - 创建一个测试订单
   - 已绑定的商户应收到通知

2. **订单支付成功通知**
   - 标记订单为已支付
   - 商户应收到支付成功通知

3. **余额变动通知**
   - 调整商户余额
   - 商户应收到余额变动通知

4. **登录通知**
   - 商户登录
   - 应收到登录成功通知

**在两种模式下分别测试**：
- 轮询模式：通知有约 2 秒延迟
- Webhook 模式：通知几乎实时送达

## 错误处理测试

### 错误 1: 无效的 Bot Token

**步骤**：
1. 设置一个无效的 Token
2. 启用服务

**预期结果**：
- 轮询模式：日志显示 API 请求失败
- Webhook 模式：日志显示设置 Webhook 失败

### 错误 2: Webhook URL 不可访问

**步骤**：
1. 设置一个无法从外网访问的 URL
2. 启用 Webhook 模式

**预期结果**：
- 设置可能成功（Telegram 不会立即验证）
- 但 Telegram 发送消息时会失败
- 查看 `getWebhookInfo` 会显示错误

### 错误 3: 切换模式时的竞态条件

**步骤**：
1. 快速切换模式多次：轮询 → Webhook → 轮询 → Webhook
2. 观察日志和服务状态

**预期结果**：
- 服务应正确停止和启动
- 不应出现 panic 或死锁
- 最终状态应与最后的配置一致

## 性能测试

### 测试 1: 轮询模式资源占用

**步骤**：
1. 启用轮询模式
2. 运行 1 小时
3. 观察 CPU 和内存使用

**预期结果**：
- CPU 占用率应该很低（< 1%）
- 内存占用稳定，无泄漏

### 测试 2: Webhook 模式并发处理

**步骤**：
1. 启用 Webhook 模式
2. 使用多个 Telegram 账号同时发送消息给 Bot
3. 观察响应时间和处理结果

**预期结果**：
- 所有消息都能正确处理
- 响应时间稳定
- 无消息丢失

## 日志检查清单

成功启动后，日志中应该包含：

### 轮询模式：
```
[Telegram] 服务启动 (轮询模式)
[Telegram] Webhook已删除
```

### Webhook 模式：
```
[Telegram] 自动生成 Webhook URL: https://... (如果留空)
[Telegram] 服务启动 (Webhook模式)
[Telegram] Webhook已设置: https://...
```

### 停止服务：
```
[Telegram] 服务停止
```

## 常见问题排查

### 问题 1: 轮询模式下 Bot 不响应

**检查项**：
1. Bot Token 是否正确
2. 网络是否能访问 api.telegram.org
3. 日志中是否有错误信息
4. 是否有防火墙阻止出站连接

### 问题 2: Webhook 模式设置失败

**检查项**：
1. 域名是否可以从公网访问
2. 是否使用 HTTPS（HTTP 不被 Telegram 支持）
3. SSL 证书是否有效
4. 端口是否正确（通常是 443 或 80）
5. Webhook URL 格式是否正确

### 问题 3: 切换模式后服务未重启

**检查项**：
1. 查看日志是否有 "服务停止" 和 "服务启动"
2. 检查配置是否正确保存到数据库
3. 尝试手动重启整个应用

### 问题 4: Webhook 收不到消息

**检查项**：
1. 调用 `getWebhookInfo` 查看状态
2. 检查 `last_error_date` 和 `last_error_message`
3. 确认路由 `/telegram/webhook` 已注册
4. 检查服务器防火墙规则

## 回归测试脚本

创建一个简单的测试脚本来验证基本功能：

```bash
#!/bin/bash

BASE_URL="http://localhost:8080"
TOKEN="你的Admin Token"

# 测试1: 禁用 Telegram
echo "测试1: 禁用 Telegram 服务"
curl -X POST "$BASE_URL/admin/api/configs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"telegram_enabled": "0"}'

sleep 2

# 测试2: 启用轮询模式
echo "测试2: 启用轮询模式"
curl -X POST "$BASE_URL/admin/api/configs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "telegram_enabled": "1",
    "telegram_mode": "polling",
    "telegram_bot_token": "你的Bot Token"
  }'

sleep 5

# 测试3: 切换到 Webhook 模式
echo "测试3: 切换到 Webhook 模式"
curl -X POST "$BASE_URL/admin/api/configs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "telegram_enabled": "1",
    "telegram_mode": "webhook",
    "telegram_webhook_url": ""
  }'

sleep 5

# 测试4: 切换回轮询模式
echo "测试4: 切换回轮询模式"
curl -X POST "$BASE_URL/admin/api/configs" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "telegram_enabled": "1",
    "telegram_mode": "polling"
  }'

echo "测试完成！请检查日志输出。"
```

## 验收标准

所有以下项目必须通过：

- [ ] 数据库迁移成功执行
- [ ] 项目编译无错误
- [ ] 管理后台配置界面正确显示所有选项
- [ ] 禁用状态：服务不启动
- [ ] 轮询模式：Bot 能正常响应命令
- [ ] Webhook URL 自动生成格式正确
- [ ] Webhook 模式：生产环境正常接收消息
- [ ] 模式切换：轮询 <-> Webhook 无错误
- [ ] 通知功能：两种模式下都能正常发送
- [ ] 错误处理：无效配置不会导致崩溃
- [ ] 日志输出：清晰准确
- [ ] 性能：无内存泄漏，CPU 占用正常

## 测试报告模板

```
测试日期：YYYY-MM-DD
测试人员：
环境：开发/测试/生产

| 测试场景 | 状态 | 备注 |
|---------|------|------|
| 禁用状态测试 | pass/fail | |
| 轮询模式测试 | pass/fail | |
| Webhook本地测试 | pass/fail | |
| Webhook生产测试 | pass/fail | |
| 自定义URL测试 | pass/fail | |
| 模式切换测试 | pass/fail | |
| 通知功能测试 | pass/fail | |
| 错误处理测试 | pass/fail | |
| 性能测试 | pass/fail | |

发现的问题：
1.
2.

结论：通过/不通过
```
