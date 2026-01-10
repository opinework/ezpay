package model

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// OrderStatus 订单状态
type OrderStatus int8

const (
	OrderStatusPending   OrderStatus = 0 // 待支付
	OrderStatusPaid      OrderStatus = 1 // 已支付
	OrderStatusExpired   OrderStatus = 2 // 已过期
	OrderStatusCancelled OrderStatus = 3 // 已取消
)

// NotifyStatus 通知状态
type NotifyStatus int8

const (
	NotifyStatusPending NotifyStatus = 0 // 待通知
	NotifyStatusSuccess NotifyStatus = 1 // 通知成功
	NotifyStatusFailed  NotifyStatus = 2 // 通知失败
)

// FeeType 手续费类型
type FeeType int8

const (
	FeeTypeBalance   FeeType = 1 // 从余额扣除 (商户自己钱包)
	FeeTypeDeduction FeeType = 2 // 从收款扣除 (平台钱包)
)

// Order 订单表
type Order struct {
	ID             uint            `gorm:"primaryKey" json:"id"`
	TradeNo        string          `gorm:"type:varchar(64);uniqueIndex;not null" json:"trade_no"`
	OutTradeNo     string          `gorm:"type:varchar(64);not null;index:idx_merchant_out_trade" json:"out_trade_no"`
	MerchantID     uint            `gorm:"not null;index:idx_merchant_out_trade" json:"merchant_id"`
	Merchant       *Merchant       `gorm:"foreignKey:MerchantID" json:"merchant,omitempty"`
	Type           string          `gorm:"type:varchar(20);not null" json:"type"` // usdt_trc20, usdt_erc20, usdt_bep20, usdt_polygon
	Name           string          `gorm:"type:varchar(200)" json:"name"`
	Currency       string          `gorm:"type:varchar(10);default:'CNY'" json:"currency"` // 原始货币: CNY, USD, USDT 等
	Money          decimal.Decimal `gorm:"type:decimal(18,6);not null" json:"money"`       // 原始金额
	PayAmount      decimal.Decimal `gorm:"type:decimal(18,6)" json:"pay_amount"`           // 实际支付金额(目标货币)
	PayCurrency    string          `gorm:"type:varchar(10)" json:"pay_currency"`           // 支付货币: USDT, TRX, CNY
	USDTAmount     decimal.Decimal `gorm:"type:decimal(18,6)" json:"usdt_amount"`          // USDT金额(兼容旧字段)
	ActualAmount   decimal.Decimal `gorm:"type:decimal(18,6)" json:"actual_amount"`       // 实际收到USDT金额
	Rate           decimal.Decimal `gorm:"type:decimal(10,4)" json:"rate"`                // 汇率
	Chain          string          `gorm:"type:varchar(20)" json:"chain"`                 // trc20, erc20, bep20, polygon
	ToAddress      string          `gorm:"type:varchar(100)" json:"to_address"`           // 收款地址
	FromAddress    string          `gorm:"type:varchar(100)" json:"from_address"`         // 付款地址
	TxHash         string          `gorm:"type:varchar(100)" json:"tx_hash"`              // 交易哈希
	QRCode         string          `gorm:"type:varchar(500)" json:"qrcode"`               // 收款二维码(微信/支付宝)
	WalletID       uint            `gorm:"default:0" json:"wallet_id"`                    // 使用的钱包ID
	Fee            decimal.Decimal `gorm:"type:decimal(18,6);default:0" json:"fee"`       // 手续费
	FeeRate        decimal.Decimal `gorm:"type:decimal(5,4);default:0" json:"fee_rate"`   // 手续费率
	FeeType        FeeType         `gorm:"default:2" json:"fee_type"`                     // 1=余额扣除 2=收款扣除
	Status         OrderStatus     `gorm:"default:0;index:idx_status_created" json:"status"`
	NotifyURL      string          `gorm:"type:varchar(500)" json:"notify_url"`
	ReturnURL      string          `gorm:"type:varchar(500)" json:"return_url"`
	NotifyCount    int             `gorm:"default:0" json:"notify_count"`
	NotifyStatus   NotifyStatus    `gorm:"default:0" json:"notify_status"`
	Param          string          `gorm:"type:text" json:"param"` // 附加参数
	ClientIP       string          `gorm:"type:varchar(50)" json:"client_ip"`
	Channel        string          `gorm:"type:varchar(20);default:'local'" json:"channel"`        // 支付通道: local, vmq, epay
	ChannelOrderID string          `gorm:"type:varchar(100)" json:"channel_order_id"`              // 上游订单号
	ChannelPayURL  string          `gorm:"type:varchar(500)" json:"channel_pay_url"`               // 上游支付链接
	CreatedAt      time.Time       `gorm:"index:idx_status_created" json:"created_at"`
	PaidAt         *time.Time      `json:"paid_at"`
	ExpiredAt      time.Time       `json:"expired_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	DeletedAt      gorm.DeletedAt  `gorm:"index" json:"-"`
}

func (Order) TableName() string {
	return "orders"
}

// OrderQuery 订单查询参数
type OrderQuery struct {
	MerchantID   uint        `form:"merchant_id"`
	TradeNo      string      `form:"trade_no"`
	OutTradeNo   string      `form:"out_trade_no"`
	Status       *OrderStatus `form:"status"`
	Type         string      `form:"type"`
	StartTime    *time.Time  `form:"start_time"`
	EndTime      *time.Time  `form:"end_time"`
	Page         int         `form:"page"`
	PageSize     int         `form:"page_size"`
}

// OrderStats 订单统计
type OrderStats struct {
	TotalOrders     int64           `json:"total_orders"`
	TotalAmount     decimal.Decimal `json:"total_amount"`
	TotalUSDT       decimal.Decimal `json:"total_usdt"`
	PendingOrders   int64           `json:"pending_orders"`
	PaidOrders      int64           `json:"paid_orders"`
	ExpiredOrders   int64           `json:"expired_orders"`
	TodayOrders     int64           `json:"today_orders"`
	TodayAmount     decimal.Decimal `json:"today_amount"`
	TodayUSDT       decimal.Decimal `json:"today_usdt"`
}
