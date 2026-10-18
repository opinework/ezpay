# EzPay API 文档

本文档说明 EzPay 对外支付接口、兼容接口、回调、商户后台接口和错误码。示例中的域名统一使用 `https://pay.example.com`，请替换为实际部署地址。

## 接口总览

| 类型 | 方法 | 路径 | 鉴权方式 | 说明 |
|------|------|------|----------|------|
| 彩虹易支付 | GET/POST | `/submit.php`、`/api/submit` | MD5 签名 | 页面跳转支付 |
| 彩虹易支付 | GET/POST | `/mapi.php`、`/api/mapi` | MD5 签名 | JSON 创建订单 |
| 彩虹易支付 | GET | `/api.php?act=order` 或 `act=query` | `pid` + `key` | 单订单查询 |
| 彩虹易支付 | GET | `/api.php?act=orders` | `pid` + `key` | 订单列表分页查询（v1.3.10+） |
| 彩虹易支付 | GET | `/api.php?act=getmoney` | `pid` + `key` | 商户余额查询（v1.3.10+） |
| 彩虹易支付 | GET | `/api.php?act=refund` | `pid` + `key` | 明确返回"不支持退款"（v1.3.10+） |
| 收银台 | GET | `/cashier/{trade_no}` | 无 | 用户支付页面 |
| 收银台状态 | GET | `/api/check_order?trade_no=...` | 无 | 收银台轮询 |
| 支付类型 | GET | `/api/payment-types?pid=...` | 无 | 获取商户可用支付方式 |
| V免签 | GET | `/createOrder` | MD5 拼接签名 | 创建订单 |
| V免签 | GET | `/appHeart`、`/appPush` | 时间戳/签名 | 监控 APP 兼容 |
| V免签 | GET | `/getState`、`/checkOrder`、`/closeOrder` | 订单号 + MD5 签名 | 查询/关闭订单 |
| 商户后台 | 多种 | `/merchant/api/*` | JWT Bearer Token | 商户自助管理 |

## 基础约定

- 字符编码：请求和响应均使用 UTF-8。
- 金额精度：加密货币支付金额通常保留 6 位小数，请用户按收银台显示金额精确转账。
- 结算货币：系统内部统一以 **USD** 结算；`money` 保留商户原始金额，`settlement_amount` 为按市场汇率折算的 USD 金额（用于计入商户余额）。
- 货币优先级：订单创建时 `currency` 取值顺序为 **请求参数 currency → 商户默认货币 (DefaultCurrency) → "USD"**。商户默认货币可在后台或商户中心配置（USD/CNY/USDT/EUR）。
  - **老协议兼容（重要）**：原生易支付 (`/submit.php`、`/mapi.php`) 和 V免签 (`/createOrder`) 不强制 `currency` 字段，老 PHP 客户端代码可以不改。把商户默认货币设为 **CNY** 即可让历史的 CNY 金额请求继续按 CNY 计价；签名验证会自动跳过空字段，不影响兼容。
- 支持的 currency 取值：`USD` / `CNY` / `USDT` / `EUR` / `TRX`。传入未识别的值会直接返回错误（避免被静默当成 USD）。
- 回调成功标志：商户 `notify_url` 必须返回纯文本 `success`，大小写和首尾空白会被忽略。
- 幂等要求：同一 `out_trade_no` 不可重复创建；商户处理回调时也必须按订单号做幂等。

## 彩虹易支付兼容接口

### 1. 发起支付 (submit.php)

**请求地址**: `POST/GET /submit.php` 或 `/api/submit`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| pid | 是 | 商户ID |
| type | 是 | 支付方式 (见下方支付类型表) |
| out_trade_no | 是 | 商户订单号 (不可重复) |
| notify_url | 否 | 异步通知地址；为空时使用商户默认回调地址 |
| return_url | 否 | 同步返回地址 |
| name | 是 | 商品名称 |
| money | 是 | 金额（按 `currency` 计价） |
| currency | 否 | 货币类型: USD/CNY/USDT/EUR/TRX。未传时按"商户默认货币 → USD"兜底；签名生成时空值不参与（老 PHP 客户端可不传） |
| param | 否 | 附加参数 (原样返回) |
| sign | 是 | 签名 |
| sign_type | 否 | 签名类型: MD5；为空时按 MD5 处理 |

**签名算法**:

1. 将参数按键名 ASCII 码从小到大排序
2. **先解码参数值**（防止双重编码），如果值已经是 URL 编码的则先解码
3. 对参数值进行 **RFC 3986 URL 编码**：
   - 空格编码为 `%20`（不是 `+`）
   - `~` 保留不编码
   - **百分号编码统一使用大写**（如 `%2F` 而非 `%2f`）
4. 拼接为 `key1=urlencode(value1)&key2=urlencode(value2)` 格式（空值和 sign、sign_type 不参与）
5. 在末尾追加商户密钥（**不加 `&`**）
6. MD5 加密（小写）

**签名标准要点**:

| 项目 | 标准 |
|------|------|
| 编码方式 | RFC 3986 |
| 空格编码 | `%20`（不是 `+`） |
| 波浪号 | `~` 保留不编码 |
| 百分号大小写 | **统一大写**（`%2F`） |
| 双重编码 | **禁止**，先解码再编码 |

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
    'currency' => 'USD',  // 可选: USD/CNY/USDT/EUR/TRX。不传则取商户默认货币(后台可配)，再兜底 USD
];

// 排序
ksort($params);

// 拼接 (使用 RFC 3986 URL 编码)
$str = '';
foreach ($params as $k => $v) {
    if ($v !== '' && $k !== 'sign' && $k !== 'sign_type') {
        // 先解码（防止双重编码），再 RFC 3986 编码
        $decoded = urldecode($v);
        $str .= $k . '=' . rawurlencode($decoded) . '&';
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

**注意**:
- PHP 中使用 `rawurlencode()` 进行 RFC 3986 编码（空格→%20）
- **不要**使用 `urlencode()`（空格→+）
- 先用 `urldecode()` 解码防止双重编码

**响应**: 跳转到收银台页面

---

### 2. API方式发起支付 (mapi.php)

**请求地址**: `GET/POST /mapi.php` 或 `/api/mapi`

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
    "pay_amount": "100.00",
    "unique_amount": "100.000023",
    "usdt_amount": "100.000023",
    "rate": "1.0000",
    "address": "TXxxxxxxxxxxxxxxxxxxxxxxxxx",
    "chain": "trc20",
    "qrcode": "https://pay.example.com/qrcode?text=TXxxxxxxxxxxxxxxxxxxxxxxxxx",
    "expired_at": "2023-12-11 13:04:56",
    "payurl": "https://pay.example.com/cashier/20231211123456789abc",
    "pay_url": "https://pay.example.com/cashier/20231211123456789abc"
}
```

**响应字段说明**:

| 字段 | 说明 |
|------|------|
| currency | 原始货币类型 (`money` 的计价单位) |
| money | 原始金额（商品标价，按 `currency` 计价） |
| pay_currency | 实际支付货币 (USDT/TRX/CNY) |
| pay_amount | **展示金额**（无小数偏移，给收银台 UI 显示用，可读性好） |
| **unique_amount** | **链上匹配金额**（含小数偏移，**用户实际必须按此金额付款**） |
| usdt_amount | `unique_amount` 的别名（向下兼容老 PHP epay 商户） |
| rate | 显示汇率（1 单位 pay_currency ≈ 多少 currency） |
| address | 收款地址（USDT/TRX）或微信/支付宝商户号（fiat 时与 qrcode 配合用） |
| chain | 支付链路 |
| **qrcode** | **二维码图片绝对 URL**（v1.3.11 起，可直接 `<img src="...">` 嵌入） |
| expired_at | 订单过期时间 |
| payurl / pay_url | 收银台跳转 URL（站点未配置时为相对路径） |

**🚨 三个金额字段的语义（关键，对接时必须分清）**：

| 字段 | 加密货币示例 | 微信/支付宝示例 | 用途 |
|------|------|------|------|
| `money` | `100.00` (USD) | `100.00` (CNY) | 商品标价，**对账用** |
| `pay_amount` | `100.00` (USDT) | `100.00` (CNY) | 展示给用户的人类友好金额 |
| **`unique_amount`** | **`100.000023`** (USDT) | **`100.01`** (CNY) | **链上/收款匹配金额，用户必须精确付这个值** |

> **匹配机制**：同一收款地址/收款码下并发多笔订单时，系统给每笔 `pay_amount` 加一个微小尾数（USDT 0.000001 递增 6 位精度，CNY 0.01 递增分位），让 `unique_amount` 在每笔订单里全局唯一。链上扫描器/微信监控收到入账后按 `(chain, address, amount)` 精确匹配 `unique_amount` 锁定订单。
>
> **如果用户按 `pay_amount`（无尾数）付款，会撞到其他订单或不匹配，订单不会自动确认**。收银台 UI 必须显示 `unique_amount`。

---

### 2.1 二维码图片端点 (`/qrcode`)（v1.3.11+）

`qrcode` 字段指向的图片由此端点动态生成。商户可直接嵌入：

```html
<img src="https://pay.example.com/qrcode?text=TXxxxxxxxxxxxxxxxxxxxxxxxxx" />
```

**请求**: `GET /qrcode?text=...&size=300`

**参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| text | 是 | 二维码内容（地址 / URL / 任意文本），≤ 1024 字符 |
| size | 否 | 图片宽高（像素），范围 [64, 1024]，默认 300 |

**响应**: `Content-Type: image/png`，PNG 二进制；`Cache-Control: public, max-age=300`

---

### 3. 异步通知 (notify_url)

支付成功后，系统会向 notify_url 发送 GET 请求。

**通知参数**（v1.3.10 起含多币种字段）:

| 参数 | 说明 |
|------|------|
| pid | 商户ID |
| trade_no | 平台订单号 |
| out_trade_no | 商户订单号 |
| type | 支付方式 |
| name | 商品名称 |
| money | **原始金额** (与发起支付时一致，按 `currency` 计价) |
| **currency** | 原始货币 (USD/CNY/USDT/EUR/TRX) **— v1.3.10 新增** |
| **pay_currency** | 实际支付货币 (USDT/TRX/CNY) **— v1.3.10 新增** |
| **pay_amount** | 用户实付金额（按 `pay_currency` 计价，等于 unique_amount） **— v1.3.10 新增** |
| trade_status | 交易状态: TRADE_SUCCESS |
| param | 附加参数 |
| sign | 签名 |
| sign_type | 签名类型 |

**重要说明**:
- `money` 返回的是**发起支付时的原始金额**，按 `currency` 计价
- 多币种场景下务必读取 `currency` 字段判断金额单位
- 老 PHP epay 商户：把商户默认货币设为 CNY，`money` 仍是 CNY 金额，行为不变
- 签名算法与发起支付相同：所有非空参数排序后拼接 + 商户密钥 → MD5
- **新字段参与签名**。商户验签代码使用通用逻辑（取所有非空参数 → 排序 → 拼接 → MD5）即可自动兼容；如果验签代码硬编码了字段清单，需要把 `currency`/`pay_currency`/`pay_amount` 加进去

**响应要求**: 收到通知后返回字符串 `success`

**回调安全限制**:
- EzPay 仅向 `http`/`https` 地址发送回调
- localhost、内网 IP、`.local`、`.internal` 等目标会被拦截
- 建议生产环境回调地址使用 HTTPS，并在商户系统中校验签名和订单幂等

### 签名验证兼容性

EzPay 在验证签名时支持多种编码方式，以兼容不同平台的实现：

| 优先级 | 编码策略 | 说明 |
|--------|----------|------|
| 1 | 标准方式 | 先解码 + RFC 3986 编码（推荐） |
| 2 | RFC 3986 | 直接 RFC 3986 编码（不解码） |
| 3 | PHP urlencode | 空格编码为 `+` |
| 4 | 原始值 | 不做任何编码 |
| 5 | 解码后重试 | 对已编码参数解码后重试上述策略 |

**兼容处理**:
- 签名大小写不敏感（统一转小写比较）
- 百分号编码大小写不敏感（`%2f` 等同于 `%2F`）

> **建议**: 商户对接时使用标准签名方式（先解码 + RFC 3986 + 大写），可获得最佳兼容性

---

### 4. 单订单查询 (api.php?act=order)

**请求地址**: `GET /api.php?act=order` 或 `act=query`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| act | 是 | 操作类型: `order` 或 `query` |
| pid | 是 | 商户ID |
| key | 是 | 商户密钥（注意：明文传，仅用于内网/HTTPS 环境） |
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

`trade_status` 取值：
- `WAIT_BUYER_PAY` — 待支付
- `TRADE_SUCCESS` — 已支付
- `TRADE_CLOSED` — 已过期/已取消

---

### 5. 订单列表分页 (api.php?act=orders)（v1.3.10+）

用于商户对账，按时间/状态/类型过滤拉取订单列表。

**请求地址**: `GET /api.php?act=orders`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| act | 是 | 固定 `orders` |
| pid | 是 | 商户ID |
| key | 是 | 商户密钥 |
| limit | 否 | 每页数量，默认 20，最大 100 |
| offset | 否 | 偏移，默认 0 |
| status | 否 | 0=待支付 1=已支付 2=已过期 3=已取消（不传返回全部） |
| type | 否 | 支付类型过滤，如 `usdt_trc20` |
| start_time | 否 | 创建时间下限 (`2006-01-02 15:04:05` 或 RFC3339) |
| end_time | 否 | 创建时间上限 |

**响应**:
```json
{
    "code": 1,
    "msg": "success",
    "pid": "10001",
    "total": 358,
    "limit": 20,
    "offset": 0,
    "orders": [
        {
            "trade_no": "...",
            "out_trade_no": "ORDER_123456",
            "type": "usdt_trc20",
            "name": "商品",
            "money": "100.00",
            "currency": "CNY",
            "pay_currency": "USDT",
            "pay_amount": "14.17",
            "trade_status": "TRADE_SUCCESS",
            "param": "...",
            "addtime": 1702271096,
            "endtime": 1702272896,
            "paidtime": 1702271500
        }
    ]
}
```

---

### 6. 商户余额查询 (api.php?act=getmoney)（v1.3.10+）

**请求地址**: `GET /api.php?act=getmoney`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| act | 是 | 固定 `getmoney` |
| pid | 是 | 商户ID |
| key | 是 | 商户密钥 |

**响应**:
```json
{
    "code": 1,
    "msg": "success",
    "pid": "10001",
    "currency": "USD",
    "balance": "1234.567890",
    "frozen_balance": "12.000000",
    "available": "1222.567890"
}
```

- `balance` — 总余额（USD）
- `frozen_balance` — 冻结余额（提现中或预扣手续费）
- `available` — 可用余额 = balance - frozen_balance

---

### 7. 退款（明确不支持）(api.php?act=refund)（v1.3.10+）

加密货币交易不可逆，平台不支持退款。老 PHP epay 商户脚本调用退款时返回明确响应：

```json
{
    "code": -1,
    "msg": "本平台不支持退款（加密货币交易不可逆）；如需处理纠纷请联系平台管理员手动调整"
}
```

如需处理纠纷，请联系平台管理员通过"调整余额"功能手动处理。

---

## V免签兼容接口

### 1. 创建订单 (createOrder)

**请求地址**: `GET /createOrder`

**请求参数**:

| 参数 | 必填 | 说明 |
|------|------|------|
| payId | 是 | 商户订单号 |
| type | 是 | 支付类型 (见下方V免签类型表) |
| price | 是 | 金额；按"商户默认货币 → USD"兜底计价。**老 V免签 客户端兼容做法：商户后台把默认货币设为 CNY，这里的 price 就会按 CNY 处理，业务代码无需改动** |
| sign | 是 | 签名: MD5(payId + param + type + price + 密钥) |
| param | 否 | 附加参数 |
| notifyUrl | 否 | 回调地址；为空时订单不发送商户回调 |
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

`reallyPrice` 表示 EzPay 用于匹配到账的实际金额：法币订单为订单货币金额，USDT 订单为对应链上的 USDT 金额。

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
| sign | 是 | 签名: MD5(payId + 密钥) |

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
| sign | 是 | 签名: MD5(payId + 密钥) |

**响应**:
```json
{
    "code": 1,
    "msg": "success"
}
```

---

## 支付类型对照表

> 具体支付方式是否可用还取决于链路开关、商户钱包模式和对应钱包是否存在。客户端可通过 `/api/payment-types?pid=商户ID` 获取运行时可用列表。

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

## 多币种支持与 USD 结算体系

系统采用 **USD 作为内部统一结算货币**，支持多币种商品标价。商户余额始终以 USD 计；用户实际支付金额按支付链路转换为对应币种（USDT / TRX / CNY）。

### 支持的货币类型

| currency | 说明 | 用途 |
|----------|------|------|
| USD | 美元 | 商品标价 / 内部结算（默认兜底） |
| CNY | 人民币 | 商品标价 / 微信支付宝收款币种 |
| USDT | 泰达币 | 商品标价（与 USD 等价） / 加密货币收款币种 |
| EUR | 欧元 | 商品标价 |
| TRX | 波场币 | 加密货币收款币种 / 商品标价（罕见） |

商户级别可配置 `default_currency`，订单未传 `currency` 时使用此值作为默认。

### 货币转换流程（v1.3.8 起）

为避免双重浮动，结算与支付分两步独立计算：

#### 步骤 1 — 结算金额（market 汇率，无浮动）
```
settlement_amount = money × marketRate(currency → USD)
```
- `settlement_amount` 是订单的**真实 USD 价值**，将记入商户余额（扣除手续费后）
- USD/USDT 直通 1:1，CNY/EUR/TRX 走市场基础汇率（不含买卖浮动）

#### 步骤 2 — 用户应付金额（按支付链路转换）

| 场景 | 公式 | 浮动 |
|------|------|------|
| `currency == pay_currency`（如 CNY 商品 + 微信） | `pay_amount = money` | 不应用 |
| `currency` 与 `pay_currency` 同属 USD 锚定族（USD ↔ USDT） | `pay_amount = money` | 不应用 |
| 跨币种 → USDT | `pay_amount = settlement_amount / (1 - 买入浮动)` | 应用 |
| 跨币种 → TRX | `pay_amount = settlement_amount / (TRX_USD价 × (1 - 买入浮动))` | 应用 |
| 跨币种 → CNY | `pay_amount = settlement_amount × cnyUsdRate × (1 + 买入浮动)` | 应用 |

> 设计原则：只在"用户实际跨币种支付"时收取浮动。同币种 / 同稳定币锚定无 FX 风险，浮动不应叠加。

### 货币转换规则

| 原始货币 | 支付方式 | 步骤 | 用户支付 | 说明 |
|---------|---------|------|---------|------|
| USD | usdt_* | settlement=USD，同锚定直通 | `money` USDT | 1:1，无浮动 |
| USDT | usdt_* | 同币种直通 | `money` USDT | 1:1，无浮动 |
| CNY | usdt_* | CNY→USD 市场价；再 USD→USDT 含浮动 | `money×(1/cnyUsd)/(1-买入浮动)` USDT | 跨币种，应用浮动 |
| EUR | usdt_* | EUR→USD 市场价；再 USD→USDT 含浮动 | `money×eurUsd/(1-买入浮动)` USDT | 跨币种，应用浮动 |
| USD | trx | settlement=USD；USD→TRX 含浮动 | `money/(trxUsd×(1-买入浮动))` TRX | 跨币种，应用浮动 |
| USD | wechat/alipay | settlement=USD；USD→CNY 含浮动 | `money×cnyUsd×(1+买入浮动)` CNY | 跨币种，应用浮动 |
| CNY | wechat/alipay | 同币种直通 | `money` CNY | 用户付商品标价，无浮动 |

### 汇率配置说明

- **买入汇率浮动** (`rate_buy_float`): 用户支付时平台多收（如 +2%）— **仅作用于跨币种支付**
- **卖出汇率浮动** (`rate_sell_float`): 商户提现时平台少给（如 -2%）— 作用于 USD → USDT/TRX/CNY 提现转换
- **汇差利润**: 买入浮动 + 卖出浮动（如 2% + 2% = 4%）— **同币种支付时仅产生 FeeRate 收入，没有浮动收入**

### 使用示例（buyFloat=2%, marketCnyUsd=0.139, marketTrxUsd=0.083, marketEurUsd=1.08）

**示例 1 — USD 商品付 USDT**
```
请求: money=100, currency=USD, type=usdt_trc20
步骤: settlement=100 USD; 同锚定直通 → pay_amount=100 USDT
响应: settlement_amount=100, pay_amount=100, pay_currency=USDT
平台浮动收入: 0（同稳定锚定不收）。仍按 FeeRate 计费。
```

**示例 2 — CNY 商品付 USDT（跨币种）**
```
请求: money=100, currency=CNY, type=usdt_trc20
步骤: settlement=100×0.139=13.89 USD; 跨币种 → pay_amount=13.89/0.98=14.17 USDT
响应: settlement_amount=13.89, pay_amount=14.17, pay_currency=USDT
平台浮动收入: 14.17-13.89=0.28 USDT (≈2%)
```

**示例 3 — CNY 商品付 CNY（微信/支付宝）**
```
请求: money=100, currency=CNY, type=alipay
步骤: settlement=100×0.139=13.89 USD; 同币种直通 → pay_amount=100 CNY
响应: settlement_amount=13.89, pay_amount=100, pay_currency=CNY
平台浮动收入: 0（用户实付商品标价）。仍按 FeeRate 计费。
```

**示例 4 — 不传 currency，商户默认 CNY（老 V免签 兼容场景）**
```
请求: payId=ABC&price=100&type=1&sign=...   (V免签，无 currency 字段)
解析: req.Currency="" → 取 merchant.DefaultCurrency="CNY"
等同于上面"示例 3"，老客户端代码完全不需要改
```

**示例 5 — USD 商品付 TRX**
```
请求: money=100, currency=USD, type=trx
步骤: settlement=100 USD; 跨币种 → pay_amount=100/(0.083×0.98)=1229.59 TRX
响应: settlement_amount=100, pay_amount=1229.59, pay_currency=TRX
平台浮动收入: ~2% TRX
```

### 汇率来源

- **USDT/CNY**: 从 Binance/OKX 自动获取，支持手动设置
- **TRX/USDT**: 从 Binance 实时获取（`/api/v3/ticker/price?symbol=TRXUSDT`）
- **EUR/USD**: Binance EURUSDT，回退默认 1.08

### 订单处理流程

```
商户发起支付                    EzPay 内部处理                    支付确认与通知
┌─────────────────┐      ┌─────────────────────────┐      ┌─────────────────────┐
│ currency: USD   │      │ 1. 保存原始货币和金额:   │      │ 1. 监听区块链交易      │
│ money: 25.00    │ ──▶  │    Currency=USD         │ ──▶  │ 2. 匹配 unique_amount │
│ type: usdt_trc20│      │    Money=25.00          │      │ 3. 记录 actual_amount │
└─────────────────┘      │                         │      └──────────┬──────────┘
                         │ 2. 转换为支付货币:       │                 │
                         │    PayCurrency=USDT     │                 ▼
                         │    PayAmount=25.00      │      ┌─────────────────────┐
                         │    UniqueAmount=        │      │ 发送通知:            │
                         │       25.000023         │      │ money=25.00          │
                         │    (含 6 位偏移)         │      │ currency=USD         │
                         │    Rate=1.0000          │      │ pay_amount=25.000023 │
                         └─────────────────────────┘      │ (含金额匹配信息)     │
                                                          └─────────────────────┘
```

**关键点**:
1. 商户发起时传入原始货币和金额
2. EzPay 内部按"市场汇率折算到 USD" + "按支付链路转 pay_currency"两步计算
3. **链上/收款匹配按 `unique_amount`**（含 6 位/2 位小数偏移），**用户必须精确付这个金额**
4. 通知商户时返回**原始金额**（`money` 按 `currency` 计价）+ **多币种字段**（`currency`/`pay_currency`/`pay_amount`），所有字段参与签名

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
| money | 是 | 金额（按 currency 计价） |
| currency | 否 | 货币类型: USD/CNY/USDT/EUR/TRX。未传时按商户默认货币兜底，最后兜底 USD |
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

## 后台统计说明

### 统计货币

管理后台和商户后台的所有统计金额**统一使用 USD**（美元）显示。

由于 USDT ≈ USD (1:1)，系统使用订单的 `actual_amount`（实际收到的 USDT 金额）作为 USD 统计值。

### 统计 API 响应

Dashboard API 响应中包含 `currency: "USD"` 字段表示统计货币：

```json
{
    "code": 1,
    "currency": "USD",
    "data": {
        "today": {
            "orders": 10,
            "amount": 250.50
        },
        "total": {
            "orders": 100,
            "amount": 5000.00
        }
    }
}
```

### 订单金额字段说明

| 字段 | 说明 |
|------|------|
| currency | 原始货币类型 (USD/EUR/CNY) |
| money | 原始金额 |
| pay_currency | 实际支付货币 (USDT/TRX/CNY) |
| pay_amount | 展示给用户的需支付金额 |
| unique_amount | 带随机小数偏移的链上匹配金额，通常与 `usdt_amount` 兼容字段一致 |
| actual_amount | 实际收到金额 |
| settlement_amount | 订单入账的 USD 结算金额 |
| rate | 使用的汇率 |

---

## 安全建议

1. **HTTPS**: 生产环境务必使用HTTPS
2. **IP白名单**: 建议配置商户API调用IP白名单
3. **签名验证**: 收到回调通知时务必验证签名
4. **幂等处理**: 回调可能重试多次，注意幂等处理
5. **密钥保护**: 商户密钥妥善保管，定期更换
