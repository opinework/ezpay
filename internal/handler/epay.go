package handler

import (
	"crypto/hmac"
	"encoding/base64"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	if pid == "" || payType == "" || outTradeNo == "" || name == "" || money == "" || sign == "" {
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

	// 验证签名 - 使用标准解码后的参数
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

	// 验证签名
	if !util.VerifySign(params, merchant.Key, sign) {
		log.Printf("[Submit] 签名验证失败, 收到: %s, 计算: %s", sign, util.GenerateSign(params, merchant.Key))
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
	if pid == "" || payType == "" || outTradeNo == "" || name == "" || money == "" || sign == "" {
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
		log.Printf("[MAPISubmit] 签名验证失败, 收到: %s, 计算: %s", sign, util.GenerateSign(params, merchant.Key))
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

	// 获取站点URL用于构建绝对路径
	siteURL := ""
	var siteConfig model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", "site_url").First(&siteConfig).Error; err == nil && siteConfig.Value != "" {
		siteURL = strings.TrimRight(siteConfig.Value, "/")
	} else {
		// 从请求中推断
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		siteURL = scheme + "://" + c.Request.Host
	}

	cashierURL := siteURL + "/cashier/" + resp.TradeNo

	// 返回JSON（兼容 PHP 彩虹易支付标准字段名）
	c.JSON(http.StatusOK, gin.H{
		"code":          1,
		"msg":           "success",
		"trade_no":      resp.TradeNo,
		"out_trade_no":  resp.OutTradeNo,
		"type":          resp.Type,
		"currency":      resp.Currency,
		"money":         resp.Money,
		"pay_currency":  resp.PayCurrency,
		"pay_amount":    resp.PayAmount,
		"unique_amount": resp.UniqueAmount,
		"usdt_amount":   resp.USDTAmount,
		"rate":          resp.Rate,
		"address":       resp.Address,
		"chain":         resp.Chain,
		"qrcode":        resp.QRCode,
		"expired_at":    resp.ExpiredAt,
		"payurl":        cashierURL, // PHP epay 标准字段名
		"pay_url":       cashierURL, // 兼容字段
	})
}

// API 通用API接口 (api.php 兼容)
// GET /api.php?act=xxx
//
// 支持的 act:
//   - order / query   — 单订单查询，鉴权: pid + key
//   - orders          — 订单列表分页查询，鉴权: pid + key
//   - getmoney        — 商户余额查询，鉴权: pid + key
//   - refund          — 退款（明确拒绝：加密货币本质不可退）
func (h *EpayHandler) API(c *gin.Context) {
	act := c.Query("act")

	switch act {
	case "order", "query":
		h.queryOrder(c)
	case "orders":
		h.listOrders(c)
	case "getmoney":
		h.getMoney(c)
	case "refund":
		// 加密货币交易不可逆，平台不支持退款；老 PHP epay 商户脚本调用时给个明确响应
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "本平台不支持退款（加密货币交易不可逆）；如需处理纠纷请联系平台管理员手动调整",
		})
	default:
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "未知操作",
		})
	}
}

// authMerchantByKey 通过 pid + key 验证商户，返回商户对象
// 用于 api.php 上的 GET 接口（query/orders/getmoney 等）
func (h *EpayHandler) authMerchantByKey(c *gin.Context) (*model.Merchant, bool) {
	pid := c.Query("pid")
	key := c.Query("key")
	if pid == "" || key == "" {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数不完整"})
		return nil, false
	}

	var merchant model.Merchant
	if err := model.GetDB().Where("p_id = ?", pid).First(&merchant).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户不存在"})
		return nil, false
	}
	if !hmac.Equal([]byte(merchant.Key), []byte(key)) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "密钥错误"})
		return nil, false
	}
	if merchant.Status != 1 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "商户已禁用"})
		return nil, false
	}
	return &merchant, true
}

// listOrders 订单列表分页查询 (api.php?act=orders)
//
// 参数:
//   - pid (必填), key (必填): 商户鉴权
//   - limit (可选, 默认 20, 最大 100): 每页数量
//   - offset (可选, 默认 0): 偏移
//   - status (可选): 0=待支付 1=已支付 2=已过期 3=已取消，不传返回全部
//   - start_time / end_time (可选, RFC3339 或 "2006-01-02 15:04:05"): 创建时间过滤
//   - type (可选): 支付类型过滤 (如 usdt_trc20)
func (h *EpayHandler) listOrders(c *gin.Context) {
	merchant, ok := h.authMerchantByKey(c)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}

	q := model.GetDB().Model(&model.Order{}).Where("merchant_id = ?", merchant.ID)
	if statusStr := c.Query("status"); statusStr != "" {
		if s, err := strconv.Atoi(statusStr); err == nil {
			q = q.Where("status = ?", s)
		}
	}
	if t := c.Query("type"); t != "" {
		q = q.Where("type = ?", t)
	}
	if st := c.Query("start_time"); st != "" {
		if parsed, err := parseFlexibleTime(st); err == nil {
			q = q.Where("created_at >= ?", parsed)
		}
	}
	if et := c.Query("end_time"); et != "" {
		if parsed, err := parseFlexibleTime(et); err == nil {
			q = q.Where("created_at <= ?", parsed)
		}
	}

	var total int64
	q.Count(&total)

	var orders []model.Order
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&orders).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "查询失败"})
		return
	}

	// 转换为列表友好的精简字段
	list := make([]gin.H, 0, len(orders))
	for _, o := range orders {
		tradeStatus := "WAIT_BUYER_PAY"
		switch o.Status {
		case model.OrderStatusPaid:
			tradeStatus = "TRADE_SUCCESS"
		case model.OrderStatusExpired, model.OrderStatusCancelled:
			tradeStatus = "TRADE_CLOSED"
		}
		item := gin.H{
			"trade_no":     o.TradeNo,
			"out_trade_no": o.OutTradeNo,
			"type":         o.Type,
			"name":         o.Name,
			"money":        o.Money.String(),
			"currency":     o.Currency,
			"pay_currency": o.PayCurrency,
			"pay_amount":   o.UniqueAmount.String(),
			"trade_status": tradeStatus,
			"param":        o.Param,
			"addtime":      o.CreatedAt.Unix(),
			"endtime":      o.ExpiredAt.Unix(),
		}
		if o.PaidAt != nil {
			item["paidtime"] = o.PaidAt.Unix()
		}
		list = append(list, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":   1,
		"msg":    "success",
		"pid":    merchant.PID,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"orders": list,
	})
}

// getMoney 商户余额查询 (api.php?act=getmoney)
//
// 鉴权: pid + key
// 返回: USD 余额、冻结余额、可用余额（balance - frozen）
func (h *EpayHandler) getMoney(c *gin.Context) {
	merchant, ok := h.authMerchantByKey(c)
	if !ok {
		return
	}

	available := merchant.Balance.Sub(merchant.FrozenBalance)
	c.JSON(http.StatusOK, gin.H{
		"code":           1,
		"msg":            "success",
		"pid":            merchant.PID,
		"currency":       "USD", // 余额始终以 USD 统一结算
		"balance":        merchant.Balance.String(),
		"frozen_balance": merchant.FrozenBalance.String(),
		"available":      available.String(),
	})
}

// parseFlexibleTime 接受 RFC3339 或 "2006-01-02 15:04:05" 两种时间格式
func parseFlexibleTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
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

	if !hmac.Equal([]byte(merchant.Key), []byte(key)) {
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
	if order.MerchantID != merchant.ID {
		c.JSON(http.StatusOK, gin.H{
			"code": -1,
			"msg":  "订单不存在",
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
		"param":        order.Param,
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
		if err := model.GetDB().First(&merchant, order.MerchantID).Error; err == nil {
			result["return_url"] = service.GetNotifyService().BuildReturnURL(order, &merchant)
		}
	}

	c.JSON(http.StatusOK, result)
}

// QRCodeImage 生成二维码 PNG 图片
// GET /qrcode?text=...&size=300
//
// 用途：mapi 响应里 qrcode 字段的目标图片端点。商户可直接 <img src="..."> 嵌入。
// 公开访问，无需鉴权（生成的内容由 text 参数完全决定，不读敏感数据）。
// 限制：text 长度 ≤ 1024，size 范围 [64, 1024]，避免被滥用做 DoS
func (h *EpayHandler) QRCodeImage(c *gin.Context) {
	text := c.Query("text")
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "msg": "text 参数不能为空"})
		return
	}
	if len(text) > 1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "msg": "text 过长"})
		return
	}

	size := 300
	if s := c.Query("size"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			size = n
		}
	}
	if size < 64 {
		size = 64
	}
	if size > 1024 {
		size = 1024
	}

	dataURI, err := util.GenerateQRCode(text, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "msg": "生成二维码失败"})
		return
	}
	// dataURI 形如 "data:image/png;base64,xxxxx"，去掉前缀拿到 base64
	idx := strings.Index(dataURI, ",")
	if idx == -1 {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "msg": "生成二维码失败"})
		return
	}
	pngBytes, err := base64.StdEncoding.DecodeString(dataURI[idx+1:])
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "msg": "生成二维码失败"})
		return
	}

	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "image/png", pngBytes)
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
