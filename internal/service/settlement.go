package service

import (
	"errors"
	"ezpay/internal/model"
	"gorm.io/gorm"
)

func creditOrderSettlement(tx *gorm.DB, order *model.Order, feeFrozen bool) error {
	if order.SettlementAmount.IsNegative() || order.Fee.IsNegative() || order.Fee.GreaterThan(order.SettlementAmount) {
		return errors.New("invalid settlement or fee")
	}
	var merchant model.Merchant
	if err := tx.First(&merchant, order.MerchantID).Error; err != nil {
		return err
	}
	if feeFrozen {
		if err := releaseOrderFee(tx, order); err != nil {
			return err
		}
	}
	return tx.Model(&merchant).Update("balance", gorm.Expr("balance + ?", order.SettlementAmount.Sub(order.Fee).String())).Error
}

func releaseOrderFee(tx *gorm.DB, order *model.Order) error {
	if order.FeeType != model.FeeTypeBalance || !order.Fee.IsPositive() {
		return nil
	}
	result := tx.Model(&model.Merchant{}).Where("id = ? AND frozen_balance >= ?", order.MerchantID, order.Fee.String()).
		Update("frozen_balance", gorm.Expr("frozen_balance - ?", order.Fee.String()))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("merchant missing or frozen fee is insufficient")
	}
	return nil
}

// finishUnpaidOrder commits the terminal status and frozen-fee release together.
func finishUnpaidOrder(db *gorm.DB, orderID uint, status model.OrderStatus) (bool, error) {
	changed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var order model.Order
		if err := tx.First(&order, orderID).Error; err != nil {
			return err
		}
		result := tx.Model(&order).Where("status = ?", model.OrderStatusPending).Update("status", status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := releaseOrderFee(tx, &order); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}
