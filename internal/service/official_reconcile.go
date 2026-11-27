package service

import (
	"context"
	"errors"
	"ezpay/internal/model"
	"github.com/shopspring/decimal"
	"github.com/smartwalle/alipay/v3"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"log"
	"time"
)

func QueryOfficialOrder(ctx context.Context, order *model.Order) (*OfficialReceipt, error) {
	cfg, err := GetOfficialConfig(order.Channel)
	if err != nil {
		return nil, err
	}
	if cfg.AppID != order.OfficialAppID || cfg.MerchantID != order.OfficialMerchantID {
		return nil, errors.New("official merchant configuration changed")
	}
	if order.Channel == string(ChannelTypeAlipayOfficial) {
		client, err := newAlipayClient(cfg)
		if err != nil {
			return nil, err
		}
		resp, err := client.TradeQuery(ctx, alipay.TradeQuery{OutTradeNo: order.TradeNo})
		if err != nil || resp == nil {
			return nil, errors.New("alipay query failed")
		}
		if !resp.IsSuccess() {
			return nil, errors.New("alipay query rejected")
		}
		if resp.TradeStatus != alipay.TradeStatusSuccess && resp.TradeStatus != alipay.TradeStatusFinished {
			return nil, nil
		}
		if resp.OutTradeNo != order.TradeNo {
			return nil, errors.New("alipay query identity mismatch")
		}
		amount, err := decimal.NewFromString(resp.TotalAmount)
		if err != nil {
			return nil, err
		}
		paidAt, err := time.ParseInLocation("2006-01-02 15:04:05", resp.SendPayDate, time.FixedZone("CST", 8*3600))
		if err != nil {
			return nil, err
		}
		return &OfficialReceipt{Channel: order.Channel, TradeNo: resp.OutTradeNo, TransactionID: resp.TradeNo, AppID: cfg.AppID, MerchantID: cfg.MerchantID, Currency: "CNY", Amount: amount, PaidAt: paidAt}, nil
	}
	client, _, err := newWechatClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{Mchid: core.String(cfg.MerchantID), OutTradeNo: core.String(order.TradeNo)})
	if err != nil || resp == nil {
		return nil, errors.New("wechat query failed")
	}
	if resp.TradeState == nil || *resp.TradeState != "SUCCESS" {
		return nil, nil
	}
	return wechatReceipt(cfg, resp)
}

func CloseOfficialOrder(order *model.Order) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	receipt, err := QueryOfficialOrder(ctx, order)
	if err == nil && receipt != nil {
		if err := CompleteOfficialReceipt(*receipt); err != nil {
			return err
		}
		return errors.New("订单已支付，不能关闭")
	}
	cfg, err := GetOfficialConfig(order.Channel)
	if err != nil {
		return err
	}
	if cfg.AppID != order.OfficialAppID || cfg.MerchantID != order.OfficialMerchantID {
		return errors.New("官方商户配置已改变")
	}
	if order.Channel == string(ChannelTypeAlipayOfficial) {
		client, err := newAlipayClient(cfg)
		if err != nil {
			return err
		}
		resp, err := client.TradeClose(ctx, alipay.TradeClose{OutTradeNo: order.TradeNo})
		if err != nil || resp == nil || !resp.IsSuccess() {
			return errors.New("官方关闭未确认，请稍后重试")
		}
		return nil
	}
	client, _, err := newWechatClient(ctx, cfg)
	if err != nil {
		return err
	}
	svc := native.NativeApiService{Client: client}
	_, err = svc.CloseOrder(ctx, native.CloseOrderRequest{Mchid: core.String(cfg.MerchantID), OutTradeNo: core.String(order.TradeNo)})
	if err != nil {
		return errors.New("官方关闭未确认，请稍后重试")
	}
	return nil
}

func ReconcileOfficialOrders() {
	var orders []model.Order
	cutoff := time.Now().Add(-time.Minute)
	if err := model.GetDB().Where("channel IN (?,?) AND status IN (?,?) AND created_at > ? AND (provider_checked_at IS NULL OR provider_checked_at < ?)",
		ChannelTypeAlipayOfficial, ChannelTypeWechatOfficial, model.OrderStatusPending, model.OrderStatusExpired, time.Now().Add(-7*24*time.Hour), cutoff).
		Order("provider_checked_at ASC, id ASC").Limit(20).Find(&orders).Error; err != nil {
		return
	}
	for _, order := range orders {
		claim := model.GetDB().Model(&model.Order{}).Where("id = ? AND (provider_checked_at IS NULL OR provider_checked_at < ?)", order.ID, cutoff).Update("provider_checked_at", time.Now())
		if claim.Error != nil || claim.RowsAffected != 1 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		receipt, err := QueryOfficialOrder(ctx, &order)
		cancel()
		if err == nil && receipt != nil {
			if err := CompleteOfficialReceipt(*receipt); err != nil {
				log.Printf("official reconciliation rejected for %s: %v", order.TradeNo, err)
			}
		}
	}
}

func StartOfficialReconciliation() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ReconcileOfficialOrders()
		}
	}()
}
