package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"ezpay/internal/middleware"
	"ezpay/internal/model"
	"ezpay/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var monitorEventIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var monitorAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.[0-9]{2}$`)
var errMonitorAuth = errors.New("merchant authentication failed")

func monitorSign(payload, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func monitorError(c *gin.Context, code int, message string, retryable bool) {
	middleware.SetAPILogContext(c, code, message, "", 0, "")
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": message, "protocol": 2, "retryable": retryable})
}

func monitorTimestamp(c *gin.Context) (int64, bool) {
	timestamp, err := strconv.ParseInt(c.Query("t"), 10, 64)
	now := time.Now().Unix()
	if err != nil || timestamp < now-300 || timestamp > now+300 {
		monitorError(c, -1, "请求时间无效或已过期，请校准手机时间", true)
		return 0, false
	}
	return timestamp, true
}

func monitorMerchant(db *gorm.DB, payload, signature string) (*model.Merchant, error) {
	if len(signature) != 64 {
		return nil, errMonitorAuth
	}
	decoded, err := hex.DecodeString(signature)
	if err != nil || len(decoded) != sha256.Size {
		return nil, errMonitorAuth
	}
	var merchants []model.Merchant
	if err := db.Where("status = 1 AND id > 0").Find(&merchants).Error; err != nil {
		return nil, err
	}
	for _, merchant := range merchants {
		expected, _ := hex.DecodeString(monitorSign(payload, merchant.Key))
		if hmac.Equal(decoded, expected) {
			return &merchant, nil
		}
	}
	return nil, errMonitorAuth
}

func (h *VmqHandler) monitorHeartbeat(c *gin.Context) {
	timestamp, ok := monitorTimestamp(c)
	if !ok {
		return
	}
	if c.Query("v") != "2" {
		// Preserve old connectivity probes, but never advertise authenticated monitoring.
		c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "connectivity only", "protocol": 2, "authenticated": false})
		return
	}
	payload := fmt.Sprintf("v2\nheart\n%d", timestamp)
	if _, err := monitorMerchant(model.GetDB(), payload, c.Query("sign")); err != nil {
		if errors.Is(err, errMonitorAuth) {
			monitorError(c, -1, "商户认证失败，请核对通讯密钥", false)
		} else {
			monitorError(c, -1, "读取商户配置失败，请稍后重试", true)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "success", "protocol": 2,
		"authenticated": true, "supported_types": []int{1, 2}})
}

type monitorReceipt struct {
	EventID    string
	ReceivedAt time.Time
	Type       string
	Price      string
	Amount     decimal.Decimal
}

func (r monitorReceipt) fingerprint() string {
	value := fmt.Sprintf("%s\n%d\n%s\n%s\nCNY", r.EventID, r.ReceivedAt.UnixMilli(), r.Type, r.Price)
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

var errMonitorConflict = errors.New("receipt identity reused with different content")

// applyMonitorReceipt reserves the event, matches the original time window, and credits once.
func applyMonitorReceipt(db *gorm.DB, merchantID uint, receipt monitorReceipt, now time.Time) (model.MonitorEvent, bool, error) {
	event := model.MonitorEvent{MerchantID: merchantID, EventID: receipt.EventID,
		Fingerprint: receipt.fingerprint(), ReceivedAt: receipt.ReceivedAt, Message: "未找到唯一匹配订单，请人工对账"}
	created := false
	err := db.Transaction(func(tx *gorm.DB) error {
		// Lock one active merchant before checking the event. Do not infer insertion
		// from MySQL upsert row counts, which depend on clientFoundRows.
		var merchant model.Merchant
		merchantQuery := tx.Select("id").Where("id = ? AND status = 1", merchantID)
		if db.Dialector.Name() != "sqlite" {
			merchantQuery = merchantQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := merchantQuery.First(&merchant).Error; err != nil {
			return err
		}
		var existing model.MonitorEvent
		lookup := tx.Where("merchant_id = ? AND event_id = ?", merchantID, receipt.EventID)
		if db.Dialector.Name() != "sqlite" {
			lookup = lookup.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		lookupErr := lookup.First(&existing).Error
		if lookupErr == nil {
			event = existing
			if event.Fingerprint != receipt.fingerprint() {
				return errMonitorConflict
			}
			return nil
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		created = true
		if receipt.ReceivedAt.After(now.Add(30*time.Second)) || receipt.ReceivedAt.Before(now.Add(-24*time.Hour)) {
			event.Message = "收款事件时间无效或过期，请人工对账"
			return tx.Save(&event).Error
		}
		chain := "wechat"
		if receipt.Type == "2" {
			chain = "alipay"
		}
		base := func() *gorm.DB {
			query := tx.Where("merchant_id = ? AND chain = ? AND status = ? AND created_at <= ? AND expired_at > ? AND expired_at > ?",
				merchantID, chain, model.OrderStatusPending, receipt.ReceivedAt, receipt.ReceivedAt, now).
				Where("(channel = ? OR channel = '' OR channel IS NULL)", "local").
				Where("(pay_currency = ? OR pay_currency = '' OR pay_currency IS NULL)", "CNY")
			if db.Dialector.Name() != "sqlite" {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			return query
		}
		var orders []model.Order
		if err := base().Where("unique_amount = ? OR ((unique_amount IS NULL OR unique_amount = 0) AND usdt_amount = ?)",
			receipt.Amount, receipt.Amount).Limit(2).Find(&orders).Error; err != nil {
			return err
		}
		if len(orders) != 1 {
			return nil
		}
		order := orders[0]
		updates := map[string]interface{}{"status": model.OrderStatusPaid, "actual_amount": receipt.Amount, "paid_at": &now}
		// Persist retry eligibility before launching the asynchronous merchant callback.
		if order.NotifyURL != "" {
			updates["notify_status"] = model.NotifyStatusFailed
		}
		result := tx.Model(&model.Order{}).Where("id = ? AND status = ?", order.ID, model.OrderStatusPending).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("order changed during receipt processing")
		}
		realAmount := order.SettlementAmount.Sub(order.Fee)
		balances := map[string]interface{}{"balance": gorm.Expr("balance + ?", realAmount.String())}
		if order.FeeType == model.FeeTypeBalance {
			if merchant.FrozenBalance.LessThan(order.Fee) {
				return errors.New("frozen fee is insufficient")
			}
			balances["frozen_balance"] = gorm.Expr("frozen_balance - ?", order.Fee.String())
		}
		credit := tx.Model(&model.Merchant{}).Where("id = ? AND status = 1", merchantID).Updates(balances)
		if credit.Error != nil {
			return credit.Error
		}
		event.Code = 1
		event.Message = "success"
		event.OrderID = order.ID
		event.TradeNo = order.TradeNo
		return tx.Save(&event).Error
	})
	return event, created, err
}

func (h *VmqHandler) monitorPush(c *gin.Context) {
	if c.Query("v") != "2" {
		monitorError(c, -1, "旧监控协议缺少事件标识，请升级监控端", false)
		return
	}
	timestamp, ok := monitorTimestamp(c)
	if !ok {
		return
	}
	typeCode := c.Query("type")
	if typeCode != "1" && typeCode != "2" {
		monitorError(c, -1, "不支持的收款通道", false)
		return
	}
	price := c.Query("price")
	if len(price) > 12 || !monitorAmountPattern.MatchString(price) {
		monitorError(c, -1, "人民币金额无效", false)
		return
	}
	amount, err := decimal.NewFromString(price)
	if err != nil || !monitorAmountPattern.MatchString(price) || amount.LessThan(decimal.NewFromFloat(0.01)) ||
		amount.GreaterThan(decimal.NewFromInt(100000000)) || c.Query("currency") != "CNY" {
		monitorError(c, -1, "人民币金额无效", false)
		return
	}
	eventID := c.Query("event_id")
	receivedMillis, err := strconv.ParseInt(c.Query("received_at"), 10, 64)
	if !monitorEventIDPattern.MatchString(eventID) || err != nil || receivedMillis <= 0 || receivedMillis > 253402300799999 {
		monitorError(c, -1, "收款事件标识或时间无效", false)
		return
	}
	payload := fmt.Sprintf("v2\npush\n%d\n%s\n%d\n%s\n%s\nCNY", timestamp, eventID, receivedMillis, typeCode, price)
	merchant, err := monitorMerchant(model.GetDB(), payload, c.Query("sign"))
	if err != nil {
		if errors.Is(err, errMonitorAuth) {
			monitorError(c, -1, "商户认证失败，请核对通讯密钥", false)
		} else {
			monitorError(c, -1, "读取商户配置失败，请稍后重试", true)
		}
		return
	}
	receipt := monitorReceipt{EventID: eventID, ReceivedAt: time.UnixMilli(receivedMillis), Type: typeCode, Price: price, Amount: amount}
	event, created, err := applyMonitorReceipt(model.GetDB(), merchant.ID, receipt, time.Now())
	if errors.Is(err, errMonitorConflict) {
		monitorError(c, -1, "收款事件标识与原始内容不一致", false)
		return
	}
	if err != nil {
		monitorError(c, -1, "保存收款事件失败，请稍后重试", true)
		return
	}
	if created && event.Code == 1 {
		service.GetOrderService().InvalidateOrderCache(event.TradeNo)
		go service.GetNotifyService().NotifyOrder(event.OrderID)
		var order model.Order
		if model.GetDB().First(&order, event.OrderID).Error == nil {
			go service.GetTelegramService().NotifyOrderPaid(&order)
		}
	}
	middleware.SetAPILogContext(c, event.Code, event.Message, event.TradeNo, merchant.ID, merchant.PID)
	c.JSON(http.StatusOK, gin.H{"code": event.Code, "msg": event.Message, "protocol": 2,
		"event_id": event.EventID, "trade_no": event.TradeNo, "retryable": false, "duplicate": !created})
}
