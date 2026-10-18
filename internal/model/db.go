package model

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
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

// InitDB 初始化数据库连接 (默认 MySQL)
func InitDB(dsn string) error {
	return InitDBWithConfig("mysql", dsn, DefaultDBConfig)
}

// InitDBWithConfig 使用自定义配置初始化数据库连接
// driver: "mysql" 或 "sqlite"
func InitDBWithConfig(driver, dsn string, cfg DBConfig) error {
	var dialector gorm.Dialector
	switch driver {
	case "sqlite":
		dialector = sqlite.Open(dsn)
	default:
		dialector = mysql.Open(dsn)
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Warn),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// 配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	if driver == "sqlite" {
		// SQLite 性能和并发优化
		sqlDB.SetMaxOpenConns(1) // SQLite 单连接避免锁争用
		sqlDB.SetMaxIdleConns(1)
		DB.Exec("PRAGMA journal_mode=WAL")
		DB.Exec("PRAGMA synchronous=NORMAL")
		DB.Exec("PRAGMA busy_timeout=5000")
		DB.Exec("PRAGMA foreign_keys=ON")
		log.Println("Database driver: SQLite (WAL mode)")
	} else {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
		sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
		log.Printf("Database driver: MySQL (MaxOpen: %d, MaxIdle: %d)", cfg.MaxOpenConns, cfg.MaxIdleConns)
	}

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
	models := []interface{}{
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
		&ExchangeRate{},
		&ExchangeRateHistory{},
		&BlockScanProgress{},
	}

	// 逐个迁移，跳过非致命错误（如 Error 1091: 索引/外键不存在）
	// GORM 在重建唯一索引时会尝试 DROP FOREIGN KEY，旧版本数据库没有该外键会报错
	for _, m := range models {
		if err := DB.AutoMigrate(m); err != nil {
			errMsg := err.Error()
			// MySQL Error 1091: Can't DROP ... check that column/key exists
			if strings.Contains(errMsg, "1091") {
				log.Printf("AutoMigrate warning (ignored): %v", err)
				continue
			}
			return err
		}
	}

	// 兜底：显式检查关键的新增列是否存在，缺失则补齐
	// 用途：GORM AutoMigrate 在历史库升级场景偶发漏检（不同版本/字符集/列顺序差异），
	//      新加的字段可能没成功 ALTER 上去。这里按列清单逐一兜底，确保上线后没有 "Unknown column" 错误。
	// 维护：每次给某个 model 加一个新字段，都应在下方追加一条对应的兜底记录。
	ensureRequiredColumns()

	return nil
}

// requiredColumn 描述一个必须存在的列以及缺失时的 ALTER DDL
type requiredColumn struct {
	Table     string
	Column    string
	MySQLDDL  string // MySQL 方言的 ADD COLUMN 语句
	SQLiteDDL string // SQLite 方言的 ADD COLUMN 语句
}

// requiredColumnsCatalog 列出所有"必须存在"的列。
//
// ⚠️ 维护规约（重要）：
// 每次给某个 model 结构体增加一个新字段，**必须** 在此追加对应的兜底 DDL。
// 这是数据迁移可靠性的安全网 —— GORM AutoMigrate 在历史库升级时偶发漏检
// （列顺序、字符集、版本差异），单纯靠 AutoMigrate 不能 100% 保证新列被加上。
//
// 添加字段的标准流程见 docs/02-BUILD.md "新增数据库字段流程"。
//
// 简版步骤：
//  1. 在 model 结构体加字段（带 GORM tag）
//  2. 在下方 catalog 追加 {Table, Column, MySQLDDL, SQLiteDDL}
//  3. 在已有库上本地启动一次，启动日志应看到 "Schema fix: column ... added"
//  4. 在新装库上启动一次，AutoMigrate 会建表，columnExists 返回 true，catalog 静默通过
//
// DDL 撰写要点：
//  - MySQL DDL 使用反引号包裹标识符；用 `AFTER existing_column` 控制列位置（可选）
//  - SQLite 不支持 AFTER；类型用 VARCHAR/INT 等通用 SQL 类型
//  - DEFAULT 值要与 GORM tag 中的 default 一致，避免新装库与升级库语义不一致
//  - 不要在 DDL 里加 FK，避免与 DisableForeignKeyConstraintWhenMigrating 冲突
var requiredColumnsCatalog = []requiredColumn{
	{
		Table:     "merchants",
		Column:    "default_currency",
		MySQLDDL:  "ALTER TABLE `merchants` ADD COLUMN `default_currency` VARCHAR(10) NOT NULL DEFAULT 'USD' AFTER `wallet_mode`",
		SQLiteDDL: "ALTER TABLE merchants ADD COLUMN default_currency VARCHAR(10) NOT NULL DEFAULT 'USD'",
	},
	// ⬆ 添加新字段时，按此格式继续追加。空行可保留以便 git diff 清晰。
}

// ensureRequiredColumns 检查清单内的每一列是否存在，缺失则执行 ADD COLUMN
func ensureRequiredColumns() {
	for _, c := range requiredColumnsCatalog {
		if columnExists(c.Table, c.Column) {
			continue
		}
		ddl := c.MySQLDDL
		if IsSQLite() {
			ddl = c.SQLiteDDL
		}
		log.Printf("Schema fix: column %s.%s missing, running ALTER", c.Table, c.Column)
		if err := DB.Exec(ddl).Error; err != nil {
			// 不 fatal —— 让服务继续启动以便排查；但后续操作会因列缺失继续报错
			log.Printf("Schema fix FAILED for %s.%s: %v (manual ALTER needed)", c.Table, c.Column, err)
			continue
		}
		log.Printf("Schema fix: column %s.%s added", c.Table, c.Column)
	}
}

// columnExists 判断指定表是否含有指定列。MySQL 用 INFORMATION_SCHEMA，SQLite 用 PRAGMA。
func columnExists(table, column string) bool {
	if DB == nil {
		return false
	}
	if IsSQLite() {
		// PRAGMA table_info 只接受字面量表名，无法用占位符；表名来自硬编码清单，无注入风险
		rows, err := DB.Raw(fmt.Sprintf("PRAGMA table_info(%s)", table)).Rows()
		if err != nil {
			return false
		}
		defer rows.Close()
		for rows.Next() {
			var (
				cid       int
				name      string
				ctype     string
				notnull   int
				dfltValue interface{}
				pk        int
			)
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
				continue
			}
			if name == column {
				return true
			}
		}
		return false
	}
	// MySQL
	var count int64
	err := DB.Raw(
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?",
		table, column,
	).Scan(&count).Error
	if err != nil {
		log.Printf("columnExists query failed for %s.%s: %v", table, column, err)
		return false
	}
	return count > 0
}

// initDefaultData 初始化默认数据
func initDefaultData() error {
	// 初始化系统商户(id=0)
	// 用于全局钱包,这样wallets表的外键约束能正常工作
	var systemMerchant Merchant
	if err := DB.Where("id = ?", 0).First(&systemMerchant).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to check system merchant: %w", err)
		}
		// 先删除任何p_id为SYSTEM的旧记录(可能id不是0)
		DB.Exec("DELETE FROM merchants WHERE p_id = 'SYSTEM' AND id != 0")

		if IsSQLite() {
			// SQLite: 直接插入 id=0，SQLite 不会将 0 视为自动递增触发
			now := time.Now()
			result := DB.Exec(`INSERT OR REPLACE INTO merchants (id, p_id, name, "key", password, status, created_at, updated_at)
				VALUES (0, 'SYSTEM', '系统钱包', 'system_key', '', 1, ?, ?)`, now, now)
			if result.Error != nil {
				log.Printf("Warning: Failed to create system merchant: %v", result.Error)
			} else {
				log.Println("System merchant (id=0) created for global wallets")
			}
		} else {
			// MySQL: 使用 NO_AUTO_VALUE_ON_ZERO 模式允许插入 id=0
			// 保存原始 sql_mode 并在完成后恢复
			var originalSqlMode string
			DB.Raw("SELECT @@SESSION.sql_mode").Scan(&originalSqlMode)
			DB.Exec("SET SESSION sql_mode = 'NO_AUTO_VALUE_ON_ZERO'")
			defer DB.Exec("SET SESSION sql_mode = ?", originalSqlMode)
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
		}
	}

	// 初始化默认管理员
	var adminCount int64
	DB.Model(&Admin{}).Count(&adminCount)
	correctHash := "$2a$10$xiL.DqGTWgs4Sxv99TBxOeUMySHTXe5K2LtTgvtUTNc6wdChhRd7G" // 默认密码，首次使用请立即修改
	if adminCount == 0 {
		admin := Admin{
			Username: "admin",
			Password: correctHash,
			Status:   1,
		}
		if err := DB.Create(&admin).Error; err != nil {
			return err
		}
	}

	// 初始化默认商户
	var merchantCount int64
	DB.Model(&Merchant{}).Where("id != 0").Count(&merchantCount)
	// 默认商户密码，首次使用请立即修改
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
		{Key: ConfigKeyRateAutoUpdate, Value: "1", Description: "汇率自动更新: 1启用 0禁用"},
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
