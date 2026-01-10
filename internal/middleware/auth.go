package middleware

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ezpay/config"
	"ezpay/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ============ 限流器实现 ============

// RateLimiter 基于令牌桶的限流器
type RateLimiter struct {
	mu          sync.RWMutex
	buckets     map[string]*tokenBucket
	rate        float64       // 每秒生成的令牌数
	capacity    int           // 桶容量
	cleanupTick time.Duration // 清理间隔
}

// tokenBucket 令牌桶
type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// NewRateLimiter 创建限流器
// rate: 每秒允许的请求数, capacity: 突发容量
func NewRateLimiter(rate float64, capacity int) *RateLimiter {
	rl := &RateLimiter{
		buckets:     make(map[string]*tokenBucket),
		rate:        rate,
		capacity:    capacity,
		cleanupTick: 5 * time.Minute,
	}
	// 启动定期清理过期桶
	go rl.cleanup()
	return rl
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, exists := rl.buckets[key]

	if !exists {
		rl.buckets[key] = &tokenBucket{
			tokens:     float64(rl.capacity) - 1,
			lastUpdate: now,
		}
		return true
	}

	// 计算经过的时间，添加令牌
	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	bucket.tokens += elapsed * rl.rate
	if bucket.tokens > float64(rl.capacity) {
		bucket.tokens = float64(rl.capacity)
	}
	bucket.lastUpdate = now

	// 检查是否有令牌
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}
	return false
}

// cleanup 定期清理过期的桶
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupTick)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, bucket := range rl.buckets {
			// 清理10分钟未使用的桶
			if now.Sub(bucket.lastUpdate) > 10*time.Minute {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// 全局限流器实例（默认值，可通过InitRateLimiters重新配置）
var (
	apiRateLimiter   *RateLimiter
	loginRateLimiter *RateLimiter
)

func init() {
	// 设置默认值
	apiRateLimiter = NewRateLimiter(20, 50)
	loginRateLimiter = NewRateLimiter(2, 5)
}

// InitRateLimiters 根据配置初始化限流器
func InitRateLimiters(apiRate float64, apiBurst int, loginRate float64, loginBurst int) {
	apiRateLimiter = NewRateLimiter(apiRate, apiBurst)
	loginRateLimiter = NewRateLimiter(loginRate, loginBurst)
}

// ============ IP黑名单缓存 ============

// IPBlacklistCache IP黑名单缓存
type IPBlacklistCache struct {
	mu         sync.RWMutex
	cache      map[string]bool
	lastUpdate time.Time
	ttl        time.Duration
}

var ipBlacklistCache *IPBlacklistCache

func init() {
	ipBlacklistCache = &IPBlacklistCache{
		cache: make(map[string]bool),
		ttl:   30 * time.Second, // 默认缓存30秒
	}
}

// SetIPBlacklistCacheTTL 设置IP黑名单缓存TTL
func SetIPBlacklistCacheTTL(seconds int) {
	if ipBlacklistCache != nil {
		ipBlacklistCache.mu.Lock()
		ipBlacklistCache.ttl = time.Duration(seconds) * time.Second
		ipBlacklistCache.mu.Unlock()
	}
}

// IsBlacklisted 检查IP是否在黑名单（带缓存）
func (c *IPBlacklistCache) IsBlacklisted(ip string) bool {
	c.mu.RLock()
	// 检查缓存是否过期
	if time.Since(c.lastUpdate) > c.ttl {
		c.mu.RUnlock()
		c.refresh()
		c.mu.RLock()
	}
	result, exists := c.cache[ip]
	c.mu.RUnlock()

	if exists {
		return result
	}
	// 不在缓存中，查数据库
	return model.IsIPBlacklisted(ip)
}

// refresh 刷新缓存
func (c *IPBlacklistCache) refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 双重检查
	if time.Since(c.lastUpdate) <= c.ttl {
		return
	}

	// 从数据库加载所有黑名单IP
	var blacklist []model.IPBlacklist
	model.GetDB().Find(&blacklist)

	newCache := make(map[string]bool)
	for _, item := range blacklist {
		newCache[item.IP] = true
	}
	c.cache = newCache
	c.lastUpdate = time.Now()
}

// Invalidate 使缓存失效
func (c *IPBlacklistCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastUpdate = time.Time{}
}

// AdminAuth 管理员认证中间件
func AdminAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "未登录",
			})
			c.Abort()
			return
		}

		// 解析Bearer Token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "Token格式错误",
			})
			c.Abort()
			return
		}

		// 验证Token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWT.Secret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "Token无效或已过期",
			})
			c.Abort()
			return
		}

		// 提取Claims
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			adminID := uint(claims["admin_id"].(float64))
			username := claims["username"].(string)

			c.Set("admin_id", adminID)
			c.Set("username", username)
		}

		c.Next()
	}
}

// CORS 跨域中间件（带可配置的域名白名单）
func CORS() gin.HandlerFunc {
	return CORSWithConfig(nil)
}

// CORSWithConfig 带配置的CORS中间件
func CORSWithConfig(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// 如果配置了白名单，则检查来源
		if len(allowedOrigins) > 0 {
			allowed := false
			for _, ao := range allowedOrigins {
				if ao == "*" || ao == origin {
					allowed = true
					break
				}
				// 支持通配符域名 *.example.com
				if strings.HasPrefix(ao, "*.") {
					suffix := ao[1:] // .example.com
					if strings.HasSuffix(origin, suffix) {
						allowed = true
						break
					}
				}
			}
			if allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
			}
		} else {
			// 未配置白名单时允许所有来源（开发模式）
			c.Header("Access-Control-Allow-Origin", "*")
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Requested-With")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// MerchantAuth 商户认证中间件
func MerchantAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "未登录",
			})
			c.Abort()
			return
		}

		// 解析Bearer Token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "Token格式错误",
			})
			c.Abort()
			return
		}

		// 验证Token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWT.Secret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "Token无效或已过期",
			})
			c.Abort()
			return
		}

		// 提取Claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "Token解析失败",
			})
			c.Abort()
			return
		}

		// 检查是否为商户Token
		tokenType, _ := claims["type"].(string)
		if tokenType != "merchant" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "非商户Token",
			})
			c.Abort()
			return
		}

		merchantID := uint(claims["merchant_id"].(float64))
		pid := claims["pid"].(string)

		// 验证商户状态
		var merchant model.Merchant
		if err := model.DB.Where("id = ? AND status = 1", merchantID).First(&merchant).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": -1,
				"msg":  "商户不存在或已禁用",
			})
			c.Abort()
			return
		}

		c.Set("merchant_id", merchantID)
		c.Set("merchant_pid", pid)
		c.Set("merchant", &merchant)

		c.Next()
	}
}

// RateLimit API限流中间件
func RateLimit() gin.HandlerFunc {
	return RateLimitWithConfig(apiRateLimiter)
}

// RateLimitWithConfig 带配置的限流中间件
func RateLimitWithConfig(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		if !limiter.Allow(clientIP) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": -1,
				"msg":  "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// LoginRateLimit 登录接口限流中间件（更严格）
func LoginRateLimit() gin.HandlerFunc {
	return RateLimitWithConfig(loginRateLimiter)
}

// CheckIPWhitelist 检查IP白名单
// 如果白名单为空则允许所有IP
func CheckIPWhitelist(clientIP string, whitelist string) bool {
	if whitelist == "" {
		return true
	}

	clientNetIP := net.ParseIP(clientIP)
	if clientNetIP == nil {
		return false
	}

	ips := strings.Split(whitelist, ",")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		// 精确匹配
		if ip == clientIP {
			return true
		}

		// CIDR格式匹配
		if strings.Contains(ip, "/") {
			_, network, err := net.ParseCIDR(ip)
			if err != nil {
				continue
			}
			if network.Contains(clientNetIP) {
				return true
			}
		} else {
			// 单个IP地址匹配
			if net.ParseIP(ip) != nil && ip == clientIP {
				return true
			}
		}
	}
	return false
}

// CheckIPInCIDR 检查IP是否在CIDR范围内
func CheckIPInCIDR(clientIP, cidr string) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	return network.Contains(ip)
}

// CheckRefererWhitelist 检查Referer域名白名单
// 如果白名单为空则允许所有来源
func CheckRefererWhitelist(referer string, whitelist string) bool {
	if whitelist == "" {
		return true
	}
	if referer == "" {
		// 如果没有Referer，可以选择是否允许
		// 这里选择允许，因为某些场景下可能没有Referer
		return true
	}

	// 提取域名
	referer = strings.ToLower(referer)
	// 移除协议前缀
	referer = strings.TrimPrefix(referer, "http://")
	referer = strings.TrimPrefix(referer, "https://")
	// 提取域名部分
	if idx := strings.Index(referer, "/"); idx > 0 {
		referer = referer[:idx]
	}
	// 移除端口
	if idx := strings.Index(referer, ":"); idx > 0 {
		referer = referer[:idx]
	}

	domains := strings.Split(whitelist, ",")
	for _, domain := range domains {
		domain = strings.TrimSpace(strings.ToLower(domain))
		if domain == "" {
			continue
		}
		// 精确匹配
		if domain == referer {
			return true
		}
		// 通配符匹配 (*.example.com)
		if strings.HasPrefix(domain, "*.") {
			suffix := domain[1:] // .example.com
			if strings.HasSuffix(referer, suffix) || referer == domain[2:] {
				return true
			}
		}
	}
	return false
}

// APILogger API调用日志中间件
func APILogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// 读取请求体
		var requestBody string
		if c.Request.Body != nil {
			bodyBytes, _ := io.ReadAll(c.Request.Body)
			// 恢复请求体以供后续处理
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			requestBody = string(bodyBytes)
			// 脱敏处理：移除sign和key参数的值
			requestBody = maskSensitiveData(requestBody)
		}

		// 获取URL参数
		if c.Request.URL.RawQuery != "" {
			queryParams := c.Request.URL.RawQuery
			queryParams = maskSensitiveData(queryParams)
			if requestBody != "" {
				requestBody = requestBody + "&" + queryParams
			} else {
				requestBody = queryParams
			}
		}

		// 设置日志开始标记
		c.Set("api_log_start_time", startTime)
		c.Set("api_log_request_body", requestBody)

		c.Next()

		// 获取响应信息
		responseCode, _ := c.Get("api_response_code")
		responseMsg, _ := c.Get("api_response_msg")
		tradeNo, _ := c.Get("api_trade_no")
		merchantID, _ := c.Get("api_merchant_id")
		merchantPID, _ := c.Get("api_merchant_pid")

		// 计算耗时
		duration := time.Since(startTime).Milliseconds()

		// 获取端点名称
		endpoint := c.Request.URL.Path

		// 创建日志记录
		log := model.APILog{
			Endpoint:    endpoint,
			Method:      c.Request.Method,
			ClientIP:    c.ClientIP(),
			Referer:     c.GetHeader("Referer"),
			UserAgent:   c.GetHeader("User-Agent"),
			RequestBody: truncateString(requestBody, 2000),
			Duration:    duration,
		}

		if code, ok := responseCode.(int); ok {
			log.ResponseCode = code
		}
		if msg, ok := responseMsg.(string); ok {
			log.ResponseMsg = truncateString(msg, 500)
		}
		if tn, ok := tradeNo.(string); ok {
			log.TradeNo = tn
		}
		if mid, ok := merchantID.(uint); ok {
			log.MerchantID = mid
		}
		if mpid, ok := merchantPID.(string); ok {
			log.MerchantPID = mpid
		}

		// 异步写入数据库
		go func() {
			model.GetDB().Create(&log)
		}()
	}
}

// maskSensitiveData 脱敏处理敏感数据
func maskSensitiveData(data string) string {
	// 简单的脱敏处理，替换sign和key参数的值
	for _, key := range []string{"sign", "key", "password"} {
		if idx := strings.Index(data, key+"="); idx >= 0 {
			endIdx := strings.Index(data[idx:], "&")
			if endIdx == -1 {
				endIdx = len(data) - idx
			}
			data = data[:idx] + key + "=***" + data[idx+endIdx:]
		}
	}
	return data
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// SetAPILogContext 设置API日志上下文 (在handler中调用)
func SetAPILogContext(c *gin.Context, code int, msg string, tradeNo string, merchantID uint, merchantPID string) {
	c.Set("api_response_code", code)
	c.Set("api_response_msg", msg)
	c.Set("api_trade_no", tradeNo)
	c.Set("api_merchant_id", merchantID)
	c.Set("api_merchant_pid", merchantPID)
}

// IPBlacklistCheck IP黑名单检查中间件（带缓存）
func IPBlacklistCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		if ipBlacklistCache.IsBlacklisted(clientIP) {
			c.JSON(http.StatusForbidden, gin.H{
				"code": -1,
				"msg":  "IP已被禁止访问",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// InvalidateIPBlacklistCache 使IP黑名单缓存失效（添加/删除黑名单后调用）
func InvalidateIPBlacklistCache() {
	ipBlacklistCache.Invalidate()
}

// GetIPBlacklistCache 获取IP黑名单缓存实例
func GetIPBlacklistCache() *IPBlacklistCache {
	return ipBlacklistCache
}
