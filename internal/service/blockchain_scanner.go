package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ezpay/internal/model"

	"github.com/shopspring/decimal"
)

// scanTRXImproved 改进的 TRX 扫描（使用 RPC 客户端和地址转换）
func (s *BlockchainService) scanTRXImproved(listener *ChainListener, addresses map[string]bool) ([]Transfer, error) {
	var transfers []Transfer
	rpcClient := s.rpcClients[listener.chain]

	for addr := range addresses {
		// 标准化地址（确保是 base58 格式）
		addr = normalizeAddress(addr, listener.chain)

		path := fmt.Sprintf("/v1/accounts/%s/transactions?only_confirmed=true&limit=200", addr)

		// 使用支持重试的 RPC 客户端
		body, err := fetchTronData(rpcClient, path)
		if err != nil {
			log.Printf("[trx] Failed to get transactions for %s: %v", addr, err)
			s.metrics.RecordRPCCall("trx", false, 0)
			continue
		}

		s.metrics.RecordRPCCall("trx", true, 0)

		var result struct {
			Data []struct {
				TxID        string `json:"txID"`
				BlockNumber int64  `json:"blockNumber"`
				RawData     struct {
					Timestamp int64 `json:"timestamp"`
					Contract  []struct {
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
			log.Printf("[trx] Failed to unmarshal response for %s: %v", addr, err)
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

			// 转换地址格式（使用改进的 hexToBase58）
			toAddr := contract.Parameter.Value.ToAddress
			if strings.HasPrefix(toAddr, "41") {
				toAddr = hexToBase58(toAddr)
			}
			// 检查是否是转入交易（wallet cache 使用小写 key）
			if !addresses[normalizeAddress(toAddr, listener.chain)] {
				continue
			}

			// 检查是否已处理（记录重复）
			var count int64
			model.GetDB().Model(&model.TransactionLog{}).Where("tx_hash = ?", tx.TxID).Count(&count)
			if count > 0 {
				s.metrics.RecordDuplicateTx("trx")
				continue
			}

			// TRX精度是6位 (1 TRX = 1,000,000 sun)
			amount := decimal.NewFromInt(contract.Parameter.Value.Amount).Div(decimal.NewFromInt(1000000))

			fromAddr := contract.Parameter.Value.OwnerAddress
			if strings.HasPrefix(fromAddr, "41") {
				fromAddr = hexToBase58(fromAddr)
			}

			transfers = append(transfers, Transfer{
				Timestamp:   time.UnixMilli(tx.RawData.Timestamp),
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

// scanTRC20Improved 改进的 TRC20 扫描
func (s *BlockchainService) scanTRC20Improved(listener *ChainListener, addresses map[string]bool) ([]Transfer, error) {
	var transfers []Transfer
	rpcClient := s.rpcClients[listener.chain]

	for addr := range addresses {
		addr = normalizeAddress(addr, listener.chain)

		path := fmt.Sprintf("/v1/accounts/%s/transactions/trc20?only_confirmed=true&limit=200&contract_address=%s",
			addr, listener.contractAddress)

		body, err := fetchTronData(rpcClient, path)
		if err != nil {
			log.Printf("[trc20] Failed to get transactions for %s: %v", addr, err)
			s.metrics.RecordRPCCall("trc20", false, 0)
			continue
		}

		s.metrics.RecordRPCCall("trc20", true, 0)

		var result struct {
			Data []struct {
				TransactionID  string `json:"transaction_id"`
				From           string `json:"from"`
				To             string `json:"to"`
				Value          string `json:"value"`
				BlockTimestamp int64  `json:"block_timestamp"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			log.Printf("[trc20] Failed to unmarshal response for %s: %v", addr, err)
			continue
		}

		for _, tx := range result.Data {
			// 检查是否是转入交易（wallet cache 使用小写 key）
			if !addresses[normalizeAddress(tx.To, listener.chain)] {
				continue
			}

			// 检查是否已处理
			var count int64
			model.GetDB().Model(&model.TransactionLog{}).Where("tx_hash = ?", tx.TransactionID).Count(&count)
			if count > 0 {
				s.metrics.RecordDuplicateTx("trc20")
				continue
			}

			// USDT TRC20 精度是6位
			amount := parseTokenAmount(tx.Value, 6)

			transfers = append(transfers, Transfer{
				Timestamp: time.UnixMilli(tx.BlockTimestamp),
				TxHash:    tx.TransactionID,
				From:      tx.From,
				To:        tx.To,
				Amount:    amount,
				Chain:     "trc20",
			})
		}
	}

	return transfers, nil
}

func fetchTronData(client *RPCClient, path string) ([]byte, error) {
	var all []json.RawMessage
	seen := make(map[string]bool)
	base, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	for page := 0; page < 100; page++ {
		if seen[path] {
			return nil, fmt.Errorf("repeated TRON pagination cursor")
		}
		seen[path] = true
		resp, err := client.Get(path)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != 200 || len(body) > 2<<20 {
			return nil, fmt.Errorf("invalid TRON response")
		}
		var result struct {
			Data    []json.RawMessage `json:"data"`
			Success *bool             `json:"success"`
			Meta    struct {
				Links struct {
					Next string `json:"next"`
				} `json:"links"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		if result.Data == nil || (result.Success != nil && !*result.Success) {
			return nil, fmt.Errorf("TRON query rejected")
		}
		all = append(all, result.Data...)
		if result.Meta.Links.Next == "" {
			return json.Marshal(map[string]interface{}{"data": all})
		}
		next, err := url.Parse(result.Meta.Links.Next)
		if err != nil || next.Path != base.Path {
			return nil, fmt.Errorf("invalid TRON pagination path")
		}
		path = next.RequestURI()
	}
	return nil, fmt.Errorf("TRON pagination limit reached; checkpoint retained")
}

// scanEVMImproved 改进的 EVM 扫描（支持重组检测）
func (s *BlockchainService) scanEVMImproved(listener *ChainListener, addresses map[string]bool) ([]Transfer, error) {
	var transfers []Transfer
	rpcClient := s.rpcClients[listener.chain]

	// 获取最新区块号
	currentBlock, err := s.getEVMBlockNumberWithRetry(rpcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get block number: %w", err)
	}

	// 检测区块重组
	if s.detectReorg(listener, currentBlock) {
		log.Printf("[%s] Reorg detected, rescanning from block %d", listener.chain, listener.lastBlock)
	}

	// 更新区块高度指标
	s.metrics.UpdateBlockHeight(listener.chain, currentBlock, listener.lastBlock)

	// 计算安全区块
	if listener.confirmations < 0 || currentBlock <= uint64(listener.confirmations) {
		return nil, nil
	}
	safeBlock := currentBlock - uint64(listener.confirmations)
	if listener.lastBlock == 0 {
		if safeBlock > 100 {
			listener.lastBlock = safeBlock - 100
		}
	}

	if listener.lastBlock >= safeBlock {
		return nil, nil
	}

	// 从配置读取查询参数，未配置则使用链类型默认值
	var defaultBlockRange, defaultBatchSize, defaultDelayMs int
	switch listener.chain {
	case "polygon":
		defaultBlockRange, defaultBatchSize, defaultDelayMs = 100, 2, 500
	case "bep20":
		defaultBlockRange, defaultBatchSize, defaultDelayMs = 500, 1, 2000
	case "arbitrum", "optimism", "base":
		defaultBlockRange, defaultBatchSize, defaultDelayMs = 2000, 5, 200
	default:
		defaultBlockRange, defaultBatchSize, defaultDelayMs = 1000, 3, 300
	}

	maxBlockRange := uint64(defaultBlockRange)
	if listener.maxBlockRange > 0 {
		maxBlockRange = uint64(listener.maxBlockRange)
	}
	maxBatchSize := defaultBatchSize
	if listener.maxBatchSize > 0 {
		maxBatchSize = listener.maxBatchSize
	}
	batchDelay := time.Duration(defaultDelayMs) * time.Millisecond
	if listener.batchDelayMs > 0 {
		batchDelay = time.Duration(listener.batchDelayMs) * time.Millisecond
	}

	queryToBlock := safeBlock
	if queryToBlock-listener.lastBlock > maxBlockRange {
		queryToBlock = listener.lastBlock + maxBlockRange
		log.Printf("[%s] 区块范围过大，限制本次查询: %d -> %d (剩余 %d 块待追赶)",
			listener.chain, listener.lastBlock+1, queryToBlock, safeBlock-queryToBlock)
	}

	// Transfer事件签名
	transferTopic := "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

	// 批量查询优化：收集所有地址的查询
	var batchRequests []BatchRequest
	requestID := 1

	for addr := range addresses {
		// 填充地址到32字节
		paddedAddr := fmt.Sprintf("0x%064s", strings.TrimPrefix(strings.ToLower(addr), "0x"))
		paddedAddr = strings.Replace(paddedAddr, " ", "0", -1)

		batchRequests = append(batchRequests, BatchRequest{
			JSONRPC: "2.0",
			Method:  "eth_getLogs",
			Params: []interface{}{
				map[string]interface{}{
					"fromBlock": fmt.Sprintf("0x%x", listener.lastBlock+1),
					"toBlock":   fmt.Sprintf("0x%x", queryToBlock),
					"address":   listener.contractAddress,
					"topics": []interface{}{
						transferTopic,
						nil,
						paddedAddr,
					},
				},
			},
			ID: requestID,
		})
		requestID++
	}

	// 解析日志响应的公共函数
	blockTimes := make(map[string]time.Time)
	parseLogResults := func(resultData json.RawMessage) error {
		var logs []struct {
			TransactionHash string   `json:"transactionHash"`
			Topics          []string `json:"topics"`
			Data            string   `json:"data"`
			BlockNumber     string   `json:"blockNumber"`
			LogIndex        string   `json:"logIndex"`
		}

		if err := json.Unmarshal(resultData, &logs); err != nil {
			log.Printf("[%s] Failed to unmarshal logs: %v", listener.chain, err)
			return err
		}

		for _, logEntry := range logs {
			// ERC20 Transfer 事件需要 3 个 Topics: event sig, from, to
			if len(logEntry.Topics) != 3 || len(logEntry.Topics[1]) != 66 || len(logEntry.Topics[2]) != 66 || logEntry.TransactionHash == "" {
				log.Printf("[%s] Skipping log with insufficient topics (tx: %s, topics: %d)",
					listener.chain, logEntry.TransactionHash, len(logEntry.Topics))
				return fmt.Errorf("malformed transfer log")
			}

			// 解析from地址
			from := "0x" + logEntry.Topics[1][26:]

			// 解析to地址
			to := "0x" + logEntry.Topics[2][26:]

			// 解析金额（BEP20 USDT 是 18 位，其它 EVM 链 USDT 是 6 位）
			amount := parseHexAmount(logEntry.Data, chainTokenDecimals(listener.chain))

			blockNum := parseHexUint64(logEntry.BlockNumber)
			if logEntry.LogIndex == "" {
				return fmt.Errorf("missing transfer log index")
			}
			stamp, exists := blockTimes[logEntry.BlockNumber]
			if !exists {
				var err error
				stamp, err = s.evmBlockTime(rpcClient, logEntry.BlockNumber)
				if err != nil {
					return err
				}
				blockTimes[logEntry.BlockNumber] = stamp
			}

			transfers = append(transfers, Transfer{
				EventID:     logEntry.TransactionHash + ":" + logEntry.LogIndex,
				Timestamp:   stamp,
				TxHash:      logEntry.TransactionHash,
				From:        from,
				To:          to,
				Amount:      amount,
				BlockNumber: blockNum,
				Chain:       listener.chain,
			})
		}
		return nil
	}

	// 发送请求
	if len(batchRequests) > 0 {
		if maxBatchSize <= 1 {
			// 逐个发送单独的 JSON-RPC 请求（不使用批量格式）
			// BSC 等链的公共节点不允许 eth_getLogs 出现在批量请求中
			for i, req := range batchRequests {
				respBody, err := rpcClient.PostJSON("", req)
				if err != nil {
					log.Printf("[%s] RPC call failed: %v", listener.chain, err)
					s.metrics.RecordRPCCall(listener.chain, false, 0)
					return nil, err
				}

				s.metrics.RecordRPCCall(listener.chain, true, 0)

				var resp BatchResponse
				if err := json.Unmarshal(respBody, &resp); err != nil {
					log.Printf("[%s] Failed to unmarshal response: %v", listener.chain, err)
					return nil, err
				}

				if resp.Error != nil {
					log.Printf("[%s] RPC error: %s", listener.chain, resp.Error.Message)
					return nil, fmt.Errorf("RPC log query: %s", resp.Error.Message)
				}

				if err := parseLogResults(resp.Result); err != nil {
					return nil, err
				}

				// 请求之间等待
				if i < len(batchRequests)-1 {
					time.Sleep(batchDelay)
				}
			}
		} else {
			// 批量发送请求（分批，避免触发速率限制）
			for i := 0; i < len(batchRequests); i += maxBatchSize {
				end := i + maxBatchSize
				if end > len(batchRequests) {
					end = len(batchRequests)
				}
				chunk := batchRequests[i:end]

				responses, err := rpcClient.BatchPostJSON("", chunk)
				if err != nil {
					log.Printf("[%s] Batch RPC call failed: %v", listener.chain, err)
					s.metrics.RecordRPCCall(listener.chain, false, 0)
					return nil, err
				}

				s.metrics.RecordRPCCall(listener.chain, true, 0)
				if len(responses) != len(chunk) {
					return nil, fmt.Errorf("incomplete RPC batch response")
				}
				seen := make(map[int]bool)
				for _, resp := range responses {
					valid := false
					for _, req := range chunk {
						if req.ID == resp.ID {
							valid = true
							break
						}
					}
					if !valid || seen[resp.ID] {
						return nil, fmt.Errorf("invalid RPC batch identity")
					}
					seen[resp.ID] = true
				}

				for _, resp := range responses {
					if resp.Error != nil {
						log.Printf("[%s] RPC error in batch response: %s", listener.chain, resp.Error.Message)
						return nil, fmt.Errorf("RPC log query: %s", resp.Error.Message)
					}
					if err := parseLogResults(resp.Result); err != nil {
						return nil, err
					}
				}

				// 批次之间等待
				if end < len(batchRequests) {
					time.Sleep(batchDelay)
				}
			}
		}
	}

	listener.lastBlock = queryToBlock
	return transfers, nil
}

func (s *BlockchainService) evmBlockTime(client *RPCClient, block string) (time.Time, error) {
	body, err := client.PostJSON("", map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "eth_getBlockByNumber", "params": []interface{}{block, false}})
	if err != nil {
		return time.Time{}, err
	}
	var response struct {
		Result *struct {
			Timestamp string `json:"timestamp"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return time.Time{}, err
	}
	if response.Result == nil || !strings.HasPrefix(response.Result.Timestamp, "0x") {
		return time.Time{}, fmt.Errorf("missing block timestamp")
	}
	stamp, err := strconv.ParseInt(strings.TrimPrefix(response.Result.Timestamp, "0x"), 16, 64)
	if err != nil || stamp <= 0 {
		return time.Time{}, fmt.Errorf("invalid block timestamp")
	}
	return time.Unix(stamp, 0), nil
}

// getEVMBlockNumberWithRetry 获取 EVM 区块号（带重试）
func (s *BlockchainService) getEVMBlockNumberWithRetry(rpcClient *RPCClient) (uint64, error) {
	params := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []interface{}{},
		"id":      1,
	}

	body, err := rpcClient.PostJSON("", params)
	if err != nil {
		return 0, err
	}

	var result struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if result.Error != nil || !strings.HasPrefix(result.Result, "0x") {
		return 0, fmt.Errorf("invalid block number response")
	}
	return strconv.ParseUint(strings.TrimPrefix(result.Result, "0x"), 16, 64)
}
