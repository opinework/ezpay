package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// NotifySettings 通知设置
type NotifySettings struct {
	OrderCreated    bool `json:"order_created"`    // 订单创建
	OrderPaid       bool `json:"order_paid"`       // 订单支付成功
	OrderExpired    bool `json:"order_expired"`    // 订单过期
	WithdrawCreated bool `json:"withdraw_created"` // 发起提现
	WithdrawSuccess bool `json:"withdraw_success"` // 提现成功
	WithdrawReject  bool `json:"withdraw_reject"`  // 提现拒绝
	ConfigChanged   bool `json:"config_changed"`   // 配置变更
	SecurityAlert   bool `json:"security_alert"`   // 安全警告
}

// DefaultNotifySettings 默认通知设置（全部开启）
func DefaultNotifySettings() NotifySettings {
	return NotifySettings{
		OrderCreated:    true,
		OrderPaid:       true,
		OrderExpired:    false,
		WithdrawCreated: true,
		WithdrawSuccess: true,
		WithdrawReject:  true,
		ConfigChanged:   true,
		SecurityAlert:   true,
	}
}

// Scan 实现 sql.Scanner 接口
func (n *NotifySettings) Scan(value interface{}) error {
	if value == nil {
		*n = DefaultNotifySettings()
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		*n = DefaultNotifySettings()
		return nil
	}
	if len(bytes) == 0 {
		*n = DefaultNotifySettings()
		return nil
	}
	return json.Unmarshal(bytes, n)
}

// Value 实现 driver.Valuer 接口
func (n NotifySettings) Value() (driver.Value, error) {
	return json.Marshal(n)
}

// Merchant 商户表
type Merchant struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	PID       string         `gorm:"column:p_id;type:varchar(32);uniqueIndex;not null" json:"pid"`
	Name      string         `gorm:"type:varchar(100)" json:"name"`
	Key       string         `gorm:"type:varchar(64);not null" json:"-"`
	Password  string         `gorm:"type:varchar(100)" json:"-"`            // 商户登录密码 (bcrypt)
	Email     string         `gorm:"type:varchar(100)" json:"email"`        // 联系邮箱
	NotifyURL string         `gorm:"type:varchar(500)" json:"notify_url"`
	ReturnURL string         `gorm:"type:varchar(500)" json:"return_url"`
	Status       int8           `gorm:"default:1" json:"status"`                         // 1:正常 0:禁用
	Balance      float64        `gorm:"type:decimal(18,2);default:0" json:"balance"`
	FrozenBalance float64       `gorm:"type:decimal(18,2);default:0" json:"frozen_balance"` // 冻结余额(提现中)
	FeeRate      float64        `gorm:"type:decimal(5,4);default:0" json:"fee_rate"`        // 手续费率 (如0.02表示2%)
	WalletLimit  int            `gorm:"default:10" json:"wallet_limit"`                     // 钱包数量限制, 0表示无限制
	IPWhitelistEnabled bool     `gorm:"default:false" json:"ip_whitelist_enabled"`          // IP白名单是否启用
	IPWhitelist  string         `gorm:"type:text" json:"ip_whitelist"`                      // IP白名单,逗号分隔
	RefererWhitelistEnabled bool `gorm:"default:false" json:"referer_whitelist_enabled"`    // Referer白名单是否启用
	RefererWhitelist string     `gorm:"type:text" json:"referer_whitelist"`                 // Referer域名白名单,逗号分隔
	TelegramChatID int64          `gorm:"default:0" json:"telegram_chat_id"`                  // Telegram Chat ID (绑定后用于接收通知)
	TelegramNotify bool           `gorm:"default:true" json:"telegram_notify"`                // 是否开启Telegram通知
	TelegramStatus string         `gorm:"type:varchar(20);default:'unbound'" json:"telegram_status"` // Telegram状态: normal正常, blocked被封禁, unbound未绑定
	NotifySettings NotifySettings `gorm:"type:json" json:"notify_settings"`                   // 通知设置详情
	WalletMode     int8           `gorm:"default:3" json:"wallet_mode"`                       // 钱包模式: 1=仅系统钱包 2=仅个人钱包 3=两者同时(优先个人)
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Merchant) TableName() string {
	return "merchants"
}

// MerchantStats 商户统计
type MerchantStats struct {
	TotalOrders   int64   `json:"total_orders"`
	TotalAmount   float64 `json:"total_amount"`
	TodayOrders   int64   `json:"today_orders"`
	TodayAmount   float64 `json:"today_amount"`
	SuccessRate   float64 `json:"success_rate"`
}

// WithdrawStatus 提现状态
type WithdrawStatus int8

const (
	WithdrawStatusPending  WithdrawStatus = 0 // 待处理
	WithdrawStatusApproved WithdrawStatus = 1 // 已通过
	WithdrawStatusRejected WithdrawStatus = 2 // 已拒绝
	WithdrawStatusPaid     WithdrawStatus = 3 // 已打款
)

// Withdrawal 提现记录
type Withdrawal struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	MerchantID      uint           `gorm:"index;not null" json:"merchant_id"`
	Merchant        Merchant       `gorm:"foreignKey:MerchantID" json:"-"`
	Amount          float64        `gorm:"type:decimal(18,2);not null" json:"amount"`          // 提现金额（USD）
	Fee             float64        `gorm:"type:decimal(18,2);default:0" json:"fee"`            // 手续费（USD）
	RealAmount      float64        `gorm:"type:decimal(18,2);not null" json:"real_amount"`     // 扣除手续费后金额（USD）
	PayoutAmount    float64        `gorm:"type:decimal(18,6);default:0" json:"payout_amount"`  // 实际打款金额（USDT/TRX等）
	PayoutCurrency  string         `gorm:"type:varchar(10)" json:"payout_currency"`            // 打款货币: USDT, TRX等
	PayoutRate      float64        `gorm:"type:decimal(10,4);default:0" json:"payout_rate"`    // 打款汇率（卖出汇率）
	PayMethod       string         `gorm:"type:varchar(20)" json:"pay_method"`                 // 打款方式: trc20, erc20, bep20等
	Account         string         `gorm:"type:varchar(200)" json:"account"`                   // 收款账号
	AccountName     string         `gorm:"type:varchar(100)" json:"account_name"`              // 收款人姓名
	BankName        string         `gorm:"type:varchar(100)" json:"bank_name"`                 // 银行名称(银行卡提现)
	Status          WithdrawStatus `gorm:"default:0" json:"status"`
	Remark          string         `gorm:"type:varchar(500)" json:"remark"`                    // 备注
	AdminRemark     string         `gorm:"type:varchar(500)" json:"admin_remark"`              // 管理员备注
	ProcessedAt     *time.Time     `json:"processed_at"`                                       // 处理时间
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (Withdrawal) TableName() string {
	return "withdrawals"
}

// WithdrawAddressStatus 提现地址状态
type WithdrawAddressStatus int8

const (
	WithdrawAddressPending  WithdrawAddressStatus = 0 // 待审核
	WithdrawAddressApproved WithdrawAddressStatus = 1 // 已审核
	WithdrawAddressRejected WithdrawAddressStatus = 2 // 已拒绝
)

// WithdrawAddress 提现地址
type WithdrawAddress struct {
	ID          uint                  `gorm:"primaryKey" json:"id"`
	MerchantID  uint                  `gorm:"index;not null" json:"merchant_id"`
	Chain       string                `gorm:"type:varchar(20);not null" json:"chain"`    // trc20, bep20, polygon, optimism
	Address     string                `gorm:"type:varchar(200);not null" json:"address"` // USDT钱包地址
	Label       string                `gorm:"type:varchar(100)" json:"label"`            // 备注名称
	IsDefault   bool                  `gorm:"default:false" json:"is_default"`           // 是否默认地址
	Status      WithdrawAddressStatus `gorm:"default:0" json:"status"`                   // 审核状态: 0待审核 1已通过 2已拒绝
	AdminRemark string                `gorm:"type:varchar(500)" json:"admin_remark"`     // 管理员备注
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	DeletedAt   gorm.DeletedAt        `gorm:"index" json:"-"`
}

func (WithdrawAddress) TableName() string {
	return "withdraw_addresses"
}
