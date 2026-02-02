# EzPay 数据库版本管理系统

## 📋 概述

基于版本号的数据库迁移管理系统，类似于 Flyway/Liquibase，支持：

- ✅ 版本追踪：记录每次迁移的版本、时间、状态
- ✅ 增量更新：只执行尚未应用的迁移
- ✅ 幂等性：可重复执行，自动跳过已应用迁移
- ✅ 自动备份：执行前自动备份数据库
- ✅ 校验和：确保迁移文件未被篡改
- ✅ 回滚支持：出错时可快速回滚

## 🗂️ 目录结构

```
db/
├── migrate.sh                    # 迁移工具主脚本
└── migrations/                   # 迁移脚本目录
    ├── README.md                 # 本文档
    ├── V001__initial_schema.sql # 版本1: 初始化
    ├── V002__usd_settlement_system.sql  # 版本2: USD结算
    └── V003__cleanup_deprecated_fields.sql  # 版本3: 清理
```

## 📝 迁移文件命名规范

```
V{版本号}__{描述}.sql
```

- **版本号**：3位数字，如 001, 002, 003（自动递增）
- **描述**：英文，使用下划线分隔单词
- **扩展名**：必须是 `.sql`

**示例**：
- `V001__initial_schema.sql` - 初始化数据库结构
- `V002__usd_settlement_system.sql` - USD结算体系
- `V003__cleanup_deprecated_fields.sql` - 清理废弃字段
- `V004__add_user_roles.sql` - 添加用户角色功能

## 🚀 使用方法

### 1. 查看当前状态

```bash
./db/migrate.sh status
```

输出示例：
```
========================================
数据库迁移状态
========================================
当前版本: V002
已应用迁移: 2

待执行迁移 (1):
  V003 - cleanup deprecated fields
```

### 2. 执行迁移

```bash
./db/migrate.sh migrate
# 或
./db/migrate.sh up
```

执行流程：
1. ✅ 检查版本管理表（不存在则自动创建）
2. ✅ 显示当前状态和待执行迁移
3. ✅ 询问确认
4. ✅ 自动备份数据库
5. ✅ 按版本顺序执行迁移
6. ✅ 记录执行结果
7. ✅ 显示最终状态

### 3. 查看迁移历史

```bash
./db/migrate.sh history
```

输出示例：
```
========================================
迁移历史
========================================
版本       描述                           执行时间                  耗时(ms)    状态
---------- ------------------------------ ------------------------- ------------ ----------
V001       initial schema                 2026-01-30 10:00:15       156          SUCCESS
V002       usd settlement system          2026-01-30 10:00:16       342          SUCCESS
V003       cleanup deprecated fields      2026-01-30 10:00:17       89           SUCCESS
```

### 4. 帮助信息

```bash
./db/migrate.sh help
```

## ✍️ 创建新的迁移

### 步骤1：创建迁移文件

```bash
# 确定下一个版本号（查看现有最大版本号 + 1）
cd db/migrations
ls -1 V*.sql | tail -1  # 查看最新版本

# 创建新迁移文件
vim V004__add_payment_channels.sql
```

### 步骤2：编写迁移脚本

```sql
-- ========================================
-- V004: 添加支付渠道管理
-- 描述: 新增多支付渠道支持
-- 日期: 2026-01-31
-- ========================================

SET NAMES utf8mb4;

-- 创建支付渠道表
CREATE TABLE IF NOT EXISTS `payment_channels` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) NOT NULL,
  `code` varchar(20) NOT NULL,
  `status` tinyint DEFAULT 1,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 插入默认数据
INSERT INTO `payment_channels` (`name`, `code`) VALUES
  ('微信支付', 'wechat'),
  ('支付宝', 'alipay');

SELECT 'V004: 支付渠道管理添加成功' AS message;
```

### 步骤3：执行迁移

```bash
cd ../..  # 回到项目根目录
./db/migrate.sh migrate
```

## 🛡️ 最佳实践

### 1. 编写迁移脚本

**DO（推荐）**：
- ✅ 使用幂等性检查（IF NOT EXISTS、IF EXISTS）
- ✅ 添加详细注释说明
- ✅ 使用事务（如果支持）
- ✅ 测试回滚脚本
- ✅ 先在测试环境验证

**DON'T（避免）**：
- ❌ 直接修改已应用的迁移文件
- ❌ 删除已应用的迁移文件
- ❌ 跳过版本号
- ❌ 在生产环境直接测试

### 2. 版本号管理

- 版本号**必须递增**：V001, V002, V003...
- 版本号**不可重复**：每个版本唯一
- 版本号**不可跳跃**：连续编号
- 多个开发者协作时，提前协调版本号

### 3. 迁移策略

**小步快跑**：
- 每次迁移只做一件事
- 拆分大的迁移为多个小迁移
- 便于问题定位和回滚

**向后兼容**：
- 先添加字段，再删除字段（分两个版本）
- 保留旧字段一段时间用于兼容
- 数据迁移与结构迁移分离

## 🔄 回滚

### 自动回滚（出错时）

迁移失败时，工具会自动提示回滚命令：

```bash
mysql -h 172.16.1.10 -u ezpay -p'Admin3579' --skip-ssl ezpay < backup_20260130_100015.sql
```

### 手动回滚

1. **查找备份文件**：
   ```bash
   ls -lt backup_*.sql | head -5
   ```

2. **确认备份内容**（可选）：
   ```bash
   head backup_20260130_100015.sql
   ```

3. **执行回滚**：
   ```bash
   mysql -h 172.16.1.10 -u ezpay -p'Admin3579' --skip-ssl ezpay < backup_20260130_100015.sql
   ```

4. **清理失败记录**（可选）：
   ```sql
   DELETE FROM database_migrations WHERE status = 'FAILED';
   ```

## 📊 版本管理表结构

```sql
CREATE TABLE `database_migrations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `version` varchar(50) NOT NULL,              -- 版本号 V001
  `description` varchar(200) NOT NULL,         -- 描述
  `script_name` varchar(100) NOT NULL,         -- 文件名
  `checksum` varchar(64) DEFAULT NULL,         -- SHA256校验和
  `executed_at` datetime(3) NOT NULL,          -- 执行时间
  `execution_time` int DEFAULT 0,              -- 耗时(ms)
  `status` enum('SUCCESS','FAILED','ROLLBACK'), -- 状态
  `error_message` text,                        -- 错误信息
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_version` (`version`)
) ENGINE=InnoDB;
```

## 🔍 故障排除

### 问题1：迁移表不存在

**症状**：首次运行提示表不存在

**解决**：工具会自动创建，无需手动处理

### 问题2：迁移失败

**症状**：SQL 执行出错

**解决步骤**：
1. 查看错误信息
2. 修复迁移脚本（需要新版本号）
3. 或者回滚到备份
4. 重新执行

### 问题3：版本冲突

**症状**：多人开发时版本号重复

**解决**：
1. 重命名迁移文件（使用下一个可用版本号）
2. 更新文件内的版本注释
3. 提交代码前同步最新版本

### 问题4：校验和不匹配

**症状**：迁移文件被修改

**解决**：
- 已应用的迁移**不应修改**
- 如需修改，创建新的迁移版本

## 📚 常见场景

### 场景1：首次部署

```bash
# 1. 查看状态
./db/migrate.sh status

# 2. 执行所有迁移
./db/migrate.sh migrate
```

### 场景2：日常更新

```bash
# 开发者添加新迁移后
git pull
./db/migrate.sh status    # 查看新迁移
./db/migrate.sh migrate   # 应用更新
```

### 场景3：生产环境升级

```bash
# 1. 备份（额外备份）
mysqldump -h xxx -u xxx -pxxx ezpay > manual_backup.sql

# 2. 查看待执行迁移
./db/migrate.sh status

# 3. 在测试环境先验证
# ... 测试 ...

# 4. 生产环境执行
./db/migrate.sh migrate

# 5. 验证应用
./db/migrate.sh history
```

## 🎯 总结

- 📦 **版本化管理**：每次数据库变更都有版本追踪
- 🔄 **自动化执行**：工具自动检测并执行待应用迁移
- 🛡️ **安全保障**：执行前备份，失败可回滚
- 📊 **可追溯性**：完整的执行历史记录
- 🚀 **简单易用**：一键命令完成迁移

## 📞 支持

遇到问题请查看：
- 项目文档：`/docs`
- 迁移历史：`./db/migrate.sh history`
- 数据库日志：`/var/log/mysql/error.log`
