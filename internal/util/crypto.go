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
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// 密码学随机数失败是严重问题，使用 panic 避免产生可预测值
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// GenerateMerchantKey 生成商户密钥
func GenerateMerchantKey() string {
	return GenerateRandomHex(16) // 32位密钥
}

// GenerateMerchantPID 生成商户PID (时间戳+随机数，降低碰撞概率)
func GenerateMerchantPID() string {
	ts := time.Now().UnixMilli() % 10000000 // 7位时间部分
	rnd := GenerateRandomHex(2)              // 4位随机部分
	// 取随机 hex 的前4个字符转为数字
	var rndNum int64
	for _, c := range rnd[:4] {
		if c >= '0' && c <= '9' {
			rndNum = rndNum*10 + int64(c-'0')
		} else {
			rndNum = rndNum*10 + int64(c-'a'+1)
		}
	}
	return fmt.Sprintf("%d%04d", ts, rndNum%10000)
}
