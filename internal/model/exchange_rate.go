package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// RateType 汇率类型
type RateType string

const (
	RateTypeManual RateType = "manual" // 手动设置
	RateTypeAuto   RateType = "auto"   // 自动获取
)

// ExchangeRate 汇率配置
type ExchangeRate struct {
	ID           uint            `gorm:"primaryKey" json:"id"`
	FromCurrency string          `gorm:"type:varchar(10);not null" json:"from_currency"` // 源货币
	ToCurrency   string          `gorm:"type:varchar(10);not null" json:"to_currency"`   // 目标货币
	Rate         decimal.Decimal `gorm:"type:decimal(18,8);not null" json:"rate"`        // 基础汇率(中间价)
	RateType     RateType        `gorm:"type:enum('manual','auto');default:'manual'" json:"rate_type"`
	Source       string          `gorm:"type:varchar(50)" json:"source"`           // 数据源(auto时)
	AutoUpdate   bool            `gorm:"default:0" json:"auto_update"`             // 是否启用自动更新
	LastUpdated  *time.Time      `gorm:"type:datetime(3)" json:"last_updated"`     // 最后更新时间
	CreatedAt    time.Time       `gorm:"type:datetime(3)" json:"created_at"`
	UpdatedAt    time.Time       `gorm:"type:datetime(3)" json:"updated_at"`
}

// TableName 指定表名
func (ExchangeRate) TableName() string {
	return "exchange_rates"
}

// GetBuyRate 获取买入价（用户支付时使用，加上浮动）
func (r *ExchangeRate) GetBuyRate(buyFloat decimal.Decimal) decimal.Decimal {
	// 买入价 = 基础汇率 × (1 + 买入浮动)
	return r.Rate.Mul(decimal.NewFromInt(1).Add(buyFloat))
}

// GetSellRate 获取卖出价（提现时使用，减去浮动）
func (r *ExchangeRate) GetSellRate(sellFloat decimal.Decimal) decimal.Decimal {
	// 卖出价 = 基础汇率 × (1 - 卖出浮动)
	return r.Rate.Mul(decimal.NewFromInt(1).Sub(sellFloat))
}
