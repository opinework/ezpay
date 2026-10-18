package service

import (
	"fmt"
	"io"
	"log"
	"net"
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
	mu     sync.Mutex
	sem    chan struct{}   // 并发限制信号量
	client *http.Client   // 复用 HTTP Client
}

var notifyService *NotifyService
var notifyOnce sync.Once

// GetNotifyService 获取通知服务单例
func GetNotifyService() *NotifyService {
	notifyOnce.Do(func() {
		notifyService = &NotifyService{
			sem: make(chan struct{}, 20), // 最多20个并发回调
			client: &http.Client{
				Timeout: 15 * time.Second,
				Transport: &http.Transport{
					MaxIdleConns:        50,
					MaxIdleConnsPerHost: 5,
					IdleConnTimeout:     90 * time.Second,
				},
			},
		}
	})
	return notifyService
}

// NotifyOrder 通知订单支付结果
func (s *NotifyService) NotifyOrder(orderID uint) {
	// 获取信号量，限制并发
	s.sem <- struct{}{}
	defer func() { <-s.sem }()

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

	// SSRF 防护：验证回调 URL
	if !s.isURLSafe(order.NotifyURL) {
		log.Printf("NotifyOrder: unsafe notify url blocked: %s (order: %s)", order.NotifyURL, order.TradeNo)
		model.GetDB().Model(&order).Updates(map[string]interface{}{
			"notify_status": model.NotifyStatusFailed,
		})
		return
	}

	// 构建通知参数 (v1.3.10 起含 currency/pay_currency/pay_amount，多币种商户可对账)
	params := util.BuildNotifyParamsV2(util.NotifyOrderInfo{
		PID:         order.Merchant.PID,
		TradeNo:     order.TradeNo,
		OutTradeNo:  order.OutTradeNo,
		Type:        order.Type,
		Name:        order.Name,
		Money:       order.Money.String(),
		Currency:    order.Currency,
		PayCurrency: order.PayCurrency,
		PayAmount:   order.UniqueAmount.String(), // 实际链上/通道匹配金额
		TradeStatus: "TRADE_SUCCESS",
		Param:       order.Param,
	}, order.Merchant.Key)

	// 重试通知
	maxRetry := s.getMaxRetry()
	for i := 0; i < maxRetry; i++ {
		success := s.sendNotify(order.NotifyURL, params)
		if success {
			// 原子更新 notify_count 和 status
			model.GetDB().Model(&model.Order{}).
				Where("id = ?", order.ID).
				Updates(map[string]interface{}{
					"notify_count":  order.NotifyCount + i + 1,
					"notify_status": model.NotifyStatusSuccess,
				})
			log.Printf("NotifyOrder: success for order: %s (attempt %d)", order.TradeNo, i+1)
			return
		}

		// 等待后重试（递增间隔）
		if i < maxRetry-1 {
			time.Sleep(time.Duration(i+1) * 10 * time.Second)
		}
	}

	// 通知失败 - 原子更新
	model.GetDB().Model(&model.Order{}).
		Where("id = ?", order.ID).
		Updates(map[string]interface{}{
			"notify_count":  order.NotifyCount + maxRetry,
			"notify_status": model.NotifyStatusFailed,
		})
	log.Printf("NotifyOrder: failed for order: %s after %d retries", order.TradeNo, maxRetry)

	// 回调失败通知
	go GetTelegramService().NotifyCallbackFailed(&order, maxRetry, "回调失败，已达到最大重试次数")
}

// isURLSafe 检查 URL 是否安全（防止 SSRF）
func (s *NotifyService) isURLSafe(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// 只允许 http/https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	// 解析 host
	host := parsed.Hostname()
	if host == "" {
		return false
	}

	// 禁止 localhost 和常见内网域名
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".local") ||
		strings.HasSuffix(lowerHost, ".internal") {
		return false
	}

	// 解析 IP 并检查是否为内网地址
	ip := net.ParseIP(host)
	if ip != nil {
		if isPrivateIP(ip) {
			return false
		}
	} else {
		// 域名：DNS 解析后检查所有返回的 IP
		addrs, err := net.LookupHost(host)
		if err != nil {
			return false // DNS 解析失败，拒绝
		}
		for _, addr := range addrs {
			resolvedIP := net.ParseIP(addr)
			if resolvedIP != nil && isPrivateIP(resolvedIP) {
				return false // DNS 解析到内网 IP，拒绝（防止 DNS 重绑定）
			}
		}
	}

	return true
}

// isPrivateIP 检查 IP 是否为内网/保留地址
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
		ip.Equal(net.ParseIP("169.254.169.254")) // 云元数据地址
}

// phpURLEncode 模拟 PHP 的 urlencode() 行为
// PHP urlencode: 空格→+, 字母数字和 -_.~ 保留, 其余 %XX
// Go  url.Values.Encode: 空格→+, 字母数字和 -_.~ 保留, 其余 %XX
// 两者行为基本一致，Go 的 url.Values.Encode() 已经兼容 PHP urlencode()
func phpURLEncode(params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	// Go 的 url.Values.Encode() 与 PHP urlencode() 兼容（空格→+）
	return values.Encode()
}

// sendNotify 发送通知请求（兼容 PHP 彩虹易支付）
// 使用 GET 请求，参数通过 URL 查询字符串传递
// URL 编码使用与 PHP urlencode() 一致的方式（空格→+）
func (s *NotifyService) sendNotify(notifyURL string, params map[string]string) bool {
	// 构建查询字符串（使用 PHP 兼容编码）
	queryString := phpURLEncode(params)

	// GET 请求（与 PHP epay 一致）
	fullURL := notifyURL
	if strings.Contains(notifyURL, "?") {
		fullURL += "&" + queryString
	} else {
		fullURL += "?" + queryString
	}

	resp, err := s.client.Get(fullURL)
	if err != nil {
		log.Printf("sendNotify error: %v (url: %s)", err, notifyURL)
		return false
	}
	defer resp.Body.Close()

	// 检查 HTTP 状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("sendNotify: HTTP %d (url: %s)", resp.StatusCode, notifyURL)
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		log.Printf("sendNotify read error: %v", err)
		return false
	}

	// 检查响应（忽略大小写和空白）
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

// RetryFailedNotify 重试失败的通知（带并发限制）
func (s *NotifyService) RetryFailedNotify() {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxRetry := s.getMaxRetry()
	maxTotalRetry := maxRetry * 3

	var orders []model.Order
	model.GetDB().
		Where("status = ? AND notify_status = ? AND notify_count < ?",
			model.OrderStatusPaid, model.NotifyStatusFailed, maxTotalRetry).
		Limit(50). // 每批最多处理50个
		Find(&orders)

	for _, order := range orders {
		go s.NotifyOrder(order.ID) // sem 信号量会自动限制并发
	}
}

// BuildReturnURL 构建同步返回URL
func (s *NotifyService) BuildReturnURL(order *model.Order, merchant *model.Merchant) string {
	if order.ReturnURL == "" {
		return ""
	}

	params := util.BuildNotifyParamsV2(util.NotifyOrderInfo{
		PID:         merchant.PID,
		TradeNo:     order.TradeNo,
		OutTradeNo:  order.OutTradeNo,
		Type:        order.Type,
		Name:        order.Name,
		Money:       order.Money.String(),
		Currency:    order.Currency,
		PayCurrency: order.PayCurrency,
		PayAmount:   order.UniqueAmount.String(),
		TradeStatus: "TRADE_SUCCESS",
		Param:       order.Param,
	}, merchant.Key)

	queryString := phpURLEncode(params)

	returnURL := order.ReturnURL
	if strings.Contains(returnURL, "?") {
		returnURL += "&" + queryString
	} else {
		returnURL += "?" + queryString
	}

	return returnURL
}

// StartNotifyWorker 启动通知工作协程
func (s *NotifyService) StartNotifyWorker() {
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
