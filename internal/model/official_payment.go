package model

import "time"

// OfficialPayment binds a provider transaction to one local order permanently.
type OfficialPayment struct {
	ID            uint   `gorm:"primaryKey"`
	Channel       string `gorm:"type:varchar(20);not null;uniqueIndex:idx_official_transaction"`
	TransactionID string `gorm:"type:varchar(100);not null;uniqueIndex:idx_official_transaction"`
	OrderID       uint   `gorm:"not null;uniqueIndex"`
	CreatedAt     time.Time
}
