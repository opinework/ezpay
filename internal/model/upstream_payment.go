package model

import "time"

type UpstreamPayment struct {
	ID            uint   `gorm:"primaryKey"`
	Channel       string `gorm:"type:varchar(20);not null;uniqueIndex:idx_upstream_transaction"`
	TransactionID string `gorm:"type:varchar(100);not null;uniqueIndex:idx_upstream_transaction"`
	OrderID       uint   `gorm:"not null;uniqueIndex"`
	CreatedAt     time.Time
}
