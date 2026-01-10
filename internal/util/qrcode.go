package util

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	qrgen "github.com/skip2/go-qrcode"
)

// DecodeQRCode 从base64图片数据解析二维码内容
// 输入: base64编码的图片数据 (data:image/png;base64,xxxxx 或纯base64)
// 输出: 二维码内容
func DecodeQRCode(base64Data string) (string, error) {
	// 去除data:image/xxx;base64,前缀
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}

	// 解码base64
	imgData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", err
	}

	// 解码图片
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return "", err
	}

	return decodeQRFromImage(img)
}

// DecodeQRCodeFromFile 从文件路径解析二维码
func DecodeQRCodeFromFile(filePath string) (string, error) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 解码图片
	img, _, err := image.Decode(file)
	if err != nil {
		return "", err
	}

	return decodeQRFromImage(img)
}

// decodeQRFromImage 从image.Image解析二维码
func decodeQRFromImage(img image.Image) (string, error) {
	// 创建二维码读取器
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}

	// 解析二维码
	reader := qrcode.NewQRCodeReader()
	result, err := reader.Decode(bmp, nil)
	if err != nil {
		return "", err
	}

	return result.GetText(), nil
}

// IsValidWechatQRCode 验证是否是有效的微信收款码
func IsValidWechatQRCode(content string) bool {
	// 微信收款码的多种格式:
	// 1. wxp:// - 微信支付协议 (个人收款码)
	// 2. weixin://wxpay/ - 微信支付链接
	// 3. https://wx.tenpay.com/ - 微信支付链接
	// 4. https://payapp.weixin.qq.com/ - 微信商家收款码
	// 5. https://pay.weixin.qq.com/ - 微信支付
	// 6. https://payapp.wechatpay.cn/ - 微信商家收款码新域名
	// 7. 包含 weixin, wxpay, wechat 的URL
	lowerContent := strings.ToLower(content)
	return strings.HasPrefix(content, "wxp://") ||
		strings.HasPrefix(lowerContent, "weixin://") ||
		strings.Contains(lowerContent, "weixin.qq.com") ||
		strings.Contains(lowerContent, "wechatpay.cn") ||
		strings.Contains(lowerContent, "wx.tenpay.com") ||
		strings.Contains(lowerContent, "weixin") ||
		strings.Contains(lowerContent, "wechat") ||
		strings.Contains(lowerContent, "wxpay")
}

// IsValidAlipayQRCode 验证是否是有效的支付宝收款码
func IsValidAlipayQRCode(content string) bool {
	// 支付宝收款码的多种格式:
	// 1. https://qr.alipay.com/ - 个人收款码
	// 2. alipays:// - 支付宝协议
	// 3. https://render.alipay.com/ - 商家收款码
	// 4. https://ds.alipay.com/ - 商家收款码短链
	// 5. 包含 alipay 的URL
	lowerContent := strings.ToLower(content)
	return strings.Contains(lowerContent, "alipay.com") ||
		strings.Contains(lowerContent, "alipay") ||
		strings.HasPrefix(lowerContent, "alipays://")
}

// IsValidFiatQRCode 验证是否是有效的法币收款码(微信/支付宝)
func IsValidFiatQRCode(content string) bool {
	return IsValidWechatQRCode(content) || IsValidAlipayQRCode(content)
}

// GenerateQRCode 生成二维码图片，返回base64编码的PNG图片
// content: 二维码内容
// size: 二维码尺寸（像素）
func GenerateQRCode(content string, size int) (string, error) {
	// 使用skip2/go-qrcode生成二维码
	png, err := qrgen.Encode(content, qrgen.Medium, size)
	if err != nil {
		return "", err
	}

	// 转换为base64
	base64Str := base64.StdEncoding.EncodeToString(png)
	return "data:image/png;base64," + base64Str, nil
}
