package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"ezpay/internal/service"

	"github.com/gin-gonic/gin"
)

// TelegramHandler Telegram处理器
type TelegramHandler struct {
	// 限制并发 webhook 处理 goroutine 数量
	semaphore chan struct{}
}

// NewTelegramHandler 创建处理器
func NewTelegramHandler() *TelegramHandler {
	return &TelegramHandler{
		semaphore: make(chan struct{}, 50), // 最多50个并发处理
	}
}

// HandleWebhook 处理 Telegram Webhook 请求
// POST /telegram/webhook
func (h *TelegramHandler) HandleWebhook(c *gin.Context) {
	telegramService := service.GetTelegramService()

	// 检查服务是否启用
	if !telegramService.IsEnabled() {
		c.String(http.StatusOK, "ok")
		return
	}

	// 验证 Telegram 请求的 secret token
	secretToken := c.GetHeader("X-Telegram-Bot-Api-Secret-Token")
	if !telegramService.VerifyWebhookSecret(secretToken) {
		log.Printf("[Telegram Webhook] 验证失败，拒绝请求 (IP: %s)", c.ClientIP())
		c.String(http.StatusForbidden, "forbidden")
		return
	}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("[Telegram Webhook] 读取请求体失败: %v", err)
		c.String(http.StatusBadRequest, "bad request")
		return
	}

	// 解析 Telegram Update
	var update service.TelegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		log.Printf("[Telegram Webhook] 解析请求失败: %v", err)
		c.String(http.StatusBadRequest, "bad request")
		return
	}

	// 异步处理更新，使用信号量限制并发数
	select {
	case h.semaphore <- struct{}{}:
		go func() {
			defer func() { <-h.semaphore }()
			telegramService.HandleWebhook(&update)
		}()
	default:
		log.Printf("[Telegram Webhook] 并发处理已满，丢弃更新 %d", update.UpdateID)
	}

	// 立即返回 200 OK 给 Telegram
	c.String(http.StatusOK, "ok")
}
