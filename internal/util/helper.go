package util

import (
	"net/http"
	"strings"
)

// GetClientIP 获取客户端IP
func GetClientIP(r *http.Request) string {
	// 优先从X-Forwarded-For获取
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// 从X-Real-IP获取
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// 从RemoteAddr获取
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// GetPaymentTypeChain 根据支付类型获取链名
func GetPaymentTypeChain(payType string) string {
	switch payType {
	case "trx", "trx_native":
		return "trx"
	case "usdt_trc20", "usdt_tron", "trc20":
		return "trc20"
	case "usdt_erc20", "usdt_eth", "erc20":
		return "erc20"
	case "usdt_bep20", "usdt_bsc", "bep20":
		return "bep20"
	case "usdt_polygon", "polygon":
		return "polygon"
	case "usdt_optimism", "optimism", "op":
		return "optimism"
	case "usdt_arbitrum", "arbitrum", "arb":
		return "arbitrum"
	case "usdt_avalanche", "avalanche", "avax":
		return "avalanche"
	case "usdt_base", "base":
		return "base"
	case "wechat", "wxpay", "1":
		return "wechat"
	case "alipay", "2":
		return "alipay"
	default:
		return payType
	}
}

// NormalizePaymentType 标准化支付类型
func NormalizePaymentType(payType string) string {
	chain := GetPaymentTypeChain(payType)
	// 微信/支付宝/TRX不需要加usdt_前缀
	if chain == "wechat" || chain == "alipay" || chain == "trx" {
		return chain
	}
	return "usdt_" + chain
}

// IsValidChain 检查链是否有效
func IsValidChain(chain string) bool {
	validChains := map[string]bool{
		"trx":       true,
		"trc20":     true,
		"erc20":     true,
		"bep20":     true,
		"polygon":   true,
		"optimism":  true,
		"arbitrum":  true,
		"avalanche": true,
		"base":      true,
		"wechat":    true,
		"alipay":    true,
	}
	return validChains[chain]
}

// IsFiatChain 检查是否为法币收款方式(微信/支付宝)
func IsFiatChain(chain string) bool {
	return chain == "wechat" || chain == "alipay"
}

// TruncateString 截断字符串
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// MaskAddress 遮蔽地址中间部分
func MaskAddress(address string) string {
	if len(address) < 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}
