package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 密码加密
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 验证密码
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateTradeNo 生成交易号
// 格式: 年月日时分秒 + 6位随机数
func GenerateTradeNo() string {
	now := time.Now()
	random := GenerateRandomHex(3) // 6位十六进制
	return fmt.Sprintf("%s%s", now.Format("20060102150405"), random)
}

// GenerateRandomHex 生成随机十六进制字符串
func GenerateRandomHex(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateMerchantKey 生成商户密钥
func GenerateMerchantKey() string {
	return GenerateRandomHex(16) // 32位密钥
}

// GenerateMerchantPID 生成商户PID
func GenerateMerchantPID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano()%1000000000)
}
