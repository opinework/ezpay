package util

import (
	"sync"
	"time"
)

// RateLimiter 令牌桶限流器
type RateLimiter struct {
	rate       float64       // 每秒生成的令牌数
	capacity   int           // 桶容量
	tokens     float64       // 当前令牌数
	lastUpdate time.Time     // 上次更新时间
	mu         sync.Mutex    // 互斥锁
}

// NewRateLimiter 创建限流器
// rate: 每秒允许的请求数
// burst: 突发容量（桶容量）
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		capacity:   burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
}

// Allow 检查是否允许请求（非阻塞）
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastUpdate).Seconds()

	// 补充令牌
	r.tokens += elapsed * r.rate
	if r.tokens > float64(r.capacity) {
		r.tokens = float64(r.capacity)
	}

	r.lastUpdate = now

	// 检查是否有可用令牌
	if r.tokens >= 1.0 {
		r.tokens -= 1.0
		return true
	}

	return false
}

// Wait 等待直到可以执行请求（阻塞）
func (r *RateLimiter) Wait() {
	for {
		if r.Allow() {
			return
		}
		// 等待一小段时间后重试
		time.Sleep(time.Millisecond * 50)
	}
}

// WaitWithTimeout 等待直到可以执行请求，带超时（阻塞）
func (r *RateLimiter) WaitWithTimeout(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if r.Allow() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond * 50)
	}
}

// GetTokens 获取当前令牌数（用于监控）
func (r *RateLimiter) GetTokens() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastUpdate).Seconds()

	tokens := r.tokens + elapsed*r.rate
	if tokens > float64(r.capacity) {
		tokens = float64(r.capacity)
	}

	return tokens
}

// APIRateLimiters 全局API限流器管理
type APIRateLimiters struct {
	limiters map[string]*RateLimiter
	mu       sync.RWMutex
}

var globalLimiters = &APIRateLimiters{
	limiters: make(map[string]*RateLimiter),
}

// GetAPILimiter 获取指定API的限流器
func GetAPILimiter(apiName string, defaultRate float64, defaultBurst int) *RateLimiter {
	globalLimiters.mu.RLock()
	limiter, exists := globalLimiters.limiters[apiName]
	globalLimiters.mu.RUnlock()

	if exists {
		return limiter
	}

	// 创建新的限流器
	globalLimiters.mu.Lock()
	defer globalLimiters.mu.Unlock()

	// 双重检查
	if limiter, exists := globalLimiters.limiters[apiName]; exists {
		return limiter
	}

	limiter = NewRateLimiter(defaultRate, defaultBurst)
	globalLimiters.limiters[apiName] = limiter
	return limiter
}
