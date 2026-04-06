package util

import (
	"crypto/md5"
	"encoding/hex"
	"log"
	"net/url"
	"sort"
	"strings"
)

// RFC3986Encode 按 RFC 3986 规范进行 URL 编码
// RFC 3986 不保留字符: A-Z a-z 0-9 - _ . ~
// Go 的 QueryEscape 会把空格编码为 +，把 ~ 编码为 %7E，需要修正
func RFC3986Encode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

// GenerateSign 生成签名 (彩虹易支付兼容)
// 签名算法:
// 1. 将参数按ASCII码排序
// 2. 对参数值按 RFC 3986 规范进行 URL 编码
// 3. 拼接为 key1=urlencode(value1)&key2=urlencode(value2) 格式
// 4. 末尾追加商户密钥
// 5. MD5加密(小写)
func GenerateSign(params map[string]string, key string) string {
	// 排除空值和签名相关参数
	filtered := make(map[string]string)
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		filtered[k] = v
	}

	// 按键名ASCII排序
	keys := make([]string, 0, len(filtered))
	for k := range filtered {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 拼接字符串（参数值使用 RFC 3986 URL 编码）
	var builder strings.Builder
	for i, k := range keys {
		if i > 0 {
			builder.WriteString("&")
		}
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(RFC3986Encode(filtered[k]))
	}
	builder.WriteString(key)

	signStr := builder.String()
	result := MD5(signStr)

	// 调试日志
	log.Printf("[GenerateSign] 过滤后的keys: %v", keys)
	log.Printf("[GenerateSign] 签名字符串: %s", signStr)
	log.Printf("[GenerateSign] MD5结果: %s", result)

	return result
}

// generateSignWithEncoder 使用指定编码函数生成签名
func generateSignWithEncoder(params map[string]string, key string, encoder func(string) string) string {
	filtered := make(map[string]string)
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		filtered[k] = v
	}

	keys := make([]string, 0, len(filtered))
	for k := range filtered {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for i, k := range keys {
		if i > 0 {
			builder.WriteString("&")
		}
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(encoder(filtered[k]))
	}
	builder.WriteString(key)

	return MD5(builder.String())
}

// urlDecodeParams 尝试对参数值进行 URL 解码，返回解码后的参数和是否有变化
func urlDecodeParams(params map[string]string) (map[string]string, bool) {
	decoded := make(map[string]string, len(params))
	changed := false
	for k, v := range params {
		d, err := url.QueryUnescape(v)
		if err == nil && d != v {
			decoded[k] = d
			changed = true
		} else {
			decoded[k] = v
		}
	}
	return decoded, changed
}

// VerifySign 验证签名（带兼容性处理）
// 兼容以下场景:
// 1. 标准 RFC 3986 编码 (空格 → %20)
// 2. PHP urlencode 编码 (空格 → +)
// 3. 参数值被双重编码的情况 (如 %20 被再次编码为 %2520)
// 4. 参数值不编码直接拼接的情况
func VerifySign(params map[string]string, key string, sign string) bool {
	// 尝试1: 标准 RFC 3986 编码 (空格 → %20)
	if strings.EqualFold(GenerateSign(params, key), sign) {
		return true
	}

	// 尝试2: PHP urlencode 编码 (空格 → +)，兼容 PHP 商户
	if strings.EqualFold(generateSignWithEncoder(params, key, url.QueryEscape), sign) {
		log.Printf("[VerifySign] 使用 QueryEscape(+编码) 验签成功")
		return true
	}

	// 尝试3: 不编码直接拼接 (部分简单实现的商户)
	if strings.EqualFold(generateSignWithEncoder(params, key, func(s string) string { return s }), sign) {
		log.Printf("[VerifySign] 使用原始值(不编码)验签成功")
		return true
	}

	// 尝试4: URL 解码参数后重新验证 (处理双重编码场景)
	decodedParams, changed := urlDecodeParams(params)
	if changed {
		if strings.EqualFold(GenerateSign(decodedParams, key), sign) {
			log.Printf("[VerifySign] 使用URL解码后的参数验签成功(RFC3986)")
			return true
		}
		if strings.EqualFold(generateSignWithEncoder(decodedParams, key, url.QueryEscape), sign) {
			log.Printf("[VerifySign] 使用URL解码后的参数验签成功(QueryEscape)")
			return true
		}
		if strings.EqualFold(generateSignWithEncoder(decodedParams, key, func(s string) string { return s }), sign) {
			log.Printf("[VerifySign] 使用URL解码后的参数验签成功(不编码)")
			return true
		}
	}

	return false
}

// GenerateVmqSign 生成V免签签名
// 签名算法: MD5(payId + param + type + price + key)
func GenerateVmqSign(payId, param, payType, price, key string) string {
	str := payId + param + payType + price + key
	return MD5(str)
}

// VerifyVmqSign 验证V免签签名
func VerifyVmqSign(payId, param, payType, price, key, sign string) bool {
	expected := GenerateVmqSign(payId, param, payType, price, key)
	return strings.EqualFold(expected, sign)
}

// MD5 计算MD5哈希值
func MD5(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}

// BuildQueryString 构建查询字符串
func BuildQueryString(params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	return values.Encode()
}

// ParseQueryString 解析查询字符串
func ParseQueryString(query string) map[string]string {
	result := make(map[string]string)
	values, err := url.ParseQuery(query)
	if err != nil {
		return result
	}
	for k, v := range values {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// BuildNotifyParams 构建通知参数
func BuildNotifyParams(pid, tradeNo, outTradeNo, payType, name, money, tradeStatus, key string) map[string]string {
	params := map[string]string{
		"pid":          pid,
		"trade_no":     tradeNo,
		"out_trade_no": outTradeNo,
		"type":         payType,
		"name":         name,
		"money":        money,
		"trade_status": tradeStatus,
		"sign_type":    "MD5",
	}
	params["sign"] = GenerateSign(params, key)
	return params
}
