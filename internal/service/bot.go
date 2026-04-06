package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"ezpay/internal/model"

	"github.com/shopspring/decimal"
)

// BotService 机器人通知服务
type BotService struct {
	telegramToken  string
	telegramChatID string
	discordWebhook string
	enabled        bool
	mu             sync.RWMutex
}

var botService *BotService
var botOnce sync.Once

// GetBotService 获取机器人服务单例
func GetBotService() *BotService {
	botOnce.Do(func() {
		botService = &BotService{}
		botService.loadConfig()
	})
	return botService
}

// loadConfig 加载配置
func (s *BotService) loadConfig() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.telegramToken = s.getConfigValue("telegram_token", "")
	s.telegramChatID = s.getConfigValue("telegram_chat_id", "")
	s.discordWebhook = s.getConfigValue("discord_webhook", "")
	s.enabled = s.telegramToken != "" || s.discordWebhook != ""
}

// ReloadConfig 重新加载配置
func (s *BotService) ReloadConfig() {
	s.loadConfig()
}

// getConfigValue 获取系统配置值
func (s *BotService) getConfigValue(key, defaultValue string) string {
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", key).First(&config).Error; err != nil {
		return defaultValue
	}
	if config.Value == "" {
		return defaultValue
	}
	return config.Value
}

// NotifyNewOrder 通知新订单
func (s *BotService) NotifyNewOrder(order *model.Order) {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	if !enabled {
		return
	}

	message := fmt.Sprintf(`📦 *新订单*

订单号: %s
商户订单号: %s
金额: ¥%s
USDT: %s
链: %s
地址: %s
创建时间: %s`,
		order.TradeNo,
		order.OutTradeNo,
		order.Money.String(),
		order.USDTAmount.String(),
		order.Chain,
		maskAddress(order.ToAddress),
		order.CreatedAt.Format("2006-01-02 15:04:05"),
	)

	go s.sendTelegram(message)
	go s.sendDiscord("新订单通知", message, 0x3498db) // 蓝色
}

// NotifyOrderPaid 通知订单支付成功
func (s *BotService) NotifyOrderPaid(order *model.Order) {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	if !enabled {
		return
	}

	message := fmt.Sprintf(`✅ *支付成功*

订单号: %s
商户订单号: %s
金额: ¥%s
实收USDT: %s
链: %s
交易哈希: %s
支付时间: %s`,
		order.TradeNo,
		order.OutTradeNo,
		order.Money.String(),
		order.ActualAmount.String(),
		order.Chain,
		maskTxHash(order.TxHash),
		order.PaidAt.Format("2006-01-02 15:04:05"),
	)

	go s.sendTelegram(message)
	go s.sendDiscord("支付成功通知", message, 0x2ecc71) // 绿色
}

// NotifyOrderExpired 通知订单过期
func (s *BotService) NotifyOrderExpired(order *model.Order) {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	if !enabled {
		return
	}

	message := fmt.Sprintf(`⏰ *订单过期*

订单号: %s
商户订单号: %s
金额: ¥%s
USDT: %s
链: %s`,
		order.TradeNo,
		order.OutTradeNo,
		order.Money.String(),
		order.USDTAmount.String(),
		order.Chain,
	)

	go s.sendTelegram(message)
	go s.sendDiscord("订单过期通知", message, 0xe74c3c) // 红色
}

// NotifyDailyReport 发送每日报告
func (s *BotService) NotifyDailyReport() {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	if !enabled {
		return
	}

	// 获取统计数据
	stats, _ := GetOrderService().GetOrderStats(0)

	message := fmt.Sprintf(`📊 *每日报告*

📅 日期: %s

今日订单: %d
今日收款: $%s USD

总订单数: %d
待支付: %d
已支付: %d
已过期: %d`,
		time.Now().Format("2006-01-02"),
		stats.TodayOrders,
		stats.TodayUSD.String(),
		stats.TotalOrders,
		stats.PendingOrders,
		stats.PaidOrders,
		stats.ExpiredOrders,
	)

	go s.sendTelegram(message)
	go s.sendDiscord("每日报告", message, 0x9b59b6) // 紫色
}

// NotifyLargePayment 通知大额支付
func (s *BotService) NotifyLargePayment(order *model.Order, threshold decimal.Decimal) {
	if order.USDTAmount.LessThan(threshold) {
		return
	}

	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	if !enabled {
		return
	}

	message := fmt.Sprintf(`💰 *大额支付*

订单号: %s
金额: ¥%s
USDT: %s
链: %s
交易哈希: %s`,
		order.TradeNo,
		order.Money.String(),
		order.ActualAmount.String(),
		order.Chain,
		order.TxHash,
	)

	go s.sendTelegram(message)
	go s.sendDiscord("大额支付通知", message, 0xf39c12) // 黄色
}

// sendTelegram 发送Telegram消息
func (s *BotService) sendTelegram(message string) {
	s.mu.RLock()
	token := s.telegramToken
	chatID := s.telegramChatID
	s.mu.RUnlock()

	if token == "" || chatID == "" {
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Telegram marshal error: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Telegram send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Telegram response error: %s", string(body))
	}
}

// NotifySystemEvent 通知系统事件给管理员
func (s *BotService) NotifySystemEvent(message string) {
	go s.sendTelegram(message)
	go s.sendDiscord("系统事件", message, 0x95a5a6) // 灰色
}

// sendDiscord 发送Discord消息
func (s *BotService) sendDiscord(title, message string, color int) {
	s.mu.RLock()
	webhook := s.discordWebhook
	s.mu.RUnlock()

	if webhook == "" {
		return
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       title,
				"description": message,
				"color":       color,
				"timestamp":   time.Now().UTC().Format(time.RFC3339),
				"footer": map[string]string{
					"text": "EzPay",
				},
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Discord marshal error: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhook, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Discord send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Discord response error: %s", string(body))
	}
}

// StartDailyReportWorker 启动每日报告工作协程
func (s *BotService) StartDailyReportWorker() {
	go func() {
		for {
			now := time.Now()
			// 计算下一个早上9点
			next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			time.Sleep(time.Until(next))
			s.NotifyDailyReport()
		}
	}()
	log.Println("Daily report worker started")
}

// maskAddress 遮蔽地址
func maskAddress(address string) string {
	if len(address) < 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}

// maskTxHash 遮蔽交易哈希
func maskTxHash(txHash string) string {
	if len(txHash) < 16 {
		return txHash
	}
	return txHash[:8] + "..." + txHash[len(txHash)-8:]
}

// SendTestMessage 发送测试消息
func (s *BotService) SendTestMessage() error {
	message := "🔔 EzPay 测试消息\n\n这是一条测试消息，如果您收到此消息，说明机器人配置正确。"

	s.mu.RLock()
	telegramOK := s.telegramToken != "" && s.telegramChatID != ""
	discordOK := s.discordWebhook != ""
	s.mu.RUnlock()

	if telegramOK {
		s.sendTelegram(message)
	}
	if discordOK {
		s.sendDiscord("测试消息", message, 0x3498db)
	}

	if !telegramOK && !discordOK {
		return fmt.Errorf("no bot configured")
	}

	return nil
}
