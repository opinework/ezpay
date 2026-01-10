package model

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Wallet 钱包地址表
type Wallet struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	MerchantID uint           `gorm:"default:0;uniqueIndex:uk_merchant_chain_address" json:"merchant_id"` // 0=系统钱包, >0=商户钱包
	Chain      string         `gorm:"type:varchar(20);not null;uniqueIndex:uk_merchant_chain_address" json:"chain"` // trc20, erc20, bep20, polygon, wechat, alipay
	Address    string         `gorm:"type:varchar(500);not null;uniqueIndex:uk_merchant_chain_address" json:"address"` // 支付链接可能较长
	Label      string         `gorm:"type:varchar(50)" json:"label"`
	QRCode     string         `gorm:"type:varchar(500)" json:"qrcode"`    // 收款码图片路径 (微信/支付宝)
	Status     int8           `gorm:"default:1" json:"status"`            // 1:启用 0:禁用
	LastUsedAt *time.Time     `gorm:"index" json:"last_used_at"`          // 最后使用时间（用于轮询）
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	Merchant *Merchant `gorm:"foreignKey:MerchantID" json:"merchant,omitempty"`
}

func (Wallet) TableName() string {
	return "wallets"
}

// WalletBalance 钱包余额 (用于展示，不存储)
type WalletBalance struct {
	Wallet  *Wallet         `json:"wallet"`
	Balance decimal.Decimal `json:"balance"`
}
