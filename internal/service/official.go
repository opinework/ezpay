package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"ezpay/internal/model"
	"github.com/shopspring/decimal"
	"github.com/smartwalle/alipay/v3"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func officialChannelMatches(channel, chain string) bool {
	return channel == chain+"_official" && (chain == "alipay" || chain == "wechat")
}

func prepareOfficialOrder(order *model.Order) error {
	if !officialChannelMatches(order.Channel, order.Chain) || order.PayCurrency != "CNY" {
		return errors.New("官方通道与支付方式不匹配")
	}
	cfg, err := GetOfficialConfig(order.Channel)
	if err != nil {
		return err
	}
	if cfg.Enabled != "1" {
		return errors.New("官方支付通道未启用")
	}
	if _, err := officialSiteURL(getSiteURL()); err != nil {
		return err
	}
	if order.PayAmount.LessThan(decimal.NewFromFloat(0.01)) || !order.PayAmount.Equal(order.PayAmount.Round(2)) || order.PayAmount.GreaterThan(decimal.NewFromInt(100000000)) {
		return errors.New("官方支付金额必须为 0.01 至 100000000 元，最多两位小数")
	}
	order.PaymentMode = cfg.Mode
	order.OfficialAppID = cfg.AppID
	order.OfficialMerchantID = cfg.MerchantID
	order.UniqueAmount = order.PayAmount
	order.USDTAmount = order.PayAmount
	return nil
}

func (s *ChannelService) CreateOfficialOrder(order *model.Order) (*ChannelOrderResult, error) {
	cfg, err := GetOfficialConfig(order.Channel)
	if err != nil {
		return nil, err
	}
	if cfg.Enabled != "1" || cfg.AppID != order.OfficialAppID || cfg.MerchantID != order.OfficialMerchantID {
		return nil, errors.New("官方通道已停用或商户配置已改变")
	}
	siteURL, err := officialSiteURL(getSiteURL())
	if err != nil {
		return nil, err
	}
	if !order.ExpiredAt.After(time.Now()) {
		return nil, errors.New("订单已过期")
	}
	notifyURL := siteURL + "/channel/notify/" + order.Channel
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result := &ChannelOrderResult{Success: true, Amount: order.PayAmount, ExpireTime: order.ExpiredAt}
	if order.Channel == string(ChannelTypeAlipayOfficial) {
		client, err := newAlipayClient(cfg)
		if err != nil {
			return nil, err
		}
		trade := alipay.Trade{NotifyURL: notifyURL, ReturnURL: siteURL + "/cashier/" + order.TradeNo,
			Subject: order.Name, OutTradeNo: order.TradeNo, TotalAmount: order.PayAmount.StringFixed(2), SellerId: cfg.MerchantID,
			TimeExpire: order.ExpiredAt.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")}
		switch order.PaymentMode {
		case "f2f":
			resp, err := client.TradePreCreate(ctx, alipay.TradePreCreate{Trade: trade})
			if err != nil {
				return nil, errors.New("支付宝预下单失败，请稍后重试")
			}
			if !resp.IsSuccess() || resp.QRCode == "" || resp.OutTradeNo != order.TradeNo {
				return nil, errors.New("支付宝预下单被拒绝，请检查签约产品和商户配置")
			}
			result.QRCode = resp.QRCode
		case "page", "wap":
			var target *url.URL
			if order.PaymentMode == "page" {
				trade.ProductCode = "FAST_INSTANT_TRADE_PAY"
				target, err = client.TradePagePay(alipay.TradePagePay{Trade: trade})
			} else {
				trade.ProductCode = "QUICK_WAP_WAY"
				target, err = client.TradeWapPay(alipay.TradeWapPay{Trade: trade})
			}
			if err != nil {
				return nil, errors.New("支付宝支付链接生成失败")
			}
			result.PayURL = target.String()
		default:
			return nil, errors.New("支付宝支付模式无效")
		}
		return result, nil
	}
	client, _, err := newWechatClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	fen := order.PayAmount.Mul(decimal.NewFromInt(100)).IntPart()
	switch order.PaymentMode {
	case "native":
		svc := native.NativeApiService{Client: client}
		resp, _, err := svc.Prepay(ctx, native.PrepayRequest{
			Appid: core.String(cfg.AppID), Mchid: core.String(cfg.MerchantID), Description: core.String(order.Name),
			OutTradeNo: core.String(order.TradeNo), TimeExpire: core.Time(order.ExpiredAt), NotifyUrl: core.String(notifyURL),
			Amount: &native.Amount{Total: core.Int64(fen), Currency: core.String("CNY")}})
		if err != nil || resp == nil || resp.CodeUrl == nil || *resp.CodeUrl == "" {
			return nil, errors.New("微信 Native 预下单失败，请检查商户配置或稍后重试")
		}
		result.QRCode = *resp.CodeUrl
	case "h5":
		if net.ParseIP(order.ClientIP) == nil {
			return nil, errors.New("微信 H5 支付需要有效的付款人 IP")
		}
		svc := h5.H5ApiService{Client: client}
		resp, _, err := svc.Prepay(ctx, h5.PrepayRequest{
			Appid: core.String(cfg.AppID), Mchid: core.String(cfg.MerchantID), Description: core.String(order.Name),
			OutTradeNo: core.String(order.TradeNo), TimeExpire: core.Time(order.ExpiredAt), NotifyUrl: core.String(notifyURL),
			Amount:    &h5.Amount{Total: core.Int64(fen), Currency: core.String("CNY")},
			SceneInfo: &h5.SceneInfo{PayerClientIp: core.String(order.ClientIP), H5Info: &h5.H5Info{Type: core.String("Wap")}}})
		if err != nil || resp == nil || resp.H5Url == nil || *resp.H5Url == "" {
			return nil, errors.New("微信 H5 预下单失败，请检查签约产品和商户配置")
		}
		result.PayURL = *resp.H5Url + "&redirect_url=" + url.QueryEscape(siteURL+"/cashier/"+order.TradeNo)
	default:
		return nil, errors.New("微信支付模式无效")
	}
	return result, nil
}

// OfficialReceipt is populated only after verifying the provider's signature.
type OfficialReceipt struct {
	Channel       string
	TradeNo       string
	TransactionID string
	AppID         string
	MerchantID    string
	Currency      string
	Amount        decimal.Decimal
	PaidAt        time.Time
}

func ApplyOfficialReceipt(db *gorm.DB, receipt OfficialReceipt) (model.Order, bool, error) {
	var order model.Order
	var applied bool
	if !IsOfficialChannel(receipt.Channel) || receipt.TransactionID == "" || len(receipt.TransactionID) > 100 || receipt.TradeNo == "" || receipt.PaidAt.IsZero() || receipt.PaidAt.After(time.Now().Add(time.Minute)) {
		return order, false, errors.New("invalid official receipt")
	}
	if err := db.Where("trade_no = ?", receipt.TradeNo).First(&order).Error; err != nil {
		return order, false, err
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var merchant model.Merchant
		lock := func(q *gorm.DB) *gorm.DB {
			if db.Dialector.Name() != "sqlite" {
				return q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			return q
		}
		if err := lock(tx.Where("id = ?", order.MerchantID)).First(&merchant).Error; err != nil {
			return err
		}
		if err := lock(tx.Where("id = ?", order.ID)).First(&order).Error; err != nil {
			return err
		}
		if order.Channel != receipt.Channel || !officialChannelMatches(order.Channel, order.Chain) ||
			order.OfficialAppID == "" || order.OfficialMerchantID == "" || order.OfficialAppID != receipt.AppID || order.OfficialMerchantID != receipt.MerchantID ||
			order.PayCurrency != "CNY" || receipt.Currency != "CNY" || !receipt.Amount.IsPositive() || !receipt.Amount.Equal(order.PayAmount) {
			return errors.New("official receipt identity or amount mismatch")
		}
		if order.Status == model.OrderStatusPaid {
			if order.ChannelOrderID != receipt.TransactionID {
				return errors.New("official transaction changed")
			}
			return nil
		}
		// A delayed authentic callback may settle an expired order if payment preceded its deadline.
		if (order.Status != model.OrderStatusPending && order.Status != model.OrderStatusExpired) || receipt.PaidAt.After(order.ExpiredAt) || receipt.PaidAt.Before(order.CreatedAt.Add(-time.Minute)) {
			return errors.New("official payment outside order window")
		}
		ledger := model.OfficialPayment{Channel: receipt.Channel, TransactionID: receipt.TransactionID, OrderID: order.ID}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"status": model.OrderStatusPaid, "actual_amount": receipt.Amount, "paid_at": receipt.PaidAt, "channel_order_id": receipt.TransactionID}
		if order.NotifyURL != "" {
			updates["notify_status"] = model.NotifyStatusFailed
		}
		if err := tx.Model(&order).Updates(updates).Error; err != nil {
			return err
		}
		if order.FeeType != model.FeeTypeDeduction {
			return errors.New("unsupported official fee type")
		}
		if err := creditOrderSettlement(tx, &order, false); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return order, applied, err
}

func CompleteOfficialReceipt(receipt OfficialReceipt) error {
	order, applied, err := ApplyOfficialReceipt(model.GetDB(), receipt)
	if err != nil {
		return fmt.Errorf("official payment failed: %w", err)
	}
	if applied {
		GetOrderService().InvalidateOrderCache(order.TradeNo)
		go GetNotifyService().NotifyOrder(order.ID)
		go GetTelegramService().NotifyOrderPaid(&order)
	}
	return nil
}

func officialPaymentReady(order *model.Order) bool {
	return strings.TrimSpace(order.QRCode) != "" || strings.TrimSpace(order.ChannelPayURL) != ""
}
