package handler

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"ezpay/config"
	"ezpay/internal/model"
	"ezpay/internal/service"
	"ezpay/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// MerchantHandler 商户处理器
type MerchantHandler struct {
	cfg *config.Config
}

// NewMerchantHandler 创建商户处理器
func NewMerchantHandler(cfg *config.Config) *MerchantHandler {
	return &MerchantHandler{cfg: cfg}
}

// Login 商户登录
func (h *MerchantHandler) Login(c *gin.Context) {
	var req struct {
		PID      string `json:"pid" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 查询商户
	var merchant model.Merchant
	log.Printf("[Merchant Login] Looking for PID: %s", req.PID)

	// 先查询所有商户数量
	var count int64
	model.DB.Model(&model.Merchant{}).Count(&count)
	log.Printf("[Merchant Login] Total merchants in database: %d", count)

	if err := model.DB.Where("p_id = ?", req.PID).First(&merchant).Error; err != nil {
		log.Printf("[Merchant Login] Query error: %v", err)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户不存在"})
		return
	}
	log.Printf("[Merchant Login] Found merchant: ID=%d, PID=%s, Name=%s, Status=%d, HasPassword=%v",
		merchant.ID, merchant.PID, merchant.Name, merchant.Status, merchant.Password != "")

	// 检查状态
	if merchant.Status != 1 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户已禁用"})
		return
	}

	// 验证密码
	if merchant.Password == "" {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户未设置密码，请联系管理员"})
		return
	}

	if !util.CheckPassword(req.Password, merchant.Password) {
		// 登录失败通知（可以在此处统计失败次数，这里简化处理）
		go service.GetTelegramService().NotifyLoginFailed(merchant.ID, c.ClientIP(), 1)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "密码错误"})
		return
	}

	// 生成JWT Token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type":        "merchant",
		"merchant_id": merchant.ID,
		"pid":         merchant.PID,
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "Token生成失败"})
		return
	}

	// 登录成功通知
	go service.GetTelegramService().NotifyLoginSuccess(merchant.ID, c.ClientIP(), c.GetHeader("User-Agent"))

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "登录成功",
		"data": gin.H{
			"token": tokenString,
			"merchant": gin.H{
				"id":    merchant.ID,
				"pid":   merchant.PID,
				"name":  merchant.Name,
				"email": merchant.Email,
			},
		},
	})
}

// GetProfile 获取商户信息
func (h *MerchantHandler) GetProfile(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"id":               merchant.ID,
			"pid":              merchant.PID,
			"name":             merchant.Name,
			"email":            merchant.Email,
			"notify_url":       merchant.NotifyURL,
			"return_url":       merchant.ReturnURL,
			"balance":          merchant.Balance,
			"status":           merchant.Status,
			"telegram_chat_id": merchant.TelegramChatID,
			"telegram_notify":  merchant.TelegramNotify,
			"created_at":       merchant.CreatedAt,
		},
	})
}

// UpdateProfile 更新商户信息
func (h *MerchantHandler) UpdateProfile(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	var req struct {
		Name      string `json:"name"`
		Email     string `json:"email"`
		NotifyURL string `json:"notify_url"`
		ReturnURL string `json:"return_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.NotifyURL != "" {
		updates["notify_url"] = req.NotifyURL
	}
	if req.ReturnURL != "" {
		updates["return_url"] = req.ReturnURL
	}

	if len(updates) > 0 {
		model.DB.Model(merchant).Updates(updates)
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功"})
}

// ChangePassword 修改密码
func (h *MerchantHandler) ChangePassword(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误，密码至少6位"})
		return
	}

	// 验证旧密码
	if !util.CheckPassword(req.OldPassword, merchant.Password) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "原密码错误"})
		return
	}

	// 更新密码
	hashedPassword, _ := util.HashPassword(req.NewPassword)
	model.DB.Model(merchant).Update("password", hashedPassword)

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "密码修改成功"})
}

// GetKey 获取API密钥
func (h *MerchantHandler) GetKey(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"pid": merchant.PID,
			"key": merchant.Key,
		},
	})
}

// ResetKey 重置API密钥
func (h *MerchantHandler) ResetKey(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	newKey := util.GenerateMerchantKey()
	model.DB.Model(merchant).Update("key", newKey)

	// 密钥重置通知
	go service.GetTelegramService().NotifyKeyRegenerated(merchant.ID, c.ClientIP())

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "密钥已重置",
		"data": gin.H{
			"key": newKey,
		},
	})
}

// Dashboard 商户仪表盘 (金额统一使用 USD)
func (h *MerchantHandler) Dashboard(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)

	// 今日统计 (使用 actual_amount 作为 USD)
	today := time.Now().Format("2006-01-02")
	var todayStats struct {
		Count     int64   `json:"count"`
		AmountUSD float64 `json:"amount_usd"`
	}
	model.DB.Model(&model.Order{}).
		Where("merchant_id = ? AND DATE(created_at) = ? AND status = 1", merchantID, today).
		Select("COUNT(*) as count, COALESCE(SUM(settlement_amount), 0) as amount_usd").
		Scan(&todayStats)

	// 总统计 (使用 settlement_amount 作为 USD)
	var totalStats struct {
		Count     int64   `json:"count"`
		AmountUSD float64 `json:"amount_usd"`
	}
	model.DB.Model(&model.Order{}).
		Where("merchant_id = ? AND status = 1", merchantID).
		Select("COUNT(*) as count, COALESCE(SUM(settlement_amount), 0) as amount_usd").
		Scan(&totalStats)

	// 待支付订单数
	var pendingCount int64
	model.DB.Model(&model.Order{}).
		Where("merchant_id = ? AND status = 0", merchantID).
		Count(&pendingCount)

	// 最近7天趋势 (使用 settlement_amount 作为 USD)
	var trends []struct {
		Date      string  `json:"date"`
		Count     int64   `json:"count"`
		AmountUSD float64 `json:"amount_usd"`
	}
	model.DB.Model(&model.Order{}).
		Where("merchant_id = ? AND status = 1 AND created_at >= ?", merchantID, time.Now().AddDate(0, 0, -7)).
		Select("DATE(created_at) as date, COUNT(*) as count, COALESCE(SUM(settlement_amount), 0) as amount_usd").
		Group("DATE(created_at)").
		Order("date ASC").
		Scan(&trends)

	c.JSON(http.StatusOK, gin.H{
		"code":     1,
		"currency": "USD",
		"data": gin.H{
			"today": gin.H{
				"orders": todayStats.Count,
				"amount": todayStats.AmountUSD,
			},
			"total": gin.H{
				"orders": totalStats.Count,
				"amount": totalStats.AmountUSD,
			},
			"pending": pendingCount,
			"trends":  trends,
		},
	})
}

// DashboardTrend 获取趋势数据
func (h *MerchantHandler) DashboardTrend(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)
	period := c.DefaultQuery("period", "week")

	var startDate time.Time
	var dateFormat string
	var groupBy string

	now := time.Now()
	switch period {
	case "today":
		// 当日按小时统计
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		dateFormat = "%H:00"
		groupBy = "HOUR(created_at)"
	case "week":
		// 最近7天按天统计
		startDate = now.AddDate(0, 0, -6)
		dateFormat = "%m-%d"
		groupBy = "DATE(created_at)"
	case "month":
		// 最近30天按天统计
		startDate = now.AddDate(0, 0, -29)
		dateFormat = "%m-%d"
		groupBy = "DATE(created_at)"
	case "3months":
		// 最近3个月按周统计
		startDate = now.AddDate(0, -3, 0)
		dateFormat = "%Y-%m-%d"
		groupBy = "YEARWEEK(created_at, 1)"
	default:
		startDate = now.AddDate(0, 0, -6)
		dateFormat = "%m-%d"
		groupBy = "DATE(created_at)"
	}

	var trends []struct {
		Label     string  `json:"label"`
		Count     int64   `json:"count"`
		AmountUSD float64 `json:"amount_usd"`
	}

	query := model.DB.Model(&model.Order{}).
		Where("merchant_id = ? AND status = 1 AND created_at >= ?", merchantID, startDate)

	// 使用 settlement_amount 作为 USD 金额
	if period == "today" {
		query.Select("DATE_FORMAT(created_at, ?) as label, COUNT(*) as count, COALESCE(SUM(settlement_amount), 0) as amount_usd", dateFormat)
	} else if period == "3months" {
		query.Select("MIN(DATE_FORMAT(created_at, ?)) as label, COUNT(*) as count, COALESCE(SUM(settlement_amount), 0) as amount_usd", dateFormat)
	} else {
		query.Select("DATE_FORMAT(created_at, ?) as label, COUNT(*) as count, COALESCE(SUM(settlement_amount), 0) as amount_usd", dateFormat)
	}

	query.Group(groupBy).Order("label ASC").Scan(&trends)

	// 构建返回数据
	labels := make([]string, len(trends))
	orders := make([]int64, len(trends))
	amounts := make([]float64, len(trends))

	for i, t := range trends {
		labels[i] = t.Label
		orders[i] = t.Count
		amounts[i] = t.AmountUSD
	}

	c.JSON(http.StatusOK, gin.H{
		"code":     1,
		"currency": "USD",
		"data": gin.H{
			"labels":  labels,
			"orders":  orders,
			"amounts": amounts,
		},
	})
}

// ListOrders 订单列表
func (h *MerchantHandler) ListOrders(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	tradeNo := c.Query("trade_no")
	outTradeNo := c.Query("out_trade_no")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := model.DB.Model(&model.Order{}).Where("merchant_id = ?", merchantID)

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if tradeNo != "" {
		query = query.Where("trade_no LIKE ?", "%"+tradeNo+"%")
	}
	if outTradeNo != "" {
		query = query.Where("out_trade_no LIKE ?", "%"+outTradeNo+"%")
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate+" 00:00:00")
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate+" 23:59:59")
	}

	var total int64
	query.Count(&total)

	var orders []model.Order
	query.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&orders)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"list":      orders,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetOrder 获取订单详情
func (h *MerchantHandler) GetOrder(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)
	tradeNo := c.Param("trade_no")

	var order model.Order
	if err := model.DB.Where("merchant_id = ? AND trade_no = ?", merchantID, tradeNo).First(&order).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "订单不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": order,
	})
}

// ConfirmPayment 商户手动确认收款
func (h *MerchantHandler) ConfirmPayment(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)
	tradeNo := c.Param("trade_no")

	var order model.Order
	if err := model.DB.Where("merchant_id = ? AND trade_no = ?", merchantID, tradeNo).First(&order).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "订单不存在"})
		return
	}

	// 只能确认待支付或已过期的订单
	if order.Status != model.OrderStatusPending && order.Status != model.OrderStatusExpired {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "订单状态不正确，只能确认待支付或已过期订单"})
		return
	}

	// 只能确认法币订单（微信/支付宝）
	if !util.IsFiatChain(order.Chain) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "只能手动确认微信/支付宝订单"})
		return
	}

	// 标记订单已支付
	now := time.Now()
	updates := map[string]interface{}{
		"status":        model.OrderStatusPaid,
		"tx_hash":       "MANUAL_CONFIRM_" + fmt.Sprintf("%d", now.Unix()),
		"actual_amount": order.UniqueAmount,
		"paid_at":       &now,
	}

	if err := model.DB.Model(&order).Updates(updates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "确认失败: " + err.Error()})
		return
	}

	// 重新加载订单数据
	model.DB.First(&order, order.ID)

	// 触发回调通知
	go service.GetNotifyService().NotifyOrder(order.ID)

	// 触发 Telegram 通知
	go service.GetTelegramService().NotifyOrderPaid(&order)

	log.Printf("Merchant %d manually confirmed order %s", merchantID, tradeNo)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "确认成功，已触发回调通知",
	})
}

// CreateTestOrder 商户创建测试订单
func (h *MerchantHandler) CreateTestOrder(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)

	var req struct {
		Type     string `json:"type" binding:"required"`
		Money    string `json:"money" binding:"required"`
		Currency string `json:"currency"` // 货币类型: CNY, USD, USDT, EUR (默认 CNY)
		Name     string `json:"name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 获取商户信息
	var merchant model.Merchant
	if err := model.GetDB().First(&merchant, merchantID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户不存在"})
		return
	}

	// 生成唯一的商户订单号
	outTradeNo := fmt.Sprintf("TEST%s%d", time.Now().Format("20060102150405"), time.Now().UnixNano()%10000)

	// 商品名称
	name := req.Name
	if name == "" {
		name = "测试订单"
	}

	// 创建订单
	orderService := service.GetOrderService()
	orderReq := &service.CreateOrderRequest{
		MerchantPID: merchant.PID,
		Type:        req.Type,
		OutTradeNo:  outTradeNo,
		NotifyURL:   merchant.NotifyURL,
		ReturnURL:   merchant.ReturnURL,
		Name:        name,
		Money:       req.Money,
		Currency:    req.Currency,
		Param:       "test=1",
		ClientIP:    c.ClientIP(),
	}

	resp, err := orderService.CreateOrder(orderReq)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	log.Printf("Merchant %d created test order %s", merchantID, resp.TradeNo)

	c.JSON(http.StatusOK, gin.H{
		"code":     1,
		"msg":      "测试订单创建成功",
		"trade_no": resp.TradeNo,
		"pay_url":  "/cashier/" + resp.TradeNo,
	})
}

// CancelOrder 商户取消订单
func (h *MerchantHandler) CancelOrder(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)
	tradeNo := c.Param("trade_no")

	var order model.Order
	if err := model.DB.Where("merchant_id = ? AND trade_no = ?", merchantID, tradeNo).First(&order).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "订单不存在"})
		return
	}

	// 只能取消待支付的订单
	if order.Status != model.OrderStatusPending {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "只能取消待支付订单"})
		return
	}

	// 标记订单已过期/取消
	if err := model.DB.Model(&order).Update("status", model.OrderStatusExpired).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "取消失败: " + err.Error()})
		return
	}

	log.Printf("Merchant %d cancelled order %s", merchantID, tradeNo)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "订单已取消",
	})
}

// ListWallets 钱包列表
func (h *MerchantHandler) ListWallets(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)

	var wallets []model.Wallet
	model.DB.Where("merchant_id = ?", merchantID).Find(&wallets)

	// 获取链状态
	chainStatus := service.GetBlockchainService().GetChainStatus()

	// 添加链状态信息
	type WalletWithChain struct {
		model.Wallet
		ChainEnabled bool `json:"chain_enabled"`
	}

	result := make([]WalletWithChain, len(wallets))
	for i, w := range wallets {
		result[i] = WalletWithChain{
			Wallet:       w,
			ChainEnabled: chainStatus[w.Chain],
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": result,
	})
}

// CreateWallet 创建钱包
func (h *MerchantHandler) CreateWallet(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)

	var req struct {
		Chain   string `json:"chain" binding:"required"`
		Address string `json:"address"` // 对于微信/支付宝可以为空，自动生成
		Label   string `json:"label"`
		QRCode  string `json:"qrcode"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 验证链类型
	if !util.IsValidChain(req.Chain) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "不支持的链类型"})
		return
	}

	// 法币收款特殊处理：使用二维码解析的地址，必须上传收款码
	if util.IsFiatChain(req.Chain) {
		// 使用从二维码解析出的支付链接作为地址
		// 如果地址为空（没有上传二维码或解析失败），报错
		if req.Address == "" {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请先上传收款码图片"})
			return
		}
		if req.QRCode == "" {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请上传收款码图片"})
			return
		}
	} else {
		// USDT等加密货币必须填写地址
		if req.Address == "" {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请输入钱包地址"})
			return
		}
	}

	// 检查钱包数量限制
	var merchant model.Merchant
	if err := model.DB.First(&merchant, merchantID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户不存在"})
		return
	}

	if merchant.WalletLimit > 0 {
		var walletCount int64
		model.DB.Model(&model.Wallet{}).Where("merchant_id = ?", merchantID).Count(&walletCount)
		if int(walletCount) >= merchant.WalletLimit {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": fmt.Sprintf("钱包数量已达上限(%d个)", merchant.WalletLimit)})
			return
		}
	}

	// 检查是否已存在 (只对非法币类型检查地址重复)
	if !util.IsFiatChain(req.Chain) {
		var count int64
		model.DB.Model(&model.Wallet{}).
			Where("merchant_id = ? AND chain = ? AND address = ?", merchantID, req.Chain, req.Address).
			Count(&count)
		if count > 0 {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "该地址已存在"})
			return
		}
	}

	wallet := model.Wallet{
		MerchantID: merchantID,
		Chain:      req.Chain,
		Address:    req.Address,
		Label:      req.Label,
		QRCode:     req.QRCode,
		Status:     1,
	}

	if err := model.DB.Create(&wallet).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "创建失败: " + err.Error()})
		return
	}

	// 钱包添加通知
	go service.GetTelegramService().NotifyWalletAdded(merchantID, req.Chain, req.Address)

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "创建成功", "data": wallet})
}

// UpdateWallet 更新钱包
func (h *MerchantHandler) UpdateWallet(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)
	id, _ := strconv.Atoi(c.Param("id"))

	var wallet model.Wallet
	if err := model.DB.Where("id = ? AND merchant_id = ?", id, merchantID).First(&wallet).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "钱包不存在"})
		return
	}

	var req struct {
		Label  string `json:"label"`
		QRCode string `json:"qrcode"`
		Status *int8  `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	updates := map[string]interface{}{}
	if req.Label != "" {
		updates["label"] = req.Label
	}
	if req.QRCode != "" {
		updates["qrcode"] = req.QRCode
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if len(updates) > 0 {
		model.DB.Model(&wallet).Updates(updates)
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功"})
}

// DeleteWallet 删除钱包
func (h *MerchantHandler) DeleteWallet(c *gin.Context) {
	merchantID := c.MustGet("merchant_id").(uint)
	id, _ := strconv.Atoi(c.Param("id"))

	// 检查钱包是否存在且属于该商户
	var wallet model.Wallet
	if err := model.DB.Where("id = ? AND merchant_id = ?", id, merchantID).First(&wallet).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "钱包不存在"})
		return
	}

	// 检查是否有订单使用过该钱包
	var orderCount int64
	model.DB.Model(&model.Order{}).Where("wallet_id = ?", id).Count(&orderCount)

	if orderCount > 0 {
		// 有使用记录，只能禁用不能删除
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": fmt.Sprintf("该钱包已有%d笔订单使用记录，无法删除，请使用禁用功能", orderCount)})
		return
	}

	// 没有使用记录，彻底删除（不是软删除）
	if err := model.DB.Unscoped().Delete(&wallet).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "删除失败"})
		return
	}

	// 钱包移除通知
	go service.GetTelegramService().NotifyWalletRemoved(merchantID, wallet.Chain, wallet.Address)

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}

// UploadQRCode 上传收款码
func (h *MerchantHandler) UploadQRCode(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请选择文件"})
		return
	}

	// 检查文件大小 (最大2MB)
	if file.Size > 2*1024*1024 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "文件大小不能超过2MB"})
		return
	}

	// 生成文件名
	ext := ".png"
	if len(file.Filename) > 4 {
		ext = file.Filename[len(file.Filename)-4:]
	}
	filename := util.GenerateRandomHex(16) + ext
	// 确保上传目录存在（使用配置的数据目录）
	dataDir := config.Get().Storage.DataDir
	qrcodeDir := dataDir + "/qrcode"
	os.MkdirAll(qrcodeDir, 0755)
	filepath := qrcodeDir + "/" + filename

	// 保存文件
	log.Printf("Upload path: %s", filepath)
	if err := c.SaveUploadedFile(file, filepath); err != nil {
		log.Printf("Save file error: %v", err)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "保存文件失败: " + err.Error()})
		return
	}

	// 解析二维码内容
	qrContent, err := util.DecodeQRCodeFromFile(filepath)
	if err != nil {
		// 删除已上传的文件
		os.Remove(filepath)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "无法识别二维码: " + err.Error()})
		return
	}

	// 验证是否是有效的支付二维码
	if !util.IsValidFiatQRCode(qrContent) {
		os.Remove(filepath)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "不是有效的微信/支付宝收款码"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "上传成功",
		"data": gin.H{
			"url":     "/static/qrcode/" + filename,
			"address": qrContent,
		},
	})
}

// GetChainStatus 获取链状态 (只读)
func (h *MerchantHandler) GetChainStatus(c *gin.Context) {
	chainStatus := service.GetBlockchainService().GetChainStatus()

	// 转换为列表格式，商户只能查看状态
	chains := []gin.H{
		{"chain": "trc20", "name": "TRC20 (Tron)", "enabled": chainStatus["trc20"]},
		{"chain": "erc20", "name": "ERC20 (Ethereum)", "enabled": chainStatus["erc20"]},
		{"chain": "bep20", "name": "BEP20 (BSC)", "enabled": chainStatus["bep20"]},
		{"chain": "polygon", "name": "Polygon", "enabled": chainStatus["polygon"]},
		{"chain": "optimism", "name": "Optimism", "enabled": chainStatus["optimism"]},
		{"chain": "arbitrum", "name": "Arbitrum", "enabled": chainStatus["arbitrum"]},
		{"chain": "avalanche", "name": "Avalanche", "enabled": chainStatus["avalanche"]},
		{"chain": "base", "name": "Base", "enabled": chainStatus["base"]},
		{"chain": "wechat", "name": "微信支付", "enabled": true},
		{"chain": "alipay", "name": "支付宝", "enabled": true},
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": chains,
	})
}

// ============ 提现管理 ============

// GetBalance 获取余额信息
func (h *MerchantHandler) GetBalance(c *gin.Context) {
	merchantID := c.GetUint("merchant_id")

	var merchant model.Merchant
	if err := model.GetDB().First(&merchant, merchantID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"balance":        merchant.Balance,
			"frozen_balance": merchant.FrozenBalance,
			"available":      merchant.Balance - merchant.FrozenBalance,
		},
	})
}

// GetRechargeAddresses 获取充值地址（系统钱包）
func (h *MerchantHandler) GetRechargeAddresses(c *gin.Context) {
	// 获取系统钱包作为充值地址（merchant_id = 0 的钱包）
	var wallets []model.Wallet
	if err := model.GetDB().Where("merchant_id = 0 AND status = 1").Find(&wallets).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []interface{}{}})
		return
	}

	// 只支持 TRC20、BEP20 两种充值方式
	supportedChains := map[string]string{
		"trc20": "TRC20 (Tron USDT)",
		"bep20": "BEP20 (BSC USDT)",
	}

	var addresses []gin.H
	for _, w := range wallets {
		name, ok := supportedChains[w.Chain]
		if !ok {
			continue
		}
		item := gin.H{
			"chain":   w.Chain,
			"name":    name,
			"address": w.Address,
		}
		addresses = append(addresses, item)
	}

	// 获取客服链接
	var serviceTelegram, serviceDiscord string
	var cfg model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", model.ConfigKeyServiceTelegram).First(&cfg).Error; err == nil {
		serviceTelegram = cfg.Value
	}
	if err := model.GetDB().Where("`key` = ?", model.ConfigKeyServiceDiscord).First(&cfg).Error; err == nil {
		serviceDiscord = cfg.Value
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"addresses":        addresses,
			"service_telegram": serviceTelegram,
			"service_discord":  serviceDiscord,
		},
	})
}

// CreateWithdrawal 申请提现
func (h *MerchantHandler) CreateWithdrawal(c *gin.Context) {
	merchantID := c.GetUint("merchant_id")

	var req service.WithdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误: " + err.Error()})
		return
	}

	withdrawal, err := service.GetWithdrawService().CreateWithdrawal(merchantID, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "提现申请已提交",
		"data": withdrawal,
	})
}

// ListWithdrawals 提现记录列表
func (h *MerchantHandler) ListWithdrawals(c *gin.Context) {
	merchantID := c.GetUint("merchant_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	statusStr := c.Query("status")

	var status *model.WithdrawStatus
	if statusStr != "" {
		s, _ := strconv.Atoi(statusStr)
		st := model.WithdrawStatus(s)
		status = &st
	}

	withdrawals, total, err := service.GetWithdrawService().ListWithdrawals(merchantID, status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"data":  withdrawals,
		"total": total,
	})
}

// UpdateWalletMode 更新钱包模式
func (h *MerchantHandler) UpdateWalletMode(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	var req struct {
		WalletMode int8 `json:"wallet_mode" binding:"required,min=1,max=3"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误，钱包模式必须为1-3"})
		return
	}

	if err := model.DB.Model(merchant).Update("wallet_mode", req.WalletMode).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "更新失败"})
		return
	}

	modeNames := map[int8]string{
		1: "仅系统钱包",
		2: "仅个人钱包",
		3: "两者同时使用",
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "钱包模式已更新为: " + modeNames[req.WalletMode],
		"data": gin.H{
			"wallet_mode": req.WalletMode,
		},
	})
}

// GetWalletMode 获取钱包模式
func (h *MerchantHandler) GetWalletMode(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	modeNames := map[int8]string{
		1: "仅系统钱包",
		2: "仅个人钱包",
		3: "两者同时使用",
	}

	// 获取系统手续费率配置
	var systemFeeRate, personalFeeRate string
	var cfgSystem, cfgPersonal model.SystemConfig
	if err := model.DB.Where("`key` = ?", model.ConfigKeySystemWalletFeeRate).First(&cfgSystem).Error; err == nil {
		systemFeeRate = cfgSystem.Value
	} else {
		systemFeeRate = "0.02"
	}
	if err := model.DB.Where("`key` = ?", model.ConfigKeyPersonalWalletFeeRate).First(&cfgPersonal).Error; err == nil {
		personalFeeRate = cfgPersonal.Value
	} else {
		personalFeeRate = "0.01"
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"wallet_mode":            merchant.WalletMode,
			"wallet_mode_name":       modeNames[merchant.WalletMode],
			"system_wallet_fee_rate":   systemFeeRate,
			"personal_wallet_fee_rate": personalFeeRate,
		},
	})
}

// ============ 提现地址管理 ============

// ListWithdrawAddresses 获取提现地址列表
func (h *MerchantHandler) ListWithdrawAddresses(c *gin.Context) {
	merchantID := c.GetUint("merchant_id")

	var addresses []model.WithdrawAddress
	model.DB.Where("merchant_id = ?", merchantID).Order("is_default DESC, id DESC").Find(&addresses)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": addresses,
	})
}

// CreateWithdrawAddress 创建提现地址
func (h *MerchantHandler) CreateWithdrawAddress(c *gin.Context) {
	merchantID := c.GetUint("merchant_id")

	var req struct {
		Chain   string `json:"chain" binding:"required"`
		Address string `json:"address" binding:"required"`
		Label   string `json:"label"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 验证链类型，支持 bep20, trc20, polygon, optimism
	validChains := map[string]bool{"bep20": true, "trc20": true, "polygon": true, "optimism": true}
	if !validChains[req.Chain] {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "不支持的链类型，仅支持 BEP20、TRC20、Polygon、Optimism"})
		return
	}

	// 检查地址是否已存在
	var count int64
	model.DB.Model(&model.WithdrawAddress{}).
		Where("merchant_id = ? AND chain = ? AND address = ?", merchantID, req.Chain, req.Address).
		Count(&count)
	if count > 0 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "该地址已存在"})
		return
	}

	address := model.WithdrawAddress{
		MerchantID: merchantID,
		Chain:      req.Chain,
		Address:    req.Address,
		Label:      req.Label,
		IsDefault:  false,
		Status:     model.WithdrawAddressPending, // 待审核
	}

	if err := model.DB.Create(&address).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "创建失败"})
		return
	}

	// 发送通知给管理员
	go service.GetTelegramService().NotifyWithdrawAddressAdded(&address)

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "提现地址已提交，等待管理员审核", "data": address})
}

// UpdateWithdrawAddress 更新提现地址
func (h *MerchantHandler) UpdateWithdrawAddress(c *gin.Context) {
	merchantID := c.GetUint("merchant_id")
	id, _ := strconv.Atoi(c.Param("id"))

	var address model.WithdrawAddress
	if err := model.DB.Where("id = ? AND merchant_id = ?", id, merchantID).First(&address).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "地址不存在"})
		return
	}

	var req struct {
		Label     *string `json:"label"`
		IsDefault *bool   `json:"is_default"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	updates := map[string]interface{}{}
	if req.Label != nil {
		updates["label"] = *req.Label
	}
	if req.IsDefault != nil && *req.IsDefault {
		// 只有已审核通过的地址才能设为默认
		if address.Status != model.WithdrawAddressApproved {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "只有审核通过的地址才能设为默认"})
			return
		}
		// 设为默认前，先取消其他默认
		model.DB.Model(&model.WithdrawAddress{}).
			Where("merchant_id = ? AND id != ?", merchantID, id).
			Update("is_default", false)
		updates["is_default"] = true
	}

	if len(updates) > 0 {
		model.DB.Model(&address).Updates(updates)
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功"})
}

// DeleteWithdrawAddress 删除提现地址
func (h *MerchantHandler) DeleteWithdrawAddress(c *gin.Context) {
	merchantID := c.GetUint("merchant_id")
	id, _ := strconv.Atoi(c.Param("id"))

	result := model.DB.Where("id = ? AND merchant_id = ?", id, merchantID).Delete(&model.WithdrawAddress{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "地址不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}

// GetTelegramBotInfo 获取 Telegram Bot 信息 (供商户查看)
func (h *MerchantHandler) GetTelegramBotInfo(c *gin.Context) {
	// 从系统配置获取 Bot 用户名
	var cfg model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", "telegram_bot_username").First(&cfg).Error; err != nil || cfg.Value == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 1,
			"data": gin.H{
				"enabled":  false,
				"username": "",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"enabled":  true,
			"username": cfg.Value,
		},
	})
}

// GetNotifySettings 获取通知设置
func (h *MerchantHandler) GetNotifySettings(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	// 如果通知设置为空，返回默认值
	settings := merchant.NotifySettings
	if settings == (model.NotifySettings{}) {
		settings = model.DefaultNotifySettings()
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"telegram_chat_id": merchant.TelegramChatID,
			"telegram_notify":  merchant.TelegramNotify,
			"notify_settings":  settings,
		},
	})
}

// UpdateNotifySettings 更新通知设置
func (h *MerchantHandler) UpdateNotifySettings(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	var req struct {
		TelegramNotify bool                 `json:"telegram_notify"`
		NotifySettings model.NotifySettings `json:"notify_settings"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 更新商户通知设置
	updates := map[string]interface{}{
		"telegram_notify":  req.TelegramNotify,
		"notify_settings":  req.NotifySettings,
	}

	if err := model.GetDB().Model(merchant).Updates(updates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "保存失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "保存成功"})
}

// ============ 监控客户端配置 ============

// GetMonitorConfig 获取监控客户端配置信息和二维码
// 用于V免签等监控APP扫码配置
func (h *MerchantHandler) GetMonitorConfig(c *gin.Context) {
	merchant := c.MustGet("merchant").(*model.Merchant)

	// 获取服务器URL配置
	var serverURL string
	var cfg model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", "server_url").First(&cfg).Error; err == nil && cfg.Value != "" {
		serverURL = cfg.Value
	} else {
		// 尝试从请求中获取
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		serverURL = scheme + "://" + c.Request.Host
	}

	// 构建配置内容 (V免签兼容格式)
	// 格式: serverURL + "\n" + key
	configContent := serverURL + "\n" + merchant.Key

	// 生成二维码
	qrcode, err := util.GenerateQRCode(configContent, 300)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "生成二维码失败: " + err.Error(),
		})
		return
	}

	// 获取最新APP版本信息
	var appInfo map[string]interface{}
	if version, err := model.GetLatestAppVersion(); err == nil {
		appInfo = map[string]interface{}{
			"has_app":      true,
			"version_code": version.VersionCode,
			"version_name": version.VersionName,
			"download_url": serverURL + version.FilePath,
			"file_size":    version.FileSize,
			"changelog":    version.Changelog,
		}
	} else {
		appInfo = map[string]interface{}{
			"has_app": false,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"server_url": serverURL,
			"key":        merchant.Key,
			"pid":        merchant.PID,
			"config":     configContent,
			"qrcode":     qrcode,
			"app":        appInfo,
			"tips": []string{
				"1. 使用EzPay APP扫描上方二维码",
				"2. APP会自动配置服务器地址和密钥",
				"3. 配置成功后，APP会自动发送心跳保持连接",
				"4. 收到支付宝/微信收款通知时，APP会自动推送到服务器",
			},
		},
	})
}

