package handler

import (
	"net/http"
	"strings"

	"ezpay/internal/middleware"
	"ezpay/internal/model"
	"ezpay/internal/service"
	"ezpay/internal/util"

	"github.com/gin-gonic/gin"
)

// EpayHandler 彩虹易支付兼容接口处理器
type EpayHandler struct{}

// NewEpayHandler 创建处理器
func NewEpayHandler() *EpayHandler {
	return &EpayHandler{}
}

// Submit 发起支付 (submit.php 兼容)
// POST/GET /submit.php
func (h *EpayHandler) Submit(c *gin.Context) {
	// 获取参数
	pid := c.DefaultQuery("pid", c.PostForm("pid"))
	payType := c.DefaultQuery("type", c.PostForm("type"))
	outTradeNo := c.DefaultQuery("out_trade_no", c.PostForm("out_trade_no"))
	notifyURL := c.DefaultQuery("notify_url", c.PostForm("notify_url"))
	returnURL := c.DefaultQuery("return_url", c.PostForm("return_url"))
	name := c.DefaultQuery("name", c.PostForm("name"))
	money := c.DefaultQuery("money", c.PostForm("money"))
	currency := c.DefaultQuery("currency", c.PostForm("currency")) // 货币类型: CNY, USD, USDT 等
	sign := c.DefaultQuery("sign", c.PostForm("sign"))
	signType := c.DefaultQuery("sign_type", c.PostForm("sign_type"))
	param := c.DefaultQuery("param", c.PostForm("param"))

	// 验证必填参数
	if pid == "" || payType == "" || outTradeNo == "" || money == "" || sign == "" {
		middleware.SetAPILogContext(c, -1, "参数不完整", "", 0, pid)
		h.renderError(c, "参数不完整")
		return
	}

	// 获取商户
	var merchant model.Merchant
	if err := model.GetDB().Where("p_id = ? AND status = 1", pid).First(&merchant).Error; err != nil {
		middleware.SetAPILogContext(c, -1, "商户不存在或已禁用", "", 0, pid)
		h.renderError(c, "商户不存在或已禁用")
		return
	}

	// 检查IP白名单 (仅当启用时检查)
	if merchant.IPWhitelistEnabled && !middleware.CheckIPWhitelist(c.ClientIP(), merchant.IPWhitelist) {
		middleware.SetAPILogContext(c, -1, "IP不在白名单内", "", merchant.ID, pid)
		h.renderError(c, "IP不在白名单内")
		return
	}

	// 检查Referer白名单 (仅当启用时检查)
	if merchant.RefererWhitelistEnabled && !middleware.CheckRefererWhitelist(c.GetHeader("Referer"), merchant.RefererWhitelist) {
		middleware.SetAPILogContext(c, -1, "请求来源不在白名单内", "", merchant.ID, pid)
		h.renderError(c, "请求来源不在白名单内")
		return
	}

	// 验证签名
	params := map[string]string{
		"pid":          pid,
		"type":         payType,
		"out_trade_no": outTradeNo,
		"notify_url":   notifyURL,
		"return_url":   returnURL,
		"name":         name,
		"money":        money,
		"currency":     currency,
		"param":        param,
	}

	if signType == "" {
		signType = "MD5"
	}

	if !util.VerifySign(params, merchant.Key, sign) {
		middleware.SetAPILogContext(c, -1, "签名验证失败", "", merchant.ID, pid)
		h.renderError(c, "签名验证失败")
		return
	}

	// 使用商户默认回调地址
	if notifyURL == "" {
		notifyURL = merchant.NotifyURL
	}
	if returnURL == "" {
		returnURL = merchant.ReturnURL
	}

	// 创建订单
	orderService := service.GetOrderService()
	req := &service.CreateOrderRequest{
		MerchantPID: pid,
		Type:        payType,
		OutTradeNo:  outTradeNo,
		NotifyURL:   notifyURL,
		ReturnURL:   returnURL,
		Name:        name,
		Money:       money,
		Currency:    currency,
		Param:       param,
		ClientIP:    c.ClientIP(),
	}

	resp, err := orderService.CreateOrder(req)
	if err != nil {
		middleware.SetAPILogContext(c, -1, err.Error(), "", merchant.ID, pid)
		h.renderError(c, err.Error())
		return
	}

	// 记录成功日志
	middleware.SetAPILogContext(c, 1, "success", resp.TradeNo, merchant.ID, pid)

	// 跳转到收银台
	c.Redirect(http.StatusFound, "/cashier/"+resp.TradeNo)
}

// MAPISubmit API方式发起支付 (mapi.php 兼容)
// POST /mapi.php
func (h *EpayHandler) MAPISubmit(c *gin.Context) {
	// 获取参数
	pid := c.DefaultQuery("pid", c.PostForm("pid"))
	payType := c.DefaultQuery("type", c.PostForm("type"))
	outTradeNo := c.DefaultQuery("out_trade_no", c.PostForm("out_trade_no"))
	notifyURL := c.DefaultQuery("notify_url", c.PostForm("notify_url"))
	returnURL := c.DefaultQuery("return_url", c.PostForm("return_url"))
	name := c.DefaultQuery("name", c.PostForm("name"))
	money := c.DefaultQuery("money", c.PostForm("money"))
	currency := c.DefaultQuery("currency", c.PostForm("currency")) // 货币类型: CNY, USD, USDT 等
	sign := c.DefaultQuery("sign", c.PostForm("sign"))
	param := c.DefaultQuery("param", c.PostForm("param"))

	// 验证必填参数
	if pid == "" || payType == "" || outTradeNo == "" || money == "" || sign == "" {
		middleware.SetAPILogContext(c, -1, "参数不完整", "", 0, pid)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "参数不完整",
		})
		return
	}

	// 获取商户
	var merchant model.Merchant
	if err := model.GetDB().Where("p_id = ? AND status = 1", pid).First(&merchant).Error; err != nil {
		middleware.SetAPILogContext(c, -1, "商户不存在或已禁用", "", 0, pid)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "商户不存在或已禁用",
		})
		return
	}

	// 检查IP白名单 (仅当启用时检查)
	if merchant.IPWhitelistEnabled && !middleware.CheckIPWhitelist(c.ClientIP(), merchant.IPWhitelist) {
		middleware.SetAPILogContext(c, -1, "IP不在白名单内", "", merchant.ID, pid)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "IP不在白名单内",
		})
		return
	}

	// 检查Referer白名单 (仅当启用时检查)
	if merchant.RefererWhitelistEnabled && !middleware.CheckRefererWhitelist(c.GetHeader("Referer"), merchant.RefererWhitelist) {
		middleware.SetAPILogContext(c, -1, "请求来源不在白名单内", "", merchant.ID, pid)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "请求来源不在白名单内",
		})
		return
	}

	// 验证签名
	params := map[string]string{
		"pid":          pid,
		"type":         payType,
		"out_trade_no": outTradeNo,
		"notify_url":   notifyURL,
		"return_url":   returnURL,
		"name":         name,
		"money":        money,
		"currency":     currency,
		"param":        param,
	}

	if !util.VerifySign(params, merchant.Key, sign) {
		middleware.SetAPILogContext(c, -1, "签名验证失败", "", merchant.ID, pid)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "签名验证失败",
		})
		return
	}

	// 使用商户默认回调地址
	if notifyURL == "" {
		notifyURL = merchant.NotifyURL
	}
	if returnURL == "" {
		returnURL = merchant.ReturnURL
	}

	// 创建订单
	orderService := service.GetOrderService()
	req := &service.CreateOrderRequest{
		MerchantPID: pid,
		Type:        payType,
		OutTradeNo:  outTradeNo,
		NotifyURL:   notifyURL,
		ReturnURL:   returnURL,
		Name:        name,
		Money:       money,
		Currency:    currency,
		Param:       param,
		ClientIP:    c.ClientIP(),
	}

	resp, err := orderService.CreateOrder(req)
	if err != nil {
		middleware.SetAPILogContext(c, -1, err.Error(), "", merchant.ID, pid)
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  err.Error(),
		})
		return
	}

	// 记录成功日志
	middleware.SetAPILogContext(c, 1, "success", resp.TradeNo, merchant.ID, pid)

	// 返回JSON
	c.JSON(http.StatusOK, gin.H{
		"code":         1,
		"msg":          "success",
		"trade_no":     resp.TradeNo,
		"out_trade_no": resp.OutTradeNo,
		"type":         resp.Type,
		"currency":     resp.Currency,
		"money":        resp.Money,
		"pay_currency": resp.PayCurrency,
		"pay_amount":   resp.PayAmount,
		"usdt_amount":  resp.USDTAmount,
		"rate":         resp.Rate,
		"address":      resp.Address,
		"chain":        resp.Chain,
		"qrcode":       resp.QRCode,
		"expired_at":   resp.ExpiredAt,
		"pay_url":      "/cashier/" + resp.TradeNo,
	})
}

// API 通用API接口 (api.php 兼容)
// GET /api.php?act=xxx
func (h *EpayHandler) API(c *gin.Context) {
	act := c.Query("act")

	switch act {
	case "order":
		h.queryOrder(c)
	case "query":
		h.queryOrder(c)
	default:
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "未知操作",
		})
	}
}

// queryOrder 查询订单
func (h *EpayHandler) queryOrder(c *gin.Context) {
	pid := c.Query("pid")
	key := c.Query("key")
	outTradeNo := c.Query("out_trade_no")
	tradeNo := c.Query("trade_no")

	if pid == "" || key == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "参数不完整",
		})
		return
	}

	// 验证商户密钥
	var merchant model.Merchant
	if err := model.GetDB().Where("p_id = ?", pid).First(&merchant).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "商户不存在",
		})
		return
	}

	if merchant.Key != key {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "密钥错误",
		})
		return
	}

	// 查询订单
	var order *model.Order
	var err error
	orderService := service.GetOrderService()

	if tradeNo != "" {
		order, err = orderService.GetOrder(tradeNo)
	} else if outTradeNo != "" {
		order, err = orderService.GetOrderByOutTradeNo(pid, outTradeNo)
	} else {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "请提供订单号",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  err.Error(),
		})
		return
	}

	// 返回订单信息
	tradeStatus := "WAIT_BUYER_PAY"
	if order.Status == model.OrderStatusPaid {
		tradeStatus = "TRADE_SUCCESS"
	} else if order.Status == model.OrderStatusExpired {
		tradeStatus = "TRADE_CLOSED"
	} else if order.Status == model.OrderStatusCancelled {
		tradeStatus = "TRADE_CLOSED"
	}

	c.JSON(http.StatusOK, gin.H{
		"code":         1,
		"msg":          "success",
		"pid":          pid,
		"trade_no":     order.TradeNo,
		"out_trade_no": order.OutTradeNo,
		"type":         order.Type,
		"name":         order.Name,
		"money":        order.Money.String(),
		"usdt_amount":  order.USDTAmount.String(),
		"trade_status": tradeStatus,
		"addtime":      order.CreatedAt.Unix(),
		"endtime":      order.ExpiredAt.Unix(),
	})
}

// CheckOrder 检查订单状态 (轮询接口)
// GET /api/check_order?trade_no=xxx
func (h *EpayHandler) CheckOrder(c *gin.Context) {
	tradeNo := c.Query("trade_no")
	if tradeNo == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "订单号不能为空",
		})
		return
	}

	orderService := service.GetOrderService()
	paid, order, err := orderService.CheckOrderPaid(tradeNo)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  err.Error(),
		})
		return
	}

	result := gin.H{
		"code":   1,
		"status": order.Status,
		"paid":   paid,
	}

	if paid && order.ReturnURL != "" {
		// 构建返回URL
		var merchant model.Merchant
		model.GetDB().First(&merchant, order.MerchantID)
		result["return_url"] = service.GetNotifyService().BuildReturnURL(order, &merchant)
	}

	c.JSON(http.StatusOK, result)
}

// renderError 渲染错误页面
func (h *EpayHandler) renderError(c *gin.Context, msg string) {
	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/json") {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  msg,
		})
		return
	}

	c.HTML(http.StatusOK, "error.html", gin.H{
		"title": "支付错误",
		"msg":   msg,
	})
}

// PaymentTypeInfo 支付类型信息
type PaymentTypeInfo struct {
	Type    string `json:"type"`    // 支付类型代码
	Name    string `json:"name"`    // 显示名称
	Chain   string `json:"chain"`   // 链名称
	Icon    string `json:"icon"`    // 图标 (CSS类名或URL)
	Logo    string `json:"logo"`    // Logo URL
	Enabled bool   `json:"enabled"` // 是否启用
}

// 定义所有支付类型
var allPaymentTypes = []PaymentTypeInfo{
	{Type: "usdt_trc20", Name: "USDT (TRC20)", Chain: "trc20", Icon: "fab fa-bitcoin", Logo: "/static/img/chains/trc20.svg"},
	{Type: "usdt_bep20", Name: "USDT (BEP20)", Chain: "bep20", Icon: "fab fa-bitcoin", Logo: "/static/img/chains/bep20.svg"},
	{Type: "usdt_erc20", Name: "USDT (ERC20)", Chain: "erc20", Icon: "fab fa-ethereum", Logo: "/static/img/chains/erc20.svg"},
	{Type: "usdt_polygon", Name: "USDT (Polygon)", Chain: "polygon", Icon: "fas fa-gem", Logo: "/static/img/chains/polygon.svg"},
	{Type: "usdt_arbitrum", Name: "USDT (Arbitrum)", Chain: "arbitrum", Icon: "fas fa-layer-group", Logo: "/static/img/chains/arbitrum.svg"},
	{Type: "usdt_optimism", Name: "USDT (Optimism)", Chain: "optimism", Icon: "fas fa-rocket", Logo: "/static/img/chains/optimism.svg"},
	{Type: "usdt_base", Name: "USDT (Base)", Chain: "base", Icon: "fas fa-cube", Logo: "/static/img/chains/base.svg"},
	{Type: "usdt_avalanche", Name: "USDT (Avalanche)", Chain: "avalanche", Icon: "fas fa-mountain", Logo: "/static/img/chains/avalanche.svg"},
	{Type: "trx", Name: "TRX", Chain: "trx", Icon: "fas fa-coins", Logo: "/static/img/chains/trx.svg"},
	{Type: "wechat", Name: "微信支付", Chain: "wechat", Icon: "fab fa-weixin", Logo: "/static/img/chains/wechat.svg"},
	{Type: "alipay", Name: "支付宝", Chain: "alipay", Icon: "fab fa-alipay", Logo: "/static/img/chains/alipay.svg"},
}

// GetPaymentTypes 获取支持的支付类型列表
// GET /api/payment-types?pid=xxx
func (h *EpayHandler) GetPaymentTypes(c *gin.Context) {
	pid := c.Query("pid")

	// pid 必填
	if pid == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "缺少商户号参数",
		})
		return
	}

	// 获取区块链服务的链状态
	blockchainService := service.GetBlockchainService()
	chainStatus := blockchainService.GetListenerStatus()

	// 检查链是否在区块链服务中启用
	isChainEnabled := func(chain string) bool {
		if status, ok := chainStatus[chain]; ok {
			if s, ok := status.(map[string]interface{}); ok {
				if enabled, _ := s["enabled"].(bool); enabled {
					return true
				}
			}
		}
		return false
	}

	// 验证商户
	var merchant model.Merchant
	if err := model.GetDB().Where("p_id = ? AND status = 1", pid).First(&merchant).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "商户不存在或已禁用",
		})
		return
	}

	// 根据商户钱包模式查询可用的链
	// WalletMode: 1=仅系统钱包 2=仅个人钱包 3=两者同时(优先个人)
	availableChains := make(map[string]bool)

	var wallets []model.Wallet
	switch merchant.WalletMode {
	case 1: // 仅系统钱包
		model.GetDB().Where("(merchant_id IS NULL OR merchant_id = 0) AND status = 1").
			Select("DISTINCT chain").Find(&wallets)
	case 2: // 仅个人钱包
		model.GetDB().Where("merchant_id = ? AND status = 1", merchant.ID).
			Select("DISTINCT chain").Find(&wallets)
	default: // 3 或其他：两者同时
		model.GetDB().Where("((merchant_id IS NULL OR merchant_id = 0) OR merchant_id = ?) AND status = 1", merchant.ID).
			Select("DISTINCT chain").Find(&wallets)
	}

	for _, w := range wallets {
		availableChains[w.Chain] = true
	}

	// 过滤出商户可用的支付类型
	var enabledTypes []PaymentTypeInfo
	for _, pt := range allPaymentTypes {
		// 必须同时满足：区块链服务启用 + 商户有对应钱包
		pt.Enabled = isChainEnabled(pt.Chain) && availableChains[pt.Chain]
		enabledTypes = append(enabledTypes, pt)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "success",
		"data": enabledTypes,
	})
}
