package model

import (
	"time"
)

// IPBlacklist IP黑名单
type IPBlacklist struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	IP        string    `gorm:"type:varchar(45);uniqueIndex;not null" json:"ip"` // 支持IPv6
	Reason    string    `gorm:"type:varchar(200)" json:"reason"`                 // 拉黑原因
	Source    string    `gorm:"type:varchar(50)" json:"source"`                  // 来源: manual(手动), api_log(API日志一键拉黑)
	CreatedAt time.Time `json:"created_at"`
}

// TableName 表名
func (IPBlacklist) TableName() string {
	return "ip_blacklist"
}

// IsIPBlacklisted 检查IP是否在黑名单中
func IsIPBlacklisted(ip string) bool {
	var count int64
	GetDB().Model(&IPBlacklist{}).Where("ip = ?", ip).Count(&count)
	return count > 0
}

// AddIPToBlacklist 添加IP到黑名单
func AddIPToBlacklist(ip, reason, source string) error {
	blacklist := IPBlacklist{
		IP:     ip,
		Reason: reason,
		Source: source,
	}
	return GetDB().Create(&blacklist).Error
}

// RemoveIPFromBlacklist 从黑名单移除IP
func RemoveIPFromBlacklist(id uint) error {
	return GetDB().Delete(&IPBlacklist{}, id).Error
}
