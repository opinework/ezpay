package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

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

		path := fmt.Sprintf("/v1/accounts/%s/transactions?only_confirmed=true&limit=50", addr)

		// 使用支持重试的 RPC 客户端
		resp, err := rpcClient.Get(path)
		if err != nil {
			log.Printf("[trx] Failed to get transactions for %s: %v", addr, err)
			s.metrics.RecordRPCCall("trx", false, 0)
			continue
		}
		defer resp.Body.Close()

		s.metrics.RecordRPCCall("trx", true, 0)

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[trx] Failed to read response for %s: %v", addr, err)
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
			toAddr = strings.ToLower(toAddr)

			// 检查是否是转入交易
			if !addresses[toAddr] {
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

		path := fmt.Sprintf("/v1/accounts/%s/transactions/trc20?only_confirmed=true&limit=50&contract_address=%s",
			addr, listener.contractAddress)

		resp, err := rpcClient.Get(path)
		if err != nil {
			log.Printf("[trc20] Failed to get transactions for %s: %v", addr, err)
			s.metrics.RecordRPCCall("trc20", false, 0)
			continue
		}
		defer resp.Body.Close()

		s.metrics.RecordRPCCall("trc20", true, 0)

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[trc20] Failed to read response for %s: %v", addr, err)
			continue
		}

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
			// 标准化地址
			toAddr := strings.ToLower(tx.To)

			// 检查是否是转入交易
			if !addresses[toAddr] {
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
				TxHash: tx.TransactionID,
				From:   tx.From,
				To:     toAddr,
				Amount: amount,
				Chain:  "trc20",
			})
		}
	}

	return transfers, nil
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
	safeBlock := currentBlock - uint64(listener.confirmations)
	if listener.lastBlock == 0 {
		listener.lastBlock = safeBlock - 100 // 首次启动，扫描最近100个区块
	}

	if listener.lastBlock >= safeBlock {
		return nil, nil
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
					"toBlock":   fmt.Sprintf("0x%x", safeBlock),
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

	// 批量发送请求
	if len(batchRequests) > 0 {
		responses, err := rpcClient.BatchPostJSON("", batchRequests)
		if err != nil {
			log.Printf("[%s] Batch RPC call failed: %v", listener.chain, err)
			s.metrics.RecordRPCCall(listener.chain, false, 0)
			return nil, err
		}

		s.metrics.RecordRPCCall(listener.chain, true, 0)

		// 处理批量响应
		for _, resp := range responses {
			if resp.Error != nil {
				log.Printf("[%s] RPC error in batch response: %s", listener.chain, resp.Error.Message)
				continue
			}

			var logs []struct {
				TransactionHash string   `json:"transactionHash"`
				Topics          []string `json:"topics"`
				Data            string   `json:"data"`
				BlockNumber     string   `json:"blockNumber"`
			}

			if err := json.Unmarshal(resp.Result, &logs); err != nil {
				log.Printf("[%s] Failed to unmarshal logs: %v", listener.chain, err)
				continue
			}

			for _, logEntry := range logs {
				// 解析from地址
				from := "0x" + logEntry.Topics[1][26:]

				// 解析to地址
				to := "0x" + logEntry.Topics[2][26:]

				// 解析金额
				amount := parseHexAmount(logEntry.Data, 6) // USDT精度是6位

				// 检查是否已处理
				var count int64
				model.GetDB().Model(&model.TransactionLog{}).Where("tx_hash = ?", logEntry.TransactionHash).Count(&count)
				if count > 0 {
					s.metrics.RecordDuplicateTx(listener.chain)
					continue
				}

				blockNum := parseHexUint64(logEntry.BlockNumber)

				transfers = append(transfers, Transfer{
					TxHash:      logEntry.TransactionHash,
					From:        from,
					To:          to,
					Amount:      amount,
					BlockNumber: blockNum,
					Chain:       listener.chain,
				})
			}
		}
	}

	listener.lastBlock = safeBlock
	return transfers, nil
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
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	return parseHexUint64(result.Result), nil
}
