# EzPay 文档

本文档以当前工作区代码为准，命令默认在仓库根目录执行。应用入口为 `main.go`，没有 `--config`、`--migrate` 等命令行参数。

| 文档 | 内容 |
| --- | --- |
| [架构](./01-ARCHITECTURE.md) | 模块、资金流程、数据表、后台任务 |
| [构建](./02-BUILD.md) | 开发、发布、跨平台、系统包、CI |
| [部署与配置](./03-DEPLOY.md) | SQLite/MySQL、配置生效范围、备份、升级、反向代理 |
| [API](./04-API.md) | 签名、下单、查询、通知、后台鉴权与通用请求 |
| [操作手册](./05-USER_GUIDE.md) | 商户、钱包、官方通道、订单、提现、APP |
| [兼容与迁移](./06-COMPATIBILITY.md) | 易支付、V免签、监控 v2 的兼容边界 |
| [Telegram](./07-TELEGRAM.md) | 商户机器人、管理员通知、Webhook |
| [国际化](./08-I18N.md) | 语言、翻译、RTL、检查命令 |
| [测试与验收](./09-TESTPLAN.md) | 自动测试、MySQL、100 轮回归、真实支付验收 |
| [监控协议](./10-MONITOR-PROTOCOL.md) | Android 收款事件认证、去重、重试 |
| [官方支付](./11-OFFICIAL-PAYMENTS.md) | 支付宝当面付/网站支付、微信 Native/H5 |
| [可靠性审查记录](./12-RELIABILITY-REVIEW.md) | 2026-11-27 的修复与历史验收结果 |
| [完整路由表](./13-ROUTES.md) | 从 Go AST 生成的路由和直接声明参数 |

## 常用命令

需要 Go 1.26.11+；`-race` 还需要可用的 C 编译器。接口示例另需 `curl`、`jq`。

```bash
go version
go test -race ./...
go vet ./...
go run docs/tools/check-docs.go
go run docs/tools/check-docs.go -routes > docs/13-ROUTES.md
CGO_ENABLED=0 go build -o release/ezpay .
```

## 统一口径

- 商户余额、冻结余额、结算金额、提现申请金额均按 USD 记账。
- `money/currency` 是原订单标价；`pay_amount/pay_currency` 是付款金额；本地金额识别看 `unique_amount`。
- 币种优先级：请求 `currency` → 商户默认币种 → USD。旧人民币客户端应将商户默认币种设置为 CNY。
- `type` 选择付款方式，后台通道选择实际收款实现，两者独立。
- YAML 配置、数据库配置、模板中的默认值并不等价，实际生效范围见[部署与配置](./03-DEPLOY.md)。
- 历史审查结果不是对后续改动的保证；真实商户、链节点和业务压力验收仍需单独执行。
