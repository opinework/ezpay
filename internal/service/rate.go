package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ezpay/config"
	"ezpay/internal/model"
	"ezpay/internal/util"

	"github.com/shopspring/decimal"
)

// RateService 汇率服务
type RateService struct {
	mu             sync.RWMutex
	cachedRate     decimal.Decimal
	cachedTRXRate  decimal.Decimal
	lastUpdate     time.Time
	lastTRXUpdate  time.Time
	cacheSeconds   int
}

var rateService *RateService
var rateOnce sync.Once

// GetRateService 获取汇率服务单例
func GetRateService() *RateService {
	rateOnce.Do(func() {
		rateService = &RateService{
			cacheSeconds: 300, // 默认5分钟缓存
		}
	})
	return rateService
}

// GetRate 获取当前汇率 (CNY/USDT)
func (s *RateService) GetRate() (decimal.Decimal, error) {
	// 获取汇率模式配置
	mode := s.GetConfigValue(model.ConfigKeyRateMode, "hybrid")

	switch mode {
	case "manual":
		return s.getManualRate()
	case "auto":
		return s.getAutoRate()
	case "hybrid":
		// 混合模式：优先自动获取，失败则使用手动
		rate, err := s.getAutoRate()
		if err != nil {
			return s.getManualRate()
		}
		return rate, nil
	default:
		return s.getManualRate()
	}
}

// getManualRate 获取手动设置的汇率
func (s *RateService) getManualRate() (decimal.Decimal, error) {
	rateStr := s.GetConfigValue(model.ConfigKeyManualRate, "7.2")
	rate, err := decimal.NewFromString(rateStr)
	if err != nil {
		return decimal.NewFromFloat(7.2), nil
	}

	// 应用浮动百分比
	floatStr := s.GetConfigValue(model.ConfigKeyFloatPercent, "0")
	floatPercent, _ := decimal.NewFromString(floatStr)
	if !floatPercent.IsZero() {
		adjustment := rate.Mul(floatPercent).Div(decimal.NewFromInt(100))
		rate = rate.Add(adjustment)
	}

	return rate, nil
}

// getAutoRate 自动获取汇率
func (s *RateService) getAutoRate() (decimal.Decimal, error) {
	s.mu.RLock()
	if !s.cachedRate.IsZero() && time.Since(s.lastUpdate) < time.Duration(s.cacheSeconds)*time.Second {
		rate := s.cachedRate
		s.mu.RUnlock()
		return rate, nil
	}
	s.mu.RUnlock()

	// 从Binance获取汇率
	rate, err := s.fetchBinanceRate()
	if err != nil {
		// 尝试OKX
		rate, err = s.fetchOKXRate()
		if err != nil {
			// 返回缓存的旧值
			s.mu.RLock()
			if !s.cachedRate.IsZero() {
				rate := s.cachedRate
				s.mu.RUnlock()
				return rate, nil
			}
			s.mu.RUnlock()
			return decimal.Zero, err
		}
	}

	// 应用浮动百分比
	floatStr := s.GetConfigValue(model.ConfigKeyFloatPercent, "0")
	floatPercent, _ := decimal.NewFromString(floatStr)
	if !floatPercent.IsZero() {
		adjustment := rate.Mul(floatPercent).Div(decimal.NewFromInt(100))
		rate = rate.Add(adjustment)
	}

	// 更新缓存
	s.mu.Lock()
	s.cachedRate = rate
	s.lastUpdate = time.Now()
	s.mu.Unlock()

	return rate, nil
}

// fetchBinanceRate 从Binance获取USDT/CNY汇率
func (s *RateService) fetchBinanceRate() (decimal.Decimal, error) {
	// API限流：Binance API限制为每分钟1200次请求权重，这里设置为每秒10次
	limiter := util.GetAPILimiter("binance", 10.0, 20)
	limiter.Wait()

	// 从配置中获取 CNY 汇率 API 地址
	cnyAPI := config.Get().Rate.CnyAPI
	if cnyAPI == "" {
		cnyAPI = "https://api.exchangerate-api.com/v4/latest/USD" // 默认使用免费外汇API
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(cnyAPI)
	if err != nil {
		return decimal.Zero, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Zero, err
	}

	var result struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return decimal.Zero, err
	}

	// 获取 USD/CNY 汇率
	cnyRate, ok := result.Rates["CNY"]
	if !ok {
		return decimal.Zero, fmt.Errorf("CNY rate not found")
	}

	return decimal.NewFromFloat(cnyRate), nil
}

// fetchOKXRate 从OKX获取USDT/CNY汇率
func (s *RateService) fetchOKXRate() (decimal.Decimal, error) {
	// API限流：OKX限制为每秒20次请求
	limiter := util.GetAPILimiter("okx", 10.0, 20)
	limiter.Wait()

	url := "https://www.okx.com/api/v5/market/ticker?instId=USDT-USDC"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return decimal.Zero, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Zero, err
	}

	var result struct {
		Data []struct {
			Last string `json:"last"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return decimal.Zero, err
	}

	if len(result.Data) == 0 {
		return decimal.Zero, fmt.Errorf("no data from OKX")
	}

	// 假设 USDT ≈ 1 USD, USD/CNY ≈ 7.2
	usdCny := decimal.NewFromFloat(7.2)

	price, err := decimal.NewFromString(result.Data[0].Last)
	if err != nil {
		return decimal.Zero, err
	}

	return price.Mul(usdCny), nil
}

// ConvertCNYToUSDT 将CNY转换为USDT
func (s *RateService) ConvertCNYToUSDT(cny decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	rate, err := s.GetRate()
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	if rate.IsZero() {
		return decimal.Zero, decimal.Zero, fmt.Errorf("rate is zero")
	}

	usdt := cny.Div(rate).Round(6)
	return usdt, rate, nil
}

// GetTRXRate 获取TRX/CNY汇率
func (s *RateService) GetTRXRate() (decimal.Decimal, error) {
	s.mu.RLock()
	if !s.cachedTRXRate.IsZero() && time.Since(s.lastTRXUpdate) < time.Duration(s.cacheSeconds)*time.Second {
		rate := s.cachedTRXRate
		s.mu.RUnlock()
		return rate, nil
	}
	s.mu.RUnlock()

	// 从Binance获取TRX价格
	rate, err := s.fetchTRXRate()
	if err != nil {
		// 返回缓存的旧值
		s.mu.RLock()
		if !s.cachedTRXRate.IsZero() {
			rate := s.cachedTRXRate
			s.mu.RUnlock()
			return rate, nil
		}
		s.mu.RUnlock()
		return decimal.Zero, err
	}

	// 应用浮动百分比
	floatStr := s.GetConfigValue(model.ConfigKeyFloatPercent, "0")
	floatPercent, _ := decimal.NewFromString(floatStr)
	if !floatPercent.IsZero() {
		adjustment := rate.Mul(floatPercent).Div(decimal.NewFromInt(100))
		rate = rate.Add(adjustment)
	}

	// 更新缓存
	s.mu.Lock()
	s.cachedTRXRate = rate
	s.lastTRXUpdate = time.Now()
	s.mu.Unlock()

	return rate, nil
}

// fetchTRXRate 从Binance获取TRX/USDT价格并转换为CNY
func (s *RateService) fetchTRXRate() (decimal.Decimal, error) {
	url := "https://api.binance.com/api/v3/ticker/price?symbol=TRXUSDT"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return decimal.Zero, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Zero, err
	}

	var result struct {
		Price string `json:"price"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return decimal.Zero, err
	}

	// TRX/USDT 价格
	trxUsdt, err := decimal.NewFromString(result.Price)
	if err != nil {
		return decimal.Zero, err
	}

	// 获取USDT/CNY汇率
	usdtCny, err := s.GetRate()
	if err != nil {
		// 使用默认值
		usdtCny = decimal.NewFromFloat(7.2)
	}

	// TRX/CNY = TRX/USDT * USDT/CNY
	return trxUsdt.Mul(usdtCny), nil
}

// ConvertCNYToTRX 将CNY转换为TRX
func (s *RateService) ConvertCNYToTRX(cny decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	rate, err := s.GetTRXRate()
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	if rate.IsZero() {
		return decimal.Zero, decimal.Zero, fmt.Errorf("TRX rate is zero")
	}

	trx := cny.Div(rate).Round(6)
	return trx, rate, nil
}

// ConvertUSDTToCNY 将USDT转换为CNY
func (s *RateService) ConvertUSDTToCNY(usdt decimal.Decimal) (decimal.Decimal, error) {
	rate, err := s.GetRate()
	if err != nil {
		return decimal.Zero, err
	}

	return usdt.Mul(rate).Round(2), nil
}

// GetConfigValue 获取系统配置值（公开方法）
func (s *RateService) GetConfigValue(key, defaultValue string) string {
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", key).First(&config).Error; err != nil {
		return defaultValue
	}
	if config.Value == "" {
		return defaultValue
	}
	return config.Value
}

// SetCacheSeconds 设置缓存时间
func (s *RateService) SetCacheSeconds(seconds int) {
	s.cacheSeconds = seconds
}

// ClearCache 清除缓存
func (s *RateService) ClearCache() {
	s.mu.Lock()
	s.cachedRate = decimal.Zero
	s.lastUpdate = time.Time{}
	s.mu.Unlock()
}

// GetCachedRate 获取缓存的汇率信息
func (s *RateService) GetCachedRate() (decimal.Decimal, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedRate, s.lastUpdate
}

// GenerateUniqueAmount 生成唯一标识金额（通过添加偏移避免金额冲突）
// 加密货币(USDT/TRX): 使用 0.000001-0.000999 随机偏移（6位小数）
// 法币(CNY): 使用 0.01 递增偏移（2位小数）
func (s *RateService) GenerateUniqueAmount(baseAmount decimal.Decimal, chain string) decimal.Decimal {
	// 判断是否为法币
	isFiat := chain == "wechat" || chain == "alipay"

	// 查找该链上待支付订单的唯一金额
	var existingAmounts []string
	model.GetDB().Model(&model.Order{}).
		Where("chain = ? AND status = ?", chain, model.OrderStatusPending).
		Pluck("unique_amount", &existingAmounts)

	existingMap := make(map[string]bool)
	for _, amt := range existingAmounts {
		// 转换为 decimal 后再转回字符串，统一格式（去除尾部0）
		if d, err := decimal.NewFromString(amt); err == nil {
			existingMap[d.String()] = true
		}
		// 同时保留原始格式以兼容
		existingMap[amt] = true
	}

	if isFiat {
		// 法币: 使用 0.01 递增偏移（微信/支付宝只支持到分）
		// 如: 10.41 → 10.41, 10.42, 10.43 ... (先尝试原始金额，冲突则递增)
		for i := 0; i < 1000; i++ {
			offset := decimal.NewFromFloat(float64(i) * 0.01)
			newAmount := baseAmount.Add(offset).Round(2)
			amountStr := newAmount.String()

			if !existingMap[amountStr] {
				return newAmount
			}
		}

		// 如果都冲突（超过1000个订单），使用时间戳生成（带去重检查）
		for retry := 0; retry < 100; retry++ {
			ts := time.Now().UnixNano() % 10000
			offset := decimal.NewFromFloat(float64(ts) * 0.01)
			newAmount := baseAmount.Add(offset).Round(2)
			amountStr := newAmount.String()

			if !existingMap[amountStr] {
				return newAmount
			}
			time.Sleep(time.Microsecond) // 避免时间戳重复
		}

		// 最终兜底：使用更大的偏移范围
		ts := time.Now().UnixNano() % 100000
		offset := decimal.NewFromFloat(float64(ts) * 0.01)
		return baseAmount.Add(offset).Round(2)
	}

	// 加密货币: 使用 0.000001 递增偏移
	// 如: 102.04 → 102.04, 102.040001, 102.040002 ... (先尝试原始金额，冲突则递增)
	for i := 0; i < 1000; i++ {
		// 生成递增 6 位小数偏移
		offset := decimal.NewFromFloat(float64(i) * 0.000001)
		newAmount := baseAmount.Add(offset).Round(6)
		amountStr := newAmount.String()

		if !existingMap[amountStr] {
			return newAmount
		}
	}

	// 如果都冲突，使用时间戳生成（带去重检查）
	for retry := 0; retry < 100; retry++ {
		ts := time.Now().UnixNano() % 1000000
		offset := decimal.NewFromFloat(float64(ts) * 0.000001)
		newAmount := baseAmount.Add(offset).Round(6)
		amountStr := newAmount.String()

		if !existingMap[amountStr] {
			return newAmount
		}
		time.Sleep(time.Microsecond) // 避免时间戳重复
	}

	// 最终兜底：使用更大的偏移范围
	ts := time.Now().UnixNano() % 10000000
	offset := decimal.NewFromFloat(float64(ts) * 0.000001)
	return baseAmount.Add(offset).Round(6)
}

// ParseDecimal 安全解析decimal
func ParseDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		f, _ := strconv.ParseFloat(s, 64)
		return decimal.NewFromFloat(f)
	}
	return d
}

// ConvertResult 货币转换结果
type ConvertResult struct {
	Amount      decimal.Decimal // 转换后金额
	Rate        decimal.Decimal // 使用的汇率
	PayCurrency string          // 支付货币
}

// RateType 汇率类型
type RateType string

const (
	RateTypeBuy  RateType = "buy"  // 买入汇率（用户支付时，平台收入）
	RateTypeSell RateType = "sell" // 卖出汇率（商户提现时，平台支出）
)

// GetRateWithType 根据类型获取汇率（买入/卖出）
// rateType: buy(买入) 或 sell(卖出)
// fromCurrency: 源货币 (CNY, USD, USDT, EUR等)
// toCurrency: 目标货币 (USDT, USD, CNY等)
func (s *RateService) GetRateWithType(rateType RateType, fromCurrency, toCurrency string) (decimal.Decimal, error) {
	// 获取基础汇率
	baseRate, err := s.getBaseRate(fromCurrency, toCurrency)
	if err != nil {
		return decimal.Zero, err
	}

	// 应用买入/卖出浮动
	var floatKey string
	if rateType == RateTypeBuy {
		floatKey = model.ConfigKeyRateBuyFloat // 买入浮动（用户支付，平台多收）
	} else {
		floatKey = model.ConfigKeyRateSellFloat // 卖出浮动（商户提现，平台少给）
	}

	floatStr := s.GetConfigValue(floatKey, "0")
	floatPercent, _ := decimal.NewFromString(floatStr)

	// 应用浮动
	if !floatPercent.IsZero() {
		var adjustment decimal.Decimal
		if rateType == RateTypeBuy {
			// 买入浮动: rate = baseRate * (1 + floatPercent)
			// 例如: EUR->USD baseRate=1.08, floatPercent=0.02 => rate=1.08*1.02=1.1016
			adjustment = decimal.NewFromInt(1).Add(floatPercent)
		} else {
			// 卖出浮动: rate = baseRate * (1 - floatPercent)
			// 例如: USD->USDT baseRate=1, floatPercent=0.02 => rate=1*0.98=0.98
			adjustment = decimal.NewFromInt(1).Sub(floatPercent)
		}
		baseRate = baseRate.Mul(adjustment)
	}

	return baseRate, nil
}

// getBaseRate 获取基础汇率（不含买卖浮动）
func (s *RateService) getBaseRate(fromCurrency, toCurrency string) (decimal.Decimal, error) {
	// 相同货币，汇率为1
	if fromCurrency == toCurrency {
		return decimal.NewFromInt(1), nil
	}

	// CNY <-> USDT
	if (fromCurrency == "CNY" && toCurrency == "USDT") || (fromCurrency == "USDT" && toCurrency == "CNY") {
		rate, err := s.GetRate() // 使用现有的 GetRate 方法（含模式选择）
		if err != nil {
			return decimal.Zero, err
		}
		if fromCurrency == "USDT" && toCurrency == "CNY" {
			// USDT -> CNY: 直接返回汇率
			return rate, nil
		}
		// CNY -> USDT: 返回倒数
		return decimal.NewFromInt(1).Div(rate), nil
	}

	// CNY <-> USD
	if (fromCurrency == "CNY" && toCurrency == "USD") || (fromCurrency == "USD" && toCurrency == "CNY") {
		// 先获取 CNY/USDT 汇率，USD ≈ USDT (1:1)
		rate, err := s.GetRate()
		if err != nil {
			return decimal.Zero, err
		}
		if fromCurrency == "USD" && toCurrency == "CNY" {
			return rate, nil
		}
		return decimal.NewFromInt(1).Div(rate), nil
	}

	// USD <-> USDT (通常 1:1，但可以配置)
	if (fromCurrency == "USD" && toCurrency == "USDT") || (fromCurrency == "USDT" && toCurrency == "USD") {
		return decimal.NewFromInt(1), nil
	}

	// EUR <-> USD
	if (fromCurrency == "EUR" && toCurrency == "USD") || (fromCurrency == "USD" && toCurrency == "EUR") {
		eurRate, err := s.getEURUSDRate()
		if err != nil {
			return decimal.Zero, err
		}
		if fromCurrency == "EUR" && toCurrency == "USD" {
			return eurRate, nil
		}
		return decimal.NewFromInt(1).Div(eurRate), nil
	}

	// TRX 相关
	if fromCurrency == "TRX" || toCurrency == "TRX" {
		// TRX/USDT 汇率
		trxRate, err := s.GetTRXRate()
		if err != nil {
			return decimal.Zero, err
		}

		if fromCurrency == "TRX" && toCurrency == "USDT" {
			return trxRate, nil
		}
		if fromCurrency == "USDT" && toCurrency == "TRX" {
			return decimal.NewFromInt(1).Div(trxRate), nil
		}

		// TRX <-> CNY: 通过 USDT 中转
		if (fromCurrency == "TRX" && toCurrency == "CNY") || (fromCurrency == "CNY" && toCurrency == "TRX") {
			cnyUsdtRate, err := s.GetRate()
			if err != nil {
				return decimal.Zero, err
			}
			if fromCurrency == "TRX" && toCurrency == "CNY" {
				// TRX -> USDT -> CNY
				return trxRate.Mul(cnyUsdtRate), nil
			}
			// CNY -> TRX: CNY -> USDT -> TRX
			usdtAmount := decimal.NewFromInt(1).Div(cnyUsdtRate)
			return usdtAmount.Div(trxRate), nil
		}
	}

	return decimal.Zero, fmt.Errorf("不支持的货币对: %s/%s", fromCurrency, toCurrency)
}

// ConvertToSettlementCurrency 将用户支付货币转换为内部结算货币（USD）
// 使用买入汇率（含浮动），用于订单创建时
// fromCurrency: 用户支付货币 (CNY, EUR, USDT 等)
// amount: 支付金额
func (s *RateService) ConvertToSettlementCurrency(fromCurrency string, amount decimal.Decimal) (*ConvertResult, error) {
	const settlementCurrency = "USD"

	// 如果已经是 USD，直接返回
	if fromCurrency == settlementCurrency {
		return &ConvertResult{
			Amount:      amount,
			Rate:        decimal.NewFromInt(1),
			PayCurrency: settlementCurrency,
		}, nil
	}

	// 使用买入汇率（用户支付，平台多收）
	rate, err := s.GetRateWithType(RateTypeBuy, fromCurrency, settlementCurrency)
	if err != nil {
		return nil, fmt.Errorf("获取买入汇率失败: %w", err)
	}

	// 转换金额
	var targetAmount decimal.Decimal
	if fromCurrency == "CNY" {
		// CNY -> USD: amount * rate (rate 是 1 CNY = X USD)
		targetAmount = amount.Mul(rate).Round(6)
	} else if fromCurrency == "EUR" {
		// EUR -> USD: amount * rate
		targetAmount = amount.Mul(rate).Round(6)
	} else if fromCurrency == "USDT" {
		// USDT -> USD: 约等于 1:1
		targetAmount = amount.Round(6)
		rate = decimal.NewFromInt(1)
	} else {
		return nil, fmt.Errorf("不支持的货币: %s", fromCurrency)
	}

	return &ConvertResult{
		Amount:      targetAmount,
		Rate:        rate,
		PayCurrency: settlementCurrency,
	}, nil
}

// ConvertFromSettlementCurrency 将内部结算货币（USD）转换为提现货币
// 使用卖出汇率（含浮动），用于商户提现时
// usdAmount: USD 金额
// targetCurrency: 目标货币 (USDT, TRX 等)
func (s *RateService) ConvertFromSettlementCurrency(usdAmount decimal.Decimal, targetCurrency string) (*ConvertResult, error) {
	const settlementCurrency = "USD"

	// 如果目标货币是 USD，直接返回
	if targetCurrency == settlementCurrency {
		return &ConvertResult{
			Amount:      usdAmount,
			Rate:        decimal.NewFromInt(1),
			PayCurrency: settlementCurrency,
		}, nil
	}

	// 使用卖出汇率（商户提现，平台少给）
	rate, err := s.GetRateWithType(RateTypeSell, settlementCurrency, targetCurrency)
	if err != nil {
		return nil, fmt.Errorf("获取卖出汇率失败: %w", err)
	}

	// 转换金额
	var targetAmount decimal.Decimal
	if targetCurrency == "USDT" {
		// USD -> USDT: amount * rate (通常 1:1，但可配置卖出浮动)
		targetAmount = usdAmount.Mul(rate).Round(6)
	} else if targetCurrency == "TRX" {
		// USD -> TRX: 先获取 TRX/USD 价格
		trxUsdRate, err := s.GetTRXUSDRate()
		if err != nil {
			return nil, fmt.Errorf("获取TRX价格失败: %w", err)
		}
		// 应用卖出浮动
		trxUsdRate = trxUsdRate.Mul(decimal.NewFromInt(1).Add(rate.Sub(decimal.NewFromInt(1))))
		targetAmount = usdAmount.Div(trxUsdRate).Round(6)
	} else if targetCurrency == "CNY" {
		// USD -> CNY: amount * rate
		targetAmount = usdAmount.Mul(rate).Round(2)
	} else {
		return nil, fmt.Errorf("不支持的提现货币: %s", targetCurrency)
	}

	return &ConvertResult{
		Amount:      targetAmount,
		Rate:        rate,
		PayCurrency: targetCurrency,
	}, nil
}

// ConvertToPayCurrency 将原始货币转换为支付货币（保留用于向后兼容）
// ⚠️ 已废弃：建议使用 ConvertToSettlementCurrency
// fromCurrency: 原始货币 (CNY, USD, USDT, EUR 等)
// amount: 原始金额
// chain: 支付链 (trc20, erc20, bep20, trx, wechat, alipay 等)
func (s *RateService) ConvertToPayCurrency(fromCurrency string, amount decimal.Decimal, chain string) (*ConvertResult, error) {
	// 确定目标支付货币
	payCurrency := s.getPayCurrencyByChain(chain)

	// 如果源货币和目标货币相同，直接返回
	if fromCurrency == payCurrency {
		return &ConvertResult{
			Amount:      amount,
			Rate:        decimal.NewFromInt(1),
			PayCurrency: payCurrency,
		}, nil
	}

	// 获取汇率并转换
	var targetAmount, rate decimal.Decimal
	var err error

	switch payCurrency {
	case "USDT":
		targetAmount, rate, err = s.convertToUSDT(fromCurrency, amount)
	case "TRX":
		targetAmount, rate, err = s.convertToTRX(fromCurrency, amount)
	case "CNY":
		targetAmount, rate, err = s.convertToCNY(fromCurrency, amount)
	default:
		return nil, fmt.Errorf("不支持的支付货币: %s", payCurrency)
	}

	if err != nil {
		return nil, err
	}

	return &ConvertResult{
		Amount:      targetAmount,
		Rate:        rate,
		PayCurrency: payCurrency,
	}, nil
}

// getPayCurrencyByChain 根据链获取支付货币
func (s *RateService) getPayCurrencyByChain(chain string) string {
	switch chain {
	case "trc20", "erc20", "bep20", "polygon", "arbitrum", "optimism", "base", "avalanche":
		return "USDT"
	case "trx":
		return "TRX"
	case "wechat", "alipay":
		return "CNY"
	default:
		return "USDT"
	}
}

// convertToUSDT 将任意货币转换为 USDT
func (s *RateService) convertToUSDT(fromCurrency string, amount decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	switch fromCurrency {
	case "USDT":
		return amount, decimal.NewFromInt(1), nil
	case "USD":
		// USD ≈ USDT (1:1)
		return amount, decimal.NewFromInt(1), nil
	case "CNY":
		return s.ConvertCNYToUSDT(amount)
	default:
		// 其他货币先转 USD，再转 USDT
		usdAmount, rate, err := s.convertToUSD(fromCurrency, amount)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		// USD ≈ USDT
		return usdAmount, rate, nil
	}
}

// convertToTRX 将任意货币转换为 TRX
func (s *RateService) convertToTRX(fromCurrency string, amount decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	switch fromCurrency {
	case "TRX":
		return amount, decimal.NewFromInt(1), nil
	case "CNY":
		return s.ConvertCNYToTRX(amount)
	case "USD", "USDT":
		// 先获取 TRX/USD 价格
		trxRate, err := s.GetTRXUSDRate()
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		// amount USD / trxRate = TRX 数量
		trxAmount := amount.Div(trxRate).Round(6)
		return trxAmount, trxRate, nil
	default:
		// 其他货币先转 USD
		usdAmount, _, err := s.convertToUSD(fromCurrency, amount)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		// 再转 TRX
		return s.convertToTRX("USD", usdAmount)
	}
}

// convertToCNY 将任意货币转换为 CNY
func (s *RateService) convertToCNY(fromCurrency string, amount decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	switch fromCurrency {
	case "CNY":
		return amount, decimal.NewFromInt(1), nil
	case "USD", "USDT":
		rate, err := s.GetUSDCNYRate()
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		cnyAmount := amount.Mul(rate).Round(2)
		return cnyAmount, rate, nil
	default:
		// 其他货币先转 USD
		usdAmount, _, err := s.convertToUSD(fromCurrency, amount)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		// 再转 CNY
		return s.convertToCNY("USD", usdAmount)
	}
}

// convertToUSD 将其他货币转换为 USD
func (s *RateService) convertToUSD(fromCurrency string, amount decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	switch fromCurrency {
	case "USD", "USDT":
		return amount, decimal.NewFromInt(1), nil
	case "CNY":
		rate, err := s.GetUSDCNYRate()
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		usdAmount := amount.Div(rate).Round(6)
		return usdAmount, rate, nil
	case "EUR":
		rate, err := s.getEURUSDRate()
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		usdAmount := amount.Mul(rate).Round(6)
		return usdAmount, rate, nil
	default:
		return decimal.Zero, decimal.Zero, fmt.Errorf("不支持的货币: %s", fromCurrency)
	}
}

// GetUSDCNYRate 获取 USD/CNY 汇率
func (s *RateService) GetUSDCNYRate() (decimal.Decimal, error) {
	// 优先使用 USDT/CNY 汇率 (因为 USDT ≈ USD)
	return s.GetRate()
}

// GetTRXUSDRate 获取 TRX/USD 价格（公开方法）
func (s *RateService) GetTRXUSDRate() (decimal.Decimal, error) {
	url := "https://api.binance.com/api/v3/ticker/price?symbol=TRXUSDT"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return decimal.Zero, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Zero, err
	}

	var result struct {
		Price string `json:"price"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return decimal.Zero, err
	}

	return decimal.NewFromString(result.Price)
}

// getEURUSDRate 获取 EUR/USD 汇率
func (s *RateService) getEURUSDRate() (decimal.Decimal, error) {
	// 使用固定汇率或从API获取
	// 这里简化处理，使用近似值
	return decimal.NewFromFloat(1.08), nil
}

// GetSupportedCurrencies 获取支持的货币列表
func (s *RateService) GetSupportedCurrencies() []string {
	return []string{"CNY", "USD", "USDT", "EUR"}
}

// NormalizeCurrency 标准化货币代码
func NormalizeCurrency(currency string) string {
	switch currency {
	case "cny", "CNY", "rmb", "RMB":
		return "CNY"
	case "usd", "USD":
		return "USD"
	case "usdt", "USDT":
		return "USDT"
	case "eur", "EUR":
		return "EUR"
	case "trx", "TRX":
		return "TRX"
	default:
		return "CNY" // 默认 CNY
	}
}

// ========== 新汇率管理系统方法 ==========

// FetchCNYToUSD 获取 CNY -> USD 汇率
func (s *RateService) FetchCNYToUSD() (decimal.Decimal, error) {
	// 从 USDT/CNY 市场倒推
	usdtCny, err := s.fetchBinanceRate()
	if err != nil {
		return decimal.Zero, err
	}

	// CNY -> USD = 1 / (USDT/CNY) ≈ 1 / 7.2 = 0.14
	return decimal.NewFromInt(1).Div(usdtCny), nil
}

// FetchEURToUSD 获取 EUR -> USD 汇率
func (s *RateService) FetchEURToUSD() (decimal.Decimal, error) {
	// 从 Binance 获取 EUR/USD
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.binance.com/api/v3/ticker/price?symbol=EURUSDT")
	if err != nil {
		return decimal.Zero, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Zero, err
	}

	var result struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return decimal.Zero, err
	}

	rate, err := decimal.NewFromString(result.Price)
	if err != nil {
		return decimal.Zero, err
	}

	// EUR/USDT ≈ EUR/USD (因为 USDT ≈ 1 USD)
	return rate, nil
}

// FetchUSDToTRX 获取 USD -> TRX 汇率
func (s *RateService) FetchUSDToTRX() (decimal.Decimal, error) {
	// 直接从 Binance 获取原始的 TRX/USDT 价格
	url := "https://api.binance.com/api/v3/ticker/price?symbol=TRXUSDT"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return decimal.Zero, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Zero, err
	}

	var result struct {
		Price string `json:"price"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return decimal.Zero, err
	}

	// 获取 TRX/USDT 价格（例如 0.2928 表示 1 TRX = 0.2928 USDT）
	trxUsdt, err := decimal.NewFromString(result.Price)
	if err != nil {
		return decimal.Zero, err
	}

	// USD -> TRX = 1 / (TRX/USD)，因为 USDT ≈ USD
	// 例如 1 TRX = 0.2928 USD，那么 1 USD = 1/0.2928 = 3.415 TRX
	if trxUsdt.IsZero() {
		return decimal.Zero, fmt.Errorf("TRX价格为0")
	}
	return decimal.NewFromInt(1).Div(trxUsdt), nil
}

// FetchUSDToCNY 获取 USD -> CNY 汇率
func (s *RateService) FetchUSDToCNY() (decimal.Decimal, error) {
	// 直接从 Binance 获取 USDT/CNY
	rate, err := s.fetchBinanceRate()
	if err != nil {
		return decimal.Zero, err
	}

	// USDT/CNY ≈ USD/CNY
	return rate, nil
}
