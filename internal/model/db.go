package model

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// DBConfig 数据库连接池配置
type DBConfig struct {
	MaxOpenConns    int           // 最大打开连接数
	MaxIdleConns    int           // 最大空闲连接数
	ConnMaxLifetime time.Duration // 连接最大生命周期
	ConnMaxIdleTime time.Duration // 空闲连接最大生命周期
}

// DefaultDBConfig 默认数据库配置
var DefaultDBConfig = DBConfig{
	MaxOpenConns:    100,              // 最大100个连接
	MaxIdleConns:    10,               // 最大10个空闲连接
	ConnMaxLifetime: time.Hour,        // 连接最长1小时
	ConnMaxIdleTime: 10 * time.Minute, // 空闲连接最长10分钟
}

// InitDB 初始化数据库连接
func InitDB(dsn string) error {
	return InitDBWithConfig(dsn, DefaultDBConfig)
}

// InitDBWithConfig 使用自定义配置初始化数据库连接
func InitDBWithConfig(dsn string, cfg DBConfig) error {
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn), // 生产环境使用 Warn 级别
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// 配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// 验证连接
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// 自动迁移
	if err := autoMigrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// 初始化默认数据
	if err := initDefaultData(); err != nil {
		return fmt.Errorf("failed to init default data: %w", err)
	}

	log.Printf("Database connected (MaxOpen: %d, MaxIdle: %d)", cfg.MaxOpenConns, cfg.MaxIdleConns)
	return nil
}

// GetDBStats 获取数据库连接池状态
func GetDBStats() map[string]interface{} {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return nil
	}
	stats := sqlDB.Stats()
	return map[string]interface{}{
		"max_open_connections": stats.MaxOpenConnections,
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration":        stats.WaitDuration.String(),
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
	}
}

// CheckDBHealth 检查数据库健康状态
func CheckDBHealth() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	return sqlDB.Ping()
}

// autoMigrate 自动迁移表结构
func autoMigrate() error {
	return DB.AutoMigrate(
		&Merchant{},
		&Order{},
		&Wallet{},
		&SystemConfig{},
		&TransactionLog{},
		&Admin{},
		&Withdrawal{},
		&APILog{},
		&IPBlacklist{},
		&WithdrawAddress{},
		&AppVersion{},
	)
}

// initDefaultData 初始化默认数据
func initDefaultData() error {
	// 初始化系统商户(id=0)
	// 用于全局钱包,这样wallets表的外键约束能正常工作
	var systemMerchant Merchant
	if err := DB.Where("id = ?", 0).First(&systemMerchant).Error; err != nil {
		// 先删除任何p_id为SYSTEM的旧记录(可能id不是0)
		DB.Exec("DELETE FROM merchants WHERE p_id = 'SYSTEM' AND id != 0")

		// 使用 NO_AUTO_VALUE_ON_ZERO 模式允许插入 id=0
		// 这是MySQL官方推荐的方式来插入0到AUTO_INCREMENT列
		DB.Exec("SET SESSION sql_mode = 'NO_AUTO_VALUE_ON_ZERO'")
		result := DB.Exec(`INSERT INTO merchants (id, p_id, name, ` + "`key`" + `, password, status, created_at, updated_at)
			VALUES (0, 'SYSTEM', '系统钱包', 'system_key', '', 1, NOW(), NOW())
			ON DUPLICATE KEY UPDATE p_id = 'SYSTEM'`)
		if result.Error != nil {
			log.Printf("Warning: Failed to create system merchant: %v", result.Error)
		} else if result.RowsAffected > 0 {
			log.Println("System merchant (id=0) created for global wallets")
		} else {
			log.Println("System merchant (id=0) already exists")
		}
		// 恢复默认sql_mode
		DB.Exec("SET SESSION sql_mode = ''")
	}

	// 初始化默认管理员
	var adminCount int64
	DB.Model(&Admin{}).Count(&adminCount)
	correctHash := "$2a$10$xiL.DqGTWgs4Sxv99TBxOeUMySHTXe5K2LtTgvtUTNc6wdChhRd7G" // admin123
	if adminCount == 0 {
		admin := Admin{
			Username: "admin",
			Password: correctHash,
			Status:   1,
		}
		if err := DB.Create(&admin).Error; err != nil {
			return err
		}
	} else {
		// 修复已存在的管理员密码（如果是旧的错误哈希值）
		var admin Admin
		if err := DB.Where("username = ?", "admin").First(&admin).Error; err == nil {
			if admin.Password != correctHash {
				DB.Model(&admin).Update("password", correctHash)
				log.Println("Admin password has been reset to default")
			}
		}
	}

	// 初始化默认商户
	var merchantCount int64
	DB.Model(&Merchant{}).Count(&merchantCount)
	// 默认商户密码: merchant123
	defaultMerchantPassword := "$2a$10$ZfUDWHWqrRcGn1mFlMklLudfG4rUnmoIwqaGFMm9ZBSg9CYbLRbvC"
	if merchantCount == 0 {
		merchant := Merchant{
			PID:      "1001",
			Name:     "默认商户",
			Key:      "test_key_123456",
			Password: defaultMerchantPassword,
			Status:   1,
		}
		if err := DB.Create(&merchant).Error; err != nil {
			return err
		}
		log.Println("Default merchant created: PID=1001, Password=merchant123")
	} else {
		// 为没有密码的商户设置默认密码
		DB.Model(&Merchant{}).Where("password = '' OR password IS NULL").Update("password", defaultMerchantPassword)
	}

	// 初始化系统配置
	defaultConfigs := []SystemConfig{
		{Key: ConfigKeyRateMode, Value: "hybrid", Description: "汇率模式: auto/manual/hybrid"},
		{Key: ConfigKeyManualRate, Value: "7.2", Description: "手动设置的汇率"},
		{Key: ConfigKeyFloatPercent, Value: "0", Description: "汇率浮动百分比"},
		{Key: ConfigKeyOrderExpire, Value: "30", Description: "订单过期时间(分钟)"},
		{Key: ConfigKeyNotifyRetry, Value: "5", Description: "通知重试次数"},
		{Key: ConfigKeySiteName, Value: "EzPay", Description: "网站名称"},
		{Key: ConfigKeySystemWalletFeeRate, Value: "0.02", Description: "系统收款码手续费率 (如0.02表示2%)"},
		{Key: ConfigKeyPersonalWalletFeeRate, Value: "0.01", Description: "个人收款码手续费率 (如0.01表示1%)"},
	}

	for _, cfg := range defaultConfigs {
		var count int64
		DB.Model(&SystemConfig{}).Where("`key` = ?", cfg.Key).Count(&count)
		if count == 0 {
			DB.Create(&cfg)
		}
	}

	return nil
}

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	return DB
}
