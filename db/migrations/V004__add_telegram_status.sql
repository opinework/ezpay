-- ========================================
-- V004: 添加 Telegram 状态字段
-- 描述: 为 merchants 表添加 telegram_status 字段，用于标识 Telegram 账号状态
-- 日期: 2026-01-30
-- ========================================

SET NAMES utf8mb4;

-- 添加 telegram_status 字段
ALTER TABLE `merchants` ADD COLUMN `telegram_status` varchar(20) DEFAULT 'normal' COMMENT 'Telegram状态: normal正常, blocked被封禁, unbound未绑定' AFTER `telegram_notify`;

-- 更新现有数据：未绑定的设置为 unbound，已绑定的设置为 normal
UPDATE `merchants` SET `telegram_status` = 'unbound' WHERE `telegram_chat_id` = 0 OR `telegram_chat_id` IS NULL;
UPDATE `merchants` SET `telegram_status` = 'normal' WHERE `telegram_chat_id` > 0;

SELECT 'V004: Telegram状态字段添加成功' AS message;
