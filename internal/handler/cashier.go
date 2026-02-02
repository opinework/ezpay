package handler

import (
	"net/http"
	"strconv"

	"ezpay/internal/model"
	"ezpay/internal/service"

	"github.com/gin-gonic/gin"
)

// CashierHandler 收银台处理器
type CashierHandler struct{}

// NewCashierHandler 创建处理器
func NewCashierHandler() *CashierHandler {
	return &CashierHandler{}
}

// ShowCashier 显示收银台页面
func (h *CashierHandler) ShowCashier(c *gin.Context) {
	tradeNo := c.Param("trade_no")

	orderService := service.GetOrderService()
	order, err := orderService.GetOrder(tradeNo)
	if err != nil {
		c.HTML(http.StatusOK, "error.html", gin.H{
			"title": "订单不存在",
			"msg":   "订单不存在或已失效",
		})
		return
	}

	// 检查订单状态
	if order.Status == model.OrderStatusPaid {
		// 已支付，跳转到成功页面或返回URL
		if order.ReturnURL != "" {
			var merchant model.Merchant
			model.GetDB().First(&merchant, order.MerchantID)
			returnURL := service.GetNotifyService().BuildReturnURL(order, &merchant)
			c.Redirect(http.StatusFound, returnURL)
			return
		}
		c.HTML(http.StatusOK, "success.html", gin.H{
			"order": order,
		})
		return
	}

	if order.Status == model.OrderStatusExpired {
		c.HTML(http.StatusOK, "error.html", gin.H{
			"title": "订单已过期",
			"msg":   "订单已过期，请重新发起支付",
		})
		return
	}

	if order.Status == model.OrderStatusCancelled {
		c.HTML(http.StatusOK, "error.html", gin.H{
			"title": "订单已取消",
			"msg":   "订单已取消",
		})
		return
	}

	// 获取订单过期时间配置
	expireMinutes := 30 // 默认30分钟
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", model.ConfigKeyOrderExpire).First(&config).Error; err == nil {
		if minutes, err := strconv.Atoi(config.Value); err == nil {
			expireMinutes = minutes
		}
	}

	// 渲染收银台页面
	c.HTML(http.StatusOK, "cashier.html", gin.H{
		"order":         order,
		"expireMinutes": expireMinutes,
		"expiredAt":     order.ExpiredAt.UnixMilli(),
	})
}

// GetOrderInfo 获取订单信息 (用于前端轮询)
func (h *CashierHandler) GetOrderInfo(c *gin.Context) {
	tradeNo := c.Param("trade_no")

	orderService := service.GetOrderService()
	order, err := orderService.GetOrder(tradeNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "订单不存在",
		})
		return
	}

	result := gin.H{
		"code":          1,
		"trade_no":      order.TradeNo,
		"status":        order.Status,
		"money":         order.Money.String(),
		"pay_amount":    order.PayAmount.String(),    // 展示金额（无偏移）
		"unique_amount": order.UniqueAmount.String(), // 唯一标识金额（含偏移，实际支付）
		"usdt_amount":   order.UniqueAmount.String(), // 兼容旧字段
		"address":       order.ToAddress,
		"chain":         order.Chain,
		"expired_at":    order.ExpiredAt,
	}

	// 如果已支付，返回返回URL
	if order.Status == model.OrderStatusPaid && order.ReturnURL != "" {
		var merchant model.Merchant
		model.GetDB().First(&merchant, order.MerchantID)
		result["return_url"] = service.GetNotifyService().BuildReturnURL(order, &merchant)
	}

	c.JSON(http.StatusOK, result)
}
