package model

import (
	"time"
)

// SystemConfig 系统配置表
type SystemConfig struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Key         string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"key"`
	Value       string    `gorm:"type:text" json:"value"`
	Description string    `gorm:"type:varchar(200)" json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (SystemConfig) TableName() string {
	return "system_configs"
}

// 系统配置键名常量
const (
	ConfigKeyRateMode            = "rate_mode"              // 汇率模式
	ConfigKeyManualRate          = "manual_rate"            // 手动汇率
	ConfigKeyFloatPercent        = "float_percent"          // 浮动百分比（已废弃，使用 rate_buy_float）
	ConfigKeyRateBuyFloat        = "rate_buy_float"         // 买入汇率浮动（用户支付时），如0.02表示+2%
	ConfigKeyRateSellFloat       = "rate_sell_float"        // 卖出汇率浮动（商户提现时），如-0.02表示-2%
	ConfigKeyRateAutoUpdate      = "rate_auto_update"       // 汇率自动更新开关: 1启用 0禁用
	ConfigKeyOrderExpire         = "order_expire"           // 订单过期时间(分钟)
	ConfigKeyNotifyRetry         = "notify_retry"           // 通知重试次数
	ConfigKeySiteName            = "site_name"              // 网站名称
	ConfigKeyAdminEmail          = "admin_email"            // 管理员邮箱
	ConfigKeyWechatEnabled       = "wechat_enabled"         // 微信支付启用状态
	ConfigKeyAlipayEnabled       = "alipay_enabled"         // 支付宝启用状态
	ConfigKeySystemWalletFeeRate   = "system_wallet_fee_rate"   // 系统收款码手续费率 (如0.02表示2%)
	ConfigKeyPersonalWalletFeeRate = "personal_wallet_fee_rate" // 个人收款码手续费率 (如0.01表示1%)
	ConfigKeyServiceTelegram       = "service_telegram"         // 客服Telegram链接
	ConfigKeyServiceDiscord        = "service_discord"          // 客服Discord链接
)

// TransactionLog 交易日志表
type TransactionLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Chain       string    `gorm:"type:varchar(20);not null;index" json:"chain"`
	TxHash      string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"tx_hash"`
	FromAddress string    `gorm:"type:varchar(100);index" json:"from_address"`
	ToAddress   string    `gorm:"type:varchar(100);index" json:"to_address"`
	Amount      string    `gorm:"type:varchar(50)" json:"amount"`
	BlockNumber uint64    `gorm:"index" json:"block_number"`
	Matched     bool      `gorm:"default:false" json:"matched"` // 是否已匹配订单
	OrderID     *uint     `json:"order_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func (TransactionLog) TableName() string {
	return "transaction_logs"
}

// Admin 管理员表
type Admin struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
	Password  string    `gorm:"type:varchar(100);not null" json:"-"`
	Email     string    `gorm:"type:varchar(100)" json:"email"`
	Status    int8      `gorm:"default:1" json:"status"`
	LastLogin *time.Time `json:"last_login"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (Admin) TableName() string {
	return "admins"
}
