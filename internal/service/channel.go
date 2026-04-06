package service

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"ezpay/internal/model"

	"github.com/shopspring/decimal"
)

// channelHTTPClient 通道服务专用HTTP客户端
var channelHTTPClient = &http.Client{
	Timeout: 30 * time.Second, // 上游通道可能较慢，给30秒超时
}

// ChannelService 支付通道服务
type ChannelService struct {
	mu sync.RWMutex
}

// ChannelType 通道类型
type ChannelType string

const (
	ChannelTypeLocal ChannelType = "local" // 本地区块链监控
	ChannelTypeVmq   ChannelType = "vmq"   // V免签
	ChannelTypeEpay  ChannelType = "epay"  // 彩虹易支付
)

// ChannelConfig 通道配置
type ChannelConfig struct {
	Type      ChannelType `json:"type"`
	Name      string      `json:"name"`
	ApiURL    string      `json:"api_url"`
	Key       string      `json:"key"`
	PayID     string      `json:"pay_id"`     // V免签的payId前缀或Epay的pid
	NotifyKey string      `json:"notify_key"` // 回调验签密钥
	Enabled   bool        `json:"enabled"`
	Priority  int         `json:"priority"` // 优先级，数字越小优先级越高
}

// ChannelOrderResult 通道订单结果
type ChannelOrderResult struct {
	Success    bool            `json:"success"`
	OrderID    string          `json:"order_id"`    // 上游订单号
	PayURL     string          `json:"pay_url"`     // 支付地址
	QRCode     string          `json:"qr_code"`     // 二维码内容
	Amount     decimal.Decimal `json:"amount"`      // 实际金额
	ExpireTime time.Time       `json:"expire_time"` // 过期时间
	Error      string          `json:"error"`
}

var channelService *ChannelService
var channelOnce sync.Once

// GetChannelService 获取通道服务单例
func GetChannelService() *ChannelService {
	channelOnce.Do(func() {
		channelService = &ChannelService{}
	})
	return channelService
}

// GetChannelConfig 获取指定类型的通道配置
func (s *ChannelService) GetChannelConfig(channelType ChannelType) (*ChannelConfig, error) {
	var configs []model.SystemConfig
	model.GetDB().Where("`key` LIKE 'channel_%'").Find(&configs)

	configMap := make(map[string]string)
	for _, cfg := range configs {
		configMap[cfg.Key] = cfg.Value
	}

	prefix := fmt.Sprintf("channel_%s_", channelType)
	if configMap[prefix+"enabled"] != "1" {
		return nil, fmt.Errorf("channel %s is not enabled", channelType)
	}

	priority, _ := strconv.Atoi(configMap[prefix+"priority"])

	return &ChannelConfig{
		Type:      channelType,
		Name:      configMap[prefix+"name"],
		ApiURL:    configMap[prefix+"api_url"],
		Key:       configMap[prefix+"key"],
		PayID:     configMap[prefix+"pay_id"],
		NotifyKey: configMap[prefix+"notify_key"],
		Enabled:   true,
		Priority:  priority,
	}, nil
}

// GetEnabledChannels 获取所有启用的通道
func (s *ChannelService) GetEnabledChannels() []*ChannelConfig {
	var configs []model.SystemConfig
	model.GetDB().Where("`key` LIKE 'channel_%_enabled' AND `value` = '1'").Find(&configs)

	var channels []*ChannelConfig
	for _, cfg := range configs {
		// 解析通道类型 channel_vmq_enabled -> vmq
		var channelType ChannelType
		if len(cfg.Key) > 16 {
			channelType = ChannelType(cfg.Key[8 : len(cfg.Key)-8])
		}

		if channel, err := s.GetChannelConfig(channelType); err == nil {
			channels = append(channels, channel)
		}
	}

	return channels
}

// CreateVmqOrder 在V免签创建订单
func (s *ChannelService) CreateVmqOrder(cfg *ChannelConfig, order *model.Order, notifyURL string) (*ChannelOrderResult, error) {
	// 构建签名: MD5(payId + param + type + price + key)
	payType := s.mapChainToVmqType(order.Chain)
	price := order.Money.StringFixed(2)
	param := order.TradeNo // 使用本地订单号作为param

	signStr := order.OutTradeNo + param + payType + price + cfg.Key
	sign := md5Hash(signStr)

	// 构建请求URL
	params := url.Values{}
	params.Set("payId", order.OutTradeNo)
	params.Set("type", payType)
	params.Set("price", price)
	params.Set("sign", sign)
	params.Set("param", param)
	params.Set("notifyUrl", notifyURL)
	if order.ReturnURL != "" {
		params.Set("returnUrl", order.ReturnURL)
	}

	reqURL := cfg.ApiURL + "/createOrder?" + params.Encode()

	resp, err := channelHTTPClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("request vmq failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read vmq response failed: %w", err)
	}

	var result struct {
		Code        int    `json:"code"`
		Msg         string `json:"msg"`
		PayID       string `json:"payId"`
		OrderID     string `json:"orderId"`
		PayType     string `json:"payType"`
		Price       string `json:"price"`
		ReallyPrice string `json:"reallyPrice"`
		PayURL      string `json:"payUrl"`
		IsAuto      int    `json:"isAuto"`
		State       int    `json:"state"`
		TimeOut     string `json:"timeOut"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse vmq response failed: %w", err)
	}

	if result.Code != 1 {
		return &ChannelOrderResult{
			Success: false,
			Error:   result.Msg,
		}, nil
	}

	reallyPrice, _ := decimal.NewFromString(result.ReallyPrice)
	expireTime, _ := time.ParseInLocation("2006-01-02 15:04:05", result.TimeOut, time.Local)

	return &ChannelOrderResult{
		Success:    true,
		OrderID:    result.OrderID,
		PayURL:     cfg.ApiURL + result.PayURL,
		QRCode:     result.ReallyPrice, // V免签返回的是金额，用户需要支付此金额
		Amount:     reallyPrice,
		ExpireTime: expireTime,
	}, nil
}

// CreateEpayOrder 在彩虹易支付创建订单
func (s *ChannelService) CreateEpayOrder(cfg *ChannelConfig, order *model.Order, notifyURL string) (*ChannelOrderResult, error) {
	// 构建参数
	params := map[string]string{
		"pid":          cfg.PayID,
		"type":         order.Type,
		"out_trade_no": order.TradeNo, // 使用本地订单号
		"notify_url":   notifyURL,
		"name":         order.Name,
		"money":        order.Money.StringFixed(2),
	}

	if order.ReturnURL != "" {
		params["return_url"] = order.ReturnURL
	}

	// 生成签名
	sign := generateEpaySign(params, cfg.Key)
	params["sign"] = sign
	params["sign_type"] = "MD5"

	// 构建请求URL
	urlParams := url.Values{}
	for k, v := range params {
		urlParams.Set(k, v)
	}

	reqURL := cfg.ApiURL + "/mapi.php?" + urlParams.Encode()

	resp, err := channelHTTPClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("request epay failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read epay response failed: %w", err)
	}

	var result struct {
		Code       int    `json:"code"`
		Msg        string `json:"msg"`
		TradeNo    string `json:"trade_no"`
		OutTradeNo string `json:"out_trade_no"`
		Type       string `json:"type"`
		Money      string `json:"money"`
		USDTAmount string `json:"usdt_amount"`
		Rate       string `json:"rate"`
		Address    string `json:"address"`
		Chain      string `json:"chain"`
		QRCode     string `json:"qrcode"`
		ExpiredAt  string `json:"expired_at"`
		PayURL     string `json:"pay_url"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse epay response failed: %w", err)
	}

	if result.Code != 1 {
		return &ChannelOrderResult{
			Success: false,
			Error:   result.Msg,
		}, nil
	}

	usdtAmount, _ := decimal.NewFromString(result.USDTAmount)
	expireTime, _ := time.ParseInLocation("2006-01-02 15:04:05", result.ExpiredAt, time.Local)

	return &ChannelOrderResult{
		Success:    true,
		OrderID:    result.TradeNo,
		PayURL:     cfg.ApiURL + result.PayURL,
		QRCode:     result.QRCode,
		Amount:     usdtAmount,
		ExpireTime: expireTime,
	}, nil
}

// VerifyVmqNotify 验证V免签回调
func (s *ChannelService) VerifyVmqNotify(cfg *ChannelConfig, payID, param, payType, price, reallyPrice, sign string) bool {
	// V免签回调签名: MD5(payId + param + type + price + reallyPrice + key)
	signStr := payID + param + payType + price + reallyPrice + cfg.Key
	expectedSign := md5Hash(signStr)
	return sign == expectedSign
}

// VerifyEpayNotify 验证易支付回调
func (s *ChannelService) VerifyEpayNotify(cfg *ChannelConfig, params map[string]string, sign string) bool {
	expectedSign := generateEpaySign(params, cfg.Key)
	return sign == expectedSign
}

// HandleUpstreamNotify 处理上游通知
func (s *ChannelService) HandleUpstreamNotify(channelType ChannelType, localTradeNo string, upstreamOrderID string, amount decimal.Decimal) error {
	// 查找本地订单
	var order model.Order
	if err := model.GetDB().Where("trade_no = ?", localTradeNo).First(&order).Error; err != nil {
		return fmt.Errorf("order not found: %s", localTradeNo)
	}

	if order.Status != model.OrderStatusPending {
		log.Printf("Order %s already processed, status: %d", localTradeNo, order.Status)
		return nil
	}

	// 更新订单状态（使用乐观锁防止并发重复处理）
	now := time.Now()
	updates := map[string]interface{}{
		"status":           model.OrderStatusPaid,
		"actual_amount":    amount,
		"paid_at":          &now,
		"channel_order_id": upstreamOrderID,
	}

	result := model.GetDB().Model(&order).
		Where("status = ?", model.OrderStatusPending).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("update order failed: %w", result.Error)
	}

	// 如果没有更新任何行，说明订单已被其他进程处理
	if result.RowsAffected == 0 {
		log.Printf("Order %s already processed by another process", localTradeNo)
		return nil
	}

	// 重新加载订单数据
	model.GetDB().First(&order, order.ID)

	log.Printf("Order %s paid via upstream channel, amount: %s", localTradeNo, amount)

	// 触发下游通知
	go GetNotifyService().NotifyOrder(order.ID)

	// 触发 Telegram 通知
	go GetTelegramService().NotifyOrderPaid(&order)

	return nil
}

// mapChainToVmqType 将链类型映射到V免签支付类型
// V免签类型: 1=微信, 2=支付宝, 3=TRC20, 4=ERC20, 5=BEP20
// 对于不支持的链类型，默认映射到TRC20
func (s *ChannelService) mapChainToVmqType(chain string) string {
	switch chain {
	case "wechat":
		return "1"
	case "alipay":
		return "2"
	case "trc20", "trx":
		return "3"
	case "erc20":
		return "4"
	case "bep20":
		return "5"
	case "polygon", "optimism", "arbitrum", "base":
		return "3" // EVM兼容链默认使用TRC20类型(实际上V免签不支持这些链)
	default:
		return "3"
	}
}

// generateEpaySign 生成易支付签名
func generateEpaySign(params map[string]string, key string) string {
	// 按键名排序
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "sign" && k != "sign_type" && params[k] != "" {
			keys = append(keys, k)
		}
	}

	// 简单排序
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	// 拼接
	var signStr string
	for _, k := range keys {
		signStr += k + "=" + params[k] + "&"
	}
	if len(signStr) > 0 {
		signStr = signStr[:len(signStr)-1]
	}
	signStr += key

	return md5Hash(signStr)
}

// md5Hash 计算MD5哈希
func md5Hash(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
