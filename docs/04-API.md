# API

命令在仓库根目录执行，需 Go 1.26.11+、curl、jq。完整方法、路径和处理器直接声明的参数见[路由表](./13-ROUTES.md)；官方配置见[官方支付](./11-OFFICIAL-PAYMENTS.md)。

## 鉴权与响应

- 支付下单：商户 PID/密钥签名；查询接口使用 PID/key，须 HTTPS。
- 管理后台：`/admin/api/*`，除 login 外需管理员 Bearer JWT。
- 商户后台：`/merchant/api/*`，除 login 外需商户 Bearer JWT，订单和钱包按商户隔离。
- 不存在统一的响应封装：检查 HTTP 状态和业务 `code/msg`；支付常见成功码 1、失败码 -1。限流或鉴权失败也可能返回非 200。
- JWT 登录有效期为 24 小时；修改 YAML 的 expire_hour 不改变此值。

```bash
export EZPAY_URL=https://pay.example.com
read -rsp '管理员密码: ' ADMIN_PASSWORD; echo
ADMIN_TOKEN=$(jq -n --arg password "$ADMIN_PASSWORD" '{username:"admin",password:$password}' |
  curl -fsS "$EZPAY_URL/admin/api/login" -H 'Content-Type: application/json' --data-binary @- |
  jq -er '.token')
unset ADMIN_PASSWORD
curl -fsS "$EZPAY_URL/admin/api/configs" -H "Authorization: Bearer $ADMIN_TOKEN"
```

商户登录 JSON 为 `{"pid":"1001","password":"实际密码"}`，Token 位于 `.data.token`，管理员则在顶层 `.token`。

## 易支付下单

`GET/POST /submit.php`（别名 `/api/submit`）跳转收银台；`/mapi.php`（别名 `/api/mapi`）返回 JSON。POST 使用表单，不接收 JSON 下单；同时存在 query/form 时 query 优先。

| 参数 | 要求 |
| --- | --- |
| pid、type、out_trade_no、name、money、sign | 必填 |
| currency | USD/CNY/USDT/EUR/TRX；省略取商户默认币种，再取 USD |
| notify_url、return_url | 可选 HTTP(S) URL；禁止 userinfo，长度最多 500 字节 |
| param、sign_type | 可选；签名类型 MD5 |
| out_trade_no | 最多 64 字符，同商户唯一，包括软删除记录 |
| name | 最多 200 字符 |
| money | 正数普通十进制，最多 12 位整数、6 位小数，不接受科学计数法 |

同一商户订单号的待支付订单，金额、币种、付款链路一致时可返回原支付入口；冲突、已支付或已关闭订单不能当作新订单创建。

### 签名

请求参与签名的白名单为 `pid,type,out_trade_no,notify_url,return_url,name,money,currency,param`。去掉空值，按键名排序，以原始值拼接 `k=v&k=v`，末尾直接追加密钥，取小写 MD5；`sign/sign_type` 不参与。服务器也兼容已有 URL 编码签名变体，新接入推荐原始值算法。签名完成后才进行 HTTP 表单编码。

以下工具调用本仓库签名实现。不要将密钥写入命令历史或版本库：

```bash
read -rsp '商户密钥: ' EZPAY_SIGN_KEY; echo
export EZPAY_SIGN_KEY
jq -n --arg no "DOC-$(date +%s)" '{
  pid:"1001",type:"alipay",out_trade_no:$no,
  name:"测试商品",money:"0.01",currency:"CNY"
}' | go run docs/tools/sign.go -mode epay > /tmp/ezpay-order.json
jq -r 'to_entries|map((.key|@uri)+"="+(.value|@uri))|join("&")' /tmp/ezpay-order.json |
  curl -fsS "$EZPAY_URL/mapi.php" -H 'Content-Type: application/x-www-form-urlencoded' --data-binary @-
unset EZPAY_SIGN_KEY
```

命令会创建订单，需先配置可用通道。测试后按业务流程关闭未付款订单。

### 支付类型与金额

| type | 链路 |
| --- | --- |
| wxpay、wechat、1 | 微信 |
| alipay、2 | 支付宝 |
| usdt_trc20、3 | TRC20 |
| usdt_erc20、4 | ERC20 |
| usdt_bep20、5 | BEP20 |
| usdt_polygon、usdt_optimism、usdt_arbitrum、usdt_avalanche、usdt_base | 对应 USDT 链 |
| trx | TRX |

`GET /api/payment-types?pid=商户PID` 返回可用性，以实际启用的链、钱包和通道为准，不能仅凭类型表判断可支付。

MAPI 成功响应含 `code,msg,trade_no,out_trade_no,type,currency,money,pay_currency,pay_amount,unique_amount,usdt_amount,rate,address,chain,qrcode,expired_at,payurl,pay_url`。

| 字段 | 含义 |
| --- | --- |
| money/currency | 商户原始标价 |
| pay_amount/pay_currency | 换算后的付款金额及币种 |
| unique_amount | 本地收款匹配金额，付款应精确遵从收银台；可能带尾数偏移 |
| usdt_amount | 兼容旧客户端的金额字段，不代表所有链路均为 USDT |
| qrcode | 二维码图片 URL，不是二维码原始内容；官方网站/H5 模式为空 |
| payurl/pay_url | EzPay 收银台 URL；site_url 未配置时可能为相对路径 |

官方通道按精确 CNY 金额收款，没有本地金额识别偏移。所有模式建议跳转 `payurl`，避免自行推断二维码或付款链接。

## 查询与退款

`GET /api.php` 与 `/api/query` 使用同一分发器，都必须传 `act`：

| act | 参数及结果 |
| --- | --- |
| order、query | pid/key，trade_no 或 out_trade_no（二者同传优先 trade_no）；验证归属；返回单订单 |
| orders | pid/key；limit 默认 20、最大 100；offset 默认 0；可筛 status/type/start_time/end_time；返回 orders/total/limit/offset |
| getmoney | pid/key；返回 USD 的 balance/frozen_balance/available |
| refund | 明确返回不支持退款，官方支付也不例外 |

列表时间支持 RFC3339 或 `YYYY-MM-DD HH:mm:ss`。单订单旧响应不含 currency/pay_currency，列表响应包含这些字段；对账应保留创建请求中的币种。

收银台使用 `GET /api/check_order?trade_no=...`，订单详情为 `GET /api/order/:trade_no`。这些公开接口不应作为商户服务端入账凭证。

## 下游通知

支付成功向 notify_url 发起 **GET**，字段：
`pid,trade_no,out_trade_no,type,name,money,currency,pay_currency,pay_amount,trade_status,param,sign_type,sign`。
其中 `trade_status=TRADE_SUCCESS`，money 为原标价，pay_amount 为实付金额。

验签时取收到的全部非空参数（排除 sign/sign_type），按原始值排序拼接加商户密钥计算 MD5。新增币种字段参与签名，不能用旧字段白名单验签。

接收方先校验签名、商户、订单、金额和币种，再以 trade_no 幂等提交业务，最后返回 2xx 与纯文本 `success`（忽略大小写及首尾空白）。通知至少一次，可能重复。发送端不跟随重定向，阻止内网/本机等不安全目标；配置公网 HTTPS 地址。

return_url 携带相同签名参数供用户返回，不能仅凭浏览器跳转认定到账。V免签创建的订单，下游通知也使用本节易支付格式。

自动通知由数据库状态恢复，批次次数采用数据库 notify_retry；累计重试有上限。管理员对已支付订单手动重试可重置预算。接入方必须允许重复通知。

## V免签

| 接口 | 参数/结果 |
| --- | --- |
| GET /createOrder | payId、price、sign 必填；type 默认 1；可选 param/notifyUrl/returnUrl/isHtml |
| GET /getState、/checkOrder | payId、sign；payId 是原商户订单号，不是返回的 orderId |
| GET /closeOrder | 同查询签名；仅允许关闭符合条件的未支付订单 |

创建签名 `MD5(payId + param + type + price + key)`；查询/关闭签名 `MD5(payId + key)`。没有 pid，服务器通过活跃商户密钥匹配身份。使用工具 `-mode vmq-create` 或 `-mode vmq-state` 生成。

类型 1/2/3/4/5 对应微信/支付宝/TRC20/ERC20/BEP20；未知类型当前回退 TRC20，请只传受支持值。接口不解析 currency/channel，币种由商户默认设置决定。

创建 JSON 含 `code,msg,payId,orderId,payType,price,reallyPrice,payUrl,isAuto,state,timeOut,date`；payUrl 为本地收银台路径，isHtml=1 时跳转。状态 0 未支付、1 已支付、2 过期/取消。官方关闭会先查单并请求提供方关闭，无法确认时保留本地状态。

`/appHeart`、`/appPush` 的认证及事件去重另见[监控协议](./10-MONITOR-PROTOCOL.md)，旧 MD5 收款推送已不支持。

## 后台请求补充

完整端点及直接声明字段见[路由表](./13-ROUTES.md)。以下请求不能仅靠该表推断：

- `POST /admin/api/configs`：扁平 `string:string` JSON，例 `{"order_expire":"30"}`；有效项、范围及 YAML 区别见[部署配置](./03-DEPLOY.md)。官方私钥/API v3 密钥空值保留原值，其他键不能假设同样行为。
- `POST /merchant/api/withdrawals`：JSON 的 amount 为必填正数，address_id 必填；可选 amount_str（精确十进制，内部优先）及 remark。只传 amount_str 会被 binding 拒绝。
- 提现默认最低 50 USD，固定手续费配置默认 1 USD，使用已审核地址。新地址链支持 bep20/trc20/polygon/optimism；更新地址只能改 label/is_default。
- 钱包模式 wallet_mode 为 1 系统、2 个人、3 混合。钱包管理的链范围不等于提现地址链范围。
- `POST /merchant/api/orders/:trade_no/confirm` 无 JSON 参数，仅允许该商户自己的微信/支付宝个人钱包订单（待支付或过期）；会实际标记已支付并触发通知，应先核实到账。系统钱包、加密货币和未绑定钱包的官方订单不可这样确认。
- 管理员 `POST /admin/api/orders/:trade_no/paid` 接收 tx_hash/amount（金额字符串）；官方订单仍需签名查单确认，不可绕过官方付款事实。
- 测试订单会进入正常订单流程，并非支付沙箱；管理员测试 currency 只接受 USD/EUR/CNY，默认 USD。
- 上传二维码为 multipart `file`；APP 上传还需 version_code/version_name，可选 changelog/force_update，APK 上限 100 MiB。
- `GET /api/app/version?version_code=...` 提供版本信息；`/api/app/download` 重定向下载，无可用版本时 404。服务端仓库不含 Android APP 源码。

## 公共运维接口

```bash
curl -fsS "$EZPAY_URL/health"
curl -fsS "$EZPAY_URL/health/detail"
curl -fsS "$EZPAY_URL/api/payment-types?pid=1001"
```

/health 检查数据库连接，失败返回 503；/health/detail 降级情况仍可能返回 200，需读取响应内容。`GET /qrcode?text=...&size=300` 返回 PNG，内容上限 1024 字符，尺寸 64–1024，默认 300。
