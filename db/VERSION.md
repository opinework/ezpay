# EzPay 数据库版本管理

## 📌 当前版本

- **版本**: V003
- **日期**: 2026-01-30
- **描述**: 汇率更新历史记录表

## 🚀 快速开始

### 首次部署

```bash
# 执行数据库迁移（会自动应用 V001）
./db/migrate.sh migrate
```

### 日常更新

当开发者添加新的迁移文件后：

```bash
# 1. 拉取最新代码
git pull

# 2. 查看待执行的迁移
./db/migrate.sh status

# 3. 应用更新
./db/migrate.sh migrate
```

## 📝 添加新的迁移

### 1. 创建迁移文件

```bash
# 版本号规则：V{三位数字}__{描述}.sql
# 示例：V002__add_payment_methods.sql

cd db/migrations
vim V002__add_payment_methods.sql
```

### 2. 编写迁移脚本

```sql
-- ========================================
-- V002: 添加支付方式管理
-- 描述: 新增支付方式配置表
-- 日期: 2026-01-31
-- ========================================

SET NAMES utf8mb4;

-- 创建支付方式表
CREATE TABLE IF NOT EXISTS `payment_methods` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `code` varchar(20) NOT NULL COMMENT '支付方式代码',
  `name` varchar(50) NOT NULL COMMENT '支付方式名称',
  `enabled` tinyint DEFAULT 1 COMMENT '是否启用: 1启用 0禁用',
  `sort_order` int DEFAULT 0 COMMENT '排序',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='支付方式表';

-- 插入默认数据
INSERT INTO `payment_methods` (`code`, `name`, `sort_order`) VALUES
  ('wechat', '微信支付', 1),
  ('alipay', '支付宝', 2),
  ('usdt_trc20', 'USDT-TRC20', 3);

SELECT 'V002: 支付方式管理添加成功' AS message;
```

### 3. 执行迁移

```bash
cd ../..  # 回到项目根目录
./db/migrate.sh migrate
```

## 📊 版本历史

| 版本 | 日期 | 描述 | 文件 |
|------|------|------|------|
| V001 | 2026-01-30 | 完整初始数据库结构（USD结算体系） | V001__initial_complete_schema.sql |
| V002 | 2026-01-30 | 汇率管理表和买入/卖出浮动配置 | V002__add_exchange_rates_table.sql |
| V003 | 2026-01-30 | 汇率更新历史记录表 | V003__add_exchange_rate_history.sql |

## 🛠️ 常用命令

```bash
# 查看当前版本和待执行迁移
./db/migrate.sh status

# 执行所有待执行的迁移
./db/migrate.sh migrate

# 查看迁移历史
./db/migrate.sh history

# 帮助信息
./db/migrate.sh help
```

## 📖 注意事项

1. **版本号连续**：从 V001 开始，每个新迁移递增 1（V002, V003...）
2. **幂等性**：所有迁移脚本应支持重复执行（使用 IF NOT EXISTS 等）
3. **字段注释**：所有字段必须添加 COMMENT 注释说明用途
4. **备份优先**：迁移工具会自动备份，但生产环境建议额外手动备份
5. **测试先行**：在测试环境验证后再应用到生产环境

## 🎯 设计原则

- ✅ **版本化**: 每次数据库变更都有唯一版本号
- ✅ **可追溯**: 完整的迁移历史记录
- ✅ **自动化**: 工具自动检测和执行待应用迁移
- ✅ **安全性**: 执行前自动备份，失败可回滚
- ✅ **注释完整**: 所有表和字段都有详细注释

## 🔍 故障排除

### 问题：迁移失败

**解决步骤**：
1. 查看错误信息
2. 使用备份文件回滚：
   ```bash
   mysql -h 172.16.1.10 -u ezpay -p'Admin3579' --skip-ssl ezpay < backup_YYYYMMDD_HHMMSS.sql
   ```
3. 修复SQL脚本（创建新版本）
4. 重新执行迁移

### 问题：版本冲突

**场景**：多人开发时版本号重复

**解决**：
- 重命名文件使用下一个可用版本号
- 提交前先拉取最新代码检查版本号

## 📚 相关文档

- 详细使用指南: [migrations/README.md](migrations/README.md)
- 迁移工具脚本: [migrate.sh](migrate.sh)
