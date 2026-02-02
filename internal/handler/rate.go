package handler

import (
	"net/http"
	"time"

	"ezpay/internal/model"
	"ezpay/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// RateHandler 汇率管理处理器
type RateHandler struct{}

// NewRateHandler 创建汇率管理处理器
func NewRateHandler() *RateHandler {
	return &RateHandler{}
}

// ListExchangeRates 汇率列表
func (h *RateHandler) ListExchangeRates(c *gin.Context) {
	var rates []model.ExchangeRate

	if err := model.GetDB().Order("from_currency, to_currency").Find(&rates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "查询失败: " + err.Error()})
		return
	}

	// 获取买入和卖出浮动配置
	buyFloat, _ := decimal.NewFromString(service.GetRateService().GetConfigValue(model.ConfigKeyRateBuyFloat, "0.02"))
	sellFloat, _ := decimal.NewFromString(service.GetRateService().GetConfigValue(model.ConfigKeyRateSellFloat, "0.02"))

	// 计算买入价和卖出价
	type RateItem struct {
		model.ExchangeRate
		BuyRate  decimal.Decimal `json:"buy_rate"`  // 买入价
		SellRate decimal.Decimal `json:"sell_rate"` // 卖出价
	}

	var result []RateItem
	for _, rate := range rates {
		result = append(result, RateItem{
			ExchangeRate: rate,
			BuyRate:      rate.GetBuyRate(buyFloat),
			SellRate:     rate.GetSellRate(sellFloat),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": result,
		"buy_float":  buyFloat.String(),
		"sell_float": sellFloat.String(),
	})
}

// UpdateExchangeRate 更新汇率
func (h *RateHandler) UpdateExchangeRate(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Rate       string `json:"rate" binding:"required"`
		RateType   string `json:"rate_type"`
		AutoUpdate bool   `json:"auto_update"`
		Source     string `json:"source"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 验证汇率
	rate, err := decimal.NewFromString(req.Rate)
	if err != nil || rate.LessThanOrEqual(decimal.Zero) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "无效的汇率"})
		return
	}

	// 查询汇率
	var exchangeRate model.ExchangeRate
	if err := model.GetDB().First(&exchangeRate, id).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "汇率不存在"})
		return
	}

	// 计算变化百分比
	oldRate := exchangeRate.Rate
	var changePercent decimal.Decimal
	if !oldRate.IsZero() && !rate.Equal(oldRate) {
		changePercent = rate.Sub(oldRate).Div(oldRate).Mul(decimal.NewFromInt(100))
	}

	// 更新
	now := time.Now()
	updates := map[string]interface{}{
		"rate":         rate,
		"last_updated": &now,
		"updated_at":   now,
	}

	if req.RateType != "" {
		updates["rate_type"] = req.RateType
	}

	updates["auto_update"] = req.AutoUpdate

	if req.Source != "" {
		updates["source"] = req.Source
	}

	if err := model.GetDB().Model(&exchangeRate).Updates(updates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "更新失败: " + err.Error()})
		return
	}

	// 记录更新历史（仅当汇率值发生变化时）
	if !rate.Equal(oldRate) {
		history := model.ExchangeRateHistory{
			RateID:        exchangeRate.ID,
			FromCurrency:  exchangeRate.FromCurrency,
			ToCurrency:    exchangeRate.ToCurrency,
			OldRate:       oldRate,
			NewRate:       rate,
			ChangePercent: changePercent,
			UpdateSource:  "manual",
			UpdatedBy:     "admin",
			CreatedAt:     now,
		}
		if err := model.GetDB().Create(&history).Error; err != nil {
			// 历史记录失败不影响主流程
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功，但记录历史失败"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功"})
}

// RefreshAutoRates 刷新自动汇率
func (h *RateHandler) RefreshAutoRates(c *gin.Context) {
	var rates []model.ExchangeRate

	// 查询启用自动更新的汇率
	if err := model.GetDB().Where("auto_update = 1 AND rate_type = 'auto'").Find(&rates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "查询失败: " + err.Error()})
		return
	}

	if len(rates) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "没有需要更新的汇率", "count": 0})
		return
	}

	rateService := service.GetRateService()
	successCount := 0

	now := time.Now()
	for _, rate := range rates {
		var newRate decimal.Decimal
		var err error

		// 根据不同的汇率对获取最新汇率
		switch {
		case rate.FromCurrency == "CNY" && rate.ToCurrency == "USD":
			// CNY -> USD: 从 USDT/CNY 倒推
			newRate, err = rateService.FetchCNYToUSD()
		case rate.FromCurrency == "EUR" && rate.ToCurrency == "USD":
			// EUR -> USD
			newRate, err = rateService.FetchEURToUSD()
		case rate.FromCurrency == "USD" && rate.ToCurrency == "USDT":
			// USD -> USDT: 通常是 1.0，或从 USDT/USD 市场获取
			newRate = decimal.NewFromInt(1)
		case rate.FromCurrency == "USD" && rate.ToCurrency == "TRX":
			// USD -> TRX: 需要先获取 TRX/USDT 价格
			newRate, err = rateService.FetchUSDToTRX()
		case rate.FromCurrency == "USD" && rate.ToCurrency == "CNY":
			// USD -> CNY
			newRate, err = rateService.FetchUSDToCNY()
		default:
			continue
		}

		if err != nil {
			continue
		}

		// 更新数据库
		if err := model.GetDB().Model(&rate).Updates(map[string]interface{}{
			"rate":         newRate,
			"last_updated": &now,
			"updated_at":   now,
		}).Error; err == nil {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"msg":   "汇率刷新完成",
		"count": successCount,
		"total": len(rates),
	})
}

// GetFloatSettings 获取浮动设置
func (h *RateHandler) GetFloatSettings(c *gin.Context) {
	buyFloat := service.GetRateService().GetConfigValue(model.ConfigKeyRateBuyFloat, "0.02")
	sellFloat := service.GetRateService().GetConfigValue(model.ConfigKeyRateSellFloat, "0.02")
	autoUpdate := service.GetRateService().GetConfigValue(model.ConfigKeyRateAutoUpdate, "1")

	c.JSON(http.StatusOK, gin.H{
		"code":        1,
		"buy_float":   buyFloat,
		"sell_float":  sellFloat,
		"auto_update": autoUpdate == "1",
	})
}

// UpdateFloatSettings 更新浮动设置
func (h *RateHandler) UpdateFloatSettings(c *gin.Context) {
	var req struct {
		BuyFloat   string `json:"buy_float" binding:"required"`
		SellFloat  string `json:"sell_float" binding:"required"`
		AutoUpdate *bool  `json:"auto_update"` // 可选参数
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "参数错误"})
		return
	}

	// 验证浮动值
	buyFloat, err := decimal.NewFromString(req.BuyFloat)
	if err != nil || buyFloat.LessThan(decimal.Zero) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "无效的买入浮动"})
		return
	}

	sellFloat, err := decimal.NewFromString(req.SellFloat)
	if err != nil || sellFloat.LessThan(decimal.Zero) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "无效的卖出浮动"})
		return
	}

	// 更新配置
	db := model.GetDB()

	if err := db.Model(&model.SystemConfig{}).Where("`key` = ?", model.ConfigKeyRateBuyFloat).
		Updates(map[string]interface{}{
			"value":      req.BuyFloat,
			"updated_at": time.Now(),
		}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "更新买入浮动失败"})
		return
	}

	if err := db.Model(&model.SystemConfig{}).Where("`key` = ?", model.ConfigKeyRateSellFloat).
		Updates(map[string]interface{}{
			"value":      req.SellFloat,
			"updated_at": time.Now(),
		}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "更新卖出浮动失败"})
		return
	}

	// 更新自动更新开关
	if req.AutoUpdate != nil {
		autoUpdateValue := "0"
		if *req.AutoUpdate {
			autoUpdateValue = "1"
		}
		if err := db.Model(&model.SystemConfig{}).Where("`key` = ?", model.ConfigKeyRateAutoUpdate).
			Updates(map[string]interface{}{
				"value":      autoUpdateValue,
				"updated_at": time.Now(),
			}).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "更新自动更新开关失败"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功"})
}
