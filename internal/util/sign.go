package util

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
)

// GenerateSign 生成签名 (彩虹易支付兼容)
// 签名算法:
// 1. 将参数按ASCII码排序
// 2. 拼接为 key1=value1&key2=value2 格式
// 3. 末尾追加商户密钥
// 4. MD5加密(小写)
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

	// 拼接字符串
	var builder strings.Builder
	for i, k := range keys {
		if i > 0 {
			builder.WriteString("&")
		}
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(filtered[k])
	}
	builder.WriteString(key)

	// MD5加密
	return MD5(builder.String())
}

// VerifySign 验证签名
func VerifySign(params map[string]string, key string, sign string) bool {
	expected := GenerateSign(params, key)
	return strings.EqualFold(expected, sign)
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
