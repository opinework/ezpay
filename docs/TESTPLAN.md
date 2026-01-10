# EzPay 测试计划

## 1. 测试概述

### 1.1 项目简介
EzPay是一个USDT收款平台，兼容彩虹易支付和V免签接口，支持多种区块链网络（TRC20、ERC20、BEP20等）以及微信/支付宝收款。

### 1.2 测试目标
- 验证所有API接口功能正确性
- 验证区块链监控和订单匹配功能
- 验证管理后台和商户后台功能
- 验证安全机制（签名、白名单、认证）
- 验证系统稳定性和性能

### 1.3 测试环境
- **服务端口**: 6088
- **数据库**: MySQL 8.0
- **测试商户**: PID=1001, Key=test_key_123456

---

## 2. 接口测试

### 2.1 彩虹易支付兼容接口

#### 2.1.1 发起支付 (submit.php)
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| EP-001 | 正常创建订单 | POST /submit.php 带完整参数和正确签名 | 302跳转到收银台 | ⬜ |
| EP-002 | 缺少必填参数 | POST /submit.php 缺少pid参数 | 返回"参数不完整" | ⬜ |
| EP-003 | 商户不存在 | POST /submit.php pid=9999 | 返回"商户不存在或已禁用" | ⬜ |
| EP-004 | 签名错误 | POST /submit.php 签名不正确 | 返回"签名验证失败" | ⬜ |
| EP-005 | IP白名单拦截 | 开启白名单后从非白名单IP调用 | 返回"IP不在白名单内" | ⬜ |
| EP-006 | Referer白名单拦截 | 开启白名单后带非白名单Referer | 返回"请求来源不在白名单内" | ⬜ |
| EP-007 | 重复订单号 | 使用已存在的out_trade_no | 返回"订单号已存在" | ⬜ |

**测试命令:**
```bash
# EP-001 正常创建订单
curl -X POST "http://localhost:6088/submit.php" \
  -d "pid=1001&type=usdt_trc20&out_trade_no=test$(date +%s)&notify_url=http://example.com/notify&name=TestOrder&money=100&sign=<正确签名>"
```

#### 2.1.2 API发起支付 (mapi.php)
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| EP-010 | 正常创建订单 | POST /mapi.php 带完整参数 | JSON返回code=1和订单信息 | ⬜ |
| EP-011 | 返回USDT金额 | 正常创建订单 | 返回usdt_amount和rate | ⬜ |
| EP-012 | 返回钱包地址 | 正常创建订单 | 返回address和chain | ⬜ |

**测试命令:**
```bash
# EP-010 API创建订单
curl -X POST "http://localhost:6088/mapi.php" \
  -d "pid=1001&type=usdt_trc20&out_trade_no=test$(date +%s)&notify_url=http://example.com/notify&name=TestOrder&money=100&sign=<正确签名>"
```

#### 2.1.3 订单查询 (api.php)
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| EP-020 | 按trade_no查询 | GET /api.php?act=order&trade_no=xxx | 返回订单详情 | ⬜ |
| EP-021 | 按out_trade_no查询 | GET /api.php?act=order&out_trade_no=xxx | 返回订单详情 | ⬜ |
| EP-022 | 订单不存在 | GET /api.php?act=order&trade_no=notexist | 返回错误信息 | ⬜ |
| EP-023 | 密钥错误 | GET /api.php?act=order&key=wrongkey | 返回"密钥错误" | ⬜ |

**测试命令:**
```bash
# EP-020 查询订单
curl "http://localhost:6088/api.php?act=order&pid=1001&key=test_key_123456&trade_no=EZ20241211xxxx"
```

#### 2.1.4 订单状态轮询
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| EP-030 | 查询待支付订单 | GET /api/check_order?trade_no=xxx | paid=false, status=0 | ⬜ |
| EP-031 | 查询已支付订单 | GET /api/check_order?trade_no=xxx | paid=true, status=1 | ⬜ |
| EP-032 | 查询已过期订单 | GET /api/check_order?trade_no=xxx | paid=false, status=2 | ⬜ |

---

### 2.2 V免签兼容接口

#### 2.2.1 创建订单
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| VM-001 | 正常创建订单 | GET /createOrder 带完整参数 | code=1, 返回订单信息 | ⬜ |
| VM-002 | isHtml=1跳转 | GET /createOrder?isHtml=1 | 302跳转到收银台 | ⬜ |
| VM-003 | 签名错误 | GET /createOrder 签名不正确 | code=-1, 签名验证失败 | ⬜ |

**测试命令:**
```bash
# VM-001 V免签创建订单
curl "http://localhost:6088/createOrder?payId=test$(date +%s)&type=1&price=100&sign=<签名>&notifyUrl=http://example.com/notify"
```

#### 2.2.2 心跳检测
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| VM-010 | 正常心跳 | GET /appHeart?t=当前时间戳 | code=1 | ⬜ |
| VM-011 | 时间戳过期 | GET /appHeart?t=过期时间戳 | code=-1 | ⬜ |

#### 2.2.3 收款推送
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| VM-020 | 正常推送匹配 | GET /appPush 推送匹配金额 | code=1, 订单更新为已支付 | ⬜ |
| VM-021 | 未找到匹配订单 | GET /appPush 推送不匹配金额 | code=0 | ⬜ |

#### 2.2.4 订单状态查询
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| VM-030 | 查询待支付 | GET /getState?payId=xxx | state=0 | ⬜ |
| VM-031 | 查询已支付 | GET /getState?payId=xxx | state=1 | ⬜ |
| VM-032 | 查询已过期 | GET /getState?payId=xxx | state=2 | ⬜ |

#### 2.2.5 关闭订单
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| VM-040 | 正常关闭 | GET /closeOrder?payId=xxx | code=1 | ⬜ |
| VM-041 | 订单不存在 | GET /closeOrder?payId=notexist | code=-1 | ⬜ |

---

### 2.3 上游通道回调

#### 2.3.1 V免签通道回调
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| CH-001 | 正常回调 | GET /channel/notify/vmq 带正确参数 | 订单更新为已支付 | ⬜ |
| CH-002 | 签名错误 | GET /channel/notify/vmq 签名不正确 | 返回错误 | ⬜ |

#### 2.3.2 易支付通道回调
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| CH-010 | 正常回调 | GET /channel/notify/epay 带正确参数 | 订单更新为已支付 | ⬜ |
| CH-011 | 签名错误 | GET /channel/notify/epay 签名不正确 | 返回错误 | ⬜ |

---

## 3. 收银台测试

| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| CS-001 | 显示收银台 | GET /cashier/{trade_no} | 显示支付信息和二维码 | ⬜ |
| CS-002 | 显示USDT金额 | 查看收银台 | 显示正确的USDT金额和汇率 | ⬜ |
| CS-003 | 显示钱包地址 | 查看收银台 | 显示收款地址和链类型 | ⬜ |
| CS-004 | 订单倒计时 | 查看收银台 | 显示正确的过期倒计时 | ⬜ |
| CS-005 | 自动轮询状态 | 等待页面轮询 | 每5秒查询订单状态 | ⬜ |
| CS-006 | 支付成功跳转 | 订单支付成功后 | 自动跳转到return_url | ⬜ |
| CS-007 | 订单不存在 | GET /cashier/notexist | 显示错误页面 | ⬜ |
| CS-008 | 订单已过期 | GET /cashier/{过期订单} | 显示已过期提示 | ⬜ |

---

## 4. 管理后台测试

### 4.1 登录认证
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-001 | 正常登录 | POST /admin/api/login 正确账号密码 | 返回token | ⬜ |
| AD-002 | 密码错误 | POST /admin/api/login 错误密码 | 返回错误 | ⬜ |
| AD-003 | Token验证 | 带有效token访问API | 正常返回数据 | ⬜ |
| AD-004 | Token过期 | 带过期token访问API | 返回401 | ⬜ |

**测试命令:**
```bash
# AD-001 管理员登录
curl -X POST "http://localhost:6088/admin/api/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

### 4.2 仪表盘
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-010 | 获取统计数据 | GET /admin/api/dashboard | 返回今日/昨日/本月统计 | ⬜ |
| AD-011 | 区块链状态 | GET /admin/api/dashboard | 返回各链监控状态 | ⬜ |

### 4.3 订单管理
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-020 | 订单列表 | GET /admin/api/orders | 返回订单列表和分页 | ⬜ |
| AD-021 | 订单筛选 | GET /admin/api/orders?status=1 | 返回筛选后的订单 | ⬜ |
| AD-022 | 订单详情 | GET /admin/api/orders/{trade_no} | 返回订单详情 | ⬜ |
| AD-023 | 手动确认支付 | POST /admin/api/orders/{trade_no}/paid | 订单状态更新为已支付 | ⬜ |
| AD-024 | 重发通知 | POST /admin/api/orders/{trade_no}/notify | 重新发送回调通知 | ⬜ |
| AD-025 | 导出订单 | GET /admin/api/orders/export | 下载CSV文件 | ⬜ |

### 4.4 商户管理
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-030 | 商户列表 | GET /admin/api/merchants | 返回商户列表 | ⬜ |
| AD-031 | 创建商户 | POST /admin/api/merchants | 创建成功返回商户信息 | ⬜ |
| AD-032 | 编辑商户 | PUT /admin/api/merchants/{id} | 更新成功 | ⬜ |
| AD-033 | 查看密钥 | GET /admin/api/merchants/{id}/key | 返回商户密钥 | ⬜ |
| AD-034 | 重置密钥 | POST /admin/api/merchants/{id}/reset-key | 生成新密钥 | ⬜ |
| AD-035 | 禁用商户 | PUT /admin/api/merchants/{id} status=0 | 商户状态变为禁用 | ⬜ |

### 4.5 钱包管理
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-040 | 钱包列表 | GET /admin/api/wallets | 返回钱包列表 | ⬜ |
| AD-041 | 添加钱包 | POST /admin/api/wallets | 创建成功 | ⬜ |
| AD-042 | 编辑钱包 | PUT /admin/api/wallets/{id} | 更新成功 | ⬜ |
| AD-043 | 删除钱包 | DELETE /admin/api/wallets/{id} | 删除成功 | ⬜ |
| AD-044 | 上传二维码 | POST /admin/api/upload/qrcode | 返回二维码URL | ⬜ |

### 4.6 链监控管理
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-050 | 获取链状态 | GET /admin/api/chains | 返回所有链状态 | ⬜ |
| AD-051 | 启用链 | POST /admin/api/chains/{chain}/enable | 链状态变为启用 | ⬜ |
| AD-052 | 禁用链 | POST /admin/api/chains/{chain}/disable | 链状态变为禁用 | ⬜ |
| AD-053 | 批量更新 | POST /admin/api/chains/batch | 批量更新成功 | ⬜ |

### 4.7 系统配置
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-060 | 获取配置 | GET /admin/api/configs | 返回系统配置 | ⬜ |
| AD-061 | 更新配置 | POST /admin/api/configs | 配置更新成功 | ⬜ |
| AD-062 | 获取汇率 | GET /admin/api/rate | 返回当前汇率 | ⬜ |
| AD-063 | 刷新汇率 | POST /admin/api/rate/refresh | 汇率刷新成功 | ⬜ |

### 4.8 日志查询
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-070 | 交易日志 | GET /admin/api/transactions | 返回交易日志列表 | ⬜ |
| AD-071 | API调用日志 | GET /admin/api/api-logs | 返回API调用日志 | ⬜ |
| AD-072 | 日志筛选 | GET /admin/api/api-logs?merchant_pid=1001 | 返回筛选后的日志 | ⬜ |

### 4.9 提现管理
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-080 | 提现列表 | GET /admin/api/withdrawals | 返回提现申请列表 | ⬜ |
| AD-081 | 审核通过 | POST /admin/api/withdrawals/{id}/approve | 状态变为已审核 | ⬜ |
| AD-082 | 审核拒绝 | POST /admin/api/withdrawals/{id}/reject | 状态变为已拒绝 | ⬜ |
| AD-083 | 完成提现 | POST /admin/api/withdrawals/{id}/complete | 状态变为已完成 | ⬜ |

### 4.10 IP黑名单管理
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-090 | 黑名单列表 | GET /admin/api/ip-blacklist | 返回IP黑名单列表 | ⬜ |
| AD-091 | 手动添加IP | POST /admin/api/ip-blacklist | 添加成功 | ⬜ |
| AD-092 | 移除IP | DELETE /admin/api/ip-blacklist/{id} | 移除成功 | ⬜ |
| AD-093 | 从日志拉黑 | POST /admin/api/ip-blacklist/block | 从API日志拉黑IP | ⬜ |
| AD-094 | 黑名单IP拦截 | 使用黑名单IP调用支付API | 返回"IP已被禁止访问" | ⬜ |
| AD-095 | 非黑名单IP | 使用正常IP调用支付API | 正常通过 | ⬜ |

**测试命令:**
```bash
# AD-090 查看IP黑名单
curl "http://localhost:6088/admin/api/ip-blacklist" -H "Authorization: Bearer $TOKEN"

# AD-091 添加IP到黑名单
curl -X POST "http://localhost:6088/admin/api/ip-blacklist" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ip":"192.168.1.100","reason":"恶意访问"}'

# AD-092 移除IP
curl -X DELETE "http://localhost:6088/admin/api/ip-blacklist/1" -H "Authorization: Bearer $TOKEN"

# AD-093 从API日志拉黑
curl -X POST "http://localhost:6088/admin/api/ip-blacklist/block" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ip":"10.0.0.1","log_id":123}'
```

### 4.11 其他功能
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| AD-100 | 修改密码 | POST /admin/api/password | 密码修改成功 | ⬜ |
| AD-101 | 测试机器人 | POST /admin/api/test-bot | 发送测试消息成功 | ⬜ |

---

## 5. 商户后台测试

### 5.1 登录认证
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| MC-001 | 正常登录 | POST /merchant/api/login | 返回token | ⬜ |
| MC-002 | 密码错误 | POST /merchant/api/login 错误密码 | 返回错误 | ⬜ |
| MC-003 | 商户禁用 | 禁用商户后登录 | 返回"商户已被禁用" | ⬜ |

**测试命令:**
```bash
# MC-001 商户登录
curl -X POST "http://localhost:6088/merchant/api/login" \
  -H "Content-Type: application/json" \
  -d '{"pid":"1001","password":"merchant123"}'
```

### 5.2 仪表盘和个人信息
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| MC-010 | 获取统计 | GET /merchant/api/dashboard | 返回商户统计数据 | ⬜ |
| MC-011 | 获取个人信息 | GET /merchant/api/profile | 返回商户信息 | ⬜ |
| MC-012 | 更新个人信息 | PUT /merchant/api/profile | 更新成功 | ⬜ |
| MC-013 | 修改密码 | POST /merchant/api/password | 密码修改成功 | ⬜ |

### 5.3 API密钥
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | ���态 |
|---------|---------|---------|---------|------|
| MC-020 | 查看密钥 | GET /merchant/api/key | 返回API密钥 | ⬜ |
| MC-021 | 重置密钥 | POST /merchant/api/key/reset | 生成新密钥 | ⬜ |

### 5.4 订单管理
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| MC-030 | 订单列表 | GET /merchant/api/orders | 只返回该商户订单 | ⬜ |
| MC-031 | 订单详情 | GET /merchant/api/orders/{trade_no} | 返回订单详情 | ⬜ |
| MC-032 | 跨商户访问 | 访问其他商户订单 | 返回权限错误 | ⬜ |

### 5.5 钱包管理
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| MC-040 | 钱包列表 | GET /merchant/api/wallets | 只返回该商户钱包 | ⬜ |
| MC-041 | 添加钱包 | POST /merchant/api/wallets | 创建成功 | ⬜ |
| MC-042 | 编辑钱包 | PUT /merchant/api/wallets/{id} | 更新成功 | ⬜ |
| MC-043 | 删除钱包 | DELETE /merchant/api/wallets/{id} | 删除成功 | ⬜ |

### 5.6 提现功能
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| MC-050 | 查看余额 | GET /merchant/api/balance | 返回可用余额 | ⬜ |
| MC-051 | 提现列表 | GET /merchant/api/withdrawals | 返回提现记录 | ⬜ |
| MC-052 | 申请提现 | POST /merchant/api/withdrawals | 创建提现申请 | ⬜ |
| MC-053 | 余额不足 | 申请超过余额的提现 | 返回余额不足错误 | ⬜ |

---

## 6. 区块链监控测试

### 6.1 TRC20 (Tron)
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| BC-001 | 监控启动 | 启动服务 | TRC20监控正常启动 | ⬜ |
| BC-002 | 交易检测 | 发送USDT到收款地址 | 检测到转账交易 | ⬜ |
| BC-003 | 订单匹配 | 转账金额匹配待支付订单 | 订单自动更新为已支付 | ⬜ |
| BC-004 | 确认数检查 | 等待19个确认 | 达到确认数后触发回调 | ⬜ |

### 6.2 ERC20 (Ethereum)
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| BC-010 | 监控启动 | 启动服务 | ERC20监控正常启动 | ⬜ |
| BC-011 | 交易检测 | 发送USDT到收款地址 | 检测到转账交易 | ⬜ |
| BC-012 | 确认数检查 | 等待12个确认 | 达到确认数后触发回调 | ⬜ |

### 6.3 BEP20 (BSC)
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| BC-020 | 监控启动 | 启动服务 | BEP20监控正常启动 | ⬜ |
| BC-021 | 交易检测 | 发送USDT到收款地址 | 检测到转账交易 | ⬜ |
| BC-022 | 确认数检查 | 等待15个确认 | 达到确认数后触发回调 | ⬜ |

### 6.4 微信/支付宝通道
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| BC-030 | 微信订单创建 | 创建type=wechat订单 | 显示微信二维码 | ⬜ |
| BC-031 | 支付宝订单创建 | 创建type=alipay订单 | 显示支付宝二维码 | ⬜ |
| BC-032 | 通道禁用 | 禁用微信通道后创建订单 | 返回通道不可用 | ⬜ |

---

## 7. 回调通知测试

| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| NT-001 | 正常回调 | 订单支付成功 | 发送回调到notify_url | ⬜ |
| NT-002 | 回调签名 | 检查回调参数 | 签名验证通过 | ⬜ |
| NT-003 | 回调失败重试 | 回调返回非success | 自动重试3次 | ⬜ |
| NT-004 | 手动重发 | POST /admin/api/orders/{trade_no}/notify | 手动触发回调 | ⬜ |
| NT-005 | 同步返回 | 收银台支付成功 | 跳转到return_url | ⬜ |

---

## 8. 汇率服务测试

| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| RT-001 | 自动获取汇率 | 启动服务 | 从API获取实时汇率 | ⬜ |
| RT-002 | 汇率缓存 | 短时间内多次调用 | 使用缓存汇率 | ⬜ |
| RT-003 | 手动设置汇率 | 后台设置固定汇率 | 使用固定汇率 | ⬜ |
| RT-004 | CNY转USDT | 创建100元订单 | USDT金额=100/汇率 | ⬜ |

---

## 9. 安全测试

### 9.1 签名验证
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| SC-001 | 正确签名 | 按规则生成签名 | 验证通过 | ⬜ |
| SC-002 | 错误签名 | 使用错误签名调用API | 验证失败 | ⬜ |
| SC-003 | 参数篡改 | 修改参数后不更新签名 | 验证失败 | ⬜ |
| SC-004 | 空签名 | 不传签名参数 | 返回参数不完整 | ⬜ |

### 9.2 IP白名单
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| SC-010 | 白名单内IP | 从白名单IP调用 | 正常通过 | ⬜ |
| SC-011 | 白名单外IP | 从非白名单IP调用 | 返回IP不在白名单 | ⬜ |
| SC-012 | 关闭白名单 | 禁用IP白名单后调用 | 任何IP都可调用 | ⬜ |

### 9.3 Referer白名单
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| SC-020 | 白名单内Referer | 带白名单域名Referer | 正常通过 | ⬜ |
| SC-021 | 白名单外Referer | 带非白名单Referer | 返回来源不在白名单 | ⬜ |
| SC-022 | 空Referer | 不带Referer头 | 正常通过（允许空） | ⬜ |

### 9.4 JWT认证
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| SC-030 | 有效Token | 带有效Token访问 | 正常访问 | ⬜ |
| SC-031 | 无效Token | 带无效Token访问 | 返回401 | ⬜ |
| SC-032 | 过期Token | 带过期Token访问 | 返回401 | ⬜ |
| SC-033 | 无Token | 不带Token访问受保护接口 | 返回401 | ⬜ |

### 9.5 敏感数据保护
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| SC-040 | API日志脱敏 | 查看API日志 | sign/key/password显示为*** | ⬜ |
| SC-041 | 密钥查看权限 | 商户查看自己密钥 | 仅自己可查看 | ⬜ |

---

## 10. Telegram通知测试

### 10.1 Bot配置
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| TG-001 | 启用Bot | config.yaml设置telegram.enabled=true | Bot轮询启动 | |
| TG-002 | Bot Token无效 | 配置无效token后启动 | 记录错误,继续运行 | |
| TG-003 | 禁用Bot | telegram.enabled=false | 不启动Bot轮询 | |

### 10.2 商户绑定
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| TG-010 | /start命令 | 发送/start给Bot | 返回欢迎消息 | |
| TG-011 | /bind成功 | 发送/bind 1001 正确密钥 | 绑定成功,返回商户信息 | |
| TG-012 | /bind商户不存在 | 发送/bind 9999 xxx | 返回"商户不存在" | |
| TG-013 | /bind密钥错误 | 发送/bind 1001 错误密钥 | 返回"密钥错误" | |
| TG-014 | /bind参数不全 | 发送/bind 1001 | 返回参数不正确提示 | |
| TG-015 | /unbind解绑 | 发送/unbind | 解除绑定成功 | |
| TG-016 | /status查看状态 | 发送/status | 返回绑定状态 | |
| TG-017 | /help帮助 | 发送/help | 返回帮助信息 | |
| TG-018 | 重复绑定同商户 | 已绑定后再次绑定同商户 | 更新绑定,返回成功 | |

### 10.3 订单通知
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| TG-020 | 订单创建通知 | 创建新订单 | 收到订单创建通知 | |
| TG-021 | 订单支付通知 | 订单收到支付 | 收到支付成功通知 | |
| TG-022 | 订单过期通知 | 订单超时过期 | 收到订单过期通知 | |
| TG-023 | 未绑定不通知 | 未绑定商户创建订单 | 不发送通知 | |
| TG-024 | 关闭通知后 | 设置telegram_notify=false | 不发送通知 | |

### 10.4 提现通知
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| TG-030 | 提现申请通知 | 提交提现申请 | 收到申请提交通知 | |
| TG-031 | 提现审核通过 | 管理员审核通过 | 收到审核通过通知 | |
| TG-032 | 提现被拒绝 | 管理员拒绝提现 | 收到拒绝通知(含原因) | |
| TG-033 | 提现已打款 | 管理员完成打款 | 收到打款完成通知 | |

### 10.5 其他通知(预留)
| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| TG-040 | IP被封通知 | IP加入黑名单 | 收到IP封禁通知 | |
| TG-041 | 链状态变更 | 链启用/禁用 | 收到链状态变更通知 | |
| TG-042 | 余额低提醒 | 钱包余额低于阈值 | 收到余额低提醒 | |

---

## 11. 订单生命周期测试

| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| OL-001 | 创建订单 | 调用创建接口 | 状态为pending(0) | ⬜ |
| OL-002 | 订单支付 | 收到区块链转账 | 状态变为paid(1) | ⬜ |
| OL-003 | 订单过期 | 等待订单超时 | 状态变为expired(2) | ⬜ |
| OL-004 | 订单取消 | 调用关闭接口 | 状态变为cancelled(3) | ⬜ |
| OL-005 | 已支付不可取消 | 尝试取消已支付订单 | 返回错误 | ⬜ |
| OL-006 | 已过期不可支付 | 过期后收到转账 | 不更新订单状态 | ⬜ |

---

## 11. 性能测试

| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| PF-001 | 并发创建订单 | 100并发创建订单 | 响应时间<500ms | ⬜ |
| PF-002 | 订单查询性能 | 10万订单后查询 | 响应时间<100ms | ⬜ |
| PF-003 | API日志写入 | 高并发API调用 | 日志不丢失 | ⬜ |
| PF-004 | 区块链监控 | 长时间运行 | 内存稳定不增长 | ⬜ |

---

## 12. 异常测试

| 测试编号 | 测试场景 | 测试步骤 | 预期结果 | 状态 |
|---------|---------|---------|---------|------|
| EX-001 | 数据库断开 | 断开数据库连接 | 返回友好错误 | ⬜ |
| EX-002 | RPC节点不可用 | 配置无效RPC | 记录错误，继续运行 | ⬜ |
| EX-003 | 汇率API失败 | 汇率API不可用 | 使用上次有效汇率 | ⬜ |
| EX-004 | 非法参数 | 传入非法金额 | 返回参数错误 | ⬜ |

---

## 13. 测试工具

### 13.1 签名生成脚本
```bash
#!/bin/bash
# generate_sign.sh - 生成彩虹易支付签名

KEY="test_key_123456"
PID="1001"
TYPE="usdt_trc20"
OUT_TRADE_NO="test$(date +%s)"
MONEY="100"
NAME="TestOrder"
NOTIFY_URL="http://example.com/notify"

# 按ASCII排序拼接
SIGN_STR="money=${MONEY}&name=${NAME}&notify_url=${NOTIFY_URL}&out_trade_no=${OUT_TRADE_NO}&pid=${PID}&type=${TYPE}${KEY}"
SIGN=$(echo -n "$SIGN_STR" | md5sum | cut -d' ' -f1)

echo "Sign String: $SIGN_STR"
echo "Sign: $SIGN"
echo ""
echo "Full URL:"
echo "http://localhost:6088/mapi.php?pid=${PID}&type=${TYPE}&out_trade_no=${OUT_TRADE_NO}&notify_url=${NOTIFY_URL}&name=${NAME}&money=${MONEY}&sign=${SIGN}"
```

### 13.2 V免签签名脚本
```bash
#!/bin/bash
# generate_vmq_sign.sh - 生成V免签签名

KEY="test_key_123456"
PAY_ID="test$(date +%s)"
PARAM=""
TYPE="1"
PRICE="100"

# 签名: MD5(payId + param + type + price + key)
SIGN_STR="${PAY_ID}${PARAM}${TYPE}${PRICE}${KEY}"
SIGN=$(echo -n "$SIGN_STR" | md5sum | cut -d' ' -f1)

echo "Sign String: $SIGN_STR"
echo "Sign: $SIGN"
echo ""
echo "Full URL:"
echo "http://localhost:6088/createOrder?payId=${PAY_ID}&type=${TYPE}&price=${PRICE}&sign=${SIGN}&notifyUrl=http://example.com/notify"
```

---

## 14. 测试结果汇总

| 模块 | 总用例数 | 通过 | 失败 | 跳过 | 通过率 |
|------|---------|------|------|------|--------|
| 彩虹易支付接口 | 15 | - | - | - | - |
| V免签接口 | 13 | - | - | - | - |
| 收银台 | 8 | - | - | - | - |
| 管理后台 | 43 | - | - | - | - |
| 商户后台 | 18 | - | - | - | - |
| 区块链监控 | 13 | - | - | - | - |
| 回调通知 | 5 | - | - | - | - |
| 汇率服务 | 4 | - | - | - | - |
| 安全测试 | 16 | - | - | - | - |
| Telegram通知 | 21 | - | - | - | - |
| 订单生命周期 | 6 | - | - | - | - |
| 性能测试 | 4 | - | - | - | - |
| 异常测试 | 4 | - | - | - | - |
| **总计** | **170** | - | - | - | - |

---

## 15. 测试执行说明

### 15.1 测试前准备
1. 确保MySQL数据库运行正常
2. 执行数据库初始化脚本
3. 配置正确的config.yaml
4. 启动EzPay服务

### 15.2 执行测试
1. 按模块顺序执行测试用例
2. 记录每个用例的测试结果
3. 发现Bug立即记录并通知开发

### 15.3 测试报告
- 更新测试结果汇总表
- 记录发现的问题和修复状态
- 提交完整测试报告

---

**文档版本**: 1.2
**创建日期**: 2024-12-11
**最后更新**: 2025-12-11
**更新内容**:
- v1.2: 添加Telegram通知测试用例 (TG-001 ~ TG-042)
- v1.1: 添加IP黑名单管理测试用例 (AD-090 ~ AD-095)
