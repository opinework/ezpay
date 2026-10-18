package handler

import (
	"crypto/hmac"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ezpay/internal/middleware"
	"ezpay/internal/model"
	"ezpay/internal/service"
	"ezpay/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

	// 验证签名: MD5(type + price + t + key)，遍历商户匹配
	var merchant model.Merchant
	var signMatched bool
	for _, m := range merchants {
		expectedSign := util.MD5(payType + price + t + m.Key)
		if strings.EqualFold(sign, expectedSign) {
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
	// 监控端发来的是法币收款通知（微信/支付宝实际到账）
	// 需要匹配法币订单的 chain（wechat/alipay），而非加密货币链
	var chain string
	switch payType {
	case "1", "wechat":
		chain = "wechat"
	case "2", "alipay":
		chain = "alipay"
	default:
		chain = "wechat"
	}

	// 使用事务 + 行锁，防止并发匹配同一订单
	var order model.Order
	var orderFound bool

	txErr := model.GetDB().Transaction(func(tx *gorm.DB) error {
		// 优先使用 unique_amount 精确匹配（MySQL 加行锁防止竞态，SQLite 单连接天然串行）
		query := tx.Where("merchant_id = ? AND chain = ? AND unique_amount = ? AND status = ?",
			merchant.ID, chain, amount, model.OrderStatusPending).
			Order("created_at ASC")
		if !model.IsSQLite() {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		err = query.First(&order).Error

		if err != nil {
			// 兼容旧订单：尝试使用 usdt_amount 精确匹配
			fallbackQuery := tx.Where("merchant_id = ? AND chain = ? AND usdt_amount = ? AND status = ?",
				merchant.ID, chain, amount, model.OrderStatusPending).
				Order("created_at ASC")
			if !model.IsSQLite() {
				fallbackQuery = fallbackQuery.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			err = fallbackQuery.First(&order).Error

			if err != nil {
				return nil // 未找到，但不是错误
			}
		}

		orderFound = true

		// 原子更新：再次确认 status=pending（双重保障幂等性）
		now := time.Now()
		result := tx.Model(&model.Order{}).
			Where("id = ? AND status = ?", order.ID, model.OrderStatusPending).
			Updates(map[string]interface{}{
				"status":        model.OrderStatusPaid,
				"actual_amount": amount,
				"paid_at":       &now,
			})

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// 订单已被其他请求处理（幂等：视为成功）
			orderFound = false
			return nil
		}

		// 增加商户余额（在同一事务内，保证原子性）
		realAmount := order.SettlementAmount.Sub(order.Fee)
		if order.FeeType == model.FeeTypeBalance {
			return tx.Model(&model.Merchant{}).Where("id = ?", order.MerchantID).Updates(map[string]interface{}{
				"balance":        gorm.Expr("balance + ?", realAmount.String()),
				"frozen_balance": gorm.Expr("frozen_balance - ?", order.Fee.String()),
			}).Error
		}
		return tx.Model(&model.Merchant{}).Where("id = ?", order.MerchantID).
			Update("balance", gorm.Expr("balance + ?", realAmount.String())).Error
	})

	if txErr != nil {
		middleware.SetAPILogContext(c, -1, "更新订单失败", "", merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "更新订单失败",
		})
		return
	}

	if !orderFound {
		middleware.SetAPILogContext(c, 0, "未找到匹配订单", "", merchant.ID, merchant.PID)
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "未找到匹配订单",
		})
		return
	}

	// 使订单缓存失效
	service.GetOrderService().InvalidateOrderCache(order.TradeNo)

	// 重新加载订单数据
	model.GetDB().First(&order, order.ID)

	// 触发回调通知
	go service.GetNotifyService().NotifyOrder(order.ID)

	// 触发 Telegram 通知
	go service.GetTelegramService().NotifyOrderPaid(&order)

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
