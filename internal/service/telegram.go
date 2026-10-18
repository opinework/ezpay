package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezpay/internal/model"

	"github.com/shopspring/decimal"
)

// TelegramService Telegram通知服务
type TelegramService struct {
	enabled     bool
	botToken    string
	apiURL      string
	client      *http.Client
	stopChan    chan struct{}
	wg          sync.WaitGroup
	running     bool
	stopped     bool // 标记 stopChan 是否已关闭
	mu          sync.Mutex
	mode        string // 接收模式: polling 或 webhook
	webhookURL  string // Webhook地址
	secretToken string // Webhook 验证密钥
}

var telegramService *TelegramService
var telegramOnce sync.Once

// NotifyType 通知类型
type NotifyType string

const (
	NotifyOrderCreated       NotifyType = "order_created"        // 订单创建
	NotifyOrderPaid          NotifyType = "order_paid"           // 订单支付成功
	NotifyOrderExpired       NotifyType = "order_expired"        // 订单过期
	NotifyBalanceChanged     NotifyType = "balance_changed"      // 余额变动
	NotifyWithdrawApplied    NotifyType = "withdraw_applied"     // 提现申请
	NotifyWithdrawApproved   NotifyType = "withdraw_approved"    // 提现审批通过
	NotifyWithdrawRejected   NotifyType = "withdraw_rejected"    // 提现被拒绝
	NotifyWithdrawPaid       NotifyType = "withdraw_paid"        // 提现已打款
	NotifyIPBlocked          NotifyType = "ip_blocked"           // IP被封禁
	NotifyChainStatusChanged NotifyType = "chain_status_changed" // 链状态变更
	NotifyWhitelistChanged   NotifyType = "whitelist_changed"    // 白名单变更
	NotifyWalletAdded        NotifyType = "wallet_added"         // 钱包添加
	NotifyWalletRemoved      NotifyType = "wallet_removed"       // 钱包移除
	NotifyWalletBalanceLow   NotifyType = "wallet_balance_low"   // 钱包余额不足
	NotifyLoginSuccess       NotifyType = "login_success"        // 登录成功
	NotifyLoginFailed        NotifyType = "login_failed"         // 登录失败(多次)
	NotifyKeyRegenerated     NotifyType = "key_regenerated"      // 密钥重置
	NotifyCallbackFailed     NotifyType = "callback_failed"      // 回调失败
	NotifySystemAlert        NotifyType = "system_alert"         // 系统警告
)

// TelegramUpdate Telegram消息更新
type TelegramUpdate struct {
	UpdateID int64           `json:"update_id"`
	Message  *TelegramMessage `json:"message"`
}

// TelegramMessage Telegram消息
type TelegramMessage struct {
	MessageID int64         `json:"message_id"`
	From      *TelegramUser `json:"from"`
	Chat      *TelegramChat `json:"chat"`
	Text      string        `json:"text"`
	Date      int64         `json:"date"`
}

// TelegramUser Telegram用户
type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// TelegramChat Telegram聊天
type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// GetTelegramService 获取Telegram服务单例
func GetTelegramService() *TelegramService {
	telegramOnce.Do(func() {
		// 默认禁用，从数据库加载配置后通过 UpdateConfig 启用
		telegramService = &TelegramService{
			enabled:     false,
			botToken:    "",
			apiURL:      "https://api.telegram.org",
			client:      &http.Client{Timeout: 40 * time.Second}, // 比 long polling timeout(30s) 长
			stopChan:    make(chan struct{}),
			mode:        "polling", // 默认轮询模式
			webhookURL:  "",
			secretToken: "",
		}
	})
	return telegramService
}

// Start 启动Telegram服务
func (s *TelegramService) Start() {
	s.mu.Lock()
	if s.running || !s.enabled || s.botToken == "" {
		s.mu.Unlock()
		return
	}
	s.running = true
	mode := s.mode
	s.mu.Unlock()

	if mode == "webhook" {
		log.Println("[Telegram] 服务启动 (Webhook模式)")
		// Webhook模式：设置webhook地址
		if err := s.setupWebhook(); err != nil {
			log.Printf("[Telegram] 设置Webhook失败: %v", err)
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return
		}
		log.Printf("[Telegram] Webhook已设置: %s", s.webhookURL)
	} else {
		log.Println("[Telegram] 服务启动 (轮询模式)")
		// 轮询模式：先删除可能存在的webhook，然后启动轮询
		s.deleteWebhook()
		s.wg.Add(1)
		go s.pollUpdates()
	}
}

// Stop 停止Telegram服务
func (s *TelegramService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	mode := s.mode
	s.running = false
	s.mu.Unlock()

	if mode == "polling" {
		// 轮询模式：关闭通道并等待goroutine结束
		s.mu.Lock()
		if !s.stopped {
			close(s.stopChan)
			s.stopped = true
		}
		s.mu.Unlock()
		s.wg.Wait()
	} else {
		// Webhook模式：删除webhook
		s.deleteWebhook()
	}

	log.Println("[Telegram] 服务停止")
}

// pollUpdates 轮询获取消息更新
func (s *TelegramService) pollUpdates() {
	defer s.wg.Done()

	var offset int64 = 0
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			updates, err := s.getUpdates(offset)
			if err != nil {
				log.Printf("[Telegram] 获取更新失败: %v", err)
				continue
			}

			for _, update := range updates {
				s.handleUpdate(update)
				offset = update.UpdateID + 1
			}
		}
	}
}

// getUpdates 获取消息更新
func (s *TelegramService) getUpdates(offset int64) ([]TelegramUpdate, error) {
	url := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=30", s.apiURL, s.botToken, offset)

	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("telegram API error")
	}

	return result.Result, nil
}

// handleUpdate 处理消息更新
func (s *TelegramService) handleUpdate(update TelegramUpdate) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}

	msg := update.Message
	if msg.Chat == nil {
		return
	}

	text := strings.TrimSpace(msg.Text)
	chatID := msg.Chat.ID

	// 处理命令
	if strings.HasPrefix(text, "/") {
		s.handleCommand(chatID, text, msg.From)
	}
}

// handleCommand 处理Bot命令
func (s *TelegramService) handleCommand(chatID int64, text string, user *TelegramUser) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/start":
		s.handleStart(chatID, user)
	case "/bind":
		s.handleBind(chatID, args, user)
	case "/unbind":
		s.handleUnbind(chatID, user)
	case "/status":
		s.handleStatus(chatID, user)
	case "/help":
		s.handleHelp(chatID)
	default:
		s.SendMessage(chatID, "❓ 未知命令，请使用 /help 查看帮助")
	}
}

// handleStart 处理 /start 命令
func (s *TelegramService) handleStart(chatID int64, user *TelegramUser) {
	if user == nil {
		s.SendMessage(chatID, "👋 欢迎使用 EzPay 通知机器人！请使用 /help 查看帮助")
		return
	}
	name := user.FirstName
	if user.LastName != "" {
		name += " " + user.LastName
	}

	msg := fmt.Sprintf(`👋 欢迎使用 EzPay 通知机器人！

您好，%s！

📌 *可用命令*:
/bind <商户号> <密钥> - 绑定商户账号
/unbind - 解除绑定
/status - 查看绑定状态
/help - 帮助信息

🔐 绑定后您将收到以下通知:
• 订单创建/支付/过期通知
• 余额变动通知
• 提现状态通知
• IP封禁警告
• 链状态变更通知
• 钱包相关通知
• 系统安全警告`, name)

	s.SendMessageMarkdown(chatID, msg)
}

// handleBind 处理 /bind 命令
func (s *TelegramService) handleBind(chatID int64, args []string, user *TelegramUser) {
	if user == nil {
		s.SendMessage(chatID, "❌ 无法获取用户信息")
		return
	}
	if len(args) < 2 {
		s.SendMessage(chatID, "❌ 用法: /bind <商户号> <密钥>\n\n例如: /bind 1001 your_merchant_key")
		return
	}

	pid := args[0]
	key := args[1]

	// 查找商户
	var merchant model.Merchant
	if err := model.GetDB().Where("p_id = ?", pid).First(&merchant).Error; err != nil {
		s.SendMessage(chatID, "❌ 商户号不存在")
		return
	}

	// 验证密钥
	if merchant.Key != key {
		s.SendMessage(chatID, "❌ 密钥错误")

		// 记录失败尝试
		log.Printf("[Telegram] 绑定失败: 商户 %s 密钥错误, ChatID: %d", pid, chatID)
		return
	}

	// 检查是否已被其他商户绑定
	var existingMerchant model.Merchant
	if err := model.GetDB().Where("telegram_chat_id = ? AND id != ?", chatID, merchant.ID).First(&existingMerchant).Error; err == nil {
		s.SendMessage(chatID, fmt.Sprintf("⚠️ 此Telegram账号已绑定商户 %s，请先解绑", existingMerchant.PID))
		return
	}

	// 更新商户的Telegram Chat ID和状态
	if err := model.GetDB().Model(&merchant).Updates(map[string]interface{}{
		"telegram_chat_id": chatID,
		"telegram_notify":  true,
		"telegram_status":  "normal",
	}).Error; err != nil {
		s.SendMessage(chatID, "❌ 绑定失败，请稍后重试")
		return
	}

	userName := user.FirstName
	if user.Username != "" {
		userName = "@" + user.Username
	}

	msg := fmt.Sprintf(`✅ *绑定成功*

商户号: %s
商户名: %s
Telegram: %s

您现在将收到该商户的所有通知消息。
使用 /unbind 可解除绑定。`, merchant.PID, merchant.Name, userName)

	s.SendMessageMarkdown(chatID, msg)
	log.Printf("[Telegram] 商户 %s 绑定成功, ChatID: %d", pid, chatID)
}

// handleUnbind 处理 /unbind 命令
func (s *TelegramService) handleUnbind(chatID int64, user *TelegramUser) {
	// 查找绑定的商户
	var merchant model.Merchant
	if err := model.GetDB().Where("telegram_chat_id = ?", chatID).First(&merchant).Error; err != nil {
		s.SendMessage(chatID, "❓ 您尚未绑定任何商户")
		return
	}

	// 解除绑定
	if err := model.GetDB().Model(&merchant).Updates(map[string]interface{}{
		"telegram_chat_id": 0,
		"telegram_notify":  false,
		"telegram_status":  "unbound",
	}).Error; err != nil {
		s.SendMessage(chatID, "❌ 解绑失败，请稍后重试")
		return
	}

	s.SendMessage(chatID, fmt.Sprintf("✅ 已解除与商户 %s (%s) 的绑定", merchant.PID, merchant.Name))
	log.Printf("[Telegram] 商户 %s 解绑, ChatID: %d", merchant.PID, chatID)
}

// handleStatus 处理 /status 命令
func (s *TelegramService) handleStatus(chatID int64, user *TelegramUser) {
	// 查找绑定的商户
	var merchant model.Merchant
	if err := model.GetDB().Where("telegram_chat_id = ?", chatID).First(&merchant).Error; err != nil {
		s.SendMessage(chatID, "❓ 您尚未绑定任何商户\n\n使用 /bind <商户号> <密钥> 进行绑定")
		return
	}

	// 获取今日统计
	var todayOrders int64
	var todayAmount float64
	today := time.Now().Format("2006-01-02")
	model.GetDB().Model(&model.Order{}).
		Where("merchant_id = ? AND DATE(created_at) = ? AND status = ?", merchant.ID, today, model.OrderStatusPaid).
		Count(&todayOrders)
	model.GetDB().Model(&model.Order{}).
		Where("merchant_id = ? AND DATE(created_at) = ? AND status = ?", merchant.ID, today, model.OrderStatusPaid).
		Select("COALESCE(SUM(money), 0)").Scan(&todayAmount)

	notifyStatus := "🔔 开启"
	if !merchant.TelegramNotify {
		notifyStatus = "🔕 关闭"
	}

	msg := fmt.Sprintf(`📊 *商户状态*

商户号: %s
商户名: %s
通知状态: %s

💰 *账户余额*
可用余额: $%s
冻结余额: $%s

📈 *今日统计*
订单数: %d
收款额: $%.2f`,
		merchant.PID, merchant.Name, notifyStatus,
		merchant.Balance.StringFixed(2), merchant.FrozenBalance.StringFixed(2),
		todayOrders, todayAmount)

	s.SendMessageMarkdown(chatID, msg)
}

// handleHelp 处理 /help 命令
func (s *TelegramService) handleHelp(chatID int64) {
	msg := `📚 *EzPay 机器人帮助*

*命令列表*:
/start - 开始使用
/bind <商户号> <密钥> - 绑定商户
/unbind - 解除绑定
/status - 查看状态和统计

*通知类型*:
📦 订单通知 - 创建、支付、过期
💰 资金通知 - 余额变动、提现状态
🚫 安全警告 - IP封禁、异常登录
⛓️ 系统通知 - 链状态、钱包变更

*注意事项*:
• 一个Telegram账号只能绑定一个商户
• 解绑后将不再收到任何通知
• 请妥善保管您的商户密钥`

	s.SendMessageMarkdown(chatID, msg)
}

// SendMessage 发送文本消息
func (s *TelegramService) SendMessage(chatID int64, text string) error {
	if !s.enabled || s.botToken == "" || chatID == 0 {
		return nil
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", s.apiURL, s.botToken)

	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	body, _ := json.Marshal(payload)
	resp, err := s.client.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	return nil
}

// SendMessageMarkdown 发送Markdown格式消息
func (s *TelegramService) SendMessageMarkdown(chatID int64, text string) error {
	if !s.enabled || s.botToken == "" || chatID == 0 {
		return nil
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", s.apiURL, s.botToken)

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, _ := json.Marshal(payload)
	resp, err := s.client.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		var result struct {
			OK          bool   `json:"ok"`
			ErrorCode   int    `json:"error_code"`
			Description string `json:"description"`
		}
		json.Unmarshal(respBody, &result)

		// 如果是用户封禁或聊天不存在，返回特殊错误
		if result.ErrorCode == 403 || result.ErrorCode == 400 {
			return fmt.Errorf("telegram_blocked: %s", result.Description)
		}
		return fmt.Errorf("telegram error %d: %s", result.ErrorCode, result.Description)
	}

	return nil
}

// SendToMerchant 发送消息给商户
func (s *TelegramService) SendToMerchant(merchantID uint, text string) error {
	var merchant model.Merchant
	if err := model.GetDB().First(&merchant, merchantID).Error; err != nil {
		return err
	}

	// 检查是否启用通知、是否绑定、状态是否正常
	if !merchant.TelegramNotify || merchant.TelegramChatID == 0 || merchant.TelegramStatus == "blocked" || merchant.TelegramStatus == "unbound" {
		return nil
	}

	err := s.SendMessageMarkdown(merchant.TelegramChatID, text)

	// 如果是用户封禁或账号问题，自动标记为 blocked
	if err != nil && (strings.Contains(err.Error(), "telegram_blocked") || strings.Contains(err.Error(), "chat not found")) {
		model.GetDB().Model(&merchant).Updates(map[string]interface{}{
			"telegram_notify": false,
			"telegram_status": "blocked",
		})
		log.Printf("Telegram账号已标记为封禁: 商户 %s (ID: %d), 原因: %v", merchant.PID, merchantID, err)
	}

	return err
}

// ============ 订单相关通知 ============

// NotifyOrderCreated 通知订单创建
func (s *TelegramService) NotifyOrderCreated(order *model.Order) {
	msg := fmt.Sprintf(`📦 *新订单创建*

订单号: %s
商户订单: %s
商品: %s
金额: ¥%s
USDT: %s
链: %s
创建时间: %s

⏰ 请等待用户支付...`,
		order.TradeNo, order.OutTradeNo, order.Name,
		order.Money.StringFixed(2), order.USDTAmount,
		strings.ToUpper(order.Chain),
		order.CreatedAt.Format("2006-01-02 15:04:05"))

	s.SendToMerchant(order.MerchantID, msg)
}

// NotifyOrderPaid 通知订单支付成功
func (s *TelegramService) NotifyOrderPaid(order *model.Order) {
	paidTime := ""
	if order.PaidAt != nil {
		paidTime = order.PaidAt.Format("2006-01-02 15:04:05")
	}

	msg := fmt.Sprintf(`✅ *订单支付成功*

订单号: %s
商户订单: %s
商品: %s
金额: ¥%s
USDT: %s
链: %s
交易哈希: %s
支付时间: %s

💰 资金已到账！`,
		order.TradeNo, order.OutTradeNo, order.Name,
		order.Money.StringFixed(2), order.USDTAmount,
		strings.ToUpper(order.Chain),
		s.maskHash(order.TxHash),
		paidTime)

	s.SendToMerchant(order.MerchantID, msg)
}

// NotifyOrderExpired 通知订单过期
func (s *TelegramService) NotifyOrderExpired(order *model.Order) {
	msg := fmt.Sprintf(`⏰ *订单已过期*

订单号: %s
商户订单: %s
商品: %s
金额: ¥%s

订单未在有效期内完成支付，已自动关闭。`,
		order.TradeNo, order.OutTradeNo, order.Name, order.Money.StringFixed(2))

	s.SendToMerchant(order.MerchantID, msg)
}

// ============ 余额相关通知 ============

// NotifyBalanceChanged 通知余额变动
func (s *TelegramService) NotifyBalanceChanged(merchantID uint, changeType string, amount decimal.Decimal, balance decimal.Decimal, remark string) {
	icon := "💰"
	if amount.IsNegative() {
		icon = "💸"
	}

	msg := fmt.Sprintf(`%s *余额变动*

类型: %s
变动: %s
当前余额: ¥%s
备注: %s
时间: %s`,
		icon, changeType,
		amount.String(),
		balance.String(),
		remark,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendToMerchant(merchantID, msg)
}

// ============ 提现相关通知 ============

// NotifyWithdrawApplied 通知提现申请已提交
func (s *TelegramService) NotifyWithdrawApplied(withdrawal *model.Withdrawal) {
	msg := fmt.Sprintf(`💳 *提现申请已提交*

提现金额: $%s
手续费: $%s
实际到账: $%s
打款方式: %s
收款账号: %s
收款人: %s

⏳ 等待管理员审核...`,
		withdrawal.Amount.StringFixed(2), withdrawal.Fee.StringFixed(2), withdrawal.RealAmount.StringFixed(2),
		withdrawal.PayMethod,
		s.maskAccount(withdrawal.Account),
		withdrawal.AccountName)

	s.SendToMerchant(withdrawal.MerchantID, msg)
}

// NotifyWithdrawApproved 通知提现审批通过
func (s *TelegramService) NotifyWithdrawApproved(withdrawal *model.Withdrawal) {
	msg := fmt.Sprintf(`✅ *提现审批通过*

提现金额: $%s
实际到账: $%s
打款方式: %s
收款账号: %s

管理员已通过您的提现申请，即将打款。`,
		withdrawal.Amount.StringFixed(2), withdrawal.RealAmount.StringFixed(2),
		withdrawal.PayMethod,
		s.maskAccount(withdrawal.Account))

	s.SendToMerchant(withdrawal.MerchantID, msg)
}

// NotifyWithdrawRejected 通知提现被拒绝
func (s *TelegramService) NotifyWithdrawRejected(withdrawal *model.Withdrawal, reason string) {
	msg := fmt.Sprintf(`❌ *提现申请被拒绝*

提现金额: $%s
拒绝原因: %s

资金已退回您的可用余额。`,
		withdrawal.Amount.StringFixed(2), reason)

	s.SendToMerchant(withdrawal.MerchantID, msg)
}

// NotifyWithdrawPaid 通知提现已打款
func (s *TelegramService) NotifyWithdrawPaid(withdrawal *model.Withdrawal) {
	msg := fmt.Sprintf(`🎉 *提现已打款*

提现金额: %s USD
实际到账: %s USD
打款方式: %s
收款账号: %s
收款人: %s

请注意查收！`,
		withdrawal.Amount.StringFixed(2), withdrawal.RealAmount.StringFixed(2),
		withdrawal.PayMethod,
		s.maskAccount(withdrawal.Account),
		withdrawal.AccountName)

	s.SendToMerchant(withdrawal.MerchantID, msg)
}

// NotifyWithdrawAddressAdded 通知管理员新增提现地址待审核
func (s *TelegramService) NotifyWithdrawAddressAdded(address *model.WithdrawAddress) {
	// 获取商户信息
	var merchant model.Merchant
	model.GetDB().First(&merchant, address.MerchantID)

	chainNames := map[string]string{
		"trc20": "TRC20 (Tron)",
		"bep20": "BEP20 (BSC)",
	}
	chainName := chainNames[address.Chain]
	if chainName == "" {
		chainName = address.Chain
	}

	msg := fmt.Sprintf(`📬 *新提现地址待审核*

商户: %s (%s)
链类型: %s
地址: %s
备注: %s
时间: %s

请前往管理后台审核！`,
		merchant.Name, merchant.PID,
		chainName,
		address.Address,
		address.Label,
		time.Now().Format("2006-01-02 15:04:05"))

	// 发送给管理员群组 (使用BotService)
	GetBotService().sendTelegram(msg)
}

// ============ 安全相关通知 ============

// NotifyIPBlocked 通知IP被封禁
func (s *TelegramService) NotifyIPBlocked(merchantID uint, ip string, reason string) {
	msg := fmt.Sprintf(`🚫 *IP已被封禁*

被封IP: %s
封禁原因: %s
时间: %s

该IP的所有请求将被拒绝。`,
		ip, reason,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendToMerchant(merchantID, msg)
}

// NotifyLoginSuccess 通知登录成功
func (s *TelegramService) NotifyLoginSuccess(merchantID uint, ip string, userAgent string) {
	msg := fmt.Sprintf(`🔓 *登录成功*

登录IP: %s
设备: %s
时间: %s

如非本人操作，请立即修改密码！`,
		ip, s.truncateUA(userAgent),
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendToMerchant(merchantID, msg)
}

// NotifyLoginFailed 通知登录失败(多次)
func (s *TelegramService) NotifyLoginFailed(merchantID uint, ip string, failCount int) {
	msg := fmt.Sprintf(`⚠️ *登录失败警告*

尝试IP: %s
失败次数: %d
时间: %s

如非本人操作，请注意账号安全！`,
		ip, failCount,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendToMerchant(merchantID, msg)
}

// NotifyKeyRegenerated 通知密钥重置
func (s *TelegramService) NotifyKeyRegenerated(merchantID uint, ip string) {
	msg := fmt.Sprintf(`🔑 *密钥已重置*

操作IP: %s
时间: %s

⚠️ 旧密钥已失效，请及时更新您的系统配置！`,
		ip, time.Now().Format("2006-01-02 15:04:05"))

	s.SendToMerchant(merchantID, msg)
}

// ============ 系统相关通知 ============

// NotifyChainStatusChanged 通知链状态变更
func (s *TelegramService) NotifyChainStatusChanged(chain string, enabled bool, reason string) {
	status := "🟢 启用"
	if !enabled {
		status = "🔴 禁用"
	}

	msg := fmt.Sprintf(`⛓️ *链状态变更*

链: %s
状态: %s
原因: %s
时间: %s`,
		strings.ToUpper(chain), status, reason,
		time.Now().Format("2006-01-02 15:04:05"))

	// 通知所有开启通知的商户
	s.broadcastToAllMerchants(msg)
}

// NotifyWhitelistChanged 通知白名单变更
func (s *TelegramService) NotifyWhitelistChanged(merchantID uint, changeType string, value string) {
	msg := fmt.Sprintf(`📋 *白名单变更*

操作: %s
内容: %s
时间: %s`,
		changeType, value,
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendToMerchant(merchantID, msg)
}

// NotifyWalletAdded 通知钱包添加
func (s *TelegramService) NotifyWalletAdded(merchantID uint, chain string, address string) {
	msg := fmt.Sprintf(`💼 *钱包已添加*

链: %s
地址: %s
时间: %s`,
		strings.ToUpper(chain),
		s.maskAddress(address),
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendToMerchant(merchantID, msg)
}

// NotifyWalletRemoved 通知钱包移除
func (s *TelegramService) NotifyWalletRemoved(merchantID uint, chain string, address string) {
	msg := fmt.Sprintf(`🗑️ *钱包已移除*

链: %s
地址: %s
时间: %s

⚠️ 该地址将不再用于收款`,
		strings.ToUpper(chain),
		s.maskAddress(address),
		time.Now().Format("2006-01-02 15:04:05"))

	s.SendToMerchant(merchantID, msg)
}

// NotifyWalletBalanceLow 通知钱包余额不足(用于TRX能量)
func (s *TelegramService) NotifyWalletBalanceLow(chain string, address string, balance string) {
	msg := fmt.Sprintf(`⚠️ *钱包余额不足*

链: %s
地址: %s
当前余额: %s

请及时充值以保证正常收款！`,
		strings.ToUpper(chain),
		s.maskAddress(address),
		balance)

	// 通知所有商户
	s.broadcastToAllMerchants(msg)
}

// NotifyCallbackFailed 通知回调失败
func (s *TelegramService) NotifyCallbackFailed(order *model.Order, failCount int, lastError string) {
	msg := fmt.Sprintf(`⚠️ *回调通知失败*

订单号: %s
商户订单: %s
失败次数: %d
错误: %s

请检查回调地址是否正常！`,
		order.TradeNo, order.OutTradeNo,
		failCount, lastError)

	s.SendToMerchant(order.MerchantID, msg)
}

// NotifySystemAlert 系统警告通知
func (s *TelegramService) NotifySystemAlert(merchantID uint, title string, content string) {
	msg := fmt.Sprintf(`🔔 *系统通知*

%s

%s

时间: %s`,
		title, content,
		time.Now().Format("2006-01-02 15:04:05"))

	if merchantID > 0 {
		s.SendToMerchant(merchantID, msg)
	} else {
		s.broadcastToAllMerchants(msg)
	}
}

// broadcastToAllMerchants 广播给所有开启通知的商户
func (s *TelegramService) broadcastToAllMerchants(msg string) {
	var merchants []model.Merchant
	model.GetDB().Where("telegram_notify = ? AND telegram_chat_id > 0", true).Find(&merchants)

	for _, merchant := range merchants {
		s.SendMessageMarkdown(merchant.TelegramChatID, msg)
	}
}

// ============ 辅助函数 ============

// maskAddress 遮蔽地址
func (s *TelegramService) maskAddress(address string) string {
	if len(address) <= 12 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}

// maskHash 遮蔽哈希
func (s *TelegramService) maskHash(hash string) string {
	if len(hash) <= 16 {
		return hash
	}
	return hash[:8] + "..." + hash[len(hash)-8:]
}

// maskAccount 遮蔽账号
func (s *TelegramService) maskAccount(account string) string {
	if len(account) <= 8 {
		return account
	}
	return account[:4] + "****" + account[len(account)-4:]
}

// truncateUA 截断UserAgent
func (s *TelegramService) truncateUA(ua string) string {
	if len(ua) > 50 {
		return ua[:50] + "..."
	}
	return ua
}

// IsEnabled 检查是否启用
func (s *TelegramService) IsEnabled() bool {
	return s.enabled && s.botToken != ""
}

// SetEnabled 设置启用状态
func (s *TelegramService) SetEnabled(enabled bool) {
	s.mu.Lock()
	s.enabled = enabled
	s.mu.Unlock()
}

// UpdateConfig 更新配置（兼容旧版本，仅更新 token 和启用状态）
func (s *TelegramService) UpdateConfig(enabled bool, botToken string) {
	s.UpdateFullConfig(enabled, botToken, s.mode, s.webhookURL, s.secretToken)
}

// UpdateFullConfig 更新完整配置（包括模式、webhook URL 和 secret）
func (s *TelegramService) UpdateFullConfig(enabled bool, botToken string, mode string, webhookURL string, secret string) {
	s.mu.Lock()
	oldToken := s.botToken
	oldMode := s.mode
	oldWebhookURL := s.webhookURL
	oldSecret := s.secretToken
	s.enabled = enabled
	s.botToken = botToken
	s.mode = mode
	s.webhookURL = webhookURL
	s.secretToken = secret
	wasRunning := s.running
	s.mu.Unlock()

	// 如果配置改变了且之前在运行，需要重启
	needRestart := wasRunning && (oldToken != botToken || oldMode != mode || oldWebhookURL != webhookURL || oldSecret != secret)
	if needRestart {
		s.Stop()
		// 重新初始化 stopChan
		s.mu.Lock()
		s.stopChan = make(chan struct{})
		s.stopped = false
		s.mu.Unlock()
	}

	if enabled && botToken != "" {
		if !s.running {
			// 启动前确保 stopChan 已初始化
			s.mu.Lock()
			if s.stopped || s.stopChan == nil {
				s.stopChan = make(chan struct{})
				s.stopped = false
			}
			s.mu.Unlock()
			s.Start()
		}
	} else if s.running {
		s.Stop()
	}
}

// GetMode 获取当前模式
func (s *TelegramService) GetMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// GetWebhookURL 获取 webhook URL
func (s *TelegramService) GetWebhookURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.webhookURL
}

// setupWebhook 设置 Telegram Webhook
func (s *TelegramService) setupWebhook() error {
	if s.webhookURL == "" {
		return fmt.Errorf("webhook URL 不能为空")
	}

	url := fmt.Sprintf("%s/bot%s/setWebhook", s.apiURL, s.botToken)
	payload := map[string]interface{}{
		"url":             s.webhookURL,
		"max_connections": 40,
		"allowed_updates": []string{"message"}, // 只接收消息更新
		"secret_token":    s.secretToken,        // 验证密钥，Telegram 会在请求头中携带
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	resp, err := s.client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.OK {
		return fmt.Errorf("设置失败: %s", result.Description)
	}

	return nil
}

// deleteWebhook 删除 Telegram Webhook
func (s *TelegramService) deleteWebhook() error {
	url := fmt.Sprintf("%s/bot%s/deleteWebhook", s.apiURL, s.botToken)

	resp, err := s.client.Post(url, "application/json", nil)
	if err != nil {
		log.Printf("[Telegram] 删除Webhook请求失败: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[Telegram] 解析删除Webhook响应失败: %v", err)
		return err
	}

	if !result.OK {
		log.Printf("[Telegram] 删除Webhook失败: %s", result.Description)
		return fmt.Errorf("删除失败: %s", result.Description)
	}

	log.Println("[Telegram] Webhook已删除")
	return nil
}

// VerifyWebhookSecret 验证 Webhook 请求的 secret token
func (s *TelegramService) VerifyWebhookSecret(token string) bool {
	s.mu.Lock()
	secret := s.secretToken
	s.mu.Unlock()

	// 未配置 secret 时拒绝所有请求
	if secret == "" {
		return false
	}
	return token != "" && token == secret
}

// HandleWebhook 处理 Telegram Webhook 请求
func (s *TelegramService) HandleWebhook(update *TelegramUpdate) {
	if !s.IsEnabled() {
		return
	}
	if update == nil || update.Message == nil {
		return
	}

	s.handleUpdate(*update)
}

// GetWebhookInfo 查询当前 Webhook 状态
func (s *TelegramService) GetWebhookInfo() (map[string]interface{}, error) {
	if s.botToken == "" {
		return nil, fmt.Errorf("未配置 Bot Token")
	}

	url := fmt.Sprintf("%s/bot%s/getWebhookInfo", s.apiURL, s.botToken)
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK     bool                   `json:"ok"`
		Result map[string]interface{} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("查询失败")
	}

	return result.Result, nil
}
