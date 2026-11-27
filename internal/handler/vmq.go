package handler

import (
	"crypto/hmac"
	"errors"
	"net/http"
	"strings"
	"time"

	"ezpay/internal/middleware"
	"ezpay/internal/model"
	"ezpay/internal/service"
	"ezpay/internal/util"

	"github.com/gin-gonic/gin"
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
	payType := c.DefaultQuery("type", "1") // 1:微信 2:支付宝 3:USDT-TRC20 4:USDT-ERC20 5:USDT-BEP20
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

	// 通过签名匹配商户（支持多商户）
	var merchants []model.Merchant
	if err := model.GetDB().Where("status = 1").Find(&merchants).Error; err != nil || len(merchants) == 0 {
		middleware.SetAPILogContext(c, -1, "商户不存在", "", 0, "")
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "商户不存在",
		})
		return
	}

	// 验证签名: MD5(payId + param + type + price + key)，遍历商户匹配
	var merchant model.Merchant
	var signMatched bool
	for _, m := range merchants {
		if util.VerifyVmqSign(payId, param, payType, price, m.Key, sign) {
			merchant = m
			signMatched = true
			break
		}
	}

	if !signMatched {
		middleware.SetAPILogContext(c, -1, "签名验证失败", "", 0, "")
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "签名验证失败",
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

	// 映射支付类型
	var chain string
	switch payType {
	case "1", "wechat", "wxpay":
		chain = "wechat"
	case "2", "alipay":
		chain = "alipay"
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
		Type:        util.NormalizePaymentType(chain),
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

// AppHeart handles authenticated monitor heartbeats.
func (h *VmqHandler) AppHeart(c *gin.Context) {
	h.monitorHeartbeat(c)
}

// AppPush handles durable, idempotent receipt events.
func (h *VmqHandler) AppPush(c *gin.Context) {
	h.monitorPush(c)
}

// GetState 获取订单状态 (V免签兼容)
// GET /getState
func (h *VmqHandler) GetState(c *gin.Context) {
	payId := c.Query("payId")
	sign := c.Query("sign")
	if payId == "" || sign == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "参数不完整",
		})
		return
	}

	order, err := h.findVmqOrderBySign(payId, sign)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  err.Error(),
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
	sign := c.Query("sign")
	if payId == "" || sign == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "参数不完整",
		})
		return
	}

	orderService := service.GetOrderService()

	order, err := h.findVmqOrderBySign(payId, sign)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  err.Error(),
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

func (h *VmqHandler) findVmqOrderBySign(payId, sign string) (*model.Order, error) {
	var orders []model.Order
	if err := model.GetDB().Preload("Merchant").Where("out_trade_no = ?", payId).Find(&orders).Error; err != nil {
		return nil, errors.New("订单不存在")
	}
	if len(orders) == 0 {
		return nil, errors.New("订单不存在")
	}

	for i := range orders {
		if orders[i].Merchant == nil {
			continue
		}
		expectedSign := util.MD5(payId + orders[i].Merchant.Key)
		if hmac.Equal([]byte(strings.ToLower(sign)), []byte(expectedSign)) {
			return &orders[i], nil
		}
	}
	return nil, errors.New("签名错误")
}
