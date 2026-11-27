package model

import "time"

// MonitorEvent retains receipts even when no order matched, preventing late replay.
type MonitorEvent struct {
	ID          uint      `gorm:"primaryKey"`
	MerchantID  uint      `gorm:"not null;uniqueIndex:idx_monitor_merchant_event"`
	EventID     string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_monitor_merchant_event"`
	Fingerprint string    `gorm:"type:varchar(64);not null"`
	ReceivedAt  time.Time `gorm:"not null"`
	OrderID     uint      `gorm:"not null;default:0"`
	TradeNo     string    `gorm:"type:varchar(64);not null;default:''"`
	Code        int       `gorm:"not null;default:0"`
	Message     string    `gorm:"type:varchar(200);not null"`
	CreatedAt   time.Time
}

func (MonitorEvent) TableName() string { return "monitor_events" }
