package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ezpay/internal/model"

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
	mode := s.getConfigValue(model.ConfigKeyRateMode, "hybrid")

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
	rateStr := s.getConfigValue(model.ConfigKeyManualRate, "7.2")
	rate, err := decimal.NewFromString(rateStr)
	if err != nil {
		return decimal.NewFromFloat(7.2), nil
	}

	// 应用浮动百分比
	floatStr := s.getConfigValue(model.ConfigKeyFloatPercent, "0")
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
	floatStr := s.getConfigValue(model.ConfigKeyFloatPercent, "0")
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
	// Binance C2C API (非官方，可能需要调整)
	// 这里使用USDT/BUSD价格 * BUSD/CNY估算
	url := "https://api.binance.com/api/v3/ticker/price?symbol=USDTBUSD"

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

	// 假设 USDT ≈ 1 USD, USD/CNY ≈ 7.2
	// 实际应该从其他API获取实时USD/CNY汇率
	usdCny := decimal.NewFromFloat(7.2) // 可以配置或从其他API获取

	price, err := decimal.NewFromString(result.Price)
	if err != nil {
		return decimal.Zero, err
	}

	return price.Mul(usdCny), nil
}

// fetchOKXRate 从OKX获取USDT/CNY汇率
func (s *RateService) fetchOKXRate() (decimal.Decimal, error) {
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
	floatStr := s.getConfigValue(model.ConfigKeyFloatPercent, "0")
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

// getConfigValue 获取系统配置值
func (s *RateService) getConfigValue(key, defaultValue string) string {
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

// GenerateUniqueAmount 生成唯一金额（通过添加随机小数位避免金额冲突）
func (s *RateService) GenerateUniqueAmount(baseAmount decimal.Decimal, chain string) decimal.Decimal {
	// 查找该链上待支付订单的金额
	var existingAmounts []string
	model.GetDB().Model(&model.Order{}).
		Where("chain = ? AND status = ?", chain, model.OrderStatusPending).
		Pluck("usdt_amount", &existingAmounts)

	existingMap := make(map[string]bool)
	for _, amt := range existingAmounts {
		existingMap[amt] = true
	}

	// 尝试不同的小数位组合
	for i := 0; i < 100; i++ {
		// 添加随机小数: 0.000001 到 0.000999
		offset := decimal.NewFromFloat(float64(i+1) * 0.000001)
		newAmount := baseAmount.Add(offset).Round(6)
		amountStr := newAmount.String()

		if !existingMap[amountStr] {
			return newAmount
		}
	}

	// 如果都冲突，使用时间戳生成
	ts := time.Now().UnixNano() % 1000000
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

// ConvertToPayCurrency 将原始货币转换为支付货币
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
		trxRate, err := s.getTRXUSDRate()
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

// getTRXUSDRate 获取 TRX/USD 价格
func (s *RateService) getTRXUSDRate() (decimal.Decimal, error) {
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
