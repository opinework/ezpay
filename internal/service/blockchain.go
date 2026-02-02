package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezpay/config"
	"ezpay/internal/model"

	"github.com/shopspring/decimal"
)

// 全局 HTTP 客户端（带超时配置）
var httpClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}

// BlockchainService 区块链监控服务
type BlockchainService struct {
	cfg          *config.Config
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	listeners    map[string]*ChainListener
	rpcClients   map[string]*RPCClient  // RPC 客户端（支持重试和故障转移）
	metrics      *BlockchainMetrics     // 监控指标
	mu           sync.RWMutex
	walletCache  *WalletCache
	gasPrices    map[string]float64     // Gas 价格缓存
	gasPriceMu   sync.RWMutex
}

// WalletCache 钱包地址缓存
type WalletCache struct {
	mu          sync.RWMutex
	cache       map[string]map[string]bool // chain -> addresses
	lastUpdate  time.Time
	ttl         time.Duration
}

// NewWalletCache 创建钱包缓存
func NewWalletCache(ttl time.Duration) *WalletCache {
	return &WalletCache{
		cache: make(map[string]map[string]bool),
		ttl:   ttl,
	}
}

// GetAddresses 获取指定链的钱包地址（带缓存）
func (c *WalletCache) GetAddresses(chain string) map[string]bool {
	c.mu.RLock()
	// 检查缓存是否过期
	if time.Since(c.lastUpdate) > c.ttl {
		c.mu.RUnlock()
		c.refresh()
		c.mu.RLock()
	}
	addresses := c.cache[chain]
	c.mu.RUnlock()

	if addresses == nil {
		return make(map[string]bool)
	}
	// 返回副本以避免并发问题
	result := make(map[string]bool, len(addresses))
	for k, v := range addresses {
		result[k] = v
	}
	return result
}

// refresh 刷新缓存
func (c *WalletCache) refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 双重检查
	if time.Since(c.lastUpdate) <= c.ttl {
		return
	}

	// 从数据库加载所有钱包
	var wallets []model.Wallet
	model.GetDB().Where("status = 1").Find(&wallets)

	newCache := make(map[string]map[string]bool)
	for _, w := range wallets {
		if newCache[w.Chain] == nil {
			newCache[w.Chain] = make(map[string]bool)
		}
		newCache[w.Chain][strings.ToLower(w.Address)] = true
	}
	c.cache = newCache
	c.lastUpdate = time.Now()
}

// Invalidate 使缓存失效
func (c *WalletCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastUpdate = time.Time{}
}

// ChainListener 链监听器
type ChainListener struct {
	chain           string
	rpc             string
	rpcBackups      []string           // RPC 备用节点
	contractAddress string
	confirmations   int
	scanInterval    int
	baseScanInterval int               // 基础扫描间隔
	lastBlock       uint64
	reorgDepth      int                // 重组检测深度
	blockHistory    []uint64           // 区块历史（用于重组检测）
	running         bool
	enabled         bool
	stopCh          chan struct{}
	mu              sync.Mutex
}

// Transfer 转账事件
type Transfer struct {
	TxHash      string
	From        string
	To          string
	Amount      decimal.Decimal
	BlockNumber uint64
	Chain       string
}

var blockchainService *BlockchainService
var blockchainOnce sync.Once

// GetBlockchainService 获取区块链服务单例
func GetBlockchainService() *BlockchainService {
	blockchainOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		blockchainService = &BlockchainService{
			ctx:         ctx,
			cancel:      cancel,
			listeners:   make(map[string]*ChainListener),
			rpcClients:  make(map[string]*RPCClient),
			metrics:     NewBlockchainMetrics(),
			walletCache: NewWalletCache(30 * time.Second), // 钱包缓存30秒
			gasPrices:   make(map[string]float64),
		}
	})
	return blockchainService
}

// InvalidateWalletCache 使钱包缓存失效（添加/修改/删除钱包后调用）
func (s *BlockchainService) InvalidateWalletCache() {
	if s.walletCache != nil {
		s.walletCache.Invalidate()
	}
}

// SetWalletCacheTTL 设置钱包缓存TTL
func (s *BlockchainService) SetWalletCacheTTL(seconds int) {
	if s.walletCache != nil {
		s.walletCache.mu.Lock()
		s.walletCache.ttl = time.Duration(seconds) * time.Second
		s.walletCache.mu.Unlock()
	}
}

// Init 初始化区块链服务
func (s *BlockchainService) Init(cfg *config.Config) {
	s.cfg = cfg
	s.mu.Lock()
	defer s.mu.Unlock()

	// 初始化所有链监听器 (包括禁用的，方便后续动态启用)
	chainConfigs := map[string]config.ChainConfig{
		"trx":       cfg.Blockchain.TRX,
		"trc20":     cfg.Blockchain.TRC20,
		"erc20":     cfg.Blockchain.ERC20,
		"bep20":     cfg.Blockchain.BEP20,
		"polygon":   cfg.Blockchain.Polygon,
		"optimism":  cfg.Blockchain.Optimism,
		"arbitrum":  cfg.Blockchain.Arbitrum,
		"avalanche": cfg.Blockchain.Avalanche,
		"base":      cfg.Blockchain.Base,
	}

	for chain, chainCfg := range chainConfigs {
		// 创建 RPC 客户端（支持多节点）
		rpcEndpoints := []string{chainCfg.RPC}
		// TODO: 从配置中读取备用 RPC 节点
		s.rpcClients[chain] = NewRPCClient(rpcEndpoints)

		s.listeners[chain] = &ChainListener{
			chain:            chain,
			rpc:              chainCfg.RPC,
			rpcBackups:       []string{}, // TODO: 从配置读取
			contractAddress:  chainCfg.ContractAddress,
			confirmations:    chainCfg.Confirmations,
			scanInterval:     chainCfg.ScanInterval,
			baseScanInterval: chainCfg.ScanInterval,
			reorgDepth:       10, // 重组检测深度
			blockHistory:     make([]uint64, 0, 20),
			enabled:          chainCfg.Enabled,
			stopCh:           make(chan struct{}),
		}
	}

	// 启动 Gas 价格监控
	go s.monitorGasPrices()
}

// Start 启动所有已启用的监听器
func (s *BlockchainService) Start() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for chain, listener := range s.listeners {
		if listener.enabled {
			s.wg.Add(1)
			go s.runListener(chain, listener)
		}
	}
	log.Println("Blockchain service started")
}

// Stop 停止所有监听器
func (s *BlockchainService) Stop() {
	s.cancel()
	// 关闭所有监听器的stopCh
	s.mu.RLock()
	for _, listener := range s.listeners {
		listener.mu.Lock()
		if listener.running {
			close(listener.stopCh)
		}
		listener.mu.Unlock()
	}
	s.mu.RUnlock()
	s.wg.Wait()
	log.Println("Blockchain service stopped")
}

// runListener 运行链监听器
func (s *BlockchainService) runListener(chain string, listener *ChainListener) {
	defer s.wg.Done()

	listener.mu.Lock()
	listener.running = true
	listener.mu.Unlock()

	log.Printf("Starting %s listener", chain)

	ticker := time.NewTicker(time.Duration(listener.scanInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			listener.mu.Lock()
			listener.running = false
			listener.mu.Unlock()
			log.Printf("Stopped %s listener (context cancelled)", chain)
			return
		case <-listener.stopCh:
			listener.mu.Lock()
			listener.running = false
			listener.mu.Unlock()
			log.Printf("Stopped %s listener", chain)
			return
		case <-ticker.C:
			s.scanChain(listener)
		}
	}
}

// scanChain 扫描链上交易
func (s *BlockchainService) scanChain(listener *ChainListener) {
	startTime := s.metrics.RecordScanStart(listener.chain)

	// 从缓存获取收款地址
	addresses := s.walletCache.GetAddresses(listener.chain)
	if len(addresses) == 0 {
		return
	}

	var transfers []Transfer
	var err error

	switch listener.chain {
	case "trx":
		transfers, err = s.scanTRXImproved(listener, addresses)
	case "trc20":
		transfers, err = s.scanTRC20Improved(listener, addresses)
	case "erc20", "bep20", "polygon", "optimism", "arbitrum", "avalanche", "base":
		transfers, err = s.scanEVMImproved(listener, addresses)
	}

	if err != nil {
		log.Printf("[%s] Scan error: %v", listener.chain, err)
		s.metrics.RecordScanFailure(listener.chain, err)

		// 检查是否需要告警
		if shouldAlert, msg := s.metrics.ShouldAlert(listener.chain); shouldAlert {
			log.Printf("⚠️  ALERT: %s", msg)
			// TODO: 发送告警通知（Telegram/邮件等）
		}
		return
	}

	// 记录成功
	s.metrics.RecordScanSuccess(listener.chain, startTime)

	// 记录发现的转账
	if len(transfers) > 0 {
		log.Printf("[%s] Found %d transfers", listener.chain, len(transfers))
		s.metrics.RecordTransfer(listener.chain, len(transfers))
	}

	// 处理转账
	for _, transfer := range transfers {
		s.processTransfer(transfer)
	}

	// 动态调整扫描间隔
	s.adjustScanInterval(listener, len(transfers))
}

// scanTRX 扫描TRX原生代币交易
func (s *BlockchainService) scanTRX(listener *ChainListener, addresses map[string]bool) ([]Transfer, error) {
	var transfers []Transfer

	for addr := range addresses {
		// 使用TronGrid API获取TRX转账记录
		url := fmt.Sprintf("%s/v1/accounts/%s/transactions?only_confirmed=true&limit=50",
			listener.rpc, addr)

		resp, err := httpClient.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var result struct {
			Data []struct {
				TxID        string `json:"txID"`
				BlockNumber int64  `json:"blockNumber"`
				RawData     struct {
					Contract []struct {
						Type      string `json:"type"`
						Parameter struct {
							Value struct {
								Amount       int64  `json:"amount"`
								OwnerAddress string `json:"owner_address"`
								ToAddress    string `json:"to_address"`
							} `json:"value"`
						} `json:"parameter"`
					} `json:"contract"`
				} `json:"raw_data"`
				Ret []struct {
					ContractRet string `json:"contractRet"`
				} `json:"ret"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		for _, tx := range result.Data {
			// 检查交易是否成功
			if len(tx.Ret) == 0 || tx.Ret[0].ContractRet != "SUCCESS" {
				continue
			}

			// 检查是否是TRX转账
			if len(tx.RawData.Contract) == 0 {
				continue
			}

			contract := tx.RawData.Contract[0]
			if contract.Type != "TransferContract" {
				continue
			}

			// 转换地址格式（hex to base58）
			toAddr := contract.Parameter.Value.ToAddress
			// TronGrid API返回的地址可能是hex格式，需要转换
			if strings.HasPrefix(toAddr, "41") {
				toAddr = hexToBase58(toAddr)
			}

			// 检查是否是转入交易
			if !addresses[strings.ToLower(toAddr)] {
				continue
			}

			// 检查是否已处理
			var count int64
			model.GetDB().Model(&model.TransactionLog{}).Where("tx_hash = ?", tx.TxID).Count(&count)
			if count > 0 {
				continue
			}

			// TRX精度是6位 (1 TRX = 1,000,000 sun)
			amount := decimal.NewFromInt(contract.Parameter.Value.Amount).Div(decimal.NewFromInt(1000000))

			fromAddr := contract.Parameter.Value.OwnerAddress
			if strings.HasPrefix(fromAddr, "41") {
				fromAddr = hexToBase58(fromAddr)
			}

			transfers = append(transfers, Transfer{
				TxHash:      tx.TxID,
				From:        fromAddr,
				To:          toAddr,
				Amount:      amount,
				BlockNumber: uint64(tx.BlockNumber),
				Chain:       "trx",
			})
		}
	}

	return transfers, nil
}

// hexToBase58 moved to blockchain_utils.go

// scanTRC20 扫描TRC20交易
func (s *BlockchainService) scanTRC20(listener *ChainListener, addresses map[string]bool) ([]Transfer, error) {
	var transfers []Transfer

	for addr := range addresses {
		url := fmt.Sprintf("%s/v1/accounts/%s/transactions/trc20?only_confirmed=true&limit=50&contract_address=%s",
			listener.rpc, addr, listener.contractAddress)

		resp, err := httpClient.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var result struct {
			Data []struct {
				TransactionID string `json:"transaction_id"`
				From          string `json:"from"`
				To            string `json:"to"`
				Value         string `json:"value"`
				BlockTimestamp int64  `json:"block_timestamp"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		for _, tx := range result.Data {
			// 检查是否是转入交易
			if !addresses[strings.ToLower(tx.To)] {
				continue
			}

			// 检查是否已处理
			var count int64
			model.GetDB().Model(&model.TransactionLog{}).Where("tx_hash = ?", tx.TransactionID).Count(&count)
			if count > 0 {
				continue
			}

			// USDT TRC20 精度是6位
			amount := parseTokenAmount(tx.Value, 6)

			transfers = append(transfers, Transfer{
				TxHash: tx.TransactionID,
				From:   tx.From,
				To:     tx.To,
				Amount: amount,
				Chain:  "trc20",
			})
		}
	}

	return transfers, nil
}

// scanEVM 扫描EVM兼容链交易 (ERC20, BEP20, Polygon)
func (s *BlockchainService) scanEVM(listener *ChainListener, addresses map[string]bool) ([]Transfer, error) {
	var transfers []Transfer

	// 获取最新区块号
	currentBlock, err := s.getEVMBlockNumber(listener.rpc)
	if err != nil {
		return nil, err
	}

	// 计算安全区块
	safeBlock := currentBlock - uint64(listener.confirmations)
	if listener.lastBlock == 0 {
		listener.lastBlock = safeBlock - 100 // 首次启动，扫描最近100个区块
	}

	if listener.lastBlock >= safeBlock {
		return nil, nil
	}

	// 构建日志过滤请求
	// Transfer事件签名: 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
	transferTopic := "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

	for addr := range addresses {
		// 将地址填充到32字节
		paddedAddr := fmt.Sprintf("0x%064s", strings.TrimPrefix(strings.ToLower(addr), "0x"))
		paddedAddr = strings.Replace(paddedAddr, " ", "0", -1)

		params := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "eth_getLogs",
			"params": []interface{}{
				map[string]interface{}{
					"fromBlock": fmt.Sprintf("0x%x", listener.lastBlock+1),
					"toBlock":   fmt.Sprintf("0x%x", safeBlock),
					"address":   listener.contractAddress,
					"topics": []interface{}{
						transferTopic,
						nil, // from address (any)
						paddedAddr, // to address
					},
				},
			},
			"id": 1,
		}

		reqBody, _ := json.Marshal(params)
		resp, err := httpClient.Post(listener.rpc, "application/json", bytes.NewReader(reqBody))
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var result struct {
			Result []struct {
				TransactionHash string   `json:"transactionHash"`
				Topics          []string `json:"topics"`
				Data            string   `json:"data"`
				BlockNumber     string   `json:"blockNumber"`
			} `json:"result"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		for _, log := range result.Result {
			// 解析from地址
			from := "0x" + log.Topics[1][26:]

			// 解析金额
			amount := parseHexAmount(log.Data, 6) // USDT精度是6位

			// 检查是否已处理
			var count int64
			model.GetDB().Model(&model.TransactionLog{}).Where("tx_hash = ?", log.TransactionHash).Count(&count)
			if count > 0 {
				continue
			}

			blockNum := parseHexUint64(log.BlockNumber)

			transfers = append(transfers, Transfer{
				TxHash:      log.TransactionHash,
				From:        from,
				To:          addr,
				Amount:      amount,
				BlockNumber: blockNum,
				Chain:       listener.chain,
			})
		}
	}

	listener.lastBlock = safeBlock
	return transfers, nil
}

// getEVMBlockNumber 获取EVM链最新区块号
func (s *BlockchainService) getEVMBlockNumber(rpc string) (uint64, error) {
	params := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []interface{}{},
		"id":      1,
	}

	reqBody, _ := json.Marshal(params)
	resp, err := httpClient.Post(rpc, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Result string `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	return parseHexUint64(result.Result), nil
}

// processTransfer 处理转账事件
func (s *BlockchainService) processTransfer(transfer Transfer) {
	// 检查交易是否已处理（防重复）
	var existingLog model.TransactionLog
	if err := model.GetDB().Where("tx_hash = ?", transfer.TxHash).First(&existingLog).Error; err == nil {
		// 交易已存在，跳过处理
		return
	}

	// 记录交易日志
	txLog := model.TransactionLog{
		Chain:       transfer.Chain,
		TxHash:      transfer.TxHash,
		FromAddress: transfer.From,
		ToAddress:   transfer.To,
		Amount:      transfer.Amount.String(),
		BlockNumber: transfer.BlockNumber,
		Matched:     false,
	}

	if err := model.GetDB().Create(&txLog).Error; err != nil {
		log.Printf("Failed to create transaction log: %v", err)
		return
	}

	// 查找匹配的订单
	order := s.matchOrder(transfer)
	if order != nil {
		// 再次检查订单状态，防止并发重复处理
		if order.Status != model.OrderStatusPending {
			log.Printf("Order %s already processed, status: %d", order.TradeNo, order.Status)
			return
		}

		// 更新订单状态（使用乐观锁）
		now := time.Now()

		// 实际支付金额也需要截断到标准精度（与unique_amount一致）
		var actualAmount decimal.Decimal
		if transfer.Chain == "wechat" || transfer.Chain == "alipay" {
			actualAmount = transfer.Amount.Round(2) // 法币2位
		} else {
			actualAmount = transfer.Amount.Round(6) // 加密货币6位
		}

		updates := map[string]interface{}{
			"status":        model.OrderStatusPaid,
			"tx_hash":       transfer.TxHash,
			"from_address":  transfer.From,
			"actual_amount": actualAmount,
			"paid_at":       &now,
		}

		// 使用 WHERE 条件确保只更新待支付的订单
		result := model.GetDB().Model(order).
			Where("status = ?", model.OrderStatusPending).
			Updates(updates)

		if result.Error != nil {
			log.Printf("Failed to update order: %v", result.Error)
			return
		}

		// 如果没有更新任何行，说明订单已被其他进程处理
		if result.RowsAffected == 0 {
			log.Printf("Order %s already processed by another process", order.TradeNo)
			return
		}

		// 更新交易日志
		model.GetDB().Model(&txLog).Updates(map[string]interface{}{
			"matched":  true,
			"order_id": order.ID,
		})

		log.Printf("Order %s matched with tx %s, amount: %s", order.TradeNo, transfer.TxHash, transfer.Amount)

		// 记录订单匹配
		s.metrics.RecordOrderMatch(transfer.Chain)

		// 增加商户余额（使用 USD 结算金额）
		settlementAmount, _ := order.SettlementAmount.Float64()
		fee, _ := order.Fee.Float64()
		if err := GetWithdrawService().AddMerchantBalance(order.MerchantID, settlementAmount, fee, order.FeeType); err != nil {
			log.Printf("Failed to add merchant balance for order %s: %v", order.TradeNo, err)
		}

		// 触发回调通知
		go GetNotifyService().NotifyOrder(order.ID)

		// 发送Telegram通知 - 订单支付成功
		go GetTelegramService().NotifyOrderPaid(order)
	}
}

// matchOrder 匹配订单
func (s *BlockchainService) matchOrder(transfer Transfer) *model.Order {
	var order model.Order

	// 确定链的标准精度并截断金额
	// TRC20/ERC20等: 6位小数
	// BEP20: 虽然链上是18位，但我们统一按6位处理
	// TRX: 6位小数
	// 法币: 2位小数
	var normalizedAmount decimal.Decimal
	if transfer.Chain == "wechat" || transfer.Chain == "alipay" {
		normalizedAmount = transfer.Amount.Round(2) // 法币2位
	} else {
		normalizedAmount = transfer.Amount.Round(6) // 加密货币6位
	}

	// 精确匹配唯一标识金额（无容差）
	// 加密货币: 102.040023 USDT
	// 法币: 100.01 CNY
	err := model.GetDB().
		Where("chain = ? AND to_address = ? AND unique_amount = ? AND status = ?",
			transfer.Chain,
			strings.ToLower(transfer.To),
			normalizedAmount,
			model.OrderStatusPending).
		Order("created_at ASC").
		First(&order).Error

	if err != nil {
		// 兼容旧订单：尝试匹配 usdt_amount (旧字段)
		err = model.GetDB().
			Where("chain = ? AND to_address = ? AND usdt_amount = ? AND status = ?",
				transfer.Chain,
				strings.ToLower(transfer.To),
				normalizedAmount,
				model.OrderStatusPending).
			Order("created_at ASC").
			First(&order).Error

		if err != nil {
			return nil
		}
	}

	return &order
}

// GetListenerStatus 获取监听器状态
func (s *BlockchainService) GetListenerStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 查询每个链的钱包数量
	walletCounts := make(map[string]int64)
	var results []struct {
		Chain string
		Count int64
	}
	model.GetDB().Model(&model.Wallet{}).
		Select("chain, COUNT(*) as count").
		Where("status = 1 AND deleted_at IS NULL").
		Group("chain").
		Scan(&results)
	for _, r := range results {
		walletCounts[r.Chain] = r.Count
	}

	status := make(map[string]interface{})
	for chain, listener := range s.listeners {
		listener.mu.Lock()
		status[chain] = map[string]interface{}{
			"enabled":      listener.enabled,
			"running":      listener.running,
			"wallet_count": walletCounts[chain],
			"rpc":          listener.rpc,
			"contract":     listener.contractAddress,
			"interval":     listener.scanInterval,
			"passive":      false, // 区块链需要主动监控
		}
		listener.mu.Unlock()
	}

	// 添加微信、支付宝（被动推送模式）
	// 从数据库读取启用状态
	wechatEnabled := true
	alipayEnabled := true

	var wechatConfig model.SystemConfig
	if model.GetDB().Where("`key` = ?", model.ConfigKeyWechatEnabled).First(&wechatConfig).Error == nil {
		wechatEnabled = wechatConfig.Value == "1" || wechatConfig.Value == "true"
	}

	var alipayConfig model.SystemConfig
	if model.GetDB().Where("`key` = ?", model.ConfigKeyAlipayEnabled).First(&alipayConfig).Error == nil {
		alipayEnabled = alipayConfig.Value == "1" || alipayConfig.Value == "true"
	}

	status["wechat"] = map[string]interface{}{
		"enabled":      wechatEnabled,
		"running":      wechatEnabled,
		"wallet_count": walletCounts["wechat"],
		"passive":      true, // 被动推送模式
	}
	status["alipay"] = map[string]interface{}{
		"enabled":      alipayEnabled,
		"running":      alipayEnabled,
		"wallet_count": walletCounts["alipay"],
		"passive":      true, // 被动推送模式
	}

	return status
}

// EnableChain 启用链监控
func (s *BlockchainService) EnableChain(chain string) error {
	// 处理微信、支付宝的特殊情况（被动推送渠道）
	if chain == "wechat" || chain == "alipay" {
		return s.setPassiveChannelEnabled(chain, true)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	listener, ok := s.listeners[chain]
	if !ok {
		return fmt.Errorf("unknown chain: %s", chain)
	}

	listener.mu.Lock()
	defer listener.mu.Unlock()

	if listener.enabled && listener.running {
		return nil // 已经在运行
	}

	listener.enabled = true

	// 如果服务已启动但监听器未运行，则启动它
	if !listener.running {
		listener.stopCh = make(chan struct{}) // 重新创建停止通道
		s.wg.Add(1)
		go s.runListener(chain, listener)
	}

	log.Printf("Chain %s enabled", chain)
	return nil
}

// DisableChain 禁用链监控
func (s *BlockchainService) DisableChain(chain string) error {
	// 处理微信、支付宝的特殊情况（被动推送渠道）
	if chain == "wechat" || chain == "alipay" {
		return s.setPassiveChannelEnabled(chain, false)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	listener, ok := s.listeners[chain]
	if !ok {
		return fmt.Errorf("unknown chain: %s", chain)
	}

	listener.mu.Lock()

	if !listener.enabled {
		listener.mu.Unlock()
		return nil // 已经禁用
	}

	listener.enabled = false

	// 如果监听器正在运行，停止它
	if listener.running {
		close(listener.stopCh)
	}
	listener.mu.Unlock()

	log.Printf("Chain %s disabled", chain)
	return nil
}

// setPassiveChannelEnabled 设置被动推送渠道启用状态
func (s *BlockchainService) setPassiveChannelEnabled(channel string, enabled bool) error {
	var configKey string
	if channel == "wechat" {
		configKey = model.ConfigKeyWechatEnabled
	} else if channel == "alipay" {
		configKey = model.ConfigKeyAlipayEnabled
	} else {
		return fmt.Errorf("unknown passive channel: %s", channel)
	}

	value := "0"
	if enabled {
		value = "1"
	}

	// 更新数据库配置
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", configKey).First(&config).Error; err != nil {
		// 不存在，创建新记录
		config = model.SystemConfig{
			Key:         configKey,
			Value:       value,
			Description: channel + " 支付启用状态",
		}
		if err := model.GetDB().Create(&config).Error; err != nil {
			return err
		}
	} else {
		// 存在，更新
		if err := model.GetDB().Model(&config).Update("value", value).Error; err != nil {
			return err
		}
	}

	log.Printf("Passive channel %s enabled: %v", channel, enabled)
	return nil
}

// IsChainEnabled 检查链是否启用
func (s *BlockchainService) IsChainEnabled(chain string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	listener, ok := s.listeners[chain]
	if !ok {
		return false
	}

	listener.mu.Lock()
	defer listener.mu.Unlock()
	return listener.enabled
}

// GetEnabledChains 获取所有已启用的链
func (s *BlockchainService) GetEnabledChains() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var chains []string
	for chain, listener := range s.listeners {
		listener.mu.Lock()
		if listener.enabled {
			chains = append(chains, chain)
		}
		listener.mu.Unlock()
	}
	return chains
}

// GetChainStatus 获取链状态 (简化版，用于商户查看)
func (s *BlockchainService) GetChainStatus() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := make(map[string]bool)
	for chain, listener := range s.listeners {
		listener.mu.Lock()
		status[chain] = listener.enabled
		listener.mu.Unlock()
	}

	// 添加被动渠道（微信/支付宝）状态
	var wechatConfig model.SystemConfig
	if model.GetDB().Where("`key` = ?", model.ConfigKeyWechatEnabled).First(&wechatConfig).Error == nil {
		status["wechat"] = wechatConfig.Value == "1" || wechatConfig.Value == "true"
	} else {
		status["wechat"] = true // 默认启用
	}

	var alipayConfig model.SystemConfig
	if model.GetDB().Where("`key` = ?", model.ConfigKeyAlipayEnabled).First(&alipayConfig).Error == nil {
		status["alipay"] = alipayConfig.Value == "1" || alipayConfig.Value == "true"
	} else {
		status["alipay"] = true // 默认启用
	}

	return status
}

// parseTokenAmount 解析代币金额

// parseHexAmount 解析十六进制金额

// parseHexUint64 解析十六进制数字

// adjustScanInterval 动态调整扫描间隔
func (s *BlockchainService) adjustScanInterval(listener *ChainListener, transferCount int) {
	listener.mu.Lock()
	defer listener.mu.Unlock()

	// 如果发现交易，减小扫描间隔
	if transferCount > 0 {
		listener.scanInterval = listener.baseScanInterval / 2
		if listener.scanInterval < 5 {
			listener.scanInterval = 5 // 最小 5 秒
		}
	} else {
		// 没有交易，逐渐恢复到基础间隔
		listener.scanInterval = listener.baseScanInterval
	}
}

// monitorGasPrices 监控 Gas 价格
func (s *BlockchainService) monitorGasPrices() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.updateGasPrices()
		}
	}
}

// updateGasPrices 更新 Gas 价格
func (s *BlockchainService) updateGasPrices() {
	evmChains := []string{"erc20", "bep20", "polygon", "optimism", "arbitrum", "avalanche", "base"}

	for _, chain := range evmChains {
		listener := s.listeners[chain]
		if listener == nil || !listener.enabled {
			continue
		}

		gasPrice, err := s.getGasPrice(chain, listener.rpc)
		if err != nil {
			log.Printf("[%s] Failed to get gas price: %v", chain, err)
			continue
		}

		s.gasPriceMu.Lock()
		s.gasPrices[chain] = gasPrice
		s.gasPriceMu.Unlock()

		log.Printf("[%s] Gas price updated: %.2f Gwei", chain, gasPrice)
	}
}

// getGasPrice 获取 Gas 价格
func (s *BlockchainService) getGasPrice(chain string, rpc string) (float64, error) {
	rpcClient := s.rpcClients[chain]
	if rpcClient == nil {
		return 0, fmt.Errorf("RPC client not found for %s", chain)
	}

	params := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_gasPrice",
		"params":  []interface{}{},
		"id":      1,
	}

	body, err := rpcClient.PostJSON("", params)
	if err != nil {
		return 0, err
	}

	var result struct {
		Result string `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	// 解析 hex 值并转换为 Gwei
	value := new(big.Int)
	value.SetString(strings.TrimPrefix(result.Result, "0x"), 16)

	// 转换为 Gwei (1 Gwei = 10^9 Wei)
	gwei := new(big.Float).SetInt(value)
	gwei.Quo(gwei, big.NewFloat(1e9))

	gweiFloat, _ := gwei.Float64()
	return gweiFloat, nil
}

// GetGasPrice 获取 Gas 价格（公开方法）
func (s *BlockchainService) GetGasPrice(chain string) float64 {
	s.gasPriceMu.RLock()
	defer s.gasPriceMu.RUnlock()
	return s.gasPrices[chain]
}

// GetMetrics 获取监控指标
func (s *BlockchainService) GetMetrics() map[string]interface{} {
	if s.metrics == nil {
		return nil
	}
	return s.metrics.GetMetrics()
}

// GetChainMetrics 获取指定链的监控指标
func (s *BlockchainService) GetChainMetrics(chain string) map[string]interface{} {
	if s.metrics == nil {
		return nil
	}
	return s.metrics.GetChainMetrics(chain)
}

// detectReorg 检测区块重组
func (s *BlockchainService) detectReorg(listener *ChainListener, currentBlock uint64) bool {
	listener.mu.Lock()
	defer listener.mu.Unlock()

	// 如果没有历史记录，添加当前区块
	if len(listener.blockHistory) == 0 {
		listener.blockHistory = append(listener.blockHistory, currentBlock)
		return false
	}

	lastBlock := listener.blockHistory[len(listener.blockHistory)-1]

	// 如果当前区块小于或等于最后记录的区块，可能发生重组
	if currentBlock <= lastBlock {
		log.Printf("[%s] Potential reorg detected: current=%d, last=%d", 
			listener.chain, currentBlock, lastBlock)
		
		// 清空历史，重新扫描
		listener.blockHistory = []uint64{currentBlock}
		listener.lastBlock = currentBlock - uint64(listener.reorgDepth)
		if listener.lastBlock < 0 {
			listener.lastBlock = 0
		}
		return true
	}

	// 添加到历史
	listener.blockHistory = append(listener.blockHistory, currentBlock)

	// 只保留最近的 20 个区块
	if len(listener.blockHistory) > 20 {
		listener.blockHistory = listener.blockHistory[len(listener.blockHistory)-20:]
	}

	return false
}
