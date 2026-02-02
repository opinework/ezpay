package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// ExchangeRateHistory 汇率更新记录
type ExchangeRateHistory struct {
	ID            uint            `gorm:"primaryKey" json:"id"`
	RateID        uint            `gorm:"not null;index" json:"rate_id"`                // 汇率ID
	FromCurrency  string          `gorm:"type:varchar(10);not null" json:"from_currency"` // 源货币
	ToCurrency    string          `gorm:"type:varchar(10);not null" json:"to_currency"`   // 目标货币
	OldRate       decimal.Decimal `gorm:"type:decimal(18,8)" json:"old_rate"`             // 旧汇率
	NewRate       decimal.Decimal `gorm:"type:decimal(18,8);not null" json:"new_rate"`    // 新汇率
	ChangePercent decimal.Decimal `gorm:"type:decimal(8,4)" json:"change_percent"`        // 变化百分比
	UpdateSource  string          `gorm:"type:varchar(50)" json:"update_source"`          // 更新来源
	UpdatedBy     string          `gorm:"type:varchar(50)" json:"updated_by"`             // 更新者
	CreatedAt     time.Time       `gorm:"type:datetime(3)" json:"created_at"`
}

// TableName 指定表名
func (ExchangeRateHistory) TableName() string {
	return "exchange_rate_history"
}
