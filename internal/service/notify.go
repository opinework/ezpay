package service

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ezpay/internal/model"
	"ezpay/internal/util"
)

// NotifyService 回调通知服务
type NotifyService struct {
	mu sync.Mutex
}

var notifyService *NotifyService
var notifyOnce sync.Once

// GetNotifyService 获取通知服务单例
func GetNotifyService() *NotifyService {
	notifyOnce.Do(func() {
		notifyService = &NotifyService{}
	})
	return notifyService
}

// NotifyOrder 通知订单支付结果
func (s *NotifyService) NotifyOrder(orderID uint) {
	var order model.Order
	if err := model.GetDB().Preload("Merchant").First(&order, orderID).Error; err != nil {
		log.Printf("NotifyOrder: order not found: %d", orderID)
		return
	}

	// 发送机器人通知
	GetBotService().NotifyOrderPaid(&order)

	if order.NotifyURL == "" {
		log.Printf("NotifyOrder: no notify url for order: %s", order.TradeNo)
		return
	}

	if order.Merchant == nil {
		log.Printf("NotifyOrder: merchant not found for order: %s", order.TradeNo)
		return
	}

	// 构建通知参数
	params := util.BuildNotifyParams(
		order.Merchant.PID,
		order.TradeNo,
		order.OutTradeNo,
		order.Type,
		order.Name,
		order.Money.String(),
		"TRADE_SUCCESS",
		order.Merchant.Key,
	)

	// 添加附加参数
	if order.Param != "" {
		params["param"] = order.Param
	}

	// 重试通知
	maxRetry := s.getMaxRetry()
	for i := 0; i < maxRetry; i++ {
		order.NotifyCount++

		success := s.sendNotify(order.NotifyURL, params)
		if success {
			model.GetDB().Model(&order).Updates(map[string]interface{}{
				"notify_count":  order.NotifyCount,
				"notify_status": model.NotifyStatusSuccess,
			})
			log.Printf("NotifyOrder: success for order: %s", order.TradeNo)
			return
		}

		// 等待后重试
		if i < maxRetry-1 {
			time.Sleep(time.Duration(i+1) * 10 * time.Second)
		}
	}

	// 通知失败
	model.GetDB().Model(&order).Updates(map[string]interface{}{
		"notify_count":  order.NotifyCount,
		"notify_status": model.NotifyStatusFailed,
	})
	log.Printf("NotifyOrder: failed for order: %s after %d retries", order.TradeNo, maxRetry)
}

// sendNotify 发送通知请求
func (s *NotifyService) sendNotify(notifyURL string, params map[string]string) bool {
	// 构建查询字符串
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	// GET 请求
	fullURL := notifyURL
	if strings.Contains(notifyURL, "?") {
		fullURL += "&" + values.Encode()
	} else {
		fullURL += "?" + values.Encode()
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fullURL)
	if err != nil {
		log.Printf("sendNotify error: %v", err)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("sendNotify read error: %v", err)
		return false
	}

	// 检查响应
	responseStr := strings.TrimSpace(strings.ToLower(string(body)))
	return responseStr == "success"
}

// getMaxRetry 获取最大重试次数
func (s *NotifyService) getMaxRetry() int {
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", model.ConfigKeyNotifyRetry).First(&config).Error; err != nil {
		return 5
	}
	retry, err := strconv.Atoi(config.Value)
	if err != nil {
		return 5
	}
	return retry
}

// RetryFailedNotify 重试失败的通知
func (s *NotifyService) RetryFailedNotify() {
	var orders []model.Order
	model.GetDB().
		Where("status = ? AND notify_status = ?", model.OrderStatusPaid, model.NotifyStatusFailed).
		Find(&orders)

	for _, order := range orders {
		go s.NotifyOrder(order.ID)
	}
}

// BuildReturnURL 构建同步返回URL
func (s *NotifyService) BuildReturnURL(order *model.Order, merchant *model.Merchant) string {
	if order.ReturnURL == "" {
		return ""
	}

	params := util.BuildNotifyParams(
		merchant.PID,
		order.TradeNo,
		order.OutTradeNo,
		order.Type,
		order.Name,
		order.Money.String(),
		"TRADE_SUCCESS",
		merchant.Key,
	)

	if order.Param != "" {
		params["param"] = order.Param
	}

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	returnURL := order.ReturnURL
	if strings.Contains(returnURL, "?") {
		returnURL += "&" + values.Encode()
	} else {
		returnURL += "?" + values.Encode()
	}

	return returnURL
}

// StartNotifyWorker 启动通知工作协程
func (s *NotifyService) StartNotifyWorker() {
	// 定期重试失败的通知
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			s.RetryFailedNotify()
		}
	}()

	log.Println("Notify worker started")
}

// ManualNotify 手动触发通知
func (s *NotifyService) ManualNotify(orderID uint) error {
	var order model.Order
	if err := model.GetDB().First(&order, orderID).Error; err != nil {
		return fmt.Errorf("order not found")
	}

	if order.Status != model.OrderStatusPaid {
		return fmt.Errorf("order not paid")
	}

	go s.NotifyOrder(orderID)
	return nil
}
