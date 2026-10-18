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
// 标准: 空格→%20, ~保留, 百分号编码统一大写
func RFC3986Encode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	// Go 的 QueryEscape 已经使用大写，无需额外处理
	return encoded
}

// DecodeIfEncoded 如果值已被 URL 编码则解码，防止双重编码
func DecodeIfEncoded(s string) string {
	if !strings.Contains(s, "%") && !strings.Contains(s, "+") {
		return s
	}
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// GenerateSign 生成签名 (彩虹易支付兼容)
// 签名算法:
// 1. 将参数按ASCII码排序
// 2. 先解码参数值（防止双重编码），再按 RFC 3986 规范进行 URL 编码
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
		// 先解码（防止双重编码），再统一编码
		filtered[k] = DecodeIfEncoded(v)
	}

	// 按键名ASCII排序
	keys := make([]string, 0, len(filtered))
	for k := range filtered {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 拼接字符串（参数值使用 RFC 3986 URL 编码，大写）
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

	return result
}

// generateSignWithEncoder 使用指定编码函数生成签名（用于验证兼容）
// decodeFirst: 是否先解码参数值
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
// 5. 百分号编码大小写不一致 (%2f vs %2F)
func VerifySign(params map[string]string, key string, sign string) bool {
	// 统一签名为大写（处理 md5 大小写不一致）
	sign = strings.ToLower(sign)

	// 尝试1: 标准 RFC 3986 编码 (空格 → %20)
	if GenerateSign(params, key) == sign {
		return true
	}

	// 尝试2: PHP urlencode 编码 (空格 → +)，兼容 PHP 商户
	if generateSignWithEncoder(params, key, url.QueryEscape) == sign {
		log.Printf("[VerifySign] 使用 QueryEscape(+编码) 验签成功")
		return true
	}

	// 尝试3: 不编码直接拼接 (部分简单实现的商户)
	if generateSignWithEncoder(params, key, func(s string) string { return s }) == sign {
		log.Printf("[VerifySign] 使用原始值(不编码)验签成功")
		return true
	}

	// 尝试4: URL 解码参数后重新验证 (处理双重编码场景)
	decodedParams, changed := urlDecodeParams(params)
	if changed {
		if GenerateSign(decodedParams, key) == sign {
			log.Printf("[VerifySign] 使用URL解码后的参数验签成功(RFC3986)")
			return true
		}
		if generateSignWithEncoder(decodedParams, key, url.QueryEscape) == sign {
			log.Printf("[VerifySign] 使用URL解码后的参数验签成功(QueryEscape)")
			return true
		}
		if generateSignWithEncoder(decodedParams, key, func(s string) string { return s }) == sign {
			log.Printf("[VerifySign] 使用URL解码后的参数验签成功(不编码)")
			return true
		}
	}

	// 尝试5: 使用百分号编码大写统一后的参数重试
	normalizedParams := make(map[string]string, len(params))
	hasPercent := false
	for k, v := range params {
		if strings.Contains(v, "%") {
			// 只对百分号编码的十六进制字符做大写（%2f → %2F），不改变参数值本身
			normalized := normalizePercentEncoding(v)
			normalizedParams[k] = normalized
			if normalized != v {
				hasPercent = true
			}
		} else {
			normalizedParams[k] = v
		}
	}
	if hasPercent {
		if generateSignWithEncoder(normalizedParams, key, func(s string) string { return s }) == sign {
			log.Printf("[VerifySign] 使用百分号大写统一后验签成功")
			return true
		}
	}

	return false
}

// normalizePercentEncoding 只对百分号编码的十六进制字符做大写
// 例如: %2f → %2F, hello%20world → hello%20world (不变)
func normalizePercentEncoding(s string) string {
	var result strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			result.WriteByte('%')
			result.WriteByte(toUpperHex(s[i+1]))
			result.WriteByte(toUpperHex(s[i+2]))
			i += 2
		} else {
			result.WriteByte(s[i])
		}
	}
	return result.String()
}

func toUpperHex(b byte) byte {
	if b >= 'a' && b <= 'f' {
		return b - 32
	}
	return b
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

// GenerateSignRaw 生成签名（与 PHP 彩虹易支付完全一致）
// 签名算法:
// 1. 过滤 sign、sign_type 和空值参数
// 2. 按 key 的 ASCII 码排序
// 3. 拼接为 key1=value1&key2=value2 格式（值不做 URL 编码）
// 4. 末尾追加商户密钥（无 & 分隔符）
// 5. MD5 加密（小写）
func GenerateSignRaw(params map[string]string, key string) string {
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
		builder.WriteString(filtered[k])
	}
	builder.WriteString(key)

	return MD5(builder.String())
}

// BuildNotifyParams 构建通知参数（兼容彩虹易支付标准）
// param 参与签名计算（非空时）
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
	// 使用与 PHP epay 一致的签名方式（值不编码）
	params["sign"] = GenerateSignRaw(params, key)
	return params
}

// NotifyOrderInfo 通知/同步跳转参数容器（v1.3.10 起，含多币种字段）
//
// 兼容性说明：
//   - 老 PHP epay 商户的通用验签代码（"取所有非空参数 → 排序 → 拼接 → MD5"）会自动
//     把新增字段纳入签名，无需改代码即兼容
//   - 仅写死字段名清单的商户（罕见）需要把 currency/pay_currency/pay_amount 加进去
type NotifyOrderInfo struct {
	PID         string
	TradeNo     string
	OutTradeNo  string
	Type        string
	Name        string
	Money       string // 原始金额，按 Currency 计价
	Currency    string // 原始货币 (USD/CNY/USDT/EUR/TRX)
	PayCurrency string // 实际支付货币 (USDT/TRX/CNY)
	PayAmount   string // 用户实付金额（按 PayCurrency 计价，与 unique_amount 一致）
	TradeStatus string
	Param       string
}

// BuildNotifyParamsV2 构建通知参数 (v1.3.10+ 含多币种字段)
// 推荐用此函数。新增字段参与签名，验签代码保持"取所有非空参数验"逻辑即可
func BuildNotifyParamsV2(info NotifyOrderInfo, key string) map[string]string {
	params := map[string]string{
		"pid":          info.PID,
		"trade_no":     info.TradeNo,
		"out_trade_no": info.OutTradeNo,
		"type":         info.Type,
		"name":         info.Name,
		"money":        info.Money,
		"currency":     info.Currency,
		"pay_currency": info.PayCurrency,
		"pay_amount":   info.PayAmount,
		"trade_status": info.TradeStatus,
		"param":        info.Param,
		"sign_type":    "MD5",
	}
	// 使用与 PHP epay 一致的签名方式（值不编码、空值不参与）
	params["sign"] = GenerateSignRaw(params, key)
	return params
}

// BuildNotifyParamsWithParam (deprecated) 仅供旧调用兼容；内部已转发到 BuildNotifyParamsV2
//
// Deprecated: 请使用 BuildNotifyParamsV2 + NotifyOrderInfo
func BuildNotifyParamsWithParam(pid, tradeNo, outTradeNo, payType, name, money, tradeStatus, param, key string) map[string]string {
	return BuildNotifyParamsV2(NotifyOrderInfo{
		PID:         pid,
		TradeNo:     tradeNo,
		OutTradeNo:  outTradeNo,
		Type:        payType,
		Name:        name,
		Money:       money,
		TradeStatus: tradeStatus,
		Param:       param,
	}, key)
}
