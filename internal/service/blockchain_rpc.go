package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezpay/internal/util"
)

// RPCClient RPC 客户端，支持重试和故障转移
type RPCClient struct {
	endpoints       []string          // RPC 端点列表
	currentIndex    int               // 当前使用的端点索引
	client          *http.Client      // HTTP 客户端
	maxRetries      int               // 最大重试次数
	retryDelay      time.Duration     // 重试基础延迟
	failureCount    map[string]int    // 端点失败计数
	lastFailureTime map[string]time.Time // 最后失败时间
	mu              sync.RWMutex      // 保护并发访问
	healthCheckInt  time.Duration     // 健康检查间隔
}

// NewRPCClient 创建 RPC 客户端
func NewRPCClient(endpoints []string) *RPCClient {
	if len(endpoints) == 0 {
		endpoints = []string{""}
	}

	return &RPCClient{
		endpoints:       endpoints,
		currentIndex:    0,
		client:          httpClient,
		maxRetries:      3,
		retryDelay:      1 * time.Second,
		failureCount:    make(map[string]int),
		lastFailureTime: make(map[string]time.Time),
		healthCheckInt:  1 * time.Minute,
	}
}

// Get 执行 GET 请求，支持重试和故障转移
func (c *RPCClient) Get(path string) (*http.Response, error) {
	// API限流：根据端点类型应用不同的限流策略
	c.applyRateLimit()

	var lastErr error
	endpointSwitches := 0
	maxSwitches := len(c.endpoints) // 最多切换端点数等于端点总数

	for retry := 0; retry <= c.maxRetries; retry++ {
		endpoint := c.getCurrentEndpoint()
		url := endpoint + path

		resp, err := c.client.Get(url)
		if err == nil && resp.StatusCode < 500 {
			c.recordSuccess(endpoint)
			return resp, nil
		}

		if resp != nil {
			resp.Body.Close()
		}

		lastErr = err
		if err == nil {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		c.recordFailure(endpoint)

		// 最后一次重试失败，尝试切换端点
		if retry == c.maxRetries {
			if endpointSwitches < maxSwitches && c.switchEndpoint() {
				endpointSwitches++
				retry = 0
				continue
			}
			break
		}

		// 指数退避
		delay := c.retryDelay * time.Duration(1<<uint(retry))
		log.Printf("RPC GET failed (attempt %d/%d): %v, retrying in %v",
			retry+1, c.maxRetries+1, lastErr, delay)
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("all RPC endpoints failed: %w", lastErr)
}

// Post 执行 POST 请求，支持重试和故障转移
func (c *RPCClient) Post(path string, contentType string, body io.Reader) (*http.Response, error) {
	// API限流：根据端点类型应用不同的限流策略
	c.applyRateLimit()

	// 读取 body 内容（用于重试）
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	var lastErr error
	endpointSwitches := 0
	maxSwitches := len(c.endpoints)

	for retry := 0; retry <= c.maxRetries; retry++ {
		endpoint := c.getCurrentEndpoint()
		url := endpoint + path

		// 创建新的 body reader
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		resp, err := c.client.Post(url, contentType, bodyReader)
		if err == nil && resp.StatusCode < 500 {
			c.recordSuccess(endpoint)
			return resp, nil
		}

		if resp != nil {
			resp.Body.Close()
		}

		lastErr = err
		if err == nil {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		c.recordFailure(endpoint)

		// 最后一次重试失败，尝试切换端点
		if retry == c.maxRetries {
			if endpointSwitches < maxSwitches && c.switchEndpoint() {
				endpointSwitches++
				retry = 0
				continue
			}
			break
		}

		// 指数退避
		delay := c.retryDelay * time.Duration(1<<uint(retry))
		log.Printf("RPC POST failed (attempt %d/%d): %v, retrying in %v",
			retry+1, c.maxRetries+1, lastErr, delay)
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("all RPC endpoints failed: %w", lastErr)
}

// PostJSON 执行 JSON-RPC 请求
func (c *RPCClient) PostJSON(path string, request interface{}) ([]byte, error) {
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.Post(path, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

// getCurrentEndpoint 获取当前端点
func (c *RPCClient) getCurrentEndpoint() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.currentIndex >= len(c.endpoints) {
		c.currentIndex = 0
	}
	return c.endpoints[c.currentIndex]
}

// switchEndpoint 切换到下一个可用端点
func (c *RPCClient) switchEndpoint() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.endpoints) <= 1 {
		return false
	}

	startIndex := c.currentIndex
	for i := 0; i < len(c.endpoints); i++ {
		c.currentIndex = (c.currentIndex + 1) % len(c.endpoints)
		endpoint := c.endpoints[c.currentIndex]

		// 检查该端点是否可用
		if c.isEndpointHealthy(endpoint) {
			log.Printf("Switched RPC endpoint from %s to %s",
				c.endpoints[startIndex], endpoint)
			return true
		}
	}

	// 所有端点都不可用，回到第一个
	c.currentIndex = 0
	return false
}

// isEndpointHealthy 检查端点是否健康
func (c *RPCClient) isEndpointHealthy(endpoint string) bool {
	failures := c.failureCount[endpoint]
	lastFailure := c.lastFailureTime[endpoint]

	// 如果失败次数少于3次，认为健康
	if failures < 3 {
		return true
	}

	// 如果最后失败时间超过健康检查间隔，给它一次机会
	if time.Since(lastFailure) > c.healthCheckInt {
		c.failureCount[endpoint] = 0
		return true
	}

	return false
}

// recordSuccess 记录成功
func (c *RPCClient) recordSuccess(endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failureCount[endpoint] = 0
}

// recordFailure 记录失败
func (c *RPCClient) recordFailure(endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failureCount[endpoint]++
	c.lastFailureTime[endpoint] = time.Now()
}

// GetStats 获取统计信息
func (c *RPCClient) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := map[string]interface{}{
		"endpoints":      c.endpoints,
		"current_index":  c.currentIndex,
		"failure_counts": c.failureCount,
	}

	return stats
}

// BatchRequest 批量 JSON-RPC 请求
type BatchRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// BatchResponse 批量响应
type BatchResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
	ID      int             `json:"id"`
}

// RPCError RPC 错误
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// BatchPostJSON 批量 JSON-RPC 请求
func (c *RPCClient) BatchPostJSON(path string, requests []BatchRequest) ([]BatchResponse, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	reqBody, err := json.Marshal(requests)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch request: %w", err)
	}

	respBody, err := c.PostJSON(path, json.RawMessage(reqBody))
	if err != nil {
		return nil, err
	}

	var responses []BatchResponse
	if err := json.Unmarshal(respBody, &responses); err != nil {
		return nil, fmt.Errorf("failed to unmarshal batch response: %w", err)
	}

	return responses, nil
}

// DoWithContext 执行带 context 的请求
func (c *RPCClient) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)

	var lastErr error
	for retry := 0; retry <= c.maxRetries; retry++ {
		resp, err := c.client.Do(req)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		if resp != nil {
			resp.Body.Close()
		}

		lastErr = err
		if err == nil {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if retry < c.maxRetries {
			delay := c.retryDelay * time.Duration(1<<uint(retry))
			time.Sleep(delay)
		}
	}

	return nil, fmt.Errorf("request failed after retries: %w", lastErr)
}

// applyRateLimit 根据端点类型应用限流策略
func (c *RPCClient) applyRateLimit() {
	endpoint := c.getCurrentEndpoint()

	// 根据不同的API提供商应用不同的限流策略
	if strings.Contains(endpoint, "trongrid.io") {
		// TronGrid免费版：限制为每秒5次请求（保守估计）
		limiter := util.GetAPILimiter("trongrid", 5.0, 10)
		limiter.Wait()
	} else if strings.Contains(endpoint, "infura.io") {
		// Infura免费版：限制为每秒10次请求
		limiter := util.GetAPILimiter("infura", 10.0, 20)
		limiter.Wait()
	} else if strings.Contains(endpoint, "binance.org") || strings.Contains(endpoint, "bsc-dataseed") {
		// BSC节点：限制为每秒10次请求
		limiter := util.GetAPILimiter("bsc", 10.0, 20)
		limiter.Wait()
	} else if strings.Contains(endpoint, "polygon") {
		// Polygon节点：限制为每秒10次请求
		limiter := util.GetAPILimiter("polygon", 10.0, 20)
		limiter.Wait()
	} else if strings.Contains(endpoint, "optimism") {
		// Optimism节点：限制为每秒10次请求
		limiter := util.GetAPILimiter("optimism", 10.0, 20)
		limiter.Wait()
	} else if strings.Contains(endpoint, "arbitrum") {
		// Arbitrum节点：限制为每秒10次请求
		limiter := util.GetAPILimiter("arbitrum", 10.0, 20)
		limiter.Wait()
	} else if strings.Contains(endpoint, "base.org") {
		// Base节点：限制为每秒10次请求
		limiter := util.GetAPILimiter("base", 10.0, 20)
		limiter.Wait()
	} else {
		// 通用限流：限制为每秒5次请求
		limiter := util.GetAPILimiter("generic-rpc", 5.0, 10)
		limiter.Wait()
	}
}
