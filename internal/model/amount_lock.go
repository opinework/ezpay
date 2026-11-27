package model

// AmountLock serializes local amount allocation for one chain across service instances.
type AmountLock struct {
	Chain string `gorm:"type:varchar(20);primaryKey"`
}
