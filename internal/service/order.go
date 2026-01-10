package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ezpay/internal/model"
	"ezpay/internal/util"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// OrderService 订单服务
type OrderService struct{}

var orderService *OrderService

// GetOrderService 获取订单服务
func GetOrderService() *OrderService {
	if orderService == nil {
		orderService = &OrderService{}
	}
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
	Currency    string `json:"currency" form:"currency"` // 货币类型: CNY, USD, USDT 等，默认 CNY
	Param       string `json:"param" form:"param"`
	ClientIP    string `json:"-"`
	Channel     string `json:"channel" form:"channel"` // 指定支付通道: local, vmq, epay (可选)
}

// CreateOrderResponse 创建订单响应
type CreateOrderResponse struct {
	Code           int    `json:"code"`
	Msg            string `json:"msg"`
	TradeNo        string `json:"trade_no,omitempty"`
	OutTradeNo     string `json:"out_trade_no,omitempty"`
	Type           string `json:"type,omitempty"`
	Currency       string `json:"currency,omitempty"`         // 原始货币
	Money          string `json:"money,omitempty"`            // 原始金额
	PayCurrency    string `json:"pay_currency,omitempty"`     // 支付货币
	PayAmount      string `json:"pay_amount,omitempty"`       // 支付金额
	USDTAmount     string `json:"usdt_amount,omitempty"`      // USDT金额(兼容)
	Rate           string `json:"rate,omitempty"`
	Address        string `json:"address,omitempty"`
	Chain          string `json:"chain,omitempty"`
	QRCode         string `json:"qrcode,omitempty"`
	ExpiredAt      string `json:"expired_at,omitempty"`
	PayURL         string `json:"pay_url,omitempty"`
	Channel        string `json:"channel,omitempty"`          // 实际使用的支付通道
	ChannelPayURL  string `json:"channel_pay_url,omitempty"`  // 上游支付链接 (如果使用通道)
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

	// 标准化货币类型（默认 CNY）
	currency := NormalizeCurrency(req.Currency)
	if currency == "" {
		currency = "CNY"
	}

	// 标准化支付类型
	payType := util.NormalizePaymentType(req.Type)
	chain := util.GetPaymentTypeChain(req.Type)

	if !util.IsValidChain(chain) {
		return nil, errors.New("不支持的支付类型")
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
	var payAmount, rate decimal.Decimal
	var payCurrency string

	if isFiat {
		// 法币收款(微信/支付宝)：转换为 CNY
		convertResult, err := rateService.ConvertToPayCurrency(currency, money, chain)
		if err != nil {
			return nil, errors.New("汇率获取失败: " + err.Error())
		}
		payAmount = convertResult.Amount
		rate = convertResult.Rate
		payCurrency = convertResult.PayCurrency
	} else {
		// 加密货币收款：转换为对应货币(USDT/TRX)
		convertResult, err := rateService.ConvertToPayCurrency(currency, money, chain)
		if err != nil {
			return nil, errors.New("汇率获取失败: " + err.Error())
		}
		payAmount = convertResult.Amount
		rate = convertResult.Rate
		payCurrency = convertResult.PayCurrency
	}

	// 创建订单
	order := model.Order{
		TradeNo:     util.GenerateTradeNo(),
		OutTradeNo:  req.OutTradeNo,
		MerchantID:  merchant.ID,
		Type:        payType,
		Name:        req.Name,
		Currency:    currency,
		Money:       money,
		PayAmount:   payAmount,
		PayCurrency: payCurrency,
		USDTAmount:  payAmount, // 兼容旧字段
		Rate:        rate,
		Chain:       chain,
		Status:      model.OrderStatusPending,
		NotifyURL:   req.NotifyURL,
		ReturnURL:   req.ReturnURL,
		Param:       req.Param,
		ClientIP:    req.ClientIP,
		ExpiredAt:   expiredAt,
		Channel:     channel,
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

		if isFiat {
			// 法币收款：使用收款码
			order.ToAddress = wallet.Address // 微信/支付宝账号
			order.QRCode = wallet.QRCode     // 收款二维码
		} else {
			// 加密货币收款：生成唯一金额
			// TRC20地址保持原始大小写(Base58编码)，ERC20/BEP20转小写
			if chain == "trc20" || chain == "trx" {
				order.ToAddress = wallet.Address
			} else {
				order.ToAddress = strings.ToLower(wallet.Address)
			}
			uniqueAmount := rateService.GenerateUniqueAmount(payAmount, chain)
			order.PayAmount = uniqueAmount
			order.USDTAmount = uniqueAmount // 兼容旧字段
		}

		// 记录使用的钱包
		order.WalletID = wallet.ID

		// 根据钱包类型获取手续费率
		var feeRate decimal.Decimal
		if useMerchantWallet {
			// 使用商户钱包：使用个人收款码手续费率
			feeRate = s.getPersonalWalletFeeRate()
			order.FeeType = model.FeeTypeBalance
		} else {
			// 使用平台钱包：使用系统收款码手续费率
			feeRate = s.getSystemWalletFeeRate()
			order.FeeType = model.FeeTypeDeduction
		}
		order.FeeRate = feeRate

		// 计算手续费 (基于订单金额)
		fee := money.Mul(feeRate)
		order.Fee = fee

		// 商户钱包模式需要预扣手续费
		if useMerchantWallet && fee.GreaterThan(decimal.Zero) {
			// 使用原子操作：检查余额并扣款，避免并发问题
			// UPDATE merchants SET frozen_balance = frozen_balance + fee
			// WHERE id = ? AND (balance - frozen_balance) >= fee
			result := model.GetDB().Model(&model.Merchant{}).
				Where("id = ? AND (balance - frozen_balance) >= ?", merchant.ID, fee.InexactFloat64()).
				Update("frozen_balance", gorm.Expr("frozen_balance + ?", fee.InexactFloat64()))

			if result.Error != nil {
				return nil, errors.New("扣除手续费失败")
			}
			if result.RowsAffected == 0 {
				return nil, errors.New("商户余额不足以支付手续费，请先充值")
			}
		}
	}

	if err := model.GetDB().Create(&order).Error; err != nil {
		return nil, errors.New("订单创建失败")
	}

	// 发送Telegram通知 - 订单创建
	go GetTelegramService().NotifyOrderCreated(&order)

	// 如果使用上游通道，在这里创建上游订单
	if channel != "local" {
		if err := s.createUpstreamOrder(&order, channel); err != nil {
			// 上游创建失败，回退到本地
			fmt.Printf("Failed to create upstream order: %v, falling back to local\n", err)
			order.Channel = "local"
			// 获取本地收款地址
			var wallet model.Wallet
			if err := model.GetDB().Where("chain = ? AND status = 1", chain).Order("RAND()").First(&wallet).Error; err != nil {
				return nil, errors.New("暂无可用的收款地址")
			}
			// TRC20地址保持原始大小写
			if chain == "trc20" || chain == "trx" {
				order.ToAddress = wallet.Address
			} else {
				order.ToAddress = strings.ToLower(wallet.Address)
			}
			uniqueAmount := rateService.GenerateUniqueAmount(payAmount, chain)
			order.PayAmount = uniqueAmount
			order.USDTAmount = uniqueAmount
			model.GetDB().Save(&order)
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
	order.USDTAmount = result.Amount
	if !result.ExpireTime.IsZero() {
		order.ExpiredAt = result.ExpireTime
	}

	return model.GetDB().Save(order).Error
}

// buildOrderResponse 构建订单响应
func (s *OrderService) buildOrderResponse(order *model.Order, merchant *model.Merchant) (*CreateOrderResponse, error) {
	// 判断是否为法币收款方式
	isFiat := util.IsFiatChain(order.Chain)

	// 生成二维码内容
	var qrcode string
	if isFiat {
		// 微信/支付宝使用存储的收款码
		qrcode = order.QRCode
	} else {
		// USDT使用收款地址
		qrcode = order.ToAddress
	}

	resp := &CreateOrderResponse{
		Code:        1,
		Msg:         "success",
		TradeNo:     order.TradeNo,
		OutTradeNo:  order.OutTradeNo,
		Type:        order.Type,
		Currency:    order.Currency,
		Money:       order.Money.String(),
		PayCurrency: order.PayCurrency,
		PayAmount:   order.PayAmount.String(),
		USDTAmount:  order.USDTAmount.String(), // 兼容旧字段
		Rate:        order.Rate.String(),
		Address:     order.ToAddress,
		Chain:       order.Chain,
		QRCode:      qrcode,
		ExpiredAt:   order.ExpiredAt.Format("2006-01-02 15:04:05"),
		Channel:     order.Channel,
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

// GetOrderStats 获取订单统计
func (s *OrderService) GetOrderStats(merchantID uint) (*model.OrderStats, error) {
	stats := &model.OrderStats{}
	db := model.GetDB().Model(&model.Order{})

	if merchantID > 0 {
		db = db.Where("merchant_id = ?", merchantID)
	}

	// 总订单数
	db.Count(&stats.TotalOrders)

	// 总金额
	var totalMoney, totalUSDT float64
	db.Where("status = ?", model.OrderStatusPaid).Select("COALESCE(SUM(money), 0)").Scan(&totalMoney)
	db.Where("status = ?", model.OrderStatusPaid).Select("COALESCE(SUM(actual_amount), 0)").Scan(&totalUSDT)
	stats.TotalAmount = decimal.NewFromFloat(totalMoney)
	stats.TotalUSDT = decimal.NewFromFloat(totalUSDT)

	// 各状态订单数
	db.Where("status = ?", model.OrderStatusPending).Count(&stats.PendingOrders)
	db.Where("status = ?", model.OrderStatusPaid).Count(&stats.PaidOrders)
	db.Where("status = ?", model.OrderStatusExpired).Count(&stats.ExpiredOrders)

	// 今日统计
	today := time.Now().Format("2006-01-02")
	todayDB := model.GetDB().Model(&model.Order{}).Where("DATE(created_at) = ?", today)
	if merchantID > 0 {
		todayDB = todayDB.Where("merchant_id = ?", merchantID)
	}

	todayDB.Count(&stats.TodayOrders)

	var todayMoney, todayUSDT float64
	todayDB.Where("status = ?", model.OrderStatusPaid).Select("COALESCE(SUM(money), 0)").Scan(&todayMoney)
	todayDB.Where("status = ?", model.OrderStatusPaid).Select("COALESCE(SUM(actual_amount), 0)").Scan(&todayUSDT)
	stats.TodayAmount = decimal.NewFromFloat(todayMoney)
	stats.TodayUSDT = decimal.NewFromFloat(todayUSDT)

	return stats, nil
}

// ExpireOrders 过期订单处理
func (s *OrderService) ExpireOrders() {
	// 查找所有即将过期的订单
	var orders []model.Order
	if err := model.GetDB().Where("status = ? AND expired_at < ?", model.OrderStatusPending, time.Now()).Find(&orders).Error; err != nil {
		return
	}

	for _, order := range orders {
		// 更新订单状态
		model.GetDB().Model(&order).Update("status", model.OrderStatusExpired)

		// 退还预扣的手续费 (仅商户钱包模式)
		if order.FeeType == model.FeeTypeBalance {
			fee, _ := order.Fee.Float64()
			if err := GetWithdrawService().RefundPreChargedFee(order.MerchantID, fee); err != nil {
				fmt.Printf("Failed to refund fee for expired order %s: %v\n", order.TradeNo, err)
			}
		}

		// 发送Telegram通知 - 订单过期
		go GetTelegramService().NotifyOrderExpired(&order)
	}

	if len(orders) > 0 {
		fmt.Printf("Expired %d orders\n", len(orders))
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

	// 更新订单状态
	if err := model.GetDB().Model(&order).Update("status", model.OrderStatusCancelled).Error; err != nil {
		return err
	}

	// 退还预扣的手续费 (仅商户钱包模式)
	if order.FeeType == model.FeeTypeBalance {
		fee, _ := order.Fee.Float64()
		if err := GetWithdrawService().RefundPreChargedFee(order.MerchantID, fee); err != nil {
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

	now := time.Now()
	updates := map[string]interface{}{
		"status":        model.OrderStatusPaid,
		"tx_hash":       txHash,
		"actual_amount": amount,
		"paid_at":       &now,
	}

	if err := model.GetDB().Model(&order).Updates(updates).Error; err != nil {
		return err
	}

	// 重新加载订单数据
	model.GetDB().First(&order, order.ID)

	// 触发回调
	go GetNotifyService().NotifyOrder(order.ID)

	// 触发 Telegram 通知
	go GetTelegramService().NotifyOrderPaid(&order)

	return nil
}

// CheckOrderPaid 检查订单是否已支付 (用于轮询)
func (s *OrderService) CheckOrderPaid(tradeNo string) (bool, *model.Order, error) {
	var order model.Order
	if err := model.GetDB().Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, errors.New("订单不存在")
		}
		return false, nil, err
	}

	return order.Status == model.OrderStatusPaid, &order, nil
}
