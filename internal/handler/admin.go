package handler

import (
	"bytes"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"ezpay/config"
	"ezpay/internal/model"
	"ezpay/internal/service"
	"ezpay/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/shopspring/decimal"
)

// 外部API调用使用的HTTP客户端（带超时）
var externalHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

// AdminHandler 管理后台处理器
type AdminHandler struct {
	cfg *config.Config
}

// NewAdminHandler 创建处理器
func NewAdminHandler(cfg *config.Config) *AdminHandler {
	return &AdminHandler{cfg: cfg}
}

// Login 管理员登录
func (h *AdminHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	var admin model.Admin
	if err := model.GetDB().Where("username = ? AND status = 1", req.Username).First(&admin).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "用户名或密码错误"})
		return
	}

	if !util.CheckPassword(req.Password, admin.Password) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "用户名或密码错误"})
		return
	}

	// 更新最后登录时间
	now := time.Now()
	model.GetDB().Model(&admin).Update("last_login", &now)

	// 生成JWT Token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"admin_id": admin.ID,
		"username": admin.Username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "登录失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"msg":   "success",
		"token": tokenString,
		"admin": gin.H{
			"id":       admin.ID,
			"username": admin.Username,
		},
	})
}

// Dashboard 仪表盘数据
func (h *AdminHandler) Dashboard(c *gin.Context) {
	orderService := service.GetOrderService()
	stats, _ := orderService.GetOrderStats(0)

	// 获取汇率
	rateService := service.GetRateService()
	rate, _ := rateService.GetRate()

	// 获取区块链监听状态
	blockchainStatus := service.GetBlockchainService().GetListenerStatus()

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"stats":      stats,
			"rate":       rate.String(),
			"blockchain": blockchainStatus,
		},
	})
}

// DashboardTrend 仪表盘趋势数据
func (h *AdminHandler) DashboardTrend(c *gin.Context) {
	period := c.DefaultQuery("period", "today")

	var startDate time.Time
	var dateFormat string
	var groupBy string

	now := time.Now()
	switch period {
	case "today":
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		dateFormat = "%H:00"
		groupBy = "HOUR(created_at)"
	case "week":
		startDate = now.AddDate(0, 0, -6)
		dateFormat = "%m-%d"
		groupBy = "DATE(created_at)"
	case "month":
		startDate = now.AddDate(0, 0, -29)
		dateFormat = "%m-%d"
		groupBy = "DATE(created_at)"
	case "3months":
		startDate = now.AddDate(0, -3, 0)
		dateFormat = "%Y-%m-%d"
		groupBy = "YEARWEEK(created_at, 1)"
	default:
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		dateFormat = "%H:00"
		groupBy = "HOUR(created_at)"
	}

	var trends []struct {
		Label  string  `json:"label"`
		Count  int64   `json:"count"`
		Amount float64 `json:"amount"`
	}

	query := model.GetDB().Model(&model.Order{}).
		Where("status = 1 AND created_at >= ?", startDate)

	if period == "3months" {
		query.Select("MIN(DATE_FORMAT(created_at, ?)) as label, COUNT(*) as count, COALESCE(SUM(money), 0) as amount", dateFormat)
	} else {
		query.Select("DATE_FORMAT(created_at, ?) as label, COUNT(*) as count, COALESCE(SUM(money), 0) as amount", dateFormat)
	}

	query.Group(groupBy).Order("label ASC").Scan(&trends)

	labels := make([]string, len(trends))
	orders := make([]int64, len(trends))
	amounts := make([]float64, len(trends))

	for i, t := range trends {
		labels[i] = t.Label
		orders[i] = t.Count
		amounts[i] = t.Amount
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"labels":  labels,
			"orders":  orders,
			"amounts": amounts,
		},
	})
}

// DashboardTop 商户排行榜
func (h *AdminHandler) DashboardTop(c *gin.Context) {
	// 金额 TOP 5
	var topAmount []struct {
		MerchantID uint    `json:"merchant_id"`
		PID        string  `json:"pid"`
		Name       string  `json:"name"`
		Amount     float64 `json:"amount"`
	}
	model.GetDB().Model(&model.Order{}).
		Select("orders.merchant_id, merchants.p_id as pid, merchants.name, COALESCE(SUM(orders.money), 0) as amount").
		Joins("LEFT JOIN merchants ON merchants.id = orders.merchant_id").
		Where("orders.status = 1").
		Group("orders.merchant_id").
		Order("amount DESC").
		Limit(5).
		Scan(&topAmount)

	// 订单数 TOP 5
	var topCount []struct {
		MerchantID uint   `json:"merchant_id"`
		PID        string `json:"pid"`
		Name       string `json:"name"`
		Count      int64  `json:"count"`
	}
	model.GetDB().Model(&model.Order{}).
		Select("orders.merchant_id, merchants.p_id as pid, merchants.name, COUNT(*) as count").
		Joins("LEFT JOIN merchants ON merchants.id = orders.merchant_id").
		Where("orders.status = 1").
		Group("orders.merchant_id").
		Order("count DESC").
		Limit(5).
		Scan(&topCount)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"top_amount": topAmount,
			"top_count":  topCount,
		},
	})
}

// ListOrders 订单列表
func (h *AdminHandler) ListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	tradeNo := c.Query("trade_no")
	outTradeNo := c.Query("out_trade_no")
	merchantID, _ := strconv.Atoi(c.Query("merchant_id"))
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := &model.OrderQuery{
		Page:       page,
		PageSize:   pageSize,
		TradeNo:    tradeNo,
		OutTradeNo: outTradeNo,
		MerchantID: uint(merchantID),
	}

	if status != "" {
		s, _ := strconv.Atoi(status)
		st := model.OrderStatus(s)
		query.Status = &st
	}

	// 解析日期筛选
	if startDate != "" {
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			query.StartTime = &t
		}
	}
	if endDate != "" {
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			// 结束日期加一天，表示到该日期的23:59:59
			t = t.AddDate(0, 0, 1)
			query.EndTime = &t
		}
	}

	orderService := service.GetOrderService()
	orders, total, err := orderService.QueryOrdersWithMerchant(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	// 构建包含商户PID的响应
	type OrderResponse struct {
		model.Order
		MerchantPID  string `json:"merchant_pid"`
		MerchantName string `json:"merchant_name"`
	}

	var result []OrderResponse
	for _, order := range orders {
		resp := OrderResponse{
			Order:        order,
			MerchantPID:  order.Merchant.PID,
			MerchantName: order.Merchant.Name,
		}
		result = append(result, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"data":  result,
		"total": total,
		"page":  page,
	})
}

// GetOrder 获取订单详情
func (h *AdminHandler) GetOrder(c *gin.Context) {
	tradeNo := c.Param("trade_no")
	orderService := service.GetOrderService()
	order, err := orderService.GetOrder(tradeNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": order})
}

// MarkOrderPaid 手动标记订单已支付
func (h *AdminHandler) MarkOrderPaid(c *gin.Context) {
	tradeNo := c.Param("trade_no")
	var req struct {
		TxHash string `json:"tx_hash"`
		Amount string `json:"amount"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	amount, _ := decimal.NewFromString(req.Amount)
	orderService := service.GetOrderService()
	if err := orderService.MarkOrderPaid(tradeNo, req.TxHash, amount); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "success"})
}

// RetryNotify 重试通知
func (h *AdminHandler) RetryNotify(c *gin.Context) {
	tradeNo := c.Param("trade_no")
	orderService := service.GetOrderService()
	order, err := orderService.GetOrder(tradeNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	go service.GetNotifyService().NotifyOrder(order.ID)

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已触发通知"})
}

// CleanInvalidOrders 清理无效订单（超过24小时未支付的订单）
func (h *AdminHandler) CleanInvalidOrders(c *gin.Context) {
	// 计算24小时前的时间
	cutoffTime := time.Now().Add(-24 * time.Hour)

	// 先查询需要清理的订单，以便退还手续费
	var orders []model.Order
	if err := model.GetDB().Where(
		"status IN (?, ?) AND created_at < ?",
		model.OrderStatusPending,
		model.OrderStatusExpired,
		cutoffTime,
	).Find(&orders).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "查询失败: " + err.Error()})
		return
	}

	// 退还预扣的手续费
	refundCount := 0
	for _, order := range orders {
		if order.FeeType == model.FeeTypeBalance && order.Status == model.OrderStatusPending {
			fee, _ := order.Fee.Float64()
			if fee > 0 {
				if err := service.GetWithdrawService().RefundPreChargedFee(order.MerchantID, fee); err == nil {
					refundCount++
				}
			}
		}
	}

	// 删除订单
	result := model.GetDB().Where(
		"status IN (?, ?) AND created_at < ?",
		model.OrderStatusPending,
		model.OrderStatusExpired,
		cutoffTime,
	).Delete(&model.Order{})

	if result.Error != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "清理失败: " + result.Error.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":         1,
		"msg":          fmt.Sprintf("清理成功，退还%d笔预扣手续费", refundCount),
		"count":        result.RowsAffected,
		"refund_count": refundCount,
	})
}

// ListMerchants 商户列表
func (h *AdminHandler) ListMerchants(c *gin.Context) {
	var merchants []model.Merchant
	model.GetDB().Order("id DESC").Find(&merchants)

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": merchants})
}

// CreateMerchant 创建商户
func (h *AdminHandler) CreateMerchant(c *gin.Context) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		Email     string `json:"email"`
		Password  string `json:"password"` // 商户登录密码
		NotifyURL string `json:"notify_url"`
		ReturnURL string `json:"return_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	merchant := model.Merchant{
		PID:       util.GenerateMerchantPID(),
		Name:      req.Name,
		Email:     req.Email,
		Key:       util.GenerateMerchantKey(),
		NotifyURL: req.NotifyURL,
		ReturnURL: req.ReturnURL,
		Status:    1,
	}

	// 设置密码
	if req.Password != "" {
		hashedPassword, _ := util.HashPassword(req.Password)
		merchant.Password = hashedPassword
	}

	if err := model.GetDB().Create(&merchant).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": merchant})
}

// UpdateMerchant 更新商户
func (h *AdminHandler) UpdateMerchant(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Name                    string `json:"name"`
		Email                   string `json:"email"`
		Password                string `json:"password"` // 重置密码
		NotifyURL               string `json:"notify_url"`
		ReturnURL               string `json:"return_url"`
		WalletLimit             *int   `json:"wallet_limit"`
		Status                  *int8  `json:"status"`
		IPWhitelistEnabled      *bool  `json:"ip_whitelist_enabled"`
		IPWhitelist             string `json:"ip_whitelist"`
		RefererWhitelistEnabled *bool  `json:"referer_whitelist_enabled"`
		RefererWhitelist        string `json:"referer_whitelist"`
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
	if req.Password != "" {
		hashedPassword, _ := util.HashPassword(req.Password)
		updates["password"] = hashedPassword
	}
	if req.NotifyURL != "" {
		updates["notify_url"] = req.NotifyURL
	}
	if req.ReturnURL != "" {
		updates["return_url"] = req.ReturnURL
	}
	if req.WalletLimit != nil {
		updates["wallet_limit"] = *req.WalletLimit
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	// 白名单设置
	if req.IPWhitelistEnabled != nil {
		updates["ip_whitelist_enabled"] = *req.IPWhitelistEnabled
	}
	updates["ip_whitelist"] = req.IPWhitelist
	if req.RefererWhitelistEnabled != nil {
		updates["referer_whitelist_enabled"] = *req.RefererWhitelistEnabled
	}
	updates["referer_whitelist"] = req.RefererWhitelist

	if err := model.GetDB().Model(&model.Merchant{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "success"})
}

// AdjustMerchantBalance 调整商户余额
func (h *AdminHandler) AdjustMerchantBalance(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Type   string  `json:"type"`   // add, subtract, set
		Amount float64 `json:"amount"` // 金额
		Remark string  `json:"remark"` // 备注
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	if req.Amount < 0 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "金额不能为负数"})
		return
	}

	var merchant model.Merchant
	if err := model.GetDB().First(&merchant, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户不存在"})
		return
	}

	var newBalance float64
	switch req.Type {
	case "add":
		newBalance = merchant.Balance + req.Amount
	case "subtract":
		if merchant.Balance < req.Amount {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "余额不足"})
			return
		}
		newBalance = merchant.Balance - req.Amount
	case "set":
		newBalance = req.Amount
	default:
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "无效的调整类型"})
		return
	}

	if err := model.GetDB().Model(&merchant).Update("balance", newBalance).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "调整失败"})
		return
	}

	// 记录日志
	log.Printf("管理员调整商户[%d]余额: %.2f -> %.2f, 类型: %s, 备注: %s", id, merchant.Balance, newBalance, req.Type, req.Remark)

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "调整成功", "data": gin.H{"balance": newBalance}})
}

// ResetMerchantKey 重置商户密钥
func (h *AdminHandler) ResetMerchantKey(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	newKey := util.GenerateMerchantKey()

	if err := model.GetDB().Model(&model.Merchant{}).Where("id = ?", id).Update("key", newKey).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "重置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "key": newKey})
}

// GetMerchantKey 获取商户密钥
func (h *AdminHandler) GetMerchantKey(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var merchant model.Merchant
	if err := model.GetDB().First(&merchant, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "key": merchant.Key})
}

// ListWallets 钱包列表
func (h *AdminHandler) ListWallets(c *gin.Context) {
	var wallets []model.Wallet
	model.GetDB().Preload("Merchant").Order("id DESC").Find(&wallets)

	// 构造包含商户信息的响应
	type WalletResponse struct {
		model.Wallet
		MerchantPID  string `json:"merchant_pid"`
		MerchantName string `json:"merchant_name"`
	}

	result := make([]WalletResponse, len(wallets))
	for i, w := range wallets {
		result[i] = WalletResponse{
			Wallet: w,
		}
		if w.Merchant != nil {
			result[i].MerchantPID = w.Merchant.PID
			result[i].MerchantName = w.Merchant.Name
		} else if w.MerchantID == 0 {
			result[i].MerchantPID = "系统"
			result[i].MerchantName = "系统钱包"
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": result})
}

// CreateWallet 创建钱包
func (h *AdminHandler) CreateWallet(c *gin.Context) {
	var req struct {
		Chain      string `json:"chain" binding:"required"`
		Address    string `json:"address"`     // 对于微信/支付宝可以为空，自动生成
		Label      string `json:"label"`
		QRCode     string `json:"qrcode"`      // 收款码图片路径 (微信/支付宝)
		MerchantID uint   `json:"merchant_id"` // 商户ID，0为系统钱包
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	if !util.IsValidChain(req.Chain) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "不支持的链类型"})
		return
	}

	// 对于加密货币链，地址必填；对于微信/支付宝，使用二维码解析的地址
	if util.IsFiatChain(req.Chain) {
		// 法币收款: 使用从二维码解析出的支付链接作为地址
		// 如果地址为空（没有上传二维码或解析失败），报错
		if req.Address == "" {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请先上传收款码图片"})
			return
		}
		// 微信/支付宝必须上传收款码
		if req.QRCode == "" {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请上传收款码图片"})
			return
		}
	} else {
		// 加密货币链必须提供地址
		if req.Address == "" {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请输入钱包地址"})
			return
		}
	}

	// 如果指定了商户ID，验证商户是否存在
	if req.MerchantID > 0 {
		var count int64
		model.GetDB().Model(&model.Merchant{}).Where("id = ?", req.MerchantID).Count(&count)
		if count == 0 {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户不存在"})
			return
		}
	}

	wallet := model.Wallet{
		MerchantID: req.MerchantID,
		Chain:      req.Chain,
		Address:    req.Address,
		Label:      req.Label,
		QRCode:     req.QRCode,
		Status:     1,
	}

	if err := model.GetDB().Create(&wallet).Error; err != nil {
		// 输出详细错误信息便于调试
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "创建失败: " + err.Error()})
		return
	}

	// 使钱包缓存失效
	service.GetBlockchainService().InvalidateWalletCache()

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": wallet})
}

// UploadQRCode 上传收款码图片
func (h *AdminHandler) UploadQRCode(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请选择文件"})
		return
	}

	// 验证文件类型
	ext := ""
	if file.Filename != "" {
		for i := len(file.Filename) - 1; i >= 0; i-- {
			if file.Filename[i] == '.' {
				ext = file.Filename[i:]
				break
			}
		}
	}

	allowedExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true}
	if !allowedExts[ext] {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "只支持 jpg/png/gif 格式"})
		return
	}

	// 生成文件名
	filename := util.GenerateTradeNo() + ext
	// 确保上传目录存在（使用配置的数据目录）
	dataDir := config.Get().Storage.DataDir
	qrcodeDir := dataDir + "/qrcode"
	os.MkdirAll(qrcodeDir, 0755)
	filepath := qrcodeDir + "/" + filename

	// 保存文件
	if err := c.SaveUploadedFile(file, filepath); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "保存文件失败"})
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
		"code":    1,
		"url":     "/static/qrcode/" + filename,
		"address": qrContent,
	})
}

// UpdateWallet 更新钱包
func (h *AdminHandler) UpdateWallet(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Label  string `json:"label"`
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
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if err := model.GetDB().Model(&model.Wallet{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "更新失败"})
		return
	}

	// 使钱包缓存失效
	service.GetBlockchainService().InvalidateWalletCache()

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "success"})
}

// DeleteWallet 删除钱包
func (h *AdminHandler) DeleteWallet(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	// 检查是否有订单使用过该钱包
	var orderCount int64
	model.GetDB().Model(&model.Order{}).Where("wallet_id = ?", id).Count(&orderCount)

	if orderCount > 0 {
		// 有使用记录，只能禁用不能删除
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": fmt.Sprintf("该钱包已有%d笔订单使用记录，无法删除，请使用禁用功能", orderCount)})
		return
	}

	// 没有使用记录，彻底删除（不是软删除）
	if err := model.GetDB().Unscoped().Delete(&model.Wallet{}, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "删除失败"})
		return
	}

	// 使钱包缓存失效
	service.GetBlockchainService().InvalidateWalletCache()

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}

// GetConfigs 获取系统配置
func (h *AdminHandler) GetConfigs(c *gin.Context) {
	var configs []model.SystemConfig
	model.GetDB().Find(&configs)

	configMap := make(map[string]string)
	for _, cfg := range configs {
		configMap[cfg.Key] = cfg.Value
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": configMap})
}

// UpdateConfigs 更新系统配置
func (h *AdminHandler) UpdateConfigs(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	for key, value := range req {
		// 使用 upsert 方式确保配置存在
		model.GetDB().Where("`key` = ?", key).Assign(model.SystemConfig{Value: value}).FirstOrCreate(&model.SystemConfig{Key: key})
	}

	// 清除汇率缓存
	service.GetRateService().ClearCache()

	// 如果更新了 Telegram Bot Token，同步更新 Telegram 服务
	if botToken, ok := req["telegram_bot_token"]; ok {
		service.GetTelegramService().UpdateConfig(botToken != "", botToken)
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "success"})
}

// TestTelegramBot 测试 Telegram Bot 连接
func (h *AdminHandler) TestTelegramBot(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}

	// 尝试从请求体获取 token
	botToken := ""
	if err := c.ShouldBindJSON(&req); err == nil && req.Token != "" {
		botToken = req.Token
	} else {
		// 如果请求体没有，从数据库获取
		var cfg model.SystemConfig
		if err := model.GetDB().Where("`key` = ?", "telegram_bot_token").First(&cfg).Error; err != nil || cfg.Value == "" {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "未配置 Bot Token，请先填写 Token"})
			return
		}
		botToken = cfg.Value
	}

	// 调用 Telegram API 验证 token
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", botToken)
	resp, err := externalHTTPClient.Get(url)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "网络请求失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			ID        int64  `json:"id"`
			IsBot     bool   `json:"is_bot"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"result"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "解析响应失败"})
		return
	}

	if !result.OK {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": result.Description})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "连接成功",
		"data": gin.H{
			"id":       result.Result.ID,
			"username": "@" + result.Result.Username,
			"name":     result.Result.FirstName,
		},
	})
}

// GetRate 获取当前汇率
func (h *AdminHandler) GetRate(c *gin.Context) {
	rateService := service.GetRateService()
	rate, err := rateService.GetRate()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	cachedRate, lastUpdate := rateService.GetCachedRate()

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"rate":        rate.String(),
			"cached_rate": cachedRate.String(),
			"last_update": lastUpdate,
		},
	})
}

// RefreshRate 刷新汇率
func (h *AdminHandler) RefreshRate(c *gin.Context) {
	rateService := service.GetRateService()
	rateService.ClearCache()
	rate, err := rateService.GetRate()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "rate": rate.String()})
}

// GetTransactionLogs 获取交易日志
func (h *AdminHandler) GetTransactionLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	chain := c.Query("chain")
	matched := c.Query("matched")

	db := model.GetDB().Model(&model.TransactionLog{})
	if chain != "" {
		db = db.Where("chain = ?", chain)
	}
	if matched != "" {
		m := matched == "1" || matched == "true"
		db = db.Where("matched = ?", m)
	}

	var total int64
	db.Count(&total)

	var logs []model.TransactionLog
	offset := (page - 1) * pageSize
	db.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs)

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"data":  logs,
		"total": total,
		"page":  page,
	})
}

// GetAPILogs 获取API调用日志
func (h *AdminHandler) GetAPILogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	merchantPID := c.Query("merchant_pid")
	endpoint := c.Query("endpoint")
	responseCode := c.Query("response_code")
	tradeNo := c.Query("trade_no")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	db := model.GetDB().Model(&model.APILog{})

	if merchantPID != "" {
		db = db.Where("merchant_pid = ?", merchantPID)
	}
	if endpoint != "" {
		db = db.Where("endpoint LIKE ?", "%"+endpoint+"%")
	}
	if responseCode != "" {
		code, _ := strconv.Atoi(responseCode)
		db = db.Where("response_code = ?", code)
	}
	if tradeNo != "" {
		db = db.Where("trade_no = ?", tradeNo)
	}
	if startDate != "" {
		db = db.Where("created_at >= ?", startDate+" 00:00:00")
	}
	if endDate != "" {
		db = db.Where("created_at <= ?", endDate+" 23:59:59")
	}

	var total int64
	db.Count(&total)

	var logs []model.APILog
	offset := (page - 1) * pageSize
	db.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs)

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"data":  logs,
		"total": total,
		"page":  page,
	})
}

// CleanAPILogs 清理API日志
func (h *AdminHandler) CleanAPILogs(c *gin.Context) {
	var req struct {
		Days int `json:"days" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 计算截止时间
	cutoffTime := time.Now().Add(-time.Duration(req.Days) * 24 * time.Hour)

	// 删除指定天数之前的日志
	result := model.GetDB().Where("created_at < ?", cutoffTime).Delete(&model.APILog{})

	if result.Error != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "清理失败: " + result.Error.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "清理成功", "count": result.RowsAffected})
}

// ChangePassword 修改密码
func (h *AdminHandler) ChangePassword(c *gin.Context) {
	adminID := c.GetUint("admin_id")
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	var admin model.Admin
	if err := model.GetDB().First(&admin, adminID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "管理员不存在"})
		return
	}

	if !util.CheckPassword(req.OldPassword, admin.Password) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "旧密码错误"})
		return
	}

	hashedPassword, _ := util.HashPassword(req.NewPassword)
	model.GetDB().Model(&admin).Update("password", hashedPassword)

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "修改成功"})
}

// GetChainStatus 获取链监控状态
func (h *AdminHandler) GetChainStatus(c *gin.Context) {
	blockchainService := service.GetBlockchainService()
	status := blockchainService.GetListenerStatus()

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": status,
	})
}

// EnableChain 启用链监控
func (h *AdminHandler) EnableChain(c *gin.Context) {
	chain := c.Param("chain")
	if chain == "" {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "链名不能为空"})
		return
	}

	blockchainService := service.GetBlockchainService()
	if err := blockchainService.EnableChain(chain); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已启用 " + chain + " 链监控"})
}

// DisableChain 禁用链监控
func (h *AdminHandler) DisableChain(c *gin.Context) {
	chain := c.Param("chain")
	if chain == "" {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "链名不能为空"})
		return
	}

	blockchainService := service.GetBlockchainService()
	if err := blockchainService.DisableChain(chain); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已禁用 " + chain + " 链监控"})
}

// BatchUpdateChains 批量更新链状态
func (h *AdminHandler) BatchUpdateChains(c *gin.Context) {
	var req struct {
		Chains map[string]bool `json:"chains" binding:"required"` // chain -> enabled
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	blockchainService := service.GetBlockchainService()
	var errors []string

	for chain, enabled := range req.Chains {
		var err error
		if enabled {
			err = blockchainService.EnableChain(chain)
		} else {
			err = blockchainService.DisableChain(chain)
		}
		if err != nil {
			errors = append(errors, chain+": "+err.Error())
		}
	}

	if len(errors) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":   -1,
			"msg":    "部分链操作失败",
			"errors": errors,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "链状态更新成功"})
}

// ExportOrders 导出订单为CSV
func (h *AdminHandler) ExportOrders(c *gin.Context) {
	status := c.Query("status")
	tradeNo := c.Query("trade_no")
	outTradeNo := c.Query("out_trade_no")
	merchantID, _ := strconv.Atoi(c.Query("merchant_id"))
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := model.GetDB().Model(&model.Order{}).Preload("Merchant")

	if status != "" {
		s, _ := strconv.Atoi(status)
		query = query.Where("status = ?", s)
	}
	if tradeNo != "" {
		query = query.Where("trade_no LIKE ?", "%"+tradeNo+"%")
	}
	if outTradeNo != "" {
		query = query.Where("out_trade_no LIKE ?", "%"+outTradeNo+"%")
	}
	if merchantID > 0 {
		query = query.Where("merchant_id = ?", merchantID)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate+" 00:00:00")
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate+" 23:59:59")
	}

	var orders []model.Order
	query.Order("id DESC").Limit(10000).Find(&orders) // 限制最多导出10000条

	// 创建CSV
	buf := new(bytes.Buffer)
	// 添加UTF-8 BOM以便Excel正确识别中文
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(buf)

	// 写入标题行
	headers := []string{"交易号", "商户订单号", "商户PID", "商户名称", "支付类型", "金额(CNY)", "USDT金额", "汇率", "状态", "链", "收款地址", "交易哈希", "创建时间", "支付时间"}
	writer.Write(headers)

	// 写入数据行
	for _, order := range orders {
		statusText := "待支付"
		switch order.Status {
		case model.OrderStatusPaid:
			statusText = "已支付"
		case model.OrderStatusExpired:
			statusText = "已过期"
		case model.OrderStatusCancelled:
			statusText = "已取消"
		}

		merchantPID := ""
		merchantName := ""
		if order.Merchant != nil {
			merchantPID = order.Merchant.PID
			merchantName = order.Merchant.Name
		}

		paidAt := ""
		if order.PaidAt != nil {
			paidAt = order.PaidAt.Format("2006-01-02 15:04:05")
		}

		row := []string{
			order.TradeNo,
			order.OutTradeNo,
			merchantPID,
			merchantName,
			order.Type,
			order.Money.String(),
			order.USDTAmount.String(),
			order.Rate.String(),
			statusText,
			order.Chain,
			order.ToAddress,
			order.TxHash,
			order.CreatedAt.Format("2006-01-02 15:04:05"),
			paidAt,
		}
		writer.Write(row)
	}

	writer.Flush()

	// 设置响应头
	filename := fmt.Sprintf("orders_%s.csv", time.Now().Format("20060102150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}

// ============ 提现管理 ============

// ListWithdrawals 提现列表
func (h *AdminHandler) ListWithdrawals(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	statusStr := c.Query("status")
	merchantID, _ := strconv.Atoi(c.Query("merchant_id"))

	var status *model.WithdrawStatus
	if statusStr != "" {
		s, _ := strconv.Atoi(statusStr)
		st := model.WithdrawStatus(s)
		status = &st
	}

	withdrawals, total, err := service.GetWithdrawService().ListWithdrawals(uint(merchantID), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	// 构建响应，包含商户信息
	type WithdrawalResponse struct {
		model.Withdrawal
		MerchantPID  string `json:"merchant_pid"`
		MerchantName string `json:"merchant_name"`
	}

	var result []WithdrawalResponse
	for _, w := range withdrawals {
		resp := WithdrawalResponse{
			Withdrawal:   w,
			MerchantPID:  w.Merchant.PID,
			MerchantName: w.Merchant.Name,
		}
		result = append(result, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"data":  result,
		"total": total,
	})
}

// ApproveWithdrawal 审核通过提现
func (h *AdminHandler) ApproveWithdrawal(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		AdminRemark string `json:"admin_remark"`
	}
	c.ShouldBindJSON(&req)

	if err := service.GetWithdrawService().ApproveWithdrawal(uint(id), req.AdminRemark); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "审核通过"})
}

// RejectWithdrawal 拒绝提现
func (h *AdminHandler) RejectWithdrawal(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		AdminRemark string `json:"admin_remark"`
	}
	c.ShouldBindJSON(&req)

	if err := service.GetWithdrawService().RejectWithdrawal(uint(id), req.AdminRemark); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已拒绝"})
}

// CompleteWithdrawal 完成打款
func (h *AdminHandler) CompleteWithdrawal(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		AdminRemark string `json:"admin_remark"`
	}
	c.ShouldBindJSON(&req)

	if err := service.GetWithdrawService().CompleteWithdrawal(uint(id), req.AdminRemark); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "打款完成"})
}

// ============ IP黑名单管理 ============

// ListIPBlacklist 获取IP黑名单列表
func (h *AdminHandler) ListIPBlacklist(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	ip := c.Query("ip")

	db := model.GetDB().Model(&model.IPBlacklist{})
	if ip != "" {
		db = db.Where("ip LIKE ?", "%"+ip+"%")
	}

	var total int64
	db.Count(&total)

	var blacklist []model.IPBlacklist
	offset := (page - 1) * pageSize
	db.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&blacklist)

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"data":  blacklist,
		"total": total,
		"page":  page,
	})
}

// AddIPBlacklist 添加IP到黑名单
func (h *AdminHandler) AddIPBlacklist(c *gin.Context) {
	var req struct {
		IP     string `json:"ip" binding:"required"`
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "IP地址不能为空"})
		return
	}

	// 检查是否已存在
	if model.IsIPBlacklisted(req.IP) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "该IP已在黑名单中"})
		return
	}

	if err := model.AddIPToBlacklist(req.IP, req.Reason, "manual"); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "添加失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "添加成功"})
}

// RemoveIPBlacklist 从黑名单移除IP
func (h *AdminHandler) RemoveIPBlacklist(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	if err := model.RemoveIPFromBlacklist(uint(id)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}

// BlockIPFromAPILog 从API日志一键拉黑IP
func (h *AdminHandler) BlockIPFromAPILog(c *gin.Context) {
	var req struct {
		IP     string `json:"ip" binding:"required"`
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "IP地址不能为空"})
		return
	}

	// 检查是否已存在
	if model.IsIPBlacklisted(req.IP) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "该IP已在黑名单中"})
		return
	}

	reason := req.Reason
	if reason == "" {
		reason = "从API日志一键拉黑"
	}

	if err := model.AddIPToBlacklist(req.IP, reason, "api_log"); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "添加失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已将 " + req.IP + " 加入黑名单"})
}

// ============ 测试支付 ============

// CreateTestOrder 创建测试订单
func (h *AdminHandler) CreateTestOrder(c *gin.Context) {
	var req struct {
		MerchantID uint   `json:"merchant_id" binding:"required"`
		Type       string `json:"type" binding:"required"`
		Money      string `json:"money" binding:"required"`
		Name       string `json:"name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 获取商户
	var merchant model.Merchant
	if err := model.GetDB().First(&merchant, req.MerchantID).Error; err != nil {
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
		Param:       "test=1",
		ClientIP:    c.ClientIP(),
	}

	resp, err := orderService.CreateOrder(orderReq)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":     1,
		"msg":      "测试订单创建成功",
		"trade_no": resp.TradeNo,
		"pay_url":  "/cashier/" + resp.TradeNo,
	})
}

// ============ 提现地址审核管理 ============

// ListWithdrawAddresses 提现地址列表 (管理员)
func (h *AdminHandler) ListWithdrawAddresses(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	statusStr := c.Query("status")
	merchantID, _ := strconv.Atoi(c.Query("merchant_id"))

	db := model.GetDB().Model(&model.WithdrawAddress{})

	if statusStr != "" {
		status, _ := strconv.Atoi(statusStr)
		db = db.Where("status = ?", status)
	}
	if merchantID > 0 {
		db = db.Where("merchant_id = ?", merchantID)
	}

	var total int64
	db.Count(&total)

	var addresses []model.WithdrawAddress
	offset := (page - 1) * pageSize
	db.Order("status ASC, created_at DESC").Offset(offset).Limit(pageSize).Find(&addresses)

	// 构建响应，包含商户信息
	type AddressResponse struct {
		model.WithdrawAddress
		MerchantPID  string `json:"merchant_pid"`
		MerchantName string `json:"merchant_name"`
	}

	var result []AddressResponse
	for _, addr := range addresses {
		var merchant model.Merchant
		model.GetDB().First(&merchant, addr.MerchantID)
		result = append(result, AddressResponse{
			WithdrawAddress: addr,
			MerchantPID:     merchant.PID,
			MerchantName:    merchant.Name,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"data":  result,
		"total": total,
		"page":  page,
	})
}

// ApproveWithdrawAddress 审核通过提现地址
func (h *AdminHandler) ApproveWithdrawAddress(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var address model.WithdrawAddress
	if err := model.GetDB().First(&address, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "地址不存在"})
		return
	}

	if address.Status != model.WithdrawAddressPending {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "该地址已审核过"})
		return
	}

	var req struct {
		AdminRemark string `json:"admin_remark"`
	}
	c.ShouldBindJSON(&req)

	// 更新状态
	model.GetDB().Model(&address).Updates(map[string]interface{}{
		"status":       model.WithdrawAddressApproved,
		"admin_remark": req.AdminRemark,
	})

	// 如果是该商户第一个审核通过的地址，设为默认
	var approvedCount int64
	model.GetDB().Model(&model.WithdrawAddress{}).
		Where("merchant_id = ? AND status = ? AND id != ?", address.MerchantID, model.WithdrawAddressApproved, address.ID).
		Count(&approvedCount)
	if approvedCount == 0 {
		model.GetDB().Model(&address).Update("is_default", true)
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "审核通过"})
}

// RejectWithdrawAddress 拒绝提现地址
func (h *AdminHandler) RejectWithdrawAddress(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var address model.WithdrawAddress
	if err := model.GetDB().First(&address, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "地址不存在"})
		return
	}

	if address.Status != model.WithdrawAddressPending {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "该地址已审核过"})
		return
	}

	var req struct {
		AdminRemark string `json:"admin_remark"`
	}
	c.ShouldBindJSON(&req)

	model.GetDB().Model(&address).Updates(map[string]interface{}{
		"status":       model.WithdrawAddressRejected,
		"admin_remark": req.AdminRemark,
	})

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已拒绝"})
}

// ============ APP版本管理 ============

// ListAppVersions 获取APP版本列表
func (h *AdminHandler) ListAppVersions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var versions []model.AppVersion
	var total int64

	query := model.GetDB().Model(&model.AppVersion{})
	query.Count(&total)
	query.Order("version_code DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&versions)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"list":      versions,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// UploadAppVersion 上传新的APP版本
func (h *AdminHandler) UploadAppVersion(c *gin.Context) {
	// 获取表单字段
	versionCodeStr := c.PostForm("version_code")
	versionName := c.PostForm("version_name")
	changelog := c.PostForm("changelog")
	forceUpdateStr := c.PostForm("force_update")

	if versionCodeStr == "" || versionName == "" {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "版本号和版本名称不能为空"})
		return
	}

	versionCode, err := strconv.Atoi(versionCodeStr)
	if err != nil || versionCode <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "版本号必须是正整数"})
		return
	}

	// 检查版本号是否已存在
	var existCount int64
	model.GetDB().Model(&model.AppVersion{}).Where("version_code = ?", versionCode).Count(&existCount)
	if existCount > 0 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "该版本号已存在"})
		return
	}

	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "请选择APK文件"})
		return
	}

	// 检查文件类型
	if filepath.Ext(file.Filename) != ".apk" {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "只支持APK文件"})
		return
	}

	// 检查文件大小 (最大100MB)
	if file.Size > 100*1024*1024 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "文件大小不能超过100MB"})
		return
	}

	// 确保上传目录存在
	uploadDir := "uploads/apk"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "创建上传目录失败"})
		return
	}

	// 生成文件名
	filename := fmt.Sprintf("ezpay_v%s_%d.apk", versionName, time.Now().Unix())
	filePath := filepath.Join(uploadDir, filename)

	// 保存文件
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "保存文件失败: " + err.Error()})
		return
	}

	// 计算MD5
	fileContent, err := os.Open(filePath)
	if err != nil {
		os.Remove(filePath)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "读取文件失败"})
		return
	}
	defer fileContent.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, fileContent); err != nil {
		os.Remove(filePath)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "计算MD5失败"})
		return
	}
	md5sum := hex.EncodeToString(hash.Sum(nil))

	// 解析force_update
	forceUpdate := forceUpdateStr == "true" || forceUpdateStr == "1"

	// 创建版本记录
	version := model.AppVersion{
		VersionCode: versionCode,
		VersionName: versionName,
		FileName:    filename,
		FilePath:    "/static/uploads/apk/" + filename,
		FileSize:    file.Size,
		MD5:         md5sum,
		Changelog:   changelog,
		ForceUpdate: forceUpdate,
		Status:      1,
	}

	if err := model.GetDB().Create(&version).Error; err != nil {
		os.Remove(filePath)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "保存版本信息失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "上传成功",
		"data": version,
	})
}

// UpdateAppVersion 更新APP版本信息
func (h *AdminHandler) UpdateAppVersion(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var version model.AppVersion
	if err := model.GetDB().First(&version, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "版本不存在"})
		return
	}

	var req struct {
		Changelog   *string `json:"changelog"`
		ForceUpdate *bool   `json:"force_update"`
		Status      *int8   `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	updates := map[string]interface{}{}
	if req.Changelog != nil {
		updates["changelog"] = *req.Changelog
	}
	if req.ForceUpdate != nil {
		updates["force_update"] = *req.ForceUpdate
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if len(updates) > 0 {
		model.GetDB().Model(&version).Updates(updates)
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功"})
}

// DeleteAppVersion 删除APP版本
func (h *AdminHandler) DeleteAppVersion(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var version model.AppVersion
	if err := model.GetDB().First(&version, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "版本不存在"})
		return
	}

	// 删除文件
	filePath := "web" + version.FilePath
	os.Remove(filePath)

	// 删除记录
	model.GetDB().Delete(&version)

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}

// GetLatestAppVersion 获取最新APP版本 (公开接口)
func (h *AdminHandler) GetLatestAppVersion(c *gin.Context) {
	currentVersionStr := c.Query("version_code")
	currentVersion := 0
	if currentVersionStr != "" {
		currentVersion, _ = strconv.Atoi(currentVersionStr)
	}

	version, err := model.GetLatestAppVersion()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 1,
			"data": gin.H{
				"has_update": false,
			},
		})
		return
	}

	// 构建下载URL
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	downloadURL := scheme + "://" + c.Request.Host + version.FilePath

	hasUpdate := version.VersionCode > currentVersion

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"has_update":    hasUpdate,
			"version_code":  version.VersionCode,
			"version_name":  version.VersionName,
			"download_url":  downloadURL,
			"file_size":     version.FileSize,
			"md5":           version.MD5,
			"changelog":     version.Changelog,
			"force_update":  version.ForceUpdate,
		},
	})
}

// DownloadApp 下载APP (公开接口，用于统计下载次数)
func (h *AdminHandler) DownloadApp(c *gin.Context) {
	version, err := model.GetLatestAppVersion()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "msg": "暂无可用版本"})
		return
	}

	// 增加下载计数
	version.IncrementDownloads()

	// 重定向到文件
	c.Redirect(http.StatusFound, version.FilePath)
}
