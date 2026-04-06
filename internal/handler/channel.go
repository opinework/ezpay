package handler

import (
	"log"
	"net/http"

	"ezpay/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ChannelHandler 支付通道回调处理器
type ChannelHandler struct{}

// NewChannelHandler 创建处理器
func NewChannelHandler() *ChannelHandler {
	return &ChannelHandler{}
}

// VmqNotify 处理V免签回调
// GET /channel/notify/vmq
func (h *ChannelHandler) VmqNotify(c *gin.Context) {
	// V免签回调参数
	payID := c.Query("payId")       // 商户订单号
	param := c.Query("param")       // 附加参数 (存储的是本地订单号)
	payType := c.Query("type")      // 支付类型
	price := c.Query("price")       // 订单金额
	reallyPrice := c.Query("reallyPrice") // 实际金额
	sign := c.Query("sign")         // 签名

	log.Printf("Vmq notify received: payId=%s, param=%s, type=%s, price=%s, reallyPrice=%s",
		payID, param, payType, price, reallyPrice)

	// 获取V免签通道配置
	channelService := service.GetChannelService()
	cfg, err := channelService.GetChannelConfig(service.ChannelTypeVmq)
	if err != nil {
		log.Printf("Vmq channel not configured: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}

	// 验证签名
	if !channelService.VerifyVmqNotify(cfg, payID, param, payType, price, reallyPrice, sign) {
		log.Printf("Vmq notify sign verify failed")
		c.String(http.StatusOK, "fail")
		return
	}

	// param 存储的是本地订单号
	localTradeNo := param
	if localTradeNo == "" {
		localTradeNo = payID
	}

	// 解析实际金额
	amount, _ := decimal.NewFromString(reallyPrice)

	// 处理上游通知
	if err := channelService.HandleUpstreamNotify(service.ChannelTypeVmq, localTradeNo, payID, amount); err != nil {
		log.Printf("Handle vmq notify failed: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}

	c.String(http.StatusOK, "success")
}

// EpayNotify 处理彩虹易支付回调
// GET /channel/notify/epay
func (h *ChannelHandler) EpayNotify(c *gin.Context) {
	// 易支付回调参数
	pid := c.Query("pid")
	tradeNo := c.Query("trade_no")
	outTradeNo := c.Query("out_trade_no") // 本地订单号
	payType := c.Query("type")
	name := c.Query("name")
	money := c.Query("money")
	tradeStatus := c.Query("trade_status")
	sign := c.Query("sign")

	log.Printf("Epay notify received: pid=%s, trade_no=%s, out_trade_no=%s, status=%s",
		pid, tradeNo, outTradeNo, tradeStatus)

	// 只处理支付成功的通知
	if tradeStatus != "TRADE_SUCCESS" {
		c.String(http.StatusOK, "success")
		return
	}

	// 获取易支付通道配置
	channelService := service.GetChannelService()
	cfg, err := channelService.GetChannelConfig(service.ChannelTypeEpay)
	if err != nil {
		log.Printf("Epay channel not configured: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}

	// 验证签名
	params := map[string]string{
		"pid":          pid,
		"trade_no":     tradeNo,
		"out_trade_no": outTradeNo,
		"type":         payType,
		"name":         name,
		"money":        money,
		"trade_status": tradeStatus,
	}

	if !channelService.VerifyEpayNotify(cfg, params, sign) {
		log.Printf("Epay notify sign verify failed")
		c.String(http.StatusOK, "fail")
		return
	}

	// 解析金额
	amount, _ := decimal.NewFromString(money)

	// 处理上游通知 (out_trade_no 是本地订单号)
	if err := channelService.HandleUpstreamNotify(service.ChannelTypeEpay, outTradeNo, tradeNo, amount); err != nil {
		log.Printf("Handle epay notify failed: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}

	c.String(http.StatusOK, "success")
}
