package service

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"ezpay/internal/model"
	"github.com/smartwalle/alipay/v3"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

const (
	ChannelTypeAlipayOfficial ChannelType = "alipay_official"
	ChannelTypeWechatOfficial ChannelType = "wechat_official"
)

func IsOfficialChannel(channel string) bool {
	return channel == string(ChannelTypeAlipayOfficial) || channel == string(ChannelTypeWechatOfficial)
}

type OfficialConfig struct {
	Enabled      string `json:"enabled"`
	Mode         string `json:"mode"`
	AppID        string `json:"app_id"`
	MerchantID   string `json:"merchant_id"`
	PrivateKey   string `json:"private_key"`
	PublicKey    string `json:"public_key"`
	PublicKeyID  string `json:"public_key_id"`
	SerialNo     string `json:"serial_no"`
	APIV3Key     string `json:"api_v3_key"`
	PlatformCert string `json:"platform_cert"`
}

func officialConfigFromMap(values map[string]string, channel string) OfficialConfig {
	fields := make(map[string]string)
	prefix := "channel_" + channel + "_"
	for key, value := range values {
		if strings.HasPrefix(key, prefix) {
			fields[strings.TrimPrefix(key, prefix)] = value
		}
	}
	data, _ := json.Marshal(fields)
	var cfg OfficialConfig
	_ = json.Unmarshal(data, &cfg)
	if cfg.Mode == "" {
		if channel == string(ChannelTypeAlipayOfficial) {
			cfg.Mode = "f2f"
		} else {
			cfg.Mode = "native"
		}
	}
	return cfg
}

// Callback configuration remains readable when a channel is disabled so in-flight payments can finish.
func GetOfficialConfig(channel string) (OfficialConfig, error) {
	if !IsOfficialChannel(channel) {
		return OfficialConfig{}, errors.New("unknown official channel")
	}
	var rows []model.SystemConfig
	if err := model.GetDB().Where("`key` LIKE ?", "channel_"+channel+"_%").Find(&rows).Error; err != nil {
		return OfficialConfig{}, err
	}
	values := make(map[string]string)
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return officialConfigFromMap(values, channel), nil
}

func IsOfficialSecret(key string) bool {
	return (strings.HasPrefix(key, "channel_alipay_official_") || strings.HasPrefix(key, "channel_wechat_official_")) &&
		(strings.HasSuffix(key, "_private_key") || strings.HasSuffix(key, "_api_v3_key"))
}

func newAlipayClient(cfg OfficialConfig) (*alipay.Client, error) {
	client, err := alipay.New(cfg.AppID, cfg.PrivateKey, true, alipay.WithHTTPClient(channelHTTPClient))
	if err != nil {
		return nil, errors.New("支付宝应用私钥格式无效")
	}
	if err := client.LoadAliPayPublicKey(cfg.PublicKey); err != nil {
		return nil, errors.New("支付宝公钥格式无效")
	}
	return client, nil
}

func newWechatClient(ctx context.Context, cfg OfficialConfig) (*core.Client, auth.Verifier, error) {
	key, err := utils.LoadPrivateKey(cfg.PrivateKey)
	if err != nil {
		if block, _ := pem.Decode([]byte(cfg.PrivateKey)); block != nil && block.Type == "RSA PRIVATE KEY" {
			key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		}
	}
	if err != nil {
		return nil, nil, errors.New("微信商户私钥格式无效")
	}
	var opt core.ClientOption
	var verifier auth.Verifier
	if cfg.PublicKey != "" {
		pub, err := utils.LoadPublicKey(cfg.PublicKey)
		if err != nil {
			return nil, nil, errors.New("微信支付公钥格式无效")
		}
		if !strings.HasPrefix(cfg.PublicKeyID, "PUB_KEY_ID_") {
			return nil, nil, errors.New("微信支付公钥 ID 无效")
		}
		opt = option.WithWechatPayPublicKeyAuthCipher(cfg.MerchantID, cfg.SerialNo, key, cfg.PublicKeyID, pub)
		verifier = verifiers.NewSHA256WithRSAPubkeyVerifier(cfg.PublicKeyID, *pub)
	} else {
		cert, err := utils.LoadCertificate(cfg.PlatformCert)
		if err != nil {
			return nil, nil, errors.New("请配置微信支付公钥或平台证书 PEM")
		}
		certs := []*x509.Certificate{cert}
		opt = option.WithWechatPayAuthCipher(cfg.MerchantID, cfg.SerialNo, key, certs)
		verifier = verifiers.NewSHA256WithRSAVerifier(core.NewCertificateMapWithList(certs))
	}
	client, err := core.NewClient(ctx, opt, option.WithHTTPClient(channelHTTPClient))
	return client, verifier, err
}

func officialSiteURL(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("官方支付需要配置公网 HTTPS 站点地址 site_url")
	}
	return strings.TrimRight(value, "/"), nil
}

// ValidateOfficialSettings validates the merged configuration without contacting payment providers.
func ValidateOfficialSettings(values map[string]string) error {
	for _, channel := range []string{string(ChannelTypeAlipayOfficial), string(ChannelTypeWechatOfficial)} {
		cfg := officialConfigFromMap(values, channel)
		if cfg.Enabled != "1" {
			continue
		}
		if _, err := officialSiteURL(values["site_url"]); err != nil {
			return err
		}
		if cfg.AppID == "" || cfg.MerchantID == "" {
			return fmt.Errorf("%s: 应用 ID 和收款商户号不能为空", channel)
		}
		if channel == string(ChannelTypeAlipayOfficial) {
			if cfg.Mode != "f2f" && cfg.Mode != "page" && cfg.Mode != "wap" {
				return errors.New("支付宝支付模式无效")
			}
			if _, err := newAlipayClient(cfg); err != nil {
				return err
			}
		} else {
			if cfg.Mode != "native" && cfg.Mode != "h5" {
				return errors.New("微信支付模式无效")
			}
			if len(cfg.APIV3Key) != 32 || cfg.SerialNo == "" {
				return errors.New("微信 API v3 密钥必须为 32 字节，并填写商户证书序列号")
			}
			if _, _, err := newWechatClient(context.Background(), cfg); err != nil {
				return err
			}
		}
	}
	for _, chain := range []string{"alipay", "wechat"} {
		selected := values["payment_channel_"+chain]
		if selected == "" || selected == "local" || selected == "vmq" || selected == "epay" {
			continue
		}
		if selected != chain+"_official" || values["channel_"+selected+"_enabled"] != "1" {
			return errors.New("默认官方通道未启用或与支付方式不匹配")
		}
	}
	return nil
}
