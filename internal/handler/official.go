package handler

import (
	"log"
	"net/http"

	"ezpay/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *ChannelHandler) AlipayOfficialNotify(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	cfg, err := service.GetOfficialConfig(string(service.ChannelTypeAlipayOfficial))
	if err != nil {
		c.String(http.StatusInternalServerError, "fail")
		return
	}
	receipt, err := service.ParseAlipayReceipt(c.Request.Context(), cfg, c.Request.PostForm)
	if err == nil && receipt != nil {
		err = service.CompleteOfficialReceipt(*receipt)
	}
	if err != nil {
		log.Printf("Alipay official callback rejected: %v", err)
		c.String(http.StatusBadRequest, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

func (h *ChannelHandler) WechatOfficialNotify(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	cfg, err := service.GetOfficialConfig(string(service.ChannelTypeWechatOfficial))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "configuration unavailable"})
		return
	}
	receipt, err := service.ParseWechatReceipt(c.Request.Context(), cfg, c.Request)
	if err == nil && receipt != nil {
		err = service.CompleteOfficialReceipt(*receipt)
	}
	if err != nil {
		log.Printf("Wechat official callback rejected: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "callback rejected"})
		return
	}
	c.Status(http.StatusNoContent)
}
