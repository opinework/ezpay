# EzPay API 文档

## 彩虹易支付兼容接口

### 1. 发起支付 (submit.php)

**请求地址**: `POST/GET /submit.php` 或 `/api/submit`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| pid | 是 | 商户ID |
| type | 是 | 支付方式 (见下方支付类型表) |
| out_trade_no | 是 | 商户订单号 (不可重复) |
| notify_url | 是 | 异步通知地址 |
| return_url | 否 | 同步返回地址 |
| name | 是 | 商品名称 |
| money | 是 | 金额 |
| currency | 否 | 货币类型: CNY, USD, USDT, EUR (默认 CNY) |
| param | 否 | 附加参数 (原样返回) |
| sign | 是 | 签名 |
| sign_type | 是 | 签名类型: MD5 |

**签名算法**:

1. 将参数按键名 ASCII 码从小到大排序
2. 拼接为 `key1=value1&key2=value2` 格式 (空值和 sign、sign_type 不参与)
3. 在末尾追加商户密钥: `key1=value1&key2=value2密钥`
4. MD5 加密 (小写)

**PHP示例**:
```php
<?php
$params = [
    'pid' => '10001',
    'type' => 'usdt_trc20',
    'out_trade_no' => 'ORDER_'.time(),
    'notify_url' => 'https://yoursite.com/notify.php',
    'return_url' => 'https://yoursite.com/return.php',
    'name' => '商品名称',
    'money' => '100.00',
    'currency' => 'USD',  // 可选: CNY, USD, USDT, EUR
];

// 排序
ksort($params);

// 拼接
$str = '';
foreach ($params as $k => $v) {
    if ($v !== '' && $k !== 'sign' && $k !== 'sign_type') {
        $str .= $k . '=' . $v . '&';
    }
}
$str = rtrim($str, '&');

// 签名
$sign = md5($str . '商户密钥');

$params['sign'] = $sign;
$params['sign_type'] = 'MD5';

// 跳转支付
header('Location: https://pay.yoursite.com/submit.php?' . http_build_query($params));
```

**响应**: 跳转到收银台页面

---

### 2. API方式发起支付 (mapi.php)

**请求地址**: `POST /mapi.php` 或 `/api/mapi`

**请求参数**: 同 submit.php

**响应**:
```json
{
    "code": 1,
    "msg": "success",
    "trade_no": "20231211123456789abc",
    "out_trade_no": "ORDER_123456",
    "type": "usdt_trc20",
    "currency": "USD",
    "money": "100.00",
    "pay_currency": "USDT",
    "pay_amount": "100.000001",
    "usdt_amount": "100.000001",
    "rate": "1.0000",
    "address": "TXxxxxxxxxxxxxxxxxxxxxxxxxx",
    "chain": "trc20",
    "qrcode": "TXxxxxxxxxxxxxxxxxxxxxxxxxx",
    "expired_at": "2023-12-11 13:04:56",
    "pay_url": "/cashier/20231211123456789abc"
}
```

**响应字段说明**:

| 字段 | 说明 |
|------|------|
| currency | 原始货币类型 |
| money | 原始金额 |
| pay_currency | 实际支付货币 (USDT/TRX/CNY) |
| pay_amount | 实际需支付金额 |
| usdt_amount | 兼容旧版本，同 pay_amount |
| rate | 使用的汇率 |

---

### 3. 异步通知 (notify_url)

支付成功后，系统会向 notify_url 发送 GET 请求。

**通知参数**:

| 参数 | 说明 |
|------|------|
| pid | 商户ID |
| trade_no | 平台订单号 |
| out_trade_no | 商户订单号 |
| type | 支付方式 |
| name | 商品名称 |
| money | 金额 |
| trade_status | 交易状态: TRADE_SUCCESS |
| param | 附加参数 |
| sign | 签名 |
| sign_type | 签名类型 |

**响应要求**: 收到通知后返回字符串 `success`

---

### 4. 查询订单 (api.php)

**请求地址**: `GET /api.php?act=order`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| act | 是 | 操作类型: order |
| pid | 是 | 商户ID |
| key | 是 | 商户密钥 |
| out_trade_no | 否 | 商户订单号 (与 trade_no 二选一) |
| trade_no | 否 | 平台订单号 |

**响应**:
```json
{
    "code": 1,
    "msg": "success",
    "pid": "10001",
    "trade_no": "20231211123456789abc",
    "out_trade_no": "ORDER_123456",
    "type": "usdt_trc20",
    "name": "商品名称",
    "money": "100.00",
    "usdt_amount": "13.888889",
    "trade_status": "TRADE_SUCCESS",
    "addtime": 1702271096,
    "endtime": 1702272896
}
```

---

## V免签兼容接口

### 1. 创建订单 (createOrder)

**请求地址**: `GET /createOrder`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| payId | 是 | 商户订单号 |
| type | 是 | 支付类型 (见下方V免签类型表) |
| price | 是 | 金额 (CNY) |
| sign | 是 | 签名 |
| param | 否 | 附加参数 |
| notifyUrl | 是 | 回调地址 |
| returnUrl | 否 | 返回地址 |
| isHtml | 否 | 是否跳转 (1:跳转) |

**签名算法**: `MD5(payId + param + type + price + 密钥)`

**响应**:
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

### 2. 心跳检测 (appHeart)

**请求地址**: `GET /appHeart`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| t | 是 | 时间戳 |

---

### 3. 收款推送 (appPush)

**请求地址**: `GET /appPush`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| t | 是 | 时间戳 |
| type | 是 | 类型 (1:微信 2:支付宝) |
| price | 是 | 金额 |
| sign | 是 | 签名: MD5(type + price + t + 密钥) |

---

### 4. 获取订单状态 (getState / checkOrder)

**请求地址**: `GET /getState` 或 `GET /checkOrder`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| payId | 是 | 商户订单号 |

**响应**:
```json
{
    "code": 1,
    "msg": "success",
    "state": 1
}
```

**state说明**: 0-未支付 1-已支付 2-已过期

---

### 5. 关闭订单 (closeOrder)

**请求地址**: `GET /closeOrder`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| payId | 是 | 商户订单号 |

**响应**:
```json
{
    "code": 1,
    "msg": "success"
}
```

---

## 支付类型对照表

### 彩虹易支付 type 参数

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

### V免签 type 参数

| type | 对应支付方式 |
|------|-------------|
| 1 | 微信支付 (wechat) |
| 2 | 支付宝 (alipay) |
| 3 | USDT TRC20 |
| 4 | USDT ERC20 |
| 5 | USDT BEP20 |

---

## 多币种支持

系统支持多种货币作为订单金额输入，会根据支付方式自动转换为对应的支付货币。

### 支持的货币类型

| currency | 说明 |
|----------|------|
| CNY | 人民币 (默认) |
| USD | 美元 |
| USDT | 泰达币 |
| EUR | 欧元 |

### 货币转换规则

| 原始货币 | 支付方式 | 支付货币 | 转换说明 |
|---------|---------|---------|---------|
| USD/USDT | usdt_* | USDT | 1:1 直接转换 |
| CNY | usdt_* | USDT | CNY ÷ USDT汇率 |
| EUR | usdt_* | USDT | EUR → USD → USDT |
| USD/USDT | trx | TRX | USD ÷ TRX价格 |
| CNY | trx | TRX | CNY ÷ TRX/CNY汇率 |
| USD/USDT | wechat/alipay | CNY | USD × USD/CNY汇率 |
| CNY | wechat/alipay | CNY | 1:1 直接使用 |

### 使用示例

**示例1**: 使用 USD 支付 USDT
```bash
# 请求: money=100, currency=USD, type=usdt_trc20
# 响应: pay_amount=100 USDT (因为 USD ≈ USDT)
```

**示例2**: 使用 CNY 支付 USDT
```bash
# 请求: money=720, currency=CNY, type=usdt_trc20
# 响应: pay_amount≈100 USDT (按汇率 7.2 转换)
```

**示例3**: 使用 USD 支付微信
```bash
# 请求: money=100, currency=USD, type=wechat
# 响应: pay_amount=720 CNY (按汇率 7.2 转换)
```

**示例4**: 不传 currency (默认 CNY)
```bash
# 请求: money=100, type=usdt_trc20
# 响应: pay_amount≈13.89 USDT (100 CNY ÷ 7.2)
```

### 汇率来源

- **USDT/CNY**: 从 Binance/OKX 自动获取，支持手动设置
- **TRX/USDT**: 从 Binance 实时获取
- **EUR/USD**: 固定汇率 1.08 (可配置)

---

## 公开接口

### 获取支持的支付类型

获取系统或指定商户支持的支付类型列表，包含名称、图标等信息，客户端可直接用于渲染支付方式选择界面。

**请求地址**: `GET /api/payment-types`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| pid | 是 | 商户ID |

**响应**:
```json
{
    "code": 1,
    "msg": "success",
    "data": [
        {
            "type": "usdt_trc20",
            "name": "USDT (TRC20)",
            "chain": "trc20",
            "icon": "fab fa-bitcoin",
            "logo": "/static/img/chains/trc20.svg",
            "enabled": true
        },
        {
            "type": "usdt_bep20",
            "name": "USDT (BEP20)",
            "chain": "bep20",
            "icon": "fab fa-bitcoin",
            "logo": "/static/img/chains/bep20.svg",
            "enabled": true
        },
        {
            "type": "wechat",
            "name": "微信支付",
            "chain": "wechat",
            "icon": "fab fa-weixin",
            "logo": "/static/img/chains/wechat.svg",
            "enabled": false
        }
    ]
}
```

**返回字段说明**:

| 字段 | 说明 |
|------|------|
| type | 支付类型代码，创建订单时使用 |
| name | 显示名称 |
| chain | 链/渠道名称 |
| icon | FontAwesome 图标类名 |
| logo | Logo 图片 URL |
| enabled | 是否可用 |

**enabled 判断逻辑**:

`enabled = true` 需同时满足以下条件：
1. 区块链服务已启用该链
2. 商户有对应链的钱包（根据商户钱包模式）：
   - 钱包模式 1 (仅系统钱包): 检查系统钱包
   - 钱包模式 2 (仅个人钱包): 检查商户自己的钱包
   - 钱包模式 3 (混合模式): 检查系统钱包 + 商户钱包

**使用示例**:
```javascript
// 获取商户可用的支付类型
fetch('/api/payment-types?pid=10001')
  .then(res => res.json())
  .then(data => {
    if (data.code === 1) {
      // 只显示 enabled 为 true 的支付类型
      const availableTypes = data.data.filter(t => t.enabled);
      availableTypes.forEach(type => {
        // 使用 type.logo 显示图标
        // 使用 type.name 显示名称
        // 创建订单时传 type.type 作为支付类型
      });
    }
  });
```

---

## 商户后台接口

商户后台接口需要先登录获取 Token，然后在请求头中携带 `Authorization: Bearer {token}`。

### 创建测试订单

商户可通过此接口快速创建测试订单，无需签名验证。

**请求地址**: `POST /api/merchant/orders/test`

**请求头**: `Authorization: Bearer {token}`

**请求参数** (JSON):

| 参数 | 必填 | 说明 |
|------|------|------|
| type | 是 | 支付类型 (如 usdt_trc20) |
| money | 是 | 金额 |
| currency | 否 | 货币类型: CNY, USD, USDT, EUR (默认 CNY) |
| name | 否 | 商品名称 (默认: 测试订单) |

**请求示例**:
```json
{
    "type": "usdt_trc20",
    "money": "100.00",
    "currency": "USD",
    "name": "测试商品"
}
```

**响应**:
```json
{
    "code": 1,
    "msg": "测试订单创建成功",
    "trade_no": "20231211123456789abc",
    "pay_url": "/cashier/20231211123456789abc"
}
```

---

### 取消订单

商户可取消待支付状态的订单。

**请求地址**: `POST /api/merchant/orders/{trade_no}/cancel`

**请求头**: `Authorization: Bearer {token}`

**响应**:
```json
{
    "code": 1,
    "msg": "订单已取消"
}
```

**错误响应**:
```json
{
    "code": -1,
    "msg": "只能取消待支付订单"
}
```

---

### 手动确认订单

商户可手动确认待支付或已过期的订单为已支付状态。

**请求地址**: `POST /api/merchant/orders/{trade_no}/confirm`

**请求头**: `Authorization: Bearer {token}`

**请求参数** (JSON):

| 参数 | 必填 | 说明 |
|------|------|------|
| tx_hash | 否 | 交易哈希 |
| amount | 否 | 实际收款金额 (USDT) |

**响应**:
```json
{
    "code": 1,
    "msg": "订单已确认支付"
}
```

---

## 收银台页面

### 访问地址

`GET /cashier/{trade_no}`

### 轮询订单状态

收银台页面会自动轮询订单状态，也可通过API获取：

**请求地址**: `GET /api/check_order?trade_no={trade_no}`

**响应**:
```json
{
    "paid": false,
    "status": 0,
    "return_url": ""
}
```

**paid为true时**: 支付成功，如有return_url会自动跳转

---

## 错误码

| code | 说明 |
|------|------|
| 1 | 成功 |
| -1 | 失败 (查看msg字段) |
| 0 | 未找到匹配订单 |

---

## 常见错误信息

| msg | 说明 |
|-----|------|
| 参数不完整 | 必填参数缺失 |
| 签名错误 | 签名验证失败，检查密钥和签名算法 |
| 商户不存在 | PID错误或商户已禁用 |
| 商户订单号已存在 | out_trade_no/payId重复 |
| 不支持的支付方式 | type参数错误或该链路未启用 |
| 暂无可用的收款地址 | 对应链路没有启用的钱包 |
| 商户余额不足以支付手续费 | 使用个人钱包模式时余额不足 |
| 订单不存在 | trade_no/out_trade_no错误 |
| 订单已过期 | 订单超时未支付 |

---

## 安全建议

1. **HTTPS**: 生产环境务必使用HTTPS
2. **IP白名单**: 建议配置商户API调用IP白名单
3. **签名验证**: 收到回调通知时务必验证签名
4. **幂等处理**: 回调可能重试多次，注意幂等处理
5. **密钥保护**: 商户密钥妥善保管，定期更换
