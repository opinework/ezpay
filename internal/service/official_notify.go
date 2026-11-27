package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
)

func ParseAlipayReceipt(ctx context.Context, cfg OfficialConfig, form url.Values) (*OfficialReceipt, error) {
	for _, values := range form {
		if len(values) != 1 {
			return nil, errors.New("duplicate callback parameter")
		}
	}
	if form.Get("sign_type") != "RSA2" {
		return nil, errors.New("RSA2 required")
	}
	client, err := newAlipayClient(cfg)
	if err != nil {
		return nil, err
	}
	n, err := client.DecodeNotification(ctx, form)
	if err != nil {
		return nil, errors.New("支付宝回调验签失败")
	}
	if n.AppId != cfg.AppID || n.SellerId != cfg.MerchantID {
		return nil, errors.New("支付宝回调商户不匹配")
	}
	if n.TradeStatus != "TRADE_SUCCESS" && n.TradeStatus != "TRADE_FINISHED" {
		return nil, nil
	}
	amount, err := decimal.NewFromString(n.TotalAmount)
	if err != nil {
		return nil, err
	}
	paidAt, err := time.ParseInLocation("2006-01-02 15:04:05", n.GmtPayment, time.FixedZone("CST", 8*3600))
	if err != nil {
		return nil, err
	}
	return &OfficialReceipt{Channel: string(ChannelTypeAlipayOfficial), TradeNo: n.OutTradeNo, TransactionID: n.TradeNo,
		AppID: n.AppId, MerchantID: n.SellerId, Amount: amount, Currency: "CNY", PaidAt: paidAt}, nil
}

func ParseWechatReceipt(ctx context.Context, cfg OfficialConfig, request *http.Request) (*OfficialReceipt, error) {
	_, verifier, err := newWechatClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	handler, err := notify.NewRSANotifyHandler(cfg.APIV3Key, verifier)
	if err != nil {
		return nil, err
	}
	var transaction payments.Transaction
	event, err := handler.ParseNotifyRequest(ctx, request, &transaction)
	if err != nil {
		return nil, errors.New("微信回调验签或解密失败")
	}
	if event.EventType != "TRANSACTION.SUCCESS" {
		return nil, errors.New("unexpected wechat event")
	}
	return wechatReceipt(cfg, &transaction)
}

func wechatReceipt(cfg OfficialConfig, transaction *payments.Transaction) (*OfficialReceipt, error) {
	if transaction.Appid == nil || transaction.Mchid == nil || *transaction.Appid != cfg.AppID || *transaction.Mchid != cfg.MerchantID ||
		transaction.TradeState == nil || *transaction.TradeState != "SUCCESS" || transaction.OutTradeNo == nil || transaction.TransactionId == nil ||
		transaction.Amount == nil || transaction.Amount.Total == nil || transaction.Amount.Currency == nil || transaction.SuccessTime == nil {
		return nil, errors.New("invalid wechat transaction")
	}
	paidAt, err := time.Parse(time.RFC3339, *transaction.SuccessTime)
	if err != nil {
		return nil, err
	}
	return &OfficialReceipt{Channel: string(ChannelTypeWechatOfficial), TradeNo: *transaction.OutTradeNo, TransactionID: *transaction.TransactionId,
		AppID: *transaction.Appid, MerchantID: *transaction.Mchid, Currency: *transaction.Amount.Currency,
		Amount: decimal.NewFromInt(*transaction.Amount.Total).Div(decimal.NewFromInt(100)), PaidAt: paidAt}, nil
}
