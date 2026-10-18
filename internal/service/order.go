package service

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ezpay/internal/model"
	"ezpay/internal/util"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// orderCacheItem 订单缓存项
type orderCacheItem struct {
	order     *model.Order
	paid      bool
	timestamp time.Time
}

// OrderService 订单服务
type OrderService struct {
	cache    sync.Map // 订单状态缓存: tradeNo -> orderCacheItem
	cacheTTL time.Duration
}

var (
	orderService     *OrderService
	orderServiceOnce sync.Once
)

// isUSDPegged 判断货币是否锚定 USD (1:1 等价)
func isUSDPegged(currency string) bool {
	return currency == "USD" || currency == "USDT"
}

// getSiteURL 从系统配置读取站点 URL（去尾部斜杠），用于构建对外可访问的绝对路径
// 没配置时返回空字符串；调用者需要做兜底（比如返回相对路径）
func getSiteURL() string {
	var siteConfig model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", "site_url").First(&siteConfig).Error; err != nil {
		return ""
	}
	return strings.TrimRight(siteConfig.Value, "/")
}

// buildQRCodeURL 构建二维码图片的绝对 URL（v1.3.11 起 mapi qrcode 字段返回此值）
// 法币（微信/支付宝）：拼接 siteURL + 商户上传的图片相对路径
// 加密货币：返回 siteURL/qrcode?text=<address> 由 /qrcode 端点动态生成 PNG
// siteURL 为空时返回相对路径，仍可工作但需要前端在同域访问
func buildQRCodeURL(order *model.Order, isFiat bool, siteURL string) string {
	if isFiat {
		// 法币订单的 order.QRCode 是商户上传图片的相对路径，如 "/static/qrcode/abc.png"
		path := order.QRCode
		if path == "" {
			return ""
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return siteURL + path
	}
	// 加密货币：动态生成 QR
	addr := order.ToAddress
	if addr == "" {
		return ""
	}
	return siteURL + "/qrcode?text=" + url.QueryEscape(addr)
}

// GetOrderService 获取订单服务
func GetOrderService() *OrderService {
	orderServiceOnce.Do(func() {
		orderService = &OrderService{
			cacheTTL: 5 * time.Second,
		}
	})
	return orderService
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	MerchantPID string `json:"pid" form:"pid"`
	Type        string `json:"type" form:"type"`
	OutTradeNo  string `json:"out_trade_no" form:"out_trade_no"`
	NotifyURL   string `json:"notify_url" form:"notify_url"`
	ReturnURL   string `json:"return_url" form:"return_url"`
	Name        string `json:"name" form:"name"`
	Money       string `json:"money" form:"money"`
	Currency    string `json:"currency" form:"currency"` // 货币类型: CNY, USD, USDT 等，默认 USD
	Param       string `json:"param" form:"param"`
	ClientIP    string `json:"-"`
	Channel     string `json:"channel" form:"channel"` // 指定支付通道: local, vmq, epay (可选)
}

// CreateOrderResponse 创建订单响应
type CreateOrderResponse struct {
	Code          int    `json:"code"`
	Msg           string `json:"msg"`
	TradeNo       string `json:"trade_no,omitempty"`
	OutTradeNo    string `json:"out_trade_no,omitempty"`
	Type          string `json:"type,omitempty"`
	Currency      string `json:"currency,omitempty"`      // 原始货币
	Money         string `json:"money,omitempty"`         // 原始金额
	PayCurrency   string `json:"pay_currency,omitempty"`  // 支付货币
	PayAmount     string `json:"pay_amount,omitempty"`    // 用户应支付金额（展示用，无偏移）
	UniqueAmount  string `json:"unique_amount,omitempty"` // 订单标识金额（含偏移，实际支付）
	USDTAmount    string `json:"usdt_amount,omitempty"`   // 支付金额(兼容旧字段，等于 unique_amount)
	Rate          string `json:"rate,omitempty"`
	Address       string `json:"address,omitempty"`
	Chain         string `json:"chain,omitempty"`
	QRCode        string `json:"qrcode,omitempty"`
	ExpiredAt     string `json:"expired_at,omitempty"`
	PayURL        string `json:"pay_url,omitempty"`
	Channel       string `json:"channel,omitempty"`         // 实际使用的支付通道
	ChannelPayURL string `json:"channel_pay_url,omitempty"` // 上游支付链接 (如果使用通道)
}

// CreateOrder 创建订单
func (s *OrderService) CreateOrder(req *CreateOrderRequest) (*CreateOrderResponse, error) {
	// 验证商户
	var merchant model.Merchant
	if err := model.GetDB().Where("p_id = ? AND status = 1", req.MerchantPID).First(&merchant).Error; err != nil {
		return nil, errors.New("商户不存在或已禁用")
	}

	// 验证金额
	money, err := decimal.NewFromString(req.Money)
	if err != nil || money.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New("金额无效")
	}

	// 标准化货币类型：优先使用请求货币，其次商户默认货币，最后兜底 USD
	// 注意: 如果客户明确传了 currency 但是非法值（如 JPY），必须拒绝，避免静默把 10000 JPY 当 10000 USD 入账
	if req.Currency != "" {
		if NormalizeCurrency(req.Currency) == "" {
			return nil, fmt.Errorf("不支持的货币: %s (支持 CNY/USD/USDT/EUR/TRX)", req.Currency)
		}
	}
	currency := NormalizeCurrency(req.Currency)
	if currency == "" {
		currency = NormalizeCurrency(merchant.DefaultCurrency)
	}
	if currency == "" {
		currency = "USD"
	}

	// 标准化支付类型
	payType := util.NormalizePaymentType(req.Type)
	chain := util.GetPaymentTypeChain(req.Type)

	if !util.IsValidChain(chain) {
		return nil, errors.New("不支持的支付类型")
	}

	// 检查法币通道是否启用
	if util.IsFiatChain(chain) {
		chainStatus := GetBlockchainService().GetChainStatus()
		if enabled, ok := chainStatus[chain]; ok && !enabled {
			return nil, errors.New("该支付通道暂未开启")
		}
	}

	// 检查订单号是否重复
	var existingOrder model.Order
	err = model.GetDB().Where("merchant_id = ? AND out_trade_no = ?", merchant.ID, req.OutTradeNo).First(&existingOrder).Error
	if err == nil {
		// 订单已存在
		if existingOrder.Status == model.OrderStatusPending {
			// 返回已存在的待支付订单
			return s.buildOrderResponse(&existingOrder, &merchant)
		}
		return nil, errors.New("订单号已存在")
	}

	// 确定使用的通道
	channel := s.determineChannel(req.Channel, chain)

	// 获取订单过期时间
	expireMinutes := s.getOrderExpireMinutes()
	expiredAt := time.Now().Add(time.Duration(expireMinutes) * time.Minute)

	// 判断是否为法币收款方式(微信/支付宝)
	isFiat := util.IsFiatChain(chain)

	// 使用新的货币转换服务
	rateService := GetRateService()
	var payAmount, settlementAmount decimal.Decimal
	var payCurrency string

	// 1. 计算结算金额（USD）- 使用买入汇率，计入商户余额
	settlementResult, err := rateService.ConvertToSettlementCurrency(currency, money)
	if err != nil {
		return nil, errors.New("结算金额计算失败: " + err.Error())
	}
	settlementAmount = settlementResult.Amount
	// rate := settlementResult.Rate // 原始货币 -> USD 的买入汇率（已不需要）

	// 2. 计算支付金额 - 基于 USD 结算金额，使用买入汇率转换为实际收款币种
	// 确定目标支付货币
	if isFiat {
		payCurrency = "CNY" // 法币收款
	} else {
		// 加密货币收款：根据链确定币种
		switch chain {
		case "trx":
			payCurrency = "TRX"
		default:
			payCurrency = "USDT" // trc20, erc20, bep20, polygon, optimism, arbitrum, avalanche, base
		}
	}

	// 计算用户应付金额
	// 同币种 / 同稳定锚定 (USD↔USDT) 直通：用户付的就是商品标价，不应用买入浮动
	// 跨币种：从 USD 结算金额反算到支付货币，应用买入浮动让用户多付
	if currency == payCurrency {
		// 严格同币种：100 CNY 商品 + CNY 通道 → 用户付 100 CNY
		payAmount = money
	} else if isUSDPegged(currency) && isUSDPegged(payCurrency) {
		// USD ↔ USDT (1:1 锚定)：等额直通
		payAmount = money
	} else if payCurrency == "USD" {
		// USD 收款：直接等于 USD 结算金额
		payAmount = settlementAmount
	} else {
		// 跨货币：USD → 支付货币，应用买入浮动
		buyFloatStr := rateService.GetConfigValue(model.ConfigKeyRateBuyFloat, "0")
		buyFloat, _ := decimal.NewFromString(buyFloatStr)

		if payCurrency == "USDT" {
			// USD -> USDT: 基础 1:1，应用买入浮动让用户多付
			// 公式: payAmount = settlementAmount / (1 - buyFloat)
			if buyFloat.IsZero() {
				payAmount = settlementAmount.Round(6)
			} else {
				divisor := decimal.NewFromInt(1).Sub(buyFloat)
				payAmount = settlementAmount.Div(divisor).Round(6)
			}
		} else if payCurrency == "TRX" {
			trxUsdRate, err := rateService.GetTRXUSDRate()
			if err != nil {
				return nil, errors.New("TRX汇率获取失败: " + err.Error())
			}
			if trxUsdRate.IsZero() {
				return nil, errors.New("TRX/USD 汇率为零")
			}
			var adjustedRate decimal.Decimal
			if buyFloat.IsZero() {
				adjustedRate = trxUsdRate
			} else {
				adjustedRate = trxUsdRate.Mul(decimal.NewFromInt(1).Sub(buyFloat))
			}
			payAmount = settlementAmount.Div(adjustedRate).Round(6)
		} else if payCurrency == "CNY" {
			// USD -> CNY: 用市场汇率 + 买入浮动 (让用户多付 CNY)
			// 公式: payAmount = settlementAmount × marketRate(USD→CNY) × (1 + buyFloat)
			cnyUsdRate, err := rateService.GetRateWithType(RateTypeBuy, "USD", "CNY")
			if err != nil {
				return nil, errors.New("CNY汇率获取失败: " + err.Error())
			}
			// GetRateWithType(Buy) 已经把买入浮动算进去了 (1+buyFloat)
			payAmount = settlementAmount.Mul(cnyUsdRate).Round(2)
		} else {
			// 防御性：未来若加入新链/新支付货币而忘了在此分支处理，立即报错而非生成 payAmount=0 的免费订单
			return nil, fmt.Errorf("不支持的支付货币: %s", payCurrency)
		}
	}

	// 计算显示汇率（用于收银台显示）
	// 显示格式: 1 {支付货币} ≈ {产品货币符号}X
	// 例如：1 USDT ≈ $1（USD产品）、1 TRX ≈ €0.84（EUR产品）
	var displayRate decimal.Decimal
	if payCurrency == currency {
		// 支付货币与产品货币相同，汇率为1
		displayRate = decimal.NewFromInt(1)
	} else if !payAmount.IsZero() {
		// 计算：1单位支付货币 = 多少产品货币
		// displayRate = money / payAmount
		displayRate = money.Div(payAmount)
	} else {
		displayRate = decimal.NewFromInt(1)
	}

	// 创建订单
	order := model.Order{
		TradeNo:          util.GenerateTradeNo(),
		OutTradeNo:       req.OutTradeNo,
		MerchantID:       merchant.ID,
		Type:             payType,
		Name:             req.Name,
		Currency:         currency,
		Money:            money,
		PayAmount:        payAmount,
		PayCurrency:      payCurrency,
		USDTAmount:       payAmount,        // 兼容旧字段
		SettlementAmount: settlementAmount, // 结算金额（USD），用于计入商户余额
		Rate:             displayRate,      // 显示汇率（支付货币 -> 原始货币）
		Chain:            chain,
		Status:           model.OrderStatusPending,
		NotifyURL:        req.NotifyURL,
		ReturnURL:        req.ReturnURL,
		Param:            req.Param,
		ClientIP:         req.ClientIP,
		ExpiredAt:        expiredAt,
		Channel:          channel,
	}

	// 根据通道类型处理
	if channel == "local" {
		var wallet model.Wallet
		var useMerchantWallet bool

		// 根据商户钱包模式选择钱包
		// WalletMode: 1=仅系统钱包, 2=仅个人钱包, 3=混合模式(优先个人)
		wallet, useMerchantWallet, err = s.selectWalletByMode(&merchant, chain)
		if err != nil {
			return nil, err
		}

		// 法币收款：验证钱包必须有收款码
		if isFiat {
			if wallet.QRCode == "" {
				return nil, errors.New("钱包收款码未配置，请先上传收款二维码")
			}
			order.ToAddress = wallet.Address
			order.QRCode = wallet.QRCode
		} else {
			// 加密货币收款
			if chain == "trc20" || chain == "trx" {
				order.ToAddress = wallet.Address
			} else {
				order.ToAddress = strings.ToLower(wallet.Address)
			}
		}

		// 记录使用的钱包
		order.WalletID = wallet.ID

		// 根据钱包类型获取手续费率
		var feeRate decimal.Decimal
		if useMerchantWallet {
			feeRate = s.getPersonalWalletFeeRate()
			order.FeeType = model.FeeTypeBalance
		} else {
			feeRate = s.getSystemWalletFeeRate()
			order.FeeType = model.FeeTypeDeduction
		}
		order.FeeRate = feeRate

		// 计算手续费 (基于 USD 结算金额)
		fee := settlementAmount.Mul(feeRate)
		order.Fee = fee

		// 使用事务保证：UniqueAmount 生成 + 费用扣除 + 订单创建 的原子性
		txErr := model.GetDB().Transaction(func(tx *gorm.DB) error {
			// 在事务中生成唯一金额（防止并发生成相同金额）
			if isFiat {
				uniqueAmount := rateService.GenerateUniqueAmountWithTx(tx, payAmount, chain)
				order.UniqueAmount = uniqueAmount
			} else {
				uniqueAmount := rateService.GenerateUniqueAmountWithTx(tx, payAmount, chain)
				order.PayAmount = payAmount
				order.UniqueAmount = uniqueAmount
				order.USDTAmount = uniqueAmount
			}

			// 商户钱包模式需要预扣手续费（在事务中确保一致性）
			if useMerchantWallet && fee.GreaterThan(decimal.Zero) {
				feeStr := fee.StringFixed(6)
				result := tx.Model(&model.Merchant{}).
					Where("id = ? AND (balance - frozen_balance) >= ?", merchant.ID, feeStr).
					Update("frozen_balance", gorm.Expr("frozen_balance + ?", feeStr))

				if result.Error != nil {
					return errors.New("扣除手续费失败")
				}
				if result.RowsAffected == 0 {
					return errors.New("商户余额不足以支付手续费，请先充值")
				}
			}

			// 在同一事务中创建订单
			if err := tx.Create(&order).Error; err != nil {
				return errors.New("订单创建失败")
			}

			return nil
		})

		if txErr != nil {
			return nil, txErr
		}
	} else {
		// 非 local 通道：计算手续费（使用系统钱包费率），然后创建订单
		feeRate := s.getSystemWalletFeeRate()
		order.FeeRate = feeRate
		order.FeeType = model.FeeTypeDeduction
		order.Fee = settlementAmount.Mul(feeRate)

		if err := model.GetDB().Create(&order).Error; err != nil {
			return nil, errors.New("订单创建失败")
		}
	}

	// 发送Telegram通知 - 订单创建
	go GetTelegramService().NotifyOrderCreated(&order)

	// 如果使用上游通道，在这里创建上游订单
	if channel != "local" {
		if err := s.createUpstreamOrder(&order, channel); err != nil {
			// 上游创建失败，回退到本地
			fmt.Printf("Failed to create upstream order: %v, falling back to local\n", err)
			order.Channel = "local"
			// 回退到本地通道：使用正确的钱包选择和费率计算
			wallet, useMerchantWallet, walletErr := s.selectWalletByMode(&merchant, chain)
			if walletErr != nil {
				return nil, errors.New("暂无可用的收款地址")
			}
			if chain == "trc20" || chain == "trx" {
				order.ToAddress = wallet.Address
			} else {
				order.ToAddress = strings.ToLower(wallet.Address)
			}
			order.WalletID = wallet.ID

			// 计算手续费
			var feeRate decimal.Decimal
			if useMerchantWallet {
				feeRate = s.getPersonalWalletFeeRate()
				order.FeeType = model.FeeTypeBalance
			} else {
				feeRate = s.getSystemWalletFeeRate()
				order.FeeType = model.FeeTypeDeduction
			}
			order.FeeRate = feeRate
			order.Fee = settlementAmount.Mul(feeRate)

			// 在事务中生成唯一金额并预扣手续费
			txErr := model.GetDB().Transaction(func(tx *gorm.DB) error {
				uniqueAmount := rateService.GenerateUniqueAmountWithTx(tx, payAmount, chain)
				order.PayAmount = payAmount
				order.UniqueAmount = uniqueAmount
				order.USDTAmount = uniqueAmount

				// 商户钱包模式需要预扣手续费
				if useMerchantWallet && order.Fee.GreaterThan(decimal.Zero) {
					feeStr := order.Fee.StringFixed(6)
					freezeResult := tx.Model(&model.Merchant{}).
						Where("id = ? AND (balance - frozen_balance) >= ?", merchant.ID, feeStr).
						Update("frozen_balance", gorm.Expr("frozen_balance + ?", feeStr))
					if freezeResult.Error != nil {
						return errors.New("扣除手续费失败")
					}
					if freezeResult.RowsAffected == 0 {
						return errors.New("商户余额不足以支付手续费")
					}
				}

				return tx.Save(&order).Error
			})
			if txErr != nil {
				return nil, txErr
			}
		}
	}

	return s.buildOrderResponse(&order, &merchant)
}

// determineChannel 确定使用的支付通道
func (s *OrderService) determineChannel(requestedChannel string, chain string) string {
	// 如果明确指定了通道，尝试使用该通道
	if requestedChannel != "" && requestedChannel != "local" {
		channelService := GetChannelService()
		if _, err := channelService.GetChannelConfig(ChannelType(requestedChannel)); err == nil {
			return requestedChannel
		}
	}

	// 检查是否有默认的上游通道配置
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", "default_channel").First(&config).Error; err == nil {
		if config.Value != "" && config.Value != "local" {
			channelService := GetChannelService()
			if _, err := channelService.GetChannelConfig(ChannelType(config.Value)); err == nil {
				return config.Value
			}
		}
	}

	return "local"
}

// createUpstreamOrder 在上游通道创建订单
func (s *OrderService) createUpstreamOrder(order *model.Order, channel string) error {
	channelService := GetChannelService()
	cfg, err := channelService.GetChannelConfig(ChannelType(channel))
	if err != nil {
		return err
	}

	// 构建本系统的回调地址
	var notifyURL string
	var siteConfig model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", "site_url").First(&siteConfig).Error; err == nil {
		notifyURL = siteConfig.Value + "/channel/notify/" + channel
	} else {
		notifyURL = "/channel/notify/" + channel
	}

	var result *ChannelOrderResult

	switch ChannelType(channel) {
	case ChannelTypeVmq:
		result, err = channelService.CreateVmqOrder(cfg, order, notifyURL)
	case ChannelTypeEpay:
		result, err = channelService.CreateEpayOrder(cfg, order, notifyURL)
	default:
		return fmt.Errorf("unsupported channel type: %s", channel)
	}

	if err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("upstream error: %s", result.Error)
	}

	// 更新订单
	order.ChannelOrderID = result.OrderID
	order.ChannelPayURL = result.PayURL
	order.PayAmount = result.Amount    // 设置展示金额
	order.UniqueAmount = result.Amount // 设置标识金额（上游通道不需要偏移）
	order.USDTAmount = result.Amount   // 兼容旧字段
	if !result.ExpireTime.IsZero() {
		order.ExpiredAt = result.ExpireTime
	}
	// 注意：SettlementAmount 已经在订单创建时设置（第202行），这里无需修改

	return model.GetDB().Save(order).Error
}

// buildOrderResponse 构建订单响应
func (s *OrderService) buildOrderResponse(order *model.Order, merchant *model.Merchant) (*CreateOrderResponse, error) {
	// 判断是否为法币收款方式
	isFiat := util.IsFiatChain(order.Chain)

	// 生成二维码字段（v1.3.11 起返回绝对图片 URL，可直接 <img src=...> 嵌入）
	// 兼容性提示：v1.3.10 及以前 qrcode 是地址文本/相对路径；如商户代码把 qrcode 当文本用，
	// 升级后需要改为读 address 字段
	siteURL := getSiteURL()
	qrcode := buildQRCodeURL(order, isFiat, siteURL)

	resp := &CreateOrderResponse{
		Code:         1,
		Msg:          "success",
		TradeNo:      order.TradeNo,
		OutTradeNo:   order.OutTradeNo,
		Type:         order.Type,
		Currency:     order.Currency,
		Money:        order.Money.String(),
		PayCurrency:  order.PayCurrency,
		PayAmount:    order.PayAmount.String(),    // 展示金额（如 102.04）
		UniqueAmount: order.UniqueAmount.String(), // 标识金额（如 102.040023）
		USDTAmount:   order.UniqueAmount.String(), // 兼容旧字段，等于 unique_amount
		Rate:         order.Rate.String(),
		Address:      order.ToAddress,
		Chain:        order.Chain,
		QRCode:       qrcode,
		ExpiredAt:    order.ExpiredAt.Format("2006-01-02 15:04:05"),
		Channel:      order.Channel,
	}

	// 如果是上游通道订单，返回上游支付链接
	if order.Channel != "local" && order.ChannelPayURL != "" {
		resp.ChannelPayURL = order.ChannelPayURL
	}

	return resp, nil
}

// GetOrder 获取订单
func (s *OrderService) GetOrder(tradeNo string) (*model.Order, error) {
	var order model.Order
	if err := model.GetDB().Preload("Merchant").Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return nil, errors.New("订单不存在")
	}
	return &order, nil
}

// GetOrderByOutTradeNo 根据商户订单号获取订单
// 如果merchantPID为空，则只根据out_trade_no查询
func (s *OrderService) GetOrderByOutTradeNo(merchantPID, outTradeNo string) (*model.Order, error) {
	var order model.Order

	if merchantPID == "" {
		// 不指定商户，直接按商户订单号查询
		if err := model.GetDB().Preload("Merchant").Where("out_trade_no = ?", outTradeNo).First(&order).Error; err != nil {
			return nil, errors.New("订单不存在")
		}
	} else {
		// 指定商户，先查商户再查订单
		var merchant model.Merchant
		if err := model.GetDB().Where("p_id = ?", merchantPID).First(&merchant).Error; err != nil {
			return nil, errors.New("商户不存在")
		}

		if err := model.GetDB().Where("merchant_id = ? AND out_trade_no = ?", merchant.ID, outTradeNo).First(&order).Error; err != nil {
			return nil, errors.New("订单不存在")
		}
		order.Merchant = &merchant
	}

	return &order, nil
}

// GetOrderStatus 获取订单状态
func (s *OrderService) GetOrderStatus(tradeNo string) (model.OrderStatus, error) {
	var order model.Order
	if err := model.GetDB().Select("status").Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return 0, errors.New("订单不存在")
	}
	return order.Status, nil
}

// QueryOrders 查询订单列表
func (s *OrderService) QueryOrders(query *model.OrderQuery) ([]model.Order, int64, error) {
	db := model.GetDB().Model(&model.Order{})

	if query.MerchantID > 0 {
		db = db.Where("merchant_id = ?", query.MerchantID)
	}
	if query.TradeNo != "" {
		db = db.Where("trade_no = ?", query.TradeNo)
	}
	if query.OutTradeNo != "" {
		db = db.Where("out_trade_no = ?", query.OutTradeNo)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.Type != "" {
		db = db.Where("type = ?", query.Type)
	}
	if query.StartTime != nil {
		db = db.Where("created_at >= ?", query.StartTime)
	}
	if query.EndTime != nil {
		db = db.Where("created_at <= ?", query.EndTime)
	}

	var total int64
	db.Count(&total)

	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	var orders []model.Order
	offset := (query.Page - 1) * query.PageSize
	if err := db.Preload("Merchant").Order("created_at DESC").Offset(offset).Limit(query.PageSize).Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// QueryOrdersWithMerchant 查询订单列表（带商户信息，用于管理后台）
func (s *OrderService) QueryOrdersWithMerchant(query *model.OrderQuery) ([]model.Order, int64, error) {
	// 直接调用 QueryOrders，因为它已经使用 Preload("Merchant")
	return s.QueryOrders(query)
}

// GetOrderStats 获取订单统计 (金额统一使用 USD，USDT ≈ USD)
func (s *OrderService) GetOrderStats(merchantID uint) (*model.OrderStats, error) {
	stats := &model.OrderStats{}
	db := model.GetDB().Model(&model.Order{})

	if merchantID > 0 {
		db = db.Where("merchant_id = ?", merchantID)
	}

	// 总订单数
	db.Count(&stats.TotalOrders)

	// 总金额 (使用 settlement_amount 统一 USD 结算)
	var totalUSD float64
	dbPaid := model.GetDB().Model(&model.Order{})
	if merchantID > 0 {
		dbPaid = dbPaid.Where("merchant_id = ?", merchantID)
	}
	dbPaid.Where("status = ?", model.OrderStatusPaid).Select("COALESCE(SUM(settlement_amount), 0)").Scan(&totalUSD)
	stats.TotalUSD = decimal.NewFromFloat(totalUSD)

	// 各状态订单数
	dbPending := model.GetDB().Model(&model.Order{})
	if merchantID > 0 {
		dbPending = dbPending.Where("merchant_id = ?", merchantID)
	}
	dbPending.Where("status = ?", model.OrderStatusPending).Count(&stats.PendingOrders)

	dbPaidCount := model.GetDB().Model(&model.Order{})
	if merchantID > 0 {
		dbPaidCount = dbPaidCount.Where("merchant_id = ?", merchantID)
	}
	dbPaidCount.Where("status = ?", model.OrderStatusPaid).Count(&stats.PaidOrders)

	dbExpired := model.GetDB().Model(&model.Order{})
	if merchantID > 0 {
		dbExpired = dbExpired.Where("merchant_id = ?", merchantID)
	}
	dbExpired.Where("status = ?", model.OrderStatusExpired).Count(&stats.ExpiredOrders)

	// 今日统计
	today := time.Now().Format("2006-01-02")
	todayDB := model.GetDB().Model(&model.Order{}).Where("DATE(created_at) = ?", today)
	if merchantID > 0 {
		todayDB = todayDB.Where("merchant_id = ?", merchantID)
	}

	todayDB.Count(&stats.TodayOrders)

	var todayUSD float64
	todayDB.Where("status = ?", model.OrderStatusPaid).Select("COALESCE(SUM(settlement_amount), 0)").Scan(&todayUSD)
	stats.TodayUSD = decimal.NewFromFloat(todayUSD)

	// 可用支付链路数量
	blockchainService := GetBlockchainService()
	listenerStatus := blockchainService.GetListenerStatus()
	availableChannels := 0
	for _, infoInterface := range listenerStatus {
		if info, ok := infoInterface.(map[string]interface{}); ok {
			running, _ := info["running"].(bool)
			walletCount, _ := info["wallet_count"].(int64)
			if running && walletCount > 0 {
				availableChannels++
			}
		}
	}
	stats.AvailableChannels = availableChannels

	return stats, nil
}

// ExpireOrders 过期订单处理（分批处理，避免大量数据加载）
func (s *OrderService) ExpireOrders() {
	batchSize := 100
	for {
		var orders []model.Order
		if err := model.GetDB().
			Where("status = ? AND expired_at < ?", model.OrderStatusPending, time.Now()).
			Limit(batchSize).
			Find(&orders).Error; err != nil {
			return
		}

		if len(orders) == 0 {
			return
		}

		for _, order := range orders {
			// 更新订单状态
			result := model.GetDB().Model(&model.Order{}).Where("id = ? AND status = ?", order.ID, model.OrderStatusPending).Update("status", model.OrderStatusExpired)
			if result.RowsAffected == 0 {
				continue // order was already paid or processed
			}

			// 退还预扣的手续费 (仅商户钱包模式)
			if order.FeeType == model.FeeTypeBalance {
				if err := GetWithdrawService().RefundPreChargedFeeDec(order.MerchantID, order.Fee); err != nil {
					fmt.Printf("Failed to refund fee for expired order %s: %v\n", order.TradeNo, err)
				}
			}

			// 发送Telegram通知 - 订单过期
			go GetTelegramService().NotifyOrderExpired(&order)
		}

		fmt.Printf("Expired %d orders\n", len(orders))

		// 如果本批不满，说明已处理完
		if len(orders) < batchSize {
			return
		}
	}
}

// StartExpireWorker 启动订单过期处理工作协程
func (s *OrderService) StartExpireWorker() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			s.ExpireOrders()
		}
	}()
}

// getOrderExpireMinutes 获取订单过期时间(分钟)
func (s *OrderService) getOrderExpireMinutes() int {
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", model.ConfigKeyOrderExpire).First(&config).Error; err != nil {
		return 30
	}
	minutes, err := strconv.Atoi(config.Value)
	if err != nil {
		return 30
	}
	return minutes
}

// selectWalletByMode 根据商户钱包模式选择钱包
// WalletMode: 1=仅系统钱包, 2=仅个人钱包, 3=混合模式(优先个人)
// 返回: 钱包, 是否为商户钱包, 错误
// 使用轮询方式选择钱包，优先选择最久未使用的钱包
func (s *OrderService) selectWalletByMode(merchant *model.Merchant, chain string) (model.Wallet, bool, error) {
	var wallet model.Wallet
	var useMerchantWallet bool

	// 轮询排序：按最后使用时间升序（NULL值优先，即从未使用的钱包优先）
	roundRobinOrder := "COALESCE(last_used_at, '1970-01-01') ASC"

	switch merchant.WalletMode {
	case 1: // 仅系统钱包
		if err := model.GetDB().Where("chain = ? AND status = 1 AND (merchant_id IS NULL OR merchant_id = 0)", chain).Order(roundRobinOrder).First(&wallet).Error; err != nil {
			return wallet, false, fmt.Errorf("暂无可用的系统收款地址")
		}
		useMerchantWallet = false

	case 2: // 仅个人钱包
		if err := model.GetDB().Where("chain = ? AND status = 1 AND merchant_id = ?", chain, merchant.ID).Order(roundRobinOrder).First(&wallet).Error; err != nil {
			return wallet, false, fmt.Errorf("暂无可用的个人收款地址，请先添加收款地址")
		}
		useMerchantWallet = true

	default: // 3=混合模式(优先个人)
		// 优先使用商户自己的钱包
		if err := model.GetDB().Where("chain = ? AND status = 1 AND merchant_id = ?", chain, merchant.ID).Order(roundRobinOrder).First(&wallet).Error; err != nil {
			// 没有商户钱包，使用系统钱包
			if err := model.GetDB().Where("chain = ? AND status = 1 AND (merchant_id IS NULL OR merchant_id = 0)", chain).Order(roundRobinOrder).First(&wallet).Error; err != nil {
				return wallet, false, fmt.Errorf("暂无可用的收款地址")
			}
			useMerchantWallet = false
		} else {
			useMerchantWallet = true
		}
	}

	// 更新钱包最后使用时间
	model.GetDB().Model(&wallet).Update("last_used_at", time.Now())

	return wallet, useMerchantWallet, nil
}

// getSystemWalletFeeRate 获取系统收款码手续费率
func (s *OrderService) getSystemWalletFeeRate() decimal.Decimal {
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", model.ConfigKeySystemWalletFeeRate).First(&config).Error; err != nil {
		return decimal.NewFromFloat(0.02) // 默认2%
	}
	rate, err := decimal.NewFromString(config.Value)
	if err != nil {
		return decimal.NewFromFloat(0.02)
	}
	return rate
}

// getPersonalWalletFeeRate 获取个人收款码手续费率
func (s *OrderService) getPersonalWalletFeeRate() decimal.Decimal {
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", model.ConfigKeyPersonalWalletFeeRate).First(&config).Error; err != nil {
		return decimal.NewFromFloat(0.01) // 默认1%
	}
	rate, err := decimal.NewFromString(config.Value)
	if err != nil {
		return decimal.NewFromFloat(0.01)
	}
	return rate
}

// CancelOrder 取消订单
func (s *OrderService) CancelOrder(tradeNo string) error {
	var order model.Order
	if err := model.GetDB().Where("trade_no = ? AND status = ?", tradeNo, model.OrderStatusPending).First(&order).Error; err != nil {
		return errors.New("订单不存在或无法取消")
	}

	// 更新订单状态（使用乐观锁防止覆盖已支付订单）
	result := model.GetDB().Model(&model.Order{}).Where("id = ? AND status = ?", order.ID, model.OrderStatusPending).Update("status", model.OrderStatusCancelled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("订单状态已变更，无法取消")
	}

	// 退还预扣的手续费 (仅商户钱包模式)
	if order.FeeType == model.FeeTypeBalance {
		if err := GetWithdrawService().RefundPreChargedFeeDec(order.MerchantID, order.Fee); err != nil {
			return fmt.Errorf("退还手续费失败: %v", err)
		}
	}

	return nil
}

// MarkOrderPaid 手动标记订单已支付 (仅管理员)
func (s *OrderService) MarkOrderPaid(tradeNo string, txHash string, amount decimal.Decimal) error {
	var order model.Order
	if err := model.GetDB().Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return errors.New("订单不存在")
	}

	if order.Status != model.OrderStatusPending && order.Status != model.OrderStatusExpired {
		return errors.New("订单状态不正确，只能确认待支付或已过期订单")
	}
	// 记录原始状态：Pending 时手续费仍在冻结，Expired 时 ExpireOrders 已退还冻结
	prevStatus := order.Status

	now := time.Now()
	updates := map[string]interface{}{
		"status":        model.OrderStatusPaid,
		"tx_hash":       txHash,
		"actual_amount": amount,
		"paid_at":       &now,
	}

	// 在事务中完成：更新订单状态 + 入账商户余额（保证原子性）
	txErr := model.GetDB().Transaction(func(tx *gorm.DB) error {
		// CAS 仅匹配原始状态，杜绝并发场景下的双写
		result := tx.Model(&model.Order{}).
			Where("id = ? AND status = ?", order.ID, prevStatus).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("订单状态已变更，无法确认")
		}

		// 入账商户余额（在同一事务内）
		realAmount := order.SettlementAmount.Sub(order.Fee)

		// FeeType=Balance + 原状态=Pending: 手续费仍处于冻结中，paid 时需要解冻
		// FeeType=Balance + 原状态=Expired: ExpireOrders 已经把冻结退还了，这里不能再扣 frozen_balance（否则会负数 → 可用余额虚高）
		// FeeType=Deduction: 手续费从收款扣除，没有冻结
		if order.FeeType == model.FeeTypeBalance && prevStatus == model.OrderStatusPending {
			return tx.Model(&model.Merchant{}).Where("id = ?", order.MerchantID).Updates(map[string]interface{}{
				"balance":        gorm.Expr("balance + ?", realAmount.String()),
				"frozen_balance": gorm.Expr("frozen_balance - ?", order.Fee.String()),
			}).Error
		}
		return tx.Model(&model.Merchant{}).Where("id = ?", order.MerchantID).
			Update("balance", gorm.Expr("balance + ?", realAmount.String())).Error
	})
	if txErr != nil {
		return txErr
	}

	// 使��存失效
	s.InvalidateOrderCache(tradeNo)

	// ��新加载订单数据
	model.GetDB().First(&order, order.ID)

	// 触发回调
	go GetNotifyService().NotifyOrder(order.ID)

	// 触发 Telegram 通知
	go GetTelegramService().NotifyOrderPaid(&order)

	return nil
}

// CheckOrderPaid 检查订单是否已支付 (用于轮询)
func (s *OrderService) CheckOrderPaid(tradeNo string) (bool, *model.Order, error) {
	// 检查缓存
	if cached, ok := s.cache.Load(tradeNo); ok {
		item := cached.(orderCacheItem)
		// 如果缓存未过期且订单已支付或已过期，直接返回缓存结果
		// 已支付和已过期的订单状态不会再变化，可以长期缓存
		if time.Since(item.timestamp) < s.cacheTTL || item.order.Status == model.OrderStatusPaid || item.order.Status == model.OrderStatusExpired {
			return item.paid, item.order, nil
		}
	}

	// 从数据库查询
	var order model.Order
	if err := model.GetDB().Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, errors.New("订单不存在")
		}
		return false, nil, err
	}

	paid := order.Status == model.OrderStatusPaid

	// 更新缓存
	s.cache.Store(tradeNo, orderCacheItem{
		order:     &order,
		paid:      paid,
		timestamp: time.Now(),
	})

	return paid, &order, nil
}

// InvalidateOrderCache 使订单缓存失效（在订单状态更新时调用）
func (s *OrderService) InvalidateOrderCache(tradeNo string) {
	s.cache.Delete(tradeNo)
}
