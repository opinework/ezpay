package service

import (
	"log"
	"time"

	"ezpay/internal/model"

	"github.com/shopspring/decimal"
)

// RateUpdater 汇率自动更新器
type RateUpdater struct {
	rateService *RateService
	ticker      *time.Ticker
	stopChan    chan struct{}
}

// NewRateUpdater 创建汇率自动更新器
func NewRateUpdater() *RateUpdater {
	return &RateUpdater{
		rateService: GetRateService(),
		stopChan:    make(chan struct{}),
	}
}

// Start 启动汇率自动更新（每小时执行一次）
func (u *RateUpdater) Start() {
	log.Println("汇率自动更新服务启动，每小时更新一次")

	// 启动时立即执行一次
	go u.updateRates()

	// 每小时执行一次
	u.ticker = time.NewTicker(1 * time.Hour)

	go func() {
		for {
			select {
			case <-u.ticker.C:
				u.updateRates()
			case <-u.stopChan:
				log.Println("汇率自动更新服务停止")
				return
			}
		}
	}()
}

// Stop 停止汇率自动更新
func (u *RateUpdater) Stop() {
	if u.ticker != nil {
		u.ticker.Stop()
	}
	close(u.stopChan)
}

// updateRates 更新汇率
func (u *RateUpdater) updateRates() {
	log.Println("开始自动更新汇率...")

	var rates []model.ExchangeRate

	// 查询启用自动更新的汇率
	if err := model.GetDB().Where("auto_update = 1 AND rate_type = 'auto'").Find(&rates).Error; err != nil {
		log.Printf("查询自动更新汇率失败: %v", err)
		return
	}

	if len(rates) == 0 {
		log.Println("没有需要自动更新的汇率")
		return
	}

	successCount := 0
	failCount := 0
	now := time.Now()

	for _, rate := range rates {
		var newRate decimal.Decimal
		var err error

		// 根据不同的汇率对获取最新汇率
		switch {
		case rate.FromCurrency == "CNY" && rate.ToCurrency == "USD":
			// CNY -> USD
			newRate, err = u.rateService.FetchCNYToUSD()

		case rate.FromCurrency == "EUR" && rate.ToCurrency == "USD":
			// EUR -> USD
			newRate, err = u.rateService.FetchEURToUSD()

		case rate.FromCurrency == "USD" && rate.ToCurrency == "USDT":
			// USD -> USDT: 通常是 1.0
			newRate = decimal.NewFromInt(1)

		case rate.FromCurrency == "USD" && rate.ToCurrency == "TRX":
			// USD -> TRX
			newRate, err = u.rateService.FetchUSDToTRX()

		case rate.FromCurrency == "USD" && rate.ToCurrency == "CNY":
			// USD -> CNY
			newRate, err = u.rateService.FetchUSDToCNY()

		default:
			log.Printf("跳过未支持的汇率对: %s -> %s", rate.FromCurrency, rate.ToCurrency)
			continue
		}

		if err != nil {
			log.Printf("获取汇率失败 %s -> %s: %v", rate.FromCurrency, rate.ToCurrency, err)
			failCount++
			continue
		}

		// 计算变化百分比
		oldRate := rate.Rate
		var changePercent decimal.Decimal
		if !oldRate.IsZero() {
			changePercent = newRate.Sub(oldRate).Div(oldRate).Mul(decimal.NewFromInt(100))
		}

		// 更新数据库
		if err := model.GetDB().Model(&rate).Updates(map[string]interface{}{
			"rate":         newRate,
			"last_updated": &now,
			"updated_at":   now,
		}).Error; err != nil {
			log.Printf("更新汇率失败 %s -> %s: %v", rate.FromCurrency, rate.ToCurrency, err)
			failCount++
		} else {
			// 记录更新历史
			history := model.ExchangeRateHistory{
				RateID:        rate.ID,
				FromCurrency:  rate.FromCurrency,
				ToCurrency:    rate.ToCurrency,
				OldRate:       oldRate,
				NewRate:       newRate,
				ChangePercent: changePercent,
				UpdateSource:  "auto",
				UpdatedBy:     "system",
				CreatedAt:     now,
			}
			if err := model.GetDB().Create(&history).Error; err != nil {
				log.Printf("记录汇率历史失败: %v", err)
			}

			log.Printf("汇率更新成功: %s -> %s = %s (变化: %s%%)", rate.FromCurrency, rate.ToCurrency, newRate.String(), changePercent.StringFixed(2))
			successCount++
		}
	}

	log.Printf("汇率自动更新完成: 成功 %d, 失败 %d, 总计 %d", successCount, failCount, len(rates))
}
