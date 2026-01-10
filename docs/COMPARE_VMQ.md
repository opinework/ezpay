# EzPay 与 V免签 接口对比文档

本文档详细对比 EzPay 系统与 V免签 (Vmq) 的接口差异，帮助开发者快速完成系统迁移或对接。

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

## 6. 签名算法对比

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

## 7. 支付类型对照表

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

## 10. 功能差异总结

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

## 11. 迁移指南

### 从 V免签 迁移到 EzPay

#### 1. 接口地址

无需修改，EzPay 完全兼容以下接口：
- `/createOrder`
- `/getState`
- `/checkOrder`
- `/closeOrder`
- `/appHeart`
- `/appPush`

#### 2. 签名算法

无需修改，完全兼容 V免签 签名方式。

#### 3. type 参数

可继续使用 `1`(微信) 和 `2`(支付宝)，EzPay 会自动映射。

如需使用 USDT，可改为：
- `3` 或 `usdt_trc20` - TRC20
- `4` 或 `usdt_erc20` - ERC20
- `5` 或 `usdt_bep20` - BEP20

#### 4. 商户配置

V免签是单商户模式，EzPay 默认使用第一个启用的商户。

如需指定商户，建议改用彩虹易支付接口 (`/mapi.php`)。

#### 5. 监控APP

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

## 12. 常见问题

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

- [V免签 GitHub (Java版)](https://github.com/szvone/Vmq)
- [V免签 GitHub (PHP版)](https://github.com/szvone/vmqphp)
- [V免签 监控APP](https://github.com/szvone/vmqApk)
- [EzPay API文档](./API.md)
