# EzPay 与 彩虹易支付 接口对比文档

本文档详细对比 EzPay 系统与原版彩虹易支付的接口差异，帮助开发者快速完成系统迁移或对接。

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

## 6. 支付类型对照表

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

## 8. 功能差异总结

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

## 9. 迁移指南

### 从彩虹易支付迁移到 EzPay

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

## 参考资料

- [彩虹易支付 GitHub](https://github.com/Blokura/Epay)
- [彩虹易支付官方文档](https://pay.ihuacn.com/doc.html)
- [EzPay API文档](./API.md)
