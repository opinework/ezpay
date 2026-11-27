package service

import (
	"errors"
	"ezpay/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func applyUpstreamPayment(db *gorm.DB, channel ChannelType, tradeNo, transactionID string, amount decimal.Decimal) (model.Order, bool, error) {
	var order model.Order
	var applied bool
	if (channel != ChannelTypeVmq && channel != ChannelTypeEpay) || transactionID == "" || len(transactionID) > 100 || !amount.IsPositive() {
		return order, false, errors.New("invalid upstream receipt")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("trade_no = ?", tradeNo)
		if db.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&order).Error; err != nil {
			return err
		}
		if order.Channel != string(channel) {
			return errors.New("callback channel does not match order")
		}
		expected := order.UniqueAmount
		if channel == ChannelTypeEpay {
			expected = order.Money
		}
		if !amount.Equal(expected) {
			return errors.New("callback amount does not match order")
		}
		if order.Status == model.OrderStatusPaid {
			if order.ChannelOrderID != transactionID {
				return errors.New("upstream transaction changed")
			}
			return nil
		}
		if order.Status != model.OrderStatusPending || !order.ExpiredAt.After(time.Now()) {
			return errors.New("order is not payable")
		}
		if channel == ChannelTypeEpay && order.ChannelOrderID != "" && order.ChannelOrderID != transactionID {
			return errors.New("upstream order identity mismatch")
		}
		if err := tx.Create(&model.UpstreamPayment{Channel: string(channel), TransactionID: transactionID, OrderID: order.ID}).Error; err != nil {
			return err
		}
		actual := amount
		if channel == ChannelTypeEpay {
			actual = order.PayAmount
		}
		updates := map[string]interface{}{"status": model.OrderStatusPaid, "actual_amount": actual, "paid_at": time.Now(), "channel_order_id": transactionID}
		if order.NotifyURL != "" {
			updates["notify_status"] = model.NotifyStatusFailed
		}
		if err := tx.Model(&order).Updates(updates).Error; err != nil {
			return err
		}
		if err := creditOrderSettlement(tx, &order, true); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return order, applied, err
}
