-- ========================================
-- V003: 添加汇率更新记录表
-- 描述: 记录汇率的更新历史，用于追踪和分析
-- 日期: 2026-01-30
-- ========================================

SET NAMES utf8mb4;

-- 创建汇率更新记录表
CREATE TABLE IF NOT EXISTS `exchange_rate_history` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `rate_id` bigint unsigned NOT NULL COMMENT '汇率ID（关联 exchange_rates.id）',
  `from_currency` varchar(10) NOT NULL COMMENT '源货币',
  `to_currency` varchar(10) NOT NULL COMMENT '目标货币',
  `old_rate` decimal(18,8) DEFAULT NULL COMMENT '旧汇率',
  `new_rate` decimal(18,8) NOT NULL COMMENT '新汇率',
  `change_percent` decimal(8,4) DEFAULT NULL COMMENT '变化百分比',
  `update_source` varchar(50) DEFAULT NULL COMMENT '更新来源: auto自动, manual手动, api接口',
  `updated_by` varchar(50) DEFAULT NULL COMMENT '更新者: system, admin, api',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_rate_id` (`rate_id`),
  KEY `idx_created_at` (`created_at`),
  KEY `idx_from_to` (`from_currency`,`to_currency`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='汇率更新记录表';

SELECT 'V003: 汇率更新记录表创建成功' AS message;
