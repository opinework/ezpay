package service

import (
	"fmt"
	"sync"
	"time"
)

// BlockchainMetrics 区块链监控指标
type BlockchainMetrics struct {
	mu sync.RWMutex

	// 扫描统计
	ScanCount       map[string]int64      // 每条链的扫描次数
	ScanSuccess     map[string]int64      // 扫描成功次数
	ScanFailure     map[string]int64      // 扫描失败次数
	LastScanTime    map[string]time.Time  // 最后扫描时间
	ScanDuration    map[string]time.Duration // 扫描耗时

	// 交易统计
	TransferFound   map[string]int64      // 发现的转账数量
	OrderMatched    map[string]int64      // 匹配的订单数量
	DuplicateTx     map[string]int64      // 重复交易数量

	// 错误统计
	ErrorCount      map[string]int64      // 错误计数
	LastError       map[string]string     // 最后错误信息
	LastErrorTime   map[string]time.Time  // 最后错误时间

	// RPC 统计
	RPCCallCount    map[string]int64      // RPC 调用次数
	RPCFailCount    map[string]int64      // RPC 失败次数
	RPCRetryCount   map[string]int64      // RPC 重试次数

	// 区块统计
	CurrentBlock    map[string]uint64     // 当前区块高度
	LastBlock       map[string]uint64     // 最后扫描区块
	BlocksBehind    map[string]uint64     // 落后区块数
}

// NewBlockchainMetrics 创建监控指标
func NewBlockchainMetrics() *BlockchainMetrics {
	return &BlockchainMetrics{
		ScanCount:       make(map[string]int64),
		ScanSuccess:     make(map[string]int64),
		ScanFailure:     make(map[string]int64),
		LastScanTime:    make(map[string]time.Time),
		ScanDuration:    make(map[string]time.Duration),
		TransferFound:   make(map[string]int64),
		OrderMatched:    make(map[string]int64),
		DuplicateTx:     make(map[string]int64),
		ErrorCount:      make(map[string]int64),
		LastError:       make(map[string]string),
		LastErrorTime:   make(map[string]time.Time),
		RPCCallCount:    make(map[string]int64),
		RPCFailCount:    make(map[string]int64),
		RPCRetryCount:   make(map[string]int64),
		CurrentBlock:    make(map[string]uint64),
		LastBlock:       make(map[string]uint64),
		BlocksBehind:    make(map[string]uint64),
	}
}

// RecordScanStart 记录扫描开始
func (m *BlockchainMetrics) RecordScanStart(chain string) time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ScanCount[chain]++
	return time.Now()
}

// RecordScanSuccess 记录扫描成功
func (m *BlockchainMetrics) RecordScanSuccess(chain string, startTime time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ScanSuccess[chain]++
	m.LastScanTime[chain] = time.Now()
	m.ScanDuration[chain] = time.Since(startTime)
}

// RecordScanFailure 记录扫描失败
func (m *BlockchainMetrics) RecordScanFailure(chain string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ScanFailure[chain]++
	m.ErrorCount[chain]++
	m.LastError[chain] = err.Error()
	m.LastErrorTime[chain] = time.Now()
}

// RecordTransfer 记录发现转账
func (m *BlockchainMetrics) RecordTransfer(chain string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TransferFound[chain] += int64(count)
}

// RecordOrderMatch 记录订单匹配
func (m *BlockchainMetrics) RecordOrderMatch(chain string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.OrderMatched[chain]++
}

// RecordDuplicateTx 记录重复交易
func (m *BlockchainMetrics) RecordDuplicateTx(chain string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DuplicateTx[chain]++
}

// RecordRPCCall 记录 RPC 调用
func (m *BlockchainMetrics) RecordRPCCall(chain string, success bool, retries int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.RPCCallCount[chain]++
	if !success {
		m.RPCFailCount[chain]++
	}
	if retries > 0 {
		m.RPCRetryCount[chain] += int64(retries)
	}
}

// UpdateBlockHeight 更新区块高度
func (m *BlockchainMetrics) UpdateBlockHeight(chain string, current, last uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CurrentBlock[chain] = current
	m.LastBlock[chain] = last
	if current > last {
		m.BlocksBehind[chain] = current - last
	} else {
		m.BlocksBehind[chain] = 0
	}
}

// GetMetrics 获取所有指标
func (m *BlockchainMetrics) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make(map[string]interface{})

	// 计算成功率
	successRate := make(map[string]float64)
	for chain, count := range m.ScanCount {
		if count > 0 {
			successRate[chain] = float64(m.ScanSuccess[chain]) / float64(count) * 100
		}
	}

	// 计算平均扫描时间
	avgDuration := make(map[string]string)
	for chain, duration := range m.ScanDuration {
		avgDuration[chain] = duration.String()
	}

	metrics["scan_count"] = m.ScanCount
	metrics["scan_success"] = m.ScanSuccess
	metrics["scan_failure"] = m.ScanFailure
	metrics["success_rate"] = successRate
	metrics["last_scan_time"] = m.LastScanTime
	metrics["avg_duration"] = avgDuration
	metrics["transfer_found"] = m.TransferFound
	metrics["order_matched"] = m.OrderMatched
	metrics["duplicate_tx"] = m.DuplicateTx
	metrics["error_count"] = m.ErrorCount
	metrics["last_error"] = m.LastError
	metrics["last_error_time"] = m.LastErrorTime
	metrics["rpc_call_count"] = m.RPCCallCount
	metrics["rpc_fail_count"] = m.RPCFailCount
	metrics["rpc_retry_count"] = m.RPCRetryCount
	metrics["current_block"] = m.CurrentBlock
	metrics["last_block"] = m.LastBlock
	metrics["blocks_behind"] = m.BlocksBehind

	return metrics
}

// GetChainMetrics 获取指定链的指标
func (m *BlockchainMetrics) GetChainMetrics(chain string) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make(map[string]interface{})

	scanCount := m.ScanCount[chain]
	successRate := 0.0
	if scanCount > 0 {
		successRate = float64(m.ScanSuccess[chain]) / float64(scanCount) * 100
	}

	metrics["chain"] = chain
	metrics["scan_count"] = scanCount
	metrics["scan_success"] = m.ScanSuccess[chain]
	metrics["scan_failure"] = m.ScanFailure[chain]
	metrics["success_rate"] = successRate
	metrics["last_scan_time"] = m.LastScanTime[chain]
	metrics["scan_duration"] = m.ScanDuration[chain].String()
	metrics["transfer_found"] = m.TransferFound[chain]
	metrics["order_matched"] = m.OrderMatched[chain]
	metrics["duplicate_tx"] = m.DuplicateTx[chain]
	metrics["error_count"] = m.ErrorCount[chain]
	metrics["last_error"] = m.LastError[chain]
	metrics["last_error_time"] = m.LastErrorTime[chain]
	metrics["rpc_call_count"] = m.RPCCallCount[chain]
	metrics["rpc_fail_count"] = m.RPCFailCount[chain]
	metrics["rpc_retry_count"] = m.RPCRetryCount[chain]
	metrics["current_block"] = m.CurrentBlock[chain]
	metrics["last_block"] = m.LastBlock[chain]
	metrics["blocks_behind"] = m.BlocksBehind[chain]

	return metrics
}

// Reset 重置指标
func (m *BlockchainMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ScanCount = make(map[string]int64)
	m.ScanSuccess = make(map[string]int64)
	m.ScanFailure = make(map[string]int64)
	m.LastScanTime = make(map[string]time.Time)
	m.ScanDuration = make(map[string]time.Duration)
	m.TransferFound = make(map[string]int64)
	m.OrderMatched = make(map[string]int64)
	m.DuplicateTx = make(map[string]int64)
	m.ErrorCount = make(map[string]int64)
	m.LastError = make(map[string]string)
	m.LastErrorTime = make(map[string]time.Time)
	m.RPCCallCount = make(map[string]int64)
	m.RPCFailCount = make(map[string]int64)
	m.RPCRetryCount = make(map[string]int64)
	m.CurrentBlock = make(map[string]uint64)
	m.LastBlock = make(map[string]uint64)
	m.BlocksBehind = make(map[string]uint64)
}

// ShouldAlert 检查是否需要告警
func (m *BlockchainMetrics) ShouldAlert(chain string) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 检查扫描失败率
	scanCount := m.ScanCount[chain]
	if scanCount >= 10 {
		failureRate := float64(m.ScanFailure[chain]) / float64(scanCount) * 100
		if failureRate > 50 {
			return true, fmt.Sprintf("Chain %s scan failure rate: %.2f%%", chain, failureRate)
		}
	}

	// 检查 RPC 失败率
	rpcCount := m.RPCCallCount[chain]
	if rpcCount >= 10 {
		failureRate := float64(m.RPCFailCount[chain]) / float64(rpcCount) * 100
		if failureRate > 30 {
			return true, fmt.Sprintf("Chain %s RPC failure rate: %.2f%%", chain, failureRate)
		}
	}

	// 检查区块落后
	if m.BlocksBehind[chain] > 100 {
		return true, fmt.Sprintf("Chain %s is %d blocks behind", chain, m.BlocksBehind[chain])
	}

	// 检查最后扫描时间
	lastScan, ok := m.LastScanTime[chain]
	if ok && time.Since(lastScan) > 5*time.Minute {
		return true, fmt.Sprintf("Chain %s hasn't been scanned for %v", chain, time.Since(lastScan))
	}

	return false, ""
}
