package handler

import (
	"net/http"
	"strconv"
	"time"

	"ezpay/internal/middleware"
	"ezpay/internal/model"
	"ezpay/internal/service"
	"ezpay/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// VmqHandler V免签兼容接口处理器
type VmqHandler struct{}

// NewVmqHandler 创建处理器
func NewVmqHandler() *VmqHandler {
	return &VmqHandler{}
}

// CreateOrder 创建订单 (V免签兼容)
// GET /createOrder
func (h *VmqHandler) CreateOrder(c *gin.Context) {
	payId := c.Query("payId")
	payType := c.DefaultQuery("type", "1") // 1:微信 2:支付宝, 这里映射到USDT
	price := c.Query("price")
	sign := c.Query("sign")
	param := c.DefaultQuery("param", "")
	notifyUrl := c.Query("notifyUrl")
	returnUrl := c.DefaultQuery("returnUrl", "")
	isHtml := c.DefaultQuery("isHtml", "0")

	// 验证必填参数
	if payId == "" || price == "" || sign == "" {
		middleware.SetAPILogContext(c, -1, "参数不完整", "", 0, "")
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "参数不完整",
		})
		return
	}

	// 获取默认商户 (V免签模式使用系统配置的商户)
	var merchant model.Merchant
	if err := model.GetDB().Where("status = 1").First(&merchant).Error; err != nil {
		middleware.SetAPILogContext(c, -1, "商户不存在", "", 0, "")
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "商户不存在",
		})
		return
	}

	// 检查IP白名单 (仅当启用时检查)
	if merchant.IPWhitelistEnabled && !middleware.CheckIPWhitelist(c.ClientIP(), merchant.IPWhitelist) {
		middleware.SetAPILogContext(c, -1, "IP不在白名单内", "", merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "IP不在白名单内",
		})
		return
	}

	// 检查Referer白名单 (仅当启用时检查)
	if merchant.RefererWhitelistEnabled && !middleware.CheckRefererWhitelist(c.GetHeader("Referer"), merchant.RefererWhitelist) {
		middleware.SetAPILogContext(c, -1, "请求来源不在白名单内", "", merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "请求来源不在白名单内",
		})
		return
	}

	// 验证签名: MD5(payId + param + type + price + key)
	if !util.VerifyVmqSign(payId, param, payType, price, merchant.Key, sign) {
		middleware.SetAPILogContext(c, -1, "签名验证失败", "", merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "签名验证失败",
		})
		return
	}

	// 映射支付类型
	var chain string
	switch payType {
	case "1", "wechat", "wxpay":
		chain = "trc20" // 默认使用TRC20
	case "2", "alipay":
		chain = "bep20" // 使用BEP20
	case "3", "usdt_trc20", "trc20":
		chain = "trc20"
	case "4", "usdt_erc20", "erc20":
		chain = "erc20"
	case "5", "usdt_bep20", "bep20":
		chain = "bep20"
	default:
		chain = "trc20"
	}

	// 创建订单
	orderService := service.GetOrderService()
	req := &service.CreateOrderRequest{
		MerchantPID: merchant.PID,
		Type:        "usdt_" + chain,
		OutTradeNo:  payId,
		NotifyURL:   notifyUrl,
		ReturnURL:   returnUrl,
		Name:        "VMQ Order",
		Money:       price,
		Param:       param,
		ClientIP:    c.ClientIP(),
	}

	resp, err := orderService.CreateOrder(req)
	if err != nil {
		middleware.SetAPILogContext(c, -1, err.Error(), "", merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  err.Error(),
		})
		return
	}

	// 记录成功日志
	middleware.SetAPILogContext(c, 1, "success", resp.TradeNo, merchant.ID, merchant.PID)

	// 根据isHtml返回不同格式
	if isHtml == "1" {
		c.Redirect(http.StatusFound, "/cashier/"+resp.TradeNo)
		return
	}

	// 返回JSON
	c.JSON(http.StatusOK, gin.H{
		"code":        1,
		"msg":         "success",
		"payId":       payId,
		"orderId":     resp.TradeNo,
		"payType":     payType,
		"price":       resp.Money,
		"reallyPrice": resp.USDTAmount,
		"payUrl":      "/cashier/" + resp.TradeNo,
		"isAuto":      1,
		"state":       0,
		"timeOut":     resp.ExpiredAt,
		"date":        time.Now().Format("2006-01-02 15:04:05"),
	})
}

// AppHeart 心跳检测 (V免签监控端兼容)
// GET /appHeart
func (h *VmqHandler) AppHeart(c *gin.Context) {
	t := c.Query("t")
	if t == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "参数错误",
		})
		return
	}

	// 验证时间戳 (5分钟内有效)
	ts, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "时间戳无效",
		})
		return
	}

	now := time.Now().Unix()
	if now-ts > 300 || ts-now > 300 {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "时间戳过期",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "success",
	})
}

// AppPush 收款推送 (V免签监控端兼容)
// GET /appPush
func (h *VmqHandler) AppPush(c *gin.Context) {
	t := c.Query("t")
	payType := c.Query("type")
	price := c.Query("price")
	sign := c.Query("sign")

	// 验证参数
	if t == "" || price == "" || sign == "" {
		middleware.SetAPILogContext(c, -1, "参数不完整", "", 0, "")
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "参数不完整",
		})
		return
	}

	// 获取默认商户
	var merchant model.Merchant
	if err := model.GetDB().Where("status = 1").First(&merchant).Error; err != nil {
		middleware.SetAPILogContext(c, -1, "商户不存在", "", 0, "")
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "商户不存在",
		})
		return
	}

	// 验证签名: MD5(type + price + t + key)
	expectedSign := util.MD5(payType + price + t + merchant.Key)
	if sign != expectedSign {
		middleware.SetAPILogContext(c, -1, "签名验证失败", "", merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "签名验证失败",
		})
		return
	}

	// 解析金额
	amount, err := decimal.NewFromString(price)
	if err != nil {
		middleware.SetAPILogContext(c, -1, "金额无效", "", merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "金额无效",
		})
		return
	}

	// 映射链类型
	var chain string
	switch payType {
	case "1", "wechat":
		chain = "trc20"
	case "2", "alipay":
		chain = "bep20"
	default:
		chain = "trc20"
	}

	// 查找匹配的待支付订单 (按金额匹配)
	var order model.Order
	err = model.GetDB().
		Where("chain = ? AND usdt_amount = ? AND status = ?", chain, amount, model.OrderStatusPending).
		Order("created_at ASC").
		First(&order).Error

	if err != nil {
		// 尝试模糊匹配
		tolerance := amount.Mul(decimal.NewFromFloat(0.0001))
		minAmount := amount.Sub(tolerance)
		maxAmount := amount.Add(tolerance)

		err = model.GetDB().
			Where("chain = ? AND usdt_amount BETWEEN ? AND ? AND status = ?",
				chain, minAmount, maxAmount, model.OrderStatusPending).
			Order("created_at ASC").
			First(&order).Error

		if err != nil {
			middleware.SetAPILogContext(c, 0, "未找到匹配订单", "", merchant.ID, merchant.PID)
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"msg":  "未找到匹配订单",
			})
			return
		}
	}

	// 更新订单状态
	now := time.Now()
	updates := map[string]interface{}{
		"status":        model.OrderStatusPaid,
		"actual_amount": amount,
		"paid_at":       &now,
	}

	if err := model.GetDB().Model(&order).Updates(updates).Error; err != nil {
		middleware.SetAPILogContext(c, -1, "更新订单失败", order.TradeNo, merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "更新订单失败",
		})
		return
	}

	// 触发回调通知
	go service.GetNotifyService().NotifyOrder(order.ID)

	// 记录成功日志
	middleware.SetAPILogContext(c, 1, "success", order.TradeNo, merchant.ID, merchant.PID)

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "success",
	})
}

// GetState 获取订单状态 (V免签兼容)
// GET /getState
func (h *VmqHandler) GetState(c *gin.Context) {
	payId := c.Query("payId")
	if payId == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "payId不能为空",
		})
		return
	}

	// 查找订单
	var order model.Order
	if err := model.GetDB().Where("out_trade_no = ?", payId).First(&order).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "订单不存在",
		})
		return
	}

	// 状态映射: 0未支付 1已支付 2已过期
	state := 0
	switch order.Status {
	case model.OrderStatusPaid:
		state = 1
	case model.OrderStatusExpired, model.OrderStatusCancelled:
		state = 2
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"msg":   "success",
		"state": state,
	})
}

// CloseOrder 关闭订单 (V免签兼容)
// GET /closeOrder
func (h *VmqHandler) CloseOrder(c *gin.Context) {
	payId := c.Query("payId")
	if payId == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "payId不能为空",
		})
		return
	}

	orderService := service.GetOrderService()

	// 先查找订单获取trade_no
	order, err := orderService.GetOrderByOutTradeNo("", payId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "订单不存在",
		})
		return
	}

	if err := orderService.CancelOrder(order.TradeNo); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "success",
	})
}
