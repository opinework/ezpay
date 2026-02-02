-- ========================================
-- V001: EzPay 完整初始数据库结构  
-- 描述: 包含所有表、字段、索引和初始配置（包含 USD 结算体系）
-- 日期: 2026-01-30
-- ========================================

SET NAMES utf8mb4;

-- EzPay 数据库初始化脚本
-- 创建数据库: CREATE DATABASE ezpay DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- =====================================================
-- 表结构 (GORM会自动创建，此处仅供参考和手动初始化)
-- =====================================================

-- 管理员表
CREATE TABLE IF NOT EXISTS `admins` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(50) NOT NULL,
  `password` varchar(100) NOT NULL,
  `status` tinyint DEFAULT '1' COMMENT '1:启用 0:禁用',
  `last_login` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_admins_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 商户表
CREATE TABLE IF NOT EXISTS `merchants` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `p_id` varchar(50) NOT NULL COMMENT '商户PID',
  `name` varchar(100) NOT NULL COMMENT '商户名称',
  `key` varchar(100) NOT NULL COMMENT '商户密钥',
  `password` varchar(100) DEFAULT '' COMMENT '登录密码',
  `email` varchar(100) DEFAULT '' COMMENT '邮箱',
  `balance` double DEFAULT '0' COMMENT '账户余额',
  `frozen_balance` double DEFAULT '0' COMMENT '冻结余额',
  `fee_rate` decimal(10,4) DEFAULT '0.0200' COMMENT '手续费率',
  `status` tinyint DEFAULT '1' COMMENT '1:启用 0:禁用',
  `wallet_mode` tinyint DEFAULT '1' COMMENT '1:仅系统钱包 2:仅个人钱包 3:混合模式',
  `wallet_limit` int DEFAULT '10' COMMENT '钱包数量限制',
  `ip_whitelist` text COMMENT 'IP白名单(JSON数组)',
  `ip_whitelist_enabled` tinyint DEFAULT '0' COMMENT '是否启用IP白名单',
  `referer_whitelist` text COMMENT 'Referer白名单(JSON数组)',
  `referer_whitelist_enabled` tinyint DEFAULT '0' COMMENT '是否启用Referer白名单',
  `telegram_chat_id` bigint DEFAULT '0' COMMENT 'Telegram Chat ID',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_merchants_p_id` (`p_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 钱包表
CREATE TABLE IF NOT EXISTS `wallets` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `merchant_id` bigint unsigned DEFAULT '0' COMMENT '0:系统钱包 >0:商户钱包',
  `chain` varchar(20) NOT NULL COMMENT '链类型: trc20,erc20,bep20,trx,wechat,alipay,redotpay',
  `address` varchar(500) NOT NULL COMMENT '钱包地址或支付链接',
  `label` varchar(50) DEFAULT '' COMMENT '标签',
  `qr_code` varchar(500) DEFAULT '' COMMENT '收款码图片路径',
  `status` tinyint DEFAULT '1' COMMENT '1:启用 0:禁用',
  `last_used_at` datetime(3) DEFAULT NULL COMMENT '最后使用时间(用于轮询)',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_merchant_chain_address` (`merchant_id`,`chain`,`address`(255)),
  KEY `idx_wallets_deleted_at` (`deleted_at`),
  KEY `idx_wallets_last_used_at` (`last_used_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 订单表
CREATE TABLE IF NOT EXISTS `orders` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `trade_no` varchar(50) NOT NULL COMMENT '平台订单号',
  `out_trade_no` varchar(100) NOT NULL COMMENT '商户订单号',
  `merchant_id` bigint unsigned NOT NULL COMMENT '商户ID',
  `wallet_id` bigint unsigned DEFAULT '0' COMMENT '钱包ID',
  `type` varchar(20) DEFAULT '' COMMENT '支付类型',
  `chain` varchar(20) NOT NULL COMMENT '链类型',
  `name` varchar(200) DEFAULT '' COMMENT '商品名称',
  `money` decimal(20,2) NOT NULL COMMENT '订单金额(CNY)',
  `usdt_amount` decimal(20,8) DEFAULT '0.00000000' COMMENT 'USDT金额',
  `actual_amount` decimal(20,8) DEFAULT '0.00000000' COMMENT '实际支付金额',
  `rate` decimal(20,4) DEFAULT '0.0000' COMMENT '汇率',
  `fee` decimal(20,8) DEFAULT '0.00000000' COMMENT '手续费',
  `fee_type` tinyint DEFAULT '0' COMMENT '手续费类型: 0:从订单扣除 1:从余额扣除',
  `to_address` varchar(500) DEFAULT '' COMMENT '收款地址',
  `from_address` varchar(200) DEFAULT '' COMMENT '付款地址',
  `tx_hash` varchar(200) DEFAULT '' COMMENT '交易哈希',
  `qr_code` varchar(500) DEFAULT '' COMMENT '收款码',
  `status` tinyint DEFAULT '0' COMMENT '0:待支付 1:已支付 2:已过期 3:已取消',
  `notify_url` varchar(500) DEFAULT '' COMMENT '回调地址',
  `notify_status` tinyint DEFAULT '0' COMMENT '0:未通知 1:已通知 2:通知失败',
  `notify_count` int DEFAULT '0' COMMENT '通知次数',
  `return_url` varchar(500) DEFAULT '' COMMENT '返回地址',
  `param` text COMMENT '附加参数',
  `channel` varchar(50) DEFAULT 'local' COMMENT '支付通道',
  `upstream_order_id` varchar(100) DEFAULT '' COMMENT '上游订单号',
  `expired_at` datetime(3) DEFAULT NULL COMMENT '过期时间',
  `paid_at` datetime(3) DEFAULT NULL COMMENT '支付时间',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_orders_trade_no` (`trade_no`),
  KEY `idx_orders_merchant_id` (`merchant_id`),
  KEY `idx_orders_out_trade_no` (`out_trade_no`),
  KEY `idx_orders_status` (`status`),
  KEY `idx_orders_created_at` (`created_at`),
  KEY `idx_orders_expired_at` (`expired_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 系统配置表
CREATE TABLE IF NOT EXISTS `system_configs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `key` varchar(50) NOT NULL,
  `value` text,
  `description` varchar(200) DEFAULT '',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_system_configs_key` (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 链路配置表
CREATE TABLE IF NOT EXISTS `chain_configs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `chain` varchar(20) NOT NULL COMMENT '链标识',
  `name` varchar(50) NOT NULL COMMENT '链名称',
  `enabled` tinyint DEFAULT '1' COMMENT '是否启用',
  `rpc_url` varchar(500) DEFAULT '' COMMENT 'RPC地址',
  `contract_address` varchar(100) DEFAULT '' COMMENT '合约地址',
  `decimals` int DEFAULT '6' COMMENT '精度',
  `min_amount` decimal(20,8) DEFAULT '0.00000000' COMMENT '最小金额',
  `max_amount` decimal(20,8) DEFAULT '0.00000000' COMMENT '最大金额',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_chain_configs_chain` (`chain`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 提现记录表
CREATE TABLE IF NOT EXISTS `withdrawals` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `merchant_id` bigint unsigned NOT NULL,
  `amount` decimal(20,8) NOT NULL COMMENT '提现金额',
  `fee` decimal(20,8) DEFAULT '0.00000000' COMMENT '手续费',
  `real_amount` decimal(20,8) DEFAULT '0.00000000' COMMENT '实际到账',
  `pay_method` varchar(20) DEFAULT '' COMMENT '提现方式',
  `address` varchar(200) DEFAULT '' COMMENT '提现地址',
  `tx_hash` varchar(200) DEFAULT '' COMMENT '交易哈希',
  `status` tinyint DEFAULT '0' COMMENT '0:待审核 1:已审核 2:已拒绝 3:已打款',
  `remark` varchar(500) DEFAULT '' COMMENT '备注',
  `admin_remark` varchar(500) DEFAULT '' COMMENT '管理员备注',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_withdrawals_merchant_id` (`merchant_id`),
  KEY `idx_withdrawals_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 提现地址表
CREATE TABLE IF NOT EXISTS `withdraw_addresses` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `merchant_id` bigint unsigned NOT NULL,
  `chain` varchar(20) NOT NULL COMMENT '链类型',
  `address` varchar(200) NOT NULL COMMENT '地址',
  `label` varchar(50) DEFAULT '' COMMENT '标签',
  `status` tinyint DEFAULT '0' COMMENT '0:待审核 1:已审核 2:已拒绝',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_withdraw_addresses_merchant_id` (`merchant_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- API日志表
CREATE TABLE IF NOT EXISTS `api_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `merchant_pid` varchar(50) DEFAULT '' COMMENT '商户PID',
  `endpoint` varchar(100) DEFAULT '' COMMENT '接口路径',
  `method` varchar(10) DEFAULT '' COMMENT '请求方法',
  `client_ip` varchar(50) DEFAULT '' COMMENT '客户端IP',
  `request_data` text COMMENT '请求数据',
  `response_code` int DEFAULT '0' COMMENT '响应码',
  `response_msg` varchar(500) DEFAULT '' COMMENT '响应消息',
  `trade_no` varchar(50) DEFAULT '' COMMENT '订单号',
  `duration` int DEFAULT '0' COMMENT '耗时(ms)',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_api_logs_merchant_pid` (`merchant_pid`),
  KEY `idx_api_logs_created_at` (`created_at`),
  KEY `idx_api_logs_trade_no` (`trade_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- IP黑名单表
CREATE TABLE IF NOT EXISTS `ip_blacklists` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `ip` varchar(50) NOT NULL,
  `reason` varchar(200) DEFAULT '' COMMENT '封禁原因',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_ip_blacklists_ip` (`ip`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 交易日志表
CREATE TABLE IF NOT EXISTS `transaction_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `merchant_id` bigint unsigned NOT NULL,
  `type` varchar(20) NOT NULL COMMENT '类型: order_income,withdraw,fee,refund等',
  `amount` decimal(20,8) NOT NULL COMMENT '金额',
  `balance_before` decimal(20,8) DEFAULT '0.00000000' COMMENT '变动前余额',
  `balance_after` decimal(20,8) DEFAULT '0.00000000' COMMENT '变动后余额',
  `related_id` varchar(100) DEFAULT '' COMMENT '关联ID',
  `remark` varchar(500) DEFAULT '' COMMENT '备注',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_transaction_logs_merchant_id` (`merchant_id`),
  KEY `idx_transaction_logs_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================
-- 初始化数据
-- =====================================================

-- 初始化系统商户 (id=0, 用于系统钱包)
SET SESSION sql_mode = 'NO_AUTO_VALUE_ON_ZERO';
INSERT INTO `merchants` (`id`, `p_id`, `name`, `key`, `password`, `status`, `created_at`, `updated_at`)
VALUES (0, 'SYSTEM', '系统钱包', 'system_key', '', 1, NOW(), NOW())
ON DUPLICATE KEY UPDATE `p_id` = 'SYSTEM';
SET SESSION sql_mode = '';

-- 初始化默认管理员 (密码: admin123)
INSERT INTO `admins` (`username`, `password`, `status`, `created_at`, `updated_at`)
VALUES ('admin', '$2a$10$xiL.DqGTWgs4Sxv99TBxOeUMySHTXe5K2LtTgvtUTNc6wdChhRd7G', 1, NOW(), NOW())
ON DUPLICATE KEY UPDATE `username` = 'admin';

-- 初始化默认商户 (PID: 10001, 密码: merchant123, 密钥: test_key_123456)
INSERT INTO `merchants` (`p_id`, `name`, `key`, `password`, `status`, `wallet_mode`, `wallet_limit`, `created_at`, `updated_at`)
VALUES ('10001', '默认商户', 'test_key_123456', '$2a$10$ZfUDWHWqrRcGn1mFlMklLudfG4rUnmoIwqaGFMm9ZBSg9CYbLRbvC', 1, 1, 10, NOW(), NOW())
ON DUPLICATE KEY UPDATE `p_id` = '10001';

-- 初始化系统配置
INSERT INTO `system_configs` (`key`, `value`, `description`, `created_at`, `updated_at`) VALUES
('rate_mode', 'hybrid', '汇率模式: auto/manual/hybrid', NOW(), NOW()),
('manual_rate', '7.2', '手动设置的汇率', NOW(), NOW()),
('float_percent', '0', '汇率浮动百分比', NOW(), NOW()),
('order_expire', '30', '订单过期时间(分钟)', NOW(), NOW()),
('notify_retry', '5', '通知重试次数', NOW(), NOW()),
('site_name', 'EzPay', '网站名称', NOW(), NOW()),
('system_wallet_fee_rate', '0.02', '系统收款码手续费率', NOW(), NOW()),
('personal_wallet_fee_rate', '0.01', '个人收款码手续费率', NOW(), NOW())
ON DUPLICATE KEY UPDATE `key` = VALUES(`key`);

-- 初始化链路配置
INSERT INTO `chain_configs` (`chain`, `name`, `enabled`, `decimals`, `created_at`, `updated_at`) VALUES
('trc20', 'TRC20 (Tron)', 1, 6, NOW(), NOW()),
('erc20', 'ERC20 (Ethereum)', 1, 6, NOW(), NOW()),
('bep20', 'BEP20 (BSC)', 1, 18, NOW(), NOW()),
('polygon', 'Polygon', 0, 6, NOW(), NOW()),
('optimism', 'Optimism', 0, 6, NOW(), NOW()),
('arbitrum', 'Arbitrum', 0, 6, NOW(), NOW()),
('avalanche', 'Avalanche', 0, 6, NOW(), NOW()),
('base', 'Base', 0, 6, NOW(), NOW()),
('trx', 'TRX (Tron原生)', 1, 6, NOW(), NOW()),
('wechat', '微信支付', 1, 2, NOW(), NOW()),
('alipay', '支付宝', 1, 2, NOW(), NOW()),
('redotpay', 'RedotPay', 0, 2, NOW(), NOW())
ON DUPLICATE KEY UPDATE `chain` = VALUES(`chain`);

-- =====================================================
-- 默认账号信息
-- =====================================================
-- 管理员: admin / admin123
-- 商户: 10001 / merchant123 (密钥: test_key_123456)

-- ========================================
-- V001 扩展：版本管理表
-- ========================================

CREATE TABLE IF NOT EXISTS `database_migrations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `version` varchar(50) NOT NULL COMMENT '版本号 (如: V001, V002)',
  `description` varchar(200) NOT NULL COMMENT '迁移描述',
  `script_name` varchar(100) NOT NULL COMMENT '脚本文件名',
  `checksum` varchar(64) DEFAULT NULL COMMENT 'SHA256校验和',
  `executed_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '执行时间',
  `execution_time` int DEFAULT 0 COMMENT '执行耗时（毫秒）',
  `status` enum('SUCCESS','FAILED','ROLLBACK') NOT NULL DEFAULT 'SUCCESS' COMMENT '执行状态',
  `error_message` text COMMENT '错误信息',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_version` (`version`),
  KEY `idx_executed_at` (`executed_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='数据库迁移版本管理表';

-- ========================================
-- V001 扩展：为 orders 表添加 USD 结算体系字段
-- ========================================

-- 添加 currency 字段
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'orders'
  AND COLUMN_NAME = 'currency';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `orders` ADD COLUMN `currency` VARCHAR(10) DEFAULT ''USD'' COMMENT ''原始货币: USD, EUR, CNY'' AFTER `name`',
    'SELECT ''currency already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 pay_amount 字段
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'orders'
  AND COLUMN_NAME = 'pay_amount';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `orders` ADD COLUMN `pay_amount` DECIMAL(20,8) DEFAULT 0.00000000 COMMENT ''用户应支付金额(展示用，无偏移)'' AFTER `usdt_amount`',
    'SELECT ''pay_amount already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 unique_amount 字段  
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'orders'
  AND COLUMN_NAME = 'unique_amount';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `orders` ADD COLUMN `unique_amount` DECIMAL(20,8) DEFAULT 0.00000000 COMMENT ''订单标识金额(含偏移，用于匹配)'' AFTER `pay_amount`',
    'SELECT ''unique_amount already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 settlement_amount 字段
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'orders'
  AND COLUMN_NAME = 'settlement_amount';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `orders` ADD COLUMN `settlement_amount` DECIMAL(20,8) DEFAULT 0.00000000 COMMENT ''结算金额（USD，计入商户余额）'' AFTER `unique_amount`',
    'SELECT ''settlement_amount already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 channel_order_id 字段
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'orders'
  AND COLUMN_NAME = 'channel_order_id';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `orders` ADD COLUMN `channel_order_id` VARCHAR(100) DEFAULT '''' COMMENT ''上游通道订单号'' AFTER `channel`',
    'SELECT ''channel_order_id already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 channel_pay_url 字段
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'orders'
  AND COLUMN_NAME = 'channel_pay_url';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `orders` ADD COLUMN `channel_pay_url` VARCHAR(500) DEFAULT '''' COMMENT ''上游通道支付URL'' AFTER `channel_order_id`',
    'SELECT ''channel_pay_url already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加索引
SET @index_exists = 0;
SELECT COUNT(*) INTO @index_exists
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'orders'
  AND INDEX_NAME = 'idx_orders_unique_amount';

SET @sql = IF(@index_exists = 0,
    'ALTER TABLE `orders` ADD INDEX `idx_orders_unique_amount` (`unique_amount`)',
    'SELECT ''idx_orders_unique_amount already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @index_exists = 0;
SELECT COUNT(*) INTO @index_exists
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'orders'
  AND INDEX_NAME = 'idx_orders_settlement_amount';

SET @sql = IF(@index_exists = 0,
    'ALTER TABLE `orders` ADD INDEX `idx_orders_settlement_amount` (`settlement_amount`)',
    'SELECT ''idx_orders_settlement_amount already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- ========================================
-- V001 扩展：为 withdrawals 表添加字段
-- ========================================

-- 添加 payout_amount 字段
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'withdrawals'
  AND COLUMN_NAME = 'payout_amount';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `withdrawals` ADD COLUMN `payout_amount` DECIMAL(20,8) DEFAULT 0.00000000 COMMENT ''实际打款金额（USDT/TRX等）'' AFTER `real_amount`',
    'SELECT ''payout_amount already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 payout_currency 字段
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'withdrawals'
  AND COLUMN_NAME = 'payout_currency';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `withdrawals` ADD COLUMN `payout_currency` VARCHAR(20) DEFAULT '''' COMMENT ''打款货币: USDT, TRX等'' AFTER `payout_amount`',
    'SELECT ''payout_currency already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 payout_rate 字段
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'withdrawals'
  AND COLUMN_NAME = 'payout_rate';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `withdrawals` ADD COLUMN `payout_rate` DECIMAL(20,4) DEFAULT 0.0000 COMMENT ''打款汇率（卖出汇率）'' AFTER `payout_currency`',
    'SELECT ''payout_rate already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- ========================================
-- V001 扩展：添加系统配置
-- ========================================

-- 买入汇率浮动配置
INSERT INTO `system_configs` (`key`, `value`, `description`, `created_at`, `updated_at`)
VALUES ('rate_buy_float', '0.02', '买入汇率浮动（用户支付时，平台收入）', NOW(), NOW())
ON DUPLICATE KEY UPDATE `updated_at` = NOW();

-- 卖出汇率浮动配置
INSERT INTO `system_configs` (`key`, `value`, `description`, `created_at`, `updated_at`)
VALUES ('rate_sell_float', '0.02', '卖出汇率浮动（商户提现时，平台支出）', NOW(), NOW())
ON DUPLICATE KEY UPDATE `updated_at` = NOW();

-- ========================================
-- V001 完成
-- ========================================

SELECT 'V001: EzPay 完整初始数据库结构创建成功' AS message;

-- ========================================
-- V001 扩展：修复 system_configs 表
-- ========================================

-- 添加 created_at 字段（如果不存在）
SET @col_exists = 0;
SELECT COUNT(*) INTO @col_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'system_configs'
  AND COLUMN_NAME = 'created_at';

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `system_configs` ADD COLUMN `created_at` datetime(3) DEFAULT NULL COMMENT ''创建时间'' AFTER `description`',
    'SELECT ''created_at already exists'' AS msg');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
