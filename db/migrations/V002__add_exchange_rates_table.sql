-- ========================================
-- V002: 添加汇率管理表
-- 描述: 创建汇率配置表，支持多币种汇率管理
-- 日期: 2026-01-30
-- ========================================

SET NAMES utf8mb4;

-- 创建汇率配置表
CREATE TABLE IF NOT EXISTS `exchange_rates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `from_currency` varchar(10) NOT NULL COMMENT '源货币',
  `to_currency` varchar(10) NOT NULL COMMENT '目标货币',
  `rate` decimal(18,8) NOT NULL COMMENT '基础汇率(中间价)',
  `rate_type` enum('manual','auto') DEFAULT 'manual' COMMENT '汇率类型: manual手动, auto自动',
  `source` varchar(50) DEFAULT NULL COMMENT '数据源(auto时): binance, okx等',
  `auto_update` tinyint(1) DEFAULT 0 COMMENT '是否启用自动更新: 1启用, 0禁用',
  `last_updated` datetime(3) DEFAULT NULL COMMENT '最后更新时间',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_from_to` (`from_currency`,`to_currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='汇率配置表';

-- 插入默认汇率配置
INSERT INTO `exchange_rates` (`from_currency`, `to_currency`, `rate`, `rate_type`, `auto_update`, `created_at`, `updated_at`) VALUES
-- 基础汇率（转换为 USD）
('EUR', 'USD', 1.08000000, 'auto', 1, NOW(3), NOW(3)),
('CNY', 'USD', 0.14000000, 'auto', 1, NOW(3), NOW(3)),

-- 支付汇率（从 USD 转换，用于用户支付和商户提现）
('USD', 'USDT', 1.00000000, 'auto', 1, NOW(3), NOW(3)),
('USD', 'TRX', 4.35000000, 'auto', 1, NOW(3), NOW(3)),
('USD', 'CNY', 7.20000000, 'auto', 1, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE
  `rate` = VALUES(`rate`),
  `updated_at` = NOW(3);

-- 添加买入/卖出浮动配置（如果不存在）
INSERT INTO `system_configs` (`key`, `value`, `description`, `created_at`, `updated_at`) VALUES
('rate_buy_float', '0.02', '买入汇率浮动（用户支付时加价），如0.02表示+2%', NOW(3), NOW(3)),
('rate_sell_float', '0.02', '卖出汇率浮动（商户提现时减价），如0.02表示-2%', NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE
  `description` = VALUES(`description`),
  `updated_at` = NOW(3);

SELECT 'V002: 汇率管理表创建成功' AS message;
