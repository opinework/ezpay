# EzPay 兼容性与迁移指南

本文档详细对比 EzPay 系统与彩虹易支付、V免签的接口差异，帮助开发者快速完成系统迁移或对接。

---

# 第一部分：彩虹易支付兼容

## 概述

| 对比项 | 彩虹易支付 | EzPay |
|--------|-----------|-------|
| 开发语言 | PHP | Go |
| 兼容性 | 原版 | 完全兼容 |
| 签名算法 | MD5 | MD5 (兼容) |
| 支付类型 | 微信/支付宝/QQ/银行卡 | USDT多链/TRX/微信/支付宝 |

---

## 1. 页面跳转支付 (submit.php)

### 接口地址

| 系统 | 地址 |
|------|------|
| 彩虹易支付 | `POST/GET /submit.php` |
| EzPay | `POST/GET /submit.php` 或 `/api/submit` |

### 请求参数对比

| 参数 | 彩虹易支付 | EzPay | 差异说明 |
|------|-----------|-------|----------|
| pid | 必填，商户ID | 必填，商户PID | **兼容** |
| type | 必填，alipay/wxpay/qqpay | 必填，usdt_trc20/alipay等 | **类型扩展**，见下方类型对照表 |
| out_trade_no | 必填，商户订单号 | 必填，商户订单号 | **兼容** |
| notify_url | 必填，异步通知地址 | 必填，异步通知地址 | **兼容** |
| return_url | 选填，同步返回地址 | 选填，同步返回地址 | **兼容** |
| name | 必填，商品名称 | 必填，商品名称 | **兼容** |
| money | 必填，金额(CNY) | 必填，金额(CNY) | **兼容** |
| sitename | 选填，网站名称 | 不支持 | **EzPay不支持** |
| sign | 必填，签名 | 必填，签名 | **兼容** |
| sign_type | 必填，MD5 | 选填，默认MD5 | **兼容** |
| param | 选填，附加参数 | 选填，附加参数 | **兼容** |

### 响应对比

两个系统均跳转到收银台页面，**完全兼容**。

---

## 2. API接口支付 (mapi.php)

### 接口地址

| 系统 | 地址 |
|------|------|
| 彩虹易支付 | `POST /mapi.php` |
| EzPay | `POST /mapi.php` 或 `/api/mapi` |

### 请求参数

与 submit.php 相同，**完全兼容**。

### 响应参数对比

| 参数 | 彩虹易支付 | EzPay | 差异说明 |
|------|-----------|-------|----------|
| code | 1成功/-1失败 | 1成功/-1失败 | **兼容** |
| msg | 消息 | 消息 | **兼容** |
| trade_no | 平台订单号 | 平台订单号 | **兼容** |
| out_trade_no | 商户订单号 | 商户订单号 | **兼容** |
| type | 支付方式 | 支付方式 | **兼容** |
| money | 金额 | 金额 | **兼容** |
| payurl | 支付链接 | - | **EzPay不返回** |
| qrcode | 二维码链接 | qrcode | **兼容** |
| urlscheme | URL Scheme | - | **EzPay不返回** |
| usdt_amount | - | USDT金额 | **EzPay新增** |
| rate | - | 汇率 | **EzPay新增** |
| address | - | 收款地址 | **EzPay新增** |
| chain | - | 链类型 | **EzPay新增** |
| expired_at | - | 过期时间 | **EzPay新增** |
| pay_url | - | 收银台地址 | **EzPay新增** |

**EzPay 响应示例**:
```json
{
    "code": 1,
    "msg": "success",
    "trade_no": "20231211123456789abc",
    "out_trade_no": "ORDER_123456",
    "type": "usdt_trc20",
    "money": "100.00",
    "usdt_amount": "13.888889",
    "rate": "7.2000",
    "address": "TXxxxxxxxxxxxxxxxxxxxxxxxxx",
    "chain": "trc20",
    "qrcode": "TXxxxxxxxxxxxxxxxxxxxxxxxxx",
    "expired_at": "2023-12-11 13:04:56",
    "pay_url": "/cashier/20231211123456789abc"
}
```

---

## 3. 查询订单接口 (api.php)

### 接口地址

| 系统 | 地址 |
|------|------|
| 彩虹易支付 | `GET /api.php?act=order` |
| EzPay | `GET /api.php?act=order` |

### 请求参数对比

| 参数 | 彩虹易支付 | EzPay | 差异说明 |
|------|-----------|-------|----------|
| act | 必填，order | 必填，order/query | **兼容** |
| pid | 必填，商户ID | 必填，商户PID | **兼容** |
| key | 必填，商户密钥 | 必填，商户密钥 | **兼容** |
| out_trade_no | 二选一，商户订单号 | 二选一，商户订单号 | **兼容** |
| trade_no | 二选一，平台订单号 | 二选一，平台订单号 | **兼容** |

### 响应参数对比

| 参数 | 彩虹易支付 | EzPay | 差异说明 |
|------|-----------|-------|----------|
| code | 状态码 | 状态码 | **兼容** |
| msg | 消息 | 消息 | **兼容** |
| pid | 商户ID | 商户PID | **兼容** |
| trade_no | 平台订单号 | 平台订单号 | **兼容** |
| out_trade_no | 商户订单号 | 商户订单号 | **兼容** |
| type | 支付方式 | 支付方式 | **兼容** |
| name | 商品名称 | 商品名称 | **兼容** |
| money | 金额 | 金额 | **兼容** |
| trade_status | 交易状态 | 交易状态 | **兼容** |
| addtime | 创建时间戳 | 创建时间戳 | **兼容** |
| endtime | 过期时间戳 | 过期时间戳 | **兼容** |
| usdt_amount | - | USDT金额 | **EzPay新增** |

### 彩虹易支付其他 API 操作 (EzPay不支持)

| act 参数 | 说明 | EzPay支持 |
|----------|------|-----------|
| apply | 申请商户 | 不支持 |
| query | 查询商户信息 | 不支持 |
| change | 修改结算账户 | 不支持 |
| settle | 查询结算记录 | 不支持 |
| orders | 批量查询订单 | 不支持 |

---

## 4. 异步通知 (notify_url)

### 通知方式

| 系统 | 方式 |
|------|------|
| 彩虹易支付 | GET 请求 |
| EzPay | GET 请求 |

### 通知参数对比

| 参数 | 彩虹易支付 | EzPay | 差异说明 |
|------|-----------|-------|----------|
| pid | 商户ID | 商户PID | **兼容** |
| trade_no | 平台订单号 | 平台订单号 | **兼容** |
| out_trade_no | 商户订单号 | 商户订单号 | **兼容** |
| type | 支付方式 | 支付方式 | **兼容** |
| name | 商品名称 | 商品名称 | **兼容** |
| money | 金额 | 金额 | **兼容** |
| trade_status | TRADE_SUCCESS | TRADE_SUCCESS | **兼容** |
| param | 附加参数 | 附加参数 | **兼容** |
| sign | 签名 | 签名 | **兼容** |
| sign_type | MD5 | MD5 | **兼容** |

### 响应要求

两个系统均要求返回字符串 `success`，**完全兼容**。

---

## 5. 签名算法对比

### 彩虹易支付签名算法

```
1. 将参数按键名 ASCII 码从小到大排序 (a-z)
2. sign、sign_type 和空值不参与签名
3. 拼接为 key1=value1&key2=value2 格式
4. 末尾追加商户密钥: key1=value1&key2=value2商户密钥
5. MD5 加密 (小写)
```

### EzPay 签名算法

**完全一致**，兼容彩虹易支付签名方式。

### PHP 签名示例 (两系统通用)

```php
<?php
function generateSign($params, $key) {
    // 过滤空值和签名参数
    $params = array_filter($params, function($v, $k) {
        return $v !== '' && !in_array($k, ['sign', 'sign_type']);
    }, ARRAY_FILTER_USE_BOTH);

    // 按键名排序
    ksort($params);

    // 拼接字符串
    $str = '';
    foreach ($params as $k => $v) {
        $str .= $k . '=' . $v . '&';
    }
    $str = rtrim($str, '&');

    // MD5签名
    return md5($str . $key);
}
```

---

## 6. 彩虹易支付支付类型对照表

### 彩虹易支付 type 参数

| type | 说明 |
|------|------|
| alipay | 支付宝 |
| wxpay | 微信支付 |
| qqpay | QQ钱包 |
| tenpay | 财付通 |
| bank | 银行卡 |
| jdpay | 京东支付 |

### EzPay type 参数

| type | 说明 |
|------|------|
| usdt_trc20 | USDT TRC20 (Tron网络) |
| usdt_erc20 | USDT ERC20 (以太坊网络) |
| usdt_bep20 | USDT BEP20 (BSC网络) |
| usdt_polygon | USDT Polygon网络 |
| usdt_optimism | USDT Optimism (Layer2) |
| usdt_arbitrum | USDT Arbitrum (Layer2) |
| usdt_avalanche | USDT Avalanche C-Chain |
| usdt_base | USDT Base (Coinbase Layer2) |
| trx | TRX (Tron原生币) |
| wechat | 微信支付 |
| alipay | 支付宝 |

### 类型映射建议

如果从彩虹易支付迁移到 EzPay，建议按以下方式映射：

| 彩虹易支付 type | 建议映射到 EzPay type |
|----------------|----------------------|
| alipay | alipay 或 usdt_trc20 |
| wxpay | wechat 或 usdt_trc20 |
| qqpay | usdt_trc20 |
| bank | usdt_trc20 |

---

## 7. 错误码对比

| 错误码 | 彩虹易支付 | EzPay |
|--------|-----------|-------|
| 1 | 成功 | 成功 |
| -1 | 失败 | 失败 |
| -2 | KEY校验失败 | - |
| -3 | PID不存在 | - |
| 0 | 未找到订单 | 未找到订单 |

---

## 8. 彩虹易支付功能差异总结

### EzPay 相比彩虹易支付的优势

1. **多链支持**: 支持 TRC20/ERC20/BEP20 等多条区块链
2. **加密货币**: 原生支持 USDT/TRX 收款
3. **自动汇率**: 支持自动获取实时汇率
4. **钱包轮询**: 支持多钱包自动轮询
5. **差异化费率**: 系统钱包和个人钱包可设置不同费率
6. **Telegram通知**: 内置 Telegram Bot 通知功能

### 彩虹易支付独有功能 (EzPay不支持)

1. **商户API**: apply/query/change/settle 等商户管理接口
2. **批量查询**: orders 批量订单查询
3. **sitename参数**: 网站名称参数
4. **payurl/urlscheme**: 原生支付跳转链接

---

## 9. 从彩虹易支付迁移到 EzPay

1. **接口地址**: 无需修改，EzPay 完全兼容 `/submit.php`、`/mapi.php`、`/api.php`
2. **签名算法**: 无需修改，完全兼容 MD5 签名
3. **type参数**: 需要修改为 EzPay 支持的类型 (如 `usdt_trc20`)
4. **响应处理**: 需要适配新增的 `usdt_amount`、`rate`、`address` 等字段
5. **回调验证**: 无需修改，签名验证完全兼容

### 代码修改示例

**修改前 (彩虹易支付)**:
```php
$params['type'] = 'alipay';
```

**修改后 (EzPay)**:
```php
$params['type'] = 'usdt_trc20';  // 或 'alipay'
```

---

# 第二部分：V免签兼容

## 概述

| 对比项 | V免签 | EzPay |
|--------|-------|-------|
| 开发语言 | PHP/Java | Go |
| 兼容性 | 原版 | 完全兼容 |
| 签名算法 | MD5 (拼接式) | MD5 (拼接式，兼容) |
| 收款方式 | 微信/支付宝 (监控APP) | USDT多链/TRX/微信/支付宝 |
| 工作原理 | APP监控收款通知 | 区块链监控/收款码 |

---

## 1. 创建订单接口 (createOrder)

### 接口地址

| 系统 | 地址 |
|------|------|
| V免签 | `GET /createOrder` |
| EzPay | `GET /createOrder` |

### 请求参数对比

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| payId | 必填，商户订单号 | 必填，商户订单号 | **兼容** |
| type | 必填，1:微信 2:支付宝 | 必填，支付类型 | **扩展**，见类型对照表 |
| price | 必填，金额(CNY) | 必填，金额(CNY) | **兼容** |
| sign | 必填，签名 | 必填，签名 | **兼容** |
| param | 选填，附加参数 | 选填，附加参数 | **兼容** |
| notifyUrl | 必填，回调地址 | 必填，回调地址 | **兼容** |
| returnUrl | 选填，返回地址 | 选填，返回地址 | **兼容** |
| isHtml | 选填，1:跳转页面 | 选填，1:跳转页面 | **兼容** |

### 响应参数对比

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| code | 1成功/-1失败 | 1成功/-1失败 | **兼容** |
| msg | 消息 | 消息 | **兼容** |
| payId | 商户订单号 | 商户订单号 | **兼容** |
| orderId | 平台订单号 | 平台订单号 | **兼容** |
| payType | 支付类型 | 支付类型 | **兼容** |
| price | 订单金额 | 订单金额 | **兼容** |
| reallyPrice | 实际支付金额 | USDT金额 | **兼容** (含义不同) |
| payUrl | 支付页面地址 | 收银台地址 | **兼容** |
| isAuto | 是否自动回调 | 固定为1 | **兼容** |
| state | 订单状态 | 订单状态 | **兼容** |
| timeOut | 过期时间 | 过期时间 | **兼容** |
| date | 创建时间 | 创建时间 | **兼容** |

**EzPay 响应示例**:
```json
{
    "code": 1,
    "msg": "success",
    "payId": "ORDER_123456",
    "orderId": "20231211123456789abc",
    "payType": "3",
    "price": "100.00",
    "reallyPrice": "13.888889",
    "payUrl": "/cashier/20231211123456789abc",
    "isAuto": 1,
    "state": 0,
    "timeOut": "2023-12-11 13:04:56",
    "date": "2023-12-11 12:34:56"
}
```

---

## 2. 查询订单状态 (getState / checkOrder)

### 接口地址

| 系统 | 地址 |
|------|------|
| V免签 | `GET /getState` 或 `GET /checkOrder` |
| EzPay | `GET /getState` 或 `GET /checkOrder` |

### 请求参数对比

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| payId | 必填，商户订单号 | 必填，商户订单号 | **兼容** |

### 响应参数对比

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| code | 状态码 | 状态码 | **兼容** |
| msg | 消息 | 消息 | **兼容** |
| state | 订单状态 | 订单状态 | **兼容** |

### state 状态值对照

| state | V免签 | EzPay |
|-------|-------|-------|
| 0 | 未支付 | 未支付 |
| 1 | 已支付 | 已支付 |
| 2 | 已过期 | 已过期/已取消 |

---

## 3. 关闭订单 (closeOrder)

### 接口地址

| 系统 | 地址 |
|------|------|
| V免签 | `GET /closeOrder` |
| EzPay | `GET /closeOrder` |

### 请求参数对比

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| payId | 必填，商户订单号 | 必填，商户订单号 | **兼容** |

### 响应参数

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| code | 状态码 | 状态码 | **兼容** |
| msg | 消息 | 消息 | **兼容** |

---

## 4. 心跳检测 (appHeart)

### 接口地址

| 系统 | 地址 |
|------|------|
| V免签 | `GET /appHeart` |
| EzPay | `GET /appHeart` |

### 请求参数对比

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| t | 必填，时间戳 | 必填，时间戳 | **兼容** |

### 时间戳验证

| 系统 | 有效期 |
|------|--------|
| V免签 | 未明确 |
| EzPay | 5分钟内有效 |

### 响应参数

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| code | 状态码 | 状态码 | **兼容** |
| msg | 消息 | 消息 | **兼容** |

---

## 5. 收款推送 (appPush)

### 接口地址

| 系统 | 地址 |
|------|------|
| V免签 | `GET /appPush` |
| EzPay | `GET /appPush` |

### 请求参数对比

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| t | 必填，时间戳 | 必填，时间戳 | **兼容** |
| type | 必填，1:微信 2:支付宝 | 必填，支付类型 | **兼容** |
| price | 必填，金额 | 必填，金额 | **兼容** |
| sign | 必填，签名 | 必填，签名 | **兼容** |

### 响应参数

| 参数 | V免签 | EzPay | 差异说明 |
|------|-------|-------|----------|
| code | 1成功/0未匹配/-1失败 | 1成功/0未匹配/-1失败 | **兼容** |
| msg | 消息 | 消息 | **兼容** |

### 订单匹配机制

| 系统 | 匹配方式 |
|------|----------|
| V免签 | 按金额精确匹配 |
| EzPay | 按金额精确匹配 + 模糊匹配(0.01%容差) |

---

## 6. V免签签名算法对比

### V免签 createOrder 签名算法

```
sign = MD5(payId + param + type + price + key)
```

### V免签 appPush 签名算法

```
sign = MD5(type + price + t + key)
```

### EzPay 签名算法

**完全一致**，兼容 V免签 签名方式。

### PHP 签名示例

**createOrder 签名**:
```php
<?php
function generateCreateOrderSign($payId, $param, $type, $price, $key) {
    return md5($payId . $param . $type . $price . $key);
}

// 示例
$sign = generateCreateOrderSign('ORDER_123', '', '1', '100.00', 'your_key');
```

**appPush 签名**:
```php
<?php
function generateAppPushSign($type, $price, $t, $key) {
    return md5($type . $price . $t . $key);
}

// 示例
$sign = generateAppPushSign('1', '100.00', time(), 'your_key');
```

---

## 7. V免签支付类型对照表

### V免签 type 参数

| type | 说明 |
|------|------|
| 1 | 微信支付 |
| 2 | 支付宝 |

### EzPay type 参数 (V免签模式)

| type | 说明 | 映射到 |
|------|------|--------|
| 1 | 微信支付 | trc20 |
| 2 | 支付宝 | bep20 |
| 3 | USDT TRC20 | trc20 |
| 4 | USDT ERC20 | erc20 |
| 5 | USDT BEP20 | bep20 |
| wechat | 微信支付 | trc20 |
| wxpay | 微信支付 | trc20 |
| alipay | 支付宝 | bep20 |
| usdt_trc20 | USDT TRC20 | trc20 |
| usdt_erc20 | USDT ERC20 | erc20 |
| usdt_bep20 | USDT BEP20 | bep20 |
| trc20 | TRC20 | trc20 |
| erc20 | ERC20 | erc20 |
| bep20 | BEP20 | bep20 |

### 类型映射说明

EzPay 在 V免签模式下，会自动将传统支付类型映射到 USDT 链：
- 微信 (1/wechat/wxpay) → TRC20
- 支付宝 (2/alipay) → BEP20

如需使用原生收款码，请使用彩虹易支付接口 (`/submit.php` 或 `/mapi.php`)。

---

## 8. 回调通知对比

### V免签回调机制

V免签通过 APP 监控手机收款通知，检测到收款后调用 `appPush` 接口推送金额，服务端根据金额匹配订单。

### EzPay 回调机制

EzPay 支持两种方式：
1. **区块链监控**: 监控链上交易自动确认
2. **appPush 兼容**: 兼容 V免签的 APP 推送方式

### 商户回调通知

订单支付成功后，两个系统都会向 `notifyUrl` 发送回调通知。

**V免签回调参数**:
```
payId={商户订单号}&param={附加参数}&type={支付类型}&price={金额}&reallyPrice={实际金额}&sign={签名}
```

**EzPay 回调参数** (彩虹易支付格式):
```
pid={商户ID}&trade_no={平台订单号}&out_trade_no={商户订单号}&type={支付类型}&name={商品名称}&money={金额}&trade_status=TRADE_SUCCESS&sign={签名}&sign_type=MD5
```

---

## 9. 架构差异

### V免签架构

```
客户端 → V免签服务端 → 生成二维码
                    ↓
用户扫码 → 付款到商家收款码
                    ↓
手机APP检测收款通知 → appPush推送
                    ↓
服务端匹配订单 → 触发回调
```

**特点**:
- 需要安装监控APP
- 依赖手机收款通知
- 实时性依赖APP状态
- 仅支持微信/支付宝

### EzPay 架构

```
客户端 → EzPay服务端 → 生成收款地址/二维码
                    ↓
用户扫码 → 付款到指定地址
                    ↓
区块链监控/链上确认 → 自动确认
                    ↓
订单状态更新 → 触发回调
```

**特点**:
- 无需监控APP
- 支持区块链自动确认
- 可选兼容 V免签 appPush
- 支持多种加密货币

---

## 10. V免签功能差异总结

### EzPay 相比 V免签的优势

| 功能 | V免签 | EzPay |
|------|-------|-------|
| 监控APP | 需要 | 不需要 |
| 支付通道 | 微信/支付宝 | 多链USDT/TRX/微信/支付宝 |
| 区块链支持 | 不支持 | 原生支持 |
| 自动确认 | 依赖APP | 区块链自动确认 |
| 商户管理 | 单一商户 | 多商户支持 |
| 钱包轮询 | 不支持 | 支持 |
| Telegram通知 | 不支持 | 支持 |
| 后台管理 | 简单 | 完整管理系统 |

### V免签独有功能

| 功能 | 说明 | EzPay 替代方案 |
|------|------|---------------|
| 监控APP | 监听手机收款 | 使用收款码上传功能 |
| 微信店员收款 | 店员账号监听 | 不支持 |

---

## 11. 从 V免签 迁移到 EzPay

### 1. 接口地址

无需修改，EzPay 完全兼容以下接口：
- `/createOrder`
- `/getState`
- `/checkOrder`
- `/closeOrder`
- `/appHeart`
- `/appPush`

### 2. 签名算法

无需修改，完全兼容 V免签 签名方式。

### 3. type 参数

可继续使用 `1`(微信) 和 `2`(支付宝)，EzPay 会自动映射。

如需使用 USDT，可改为：
- `3` 或 `usdt_trc20` - TRC20
- `4` 或 `usdt_erc20` - ERC20
- `5` 或 `usdt_bep20` - BEP20

### 4. 商户配置

V免签是单商户模式，EzPay 默认使用第一个启用的商户。

如需指定商户，建议改用彩虹易支付接口 (`/mapi.php`)。

### 5. 监控APP

如果不再使用监控APP，可以：
- 直接使用 USDT 收款，无需 APP
- 上传微信/支付宝收款码到 EzPay

### 代码修改示例

**修改前 (V免签)**:
```php
<?php
$params = [
    'payId' => 'ORDER_' . time(),
    'type' => '1',  // 微信
    'price' => '100.00',
    'notifyUrl' => 'https://yoursite.com/notify.php',
];
$params['sign'] = md5($params['payId'] . '' . $params['type'] . $params['price'] . $key);
```

**修改后 (EzPay，使用USDT)**:
```php
<?php
$params = [
    'payId' => 'ORDER_' . time(),
    'type' => '3',  // USDT TRC20
    'price' => '100.00',
    'notifyUrl' => 'https://yoursite.com/notify.php',
];
$params['sign'] = md5($params['payId'] . '' . $params['type'] . $params['price'] . $key);
```

---

## 常见问题

### Q1: EzPay 是否需要安装监控APP？

不需要。EzPay 支持：
- USDT/TRX 通过区块链自动确认
- 微信/支付宝可上传收款码，用户扫码后手动确认

### Q2: V免签的 type=1/2 在 EzPay 中如何处理？

EzPay 会自动映射：
- type=1 (微信) → TRC20 USDT
- type=2 (支付宝) → BEP20 USDT

如需使用原生收款码，请切换到彩虹易支付接口。

### Q3: appPush 接口在 EzPay 中还能用吗？

可以。如果你仍使用监控APP，可以继续使用 appPush 接口推送收款信息。

### Q4: 为什么 reallyPrice 和 price 不一样？

在 EzPay 中：
- `price`: 人民币金额
- `reallyPrice`: 换算后的 USDT 金额

这与 V免签 的含义不同（V免签 中 reallyPrice 是实际收款金额）。

---

## 参考资料

- [彩虹易支付 GitHub](https://github.com/Blokura/Epay)
- [彩虹易支付官方文档](https://pay.ihuacn.com/doc.html)
- [V免签 GitHub (Java版)](https://github.com/szvone/Vmq)
- [V免签 GitHub (PHP版)](https://github.com/szvone/vmqphp)
- [V免签 监控APP](https://github.com/szvone/vmqApk)
- [EzPay API文档](./04-API.md)
