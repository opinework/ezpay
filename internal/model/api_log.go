package model

import (
	"time"
)

// APILog API调用日志
type APILog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	MerchantID   uint      `gorm:"index" json:"merchant_id"`
	MerchantPID  string    `gorm:"type:varchar(32);index" json:"merchant_pid"`
	Endpoint     string    `gorm:"type:varchar(100)" json:"endpoint"`       // API端点: submit, mapi, query等
	Method       string    `gorm:"type:varchar(10)" json:"method"`          // HTTP方法
	ClientIP     string    `gorm:"type:varchar(50)" json:"client_ip"`
	Referer      string    `gorm:"type:varchar(500)" json:"referer"`
	UserAgent    string    `gorm:"type:varchar(500)" json:"user_agent"`
	RequestBody  string    `gorm:"type:text" json:"request_body"`           // 请求参数 (脱敏)
	ResponseCode int       `gorm:"default:0" json:"response_code"`          // 响应码: 1=成功, -1=失败
	ResponseMsg  string    `gorm:"type:varchar(500)" json:"response_msg"`   // 响应消息
	TradeNo      string    `gorm:"type:varchar(64);index" json:"trade_no"`  // 关联订单号
	Duration     int64     `gorm:"default:0" json:"duration"`               // 耗时(毫秒)
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

func (APILog) TableName() string {
	return "api_logs"
}
