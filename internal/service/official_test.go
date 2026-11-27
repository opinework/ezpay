package service

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"ezpay/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func officialTestDB(t *testing.T) (*gorm.DB, model.Order, OfficialReceipt) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "official.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.Order{}, &model.Merchant{}, &model.OfficialPayment{}, &model.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	merchant := model.Merchant{PID: "official-test", Status: 1, Balance: decimal.NewFromInt(10)}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	order := model.Order{TradeNo: "official-order", OutTradeNo: "downstream-order", MerchantID: merchant.ID, Type: "alipay", Chain: "alipay",
		Channel: string(ChannelTypeAlipayOfficial), PayCurrency: "CNY", PayAmount: decimal.NewFromInt(72), Money: decimal.NewFromInt(10), Currency: "USD",
		UniqueAmount: decimal.NewFromInt(72), SettlementAmount: decimal.NewFromInt(10), Fee: decimal.NewFromInt(1), FeeType: model.FeeTypeDeduction,
		OfficialAppID: "app", OfficialMerchantID: "seller", PaymentMode: "f2f", NotifyURL: "https://merchant.example/notify",
		CreatedAt: now.Add(-time.Minute), ExpiredAt: now.Add(time.Hour)}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	receipt := OfficialReceipt{Channel: order.Channel, TradeNo: order.TradeNo, TransactionID: "provider-transaction", AppID: "app", MerchantID: "seller", Currency: "CNY", Amount: order.PayAmount, PaidAt: now}
	return db, order, receipt
}

func TestOfficialSettlementConcurrencyAndReplay(t *testing.T) {
	db, order, receipt := officialTestDB(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := ApplyOfficialReceipt(db, receipt); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var merchant model.Merchant
	db.First(&merchant, order.MerchantID)
	if !merchant.Balance.Equal(decimal.NewFromInt(19)) {
		t.Fatal("duplicate credit", merchant.Balance)
	}
	var paid model.Order
	db.First(&paid, order.ID)
	if paid.Status != model.OrderStatusPaid || paid.NotifyStatus != model.NotifyStatusFailed || paid.ChannelOrderID != receipt.TransactionID {
		t.Fatalf("payment not durable: %+v", paid)
	}
	var count int64
	db.Model(&model.OfficialPayment{}).Count(&count)
	if count != 1 {
		t.Fatal("invalid ledger size", count)
	}
	second := order
	second.ID = 0
	second.TradeNo = "second"
	second.OutTradeNo = "second"
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	receipt.TradeNo = second.TradeNo
	if _, _, err := ApplyOfficialReceipt(db, receipt); err == nil {
		t.Fatal("reused official transaction accepted")
	}
	db.First(&second, second.ID)
	if second.Status != model.OrderStatusPending {
		t.Fatal("replay changed order")
	}
}

func TestOfficialReceiptRejectsMismatches(t *testing.T) {
	for _, field := range []string{"channel", "app", "merchant", "currency", "amount", "late", "early", "transaction", "cancelled", "local"} {
		t.Run(field, func(t *testing.T) {
			db, order, receipt := officialTestDB(t)
			switch field {
			case "channel":
				receipt.Channel = "vmq"
			case "app":
				receipt.AppID = "another-app"
			case "merchant":
				receipt.MerchantID = "another-merchant"
			case "currency":
				receipt.Currency = "USD"
			case "amount":
				receipt.Amount = order.Money // Original USD price must never replace the CNY payable amount.
			case "late":
				receipt.PaidAt = order.ExpiredAt.Add(time.Second)
			case "early":
				receipt.PaidAt = order.CreatedAt.Add(-time.Hour)
			case "transaction":
				receipt.TransactionID = ""
			case "cancelled":
				db.Model(&order).Update("status", model.OrderStatusCancelled)
			case "local":
				db.Model(&order).Update("channel", "local")
			}
			if _, _, err := ApplyOfficialReceipt(db, receipt); err == nil {
				t.Fatal("invalid receipt accepted")
			}
			var merchant model.Merchant
			db.First(&merchant, order.MerchantID)
			if !merchant.Balance.Equal(decimal.NewFromInt(10)) {
				t.Fatal("invalid receipt credited balance")
			}
		})
	}
}

func TestOfficialRollbackAndDelayedCallback(t *testing.T) {
	db, order, receipt := officialTestDB(t)
	if err := db.Exec("CREATE TRIGGER reject_credit BEFORE UPDATE OF balance ON merchants BEGIN SELECT RAISE(ABORT, 'credit failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := ApplyOfficialReceipt(db, receipt); err == nil {
		t.Fatal("credit failure ignored")
	}
	var current model.Order
	db.First(&current, order.ID)
	var count int64
	db.Model(&model.OfficialPayment{}).Count(&count)
	if current.Status != model.OrderStatusPending || count != 0 {
		t.Fatal("payment was not rolled back")
	}
	db.Exec("DROP TRIGGER reject_credit")
	db.Model(&order).Updates(map[string]interface{}{"status": model.OrderStatusExpired, "expired_at": receipt.PaidAt.Add(time.Second)})
	if _, applied, err := ApplyOfficialReceipt(db, receipt); err != nil || !applied {
		t.Fatal("delayed callback rejected", err)
	}
}

func officialKeys(t *testing.T) (*rsa.PrivateKey, string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	return key, private, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))
}

func officialSign(t *testing.T, key *rsa.PrivateKey, message string) string {
	t.Helper()
	digest := sha256.Sum256([]byte(message))
	signed, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(signed)
}

func TestAlipayOfficialSignedCallback(t *testing.T) {
	key, private, public := officialKeys(t)
	cfg := OfficialConfig{AppID: "app", MerchantID: "seller", PrivateKey: private, PublicKey: public}
	form := url.Values{"app_id": {"app"}, "seller_id": {"seller"}, "out_trade_no": {"local-order"}, "trade_no": {"ali-tx"}, "total_amount": {"72.00"}, "trade_status": {"TRADE_SUCCESS"}, "gmt_payment": {"2026-11-27 12:00:00"}}
	var keys []string
	for key := range form {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var entries []string
	for _, key := range keys {
		entries = append(entries, key+"="+form.Get(key))
	}
	form.Set("sign", officialSign(t, key, strings.Join(entries, "&")))
	form.Set("sign_type", "RSA2")
	receipt, err := ParseAlipayReceipt(context.Background(), cfg, form)
	if err != nil || receipt.TransactionID != "ali-tx" || !receipt.Amount.Equal(decimal.NewFromInt(72)) {
		t.Fatal(receipt, err)
	}
	form.Set("total_amount", "0.01")
	if _, err := ParseAlipayReceipt(context.Background(), cfg, form); err == nil {
		t.Fatal("tampered callback accepted")
	}
	form.Set("total_amount", "72.00")
	cfg.MerchantID = "other"
	if _, err := ParseAlipayReceipt(context.Background(), cfg, form); err == nil {
		t.Fatal("wrong seller accepted")
	}
}

func TestWechatOfficialEncryptedCallback(t *testing.T) {
	key, private, public := officialKeys(t)
	cfg := OfficialConfig{AppID: "wx-app", MerchantID: "mch", PrivateKey: private, PublicKey: public, PublicKeyID: "PUB_KEY_ID_TEST", SerialNo: "serial", APIV3Key: strings.Repeat("k", 32)}
	plaintext := `{"appid":"wx-app","mchid":"mch","out_trade_no":"local-order","transaction_id":"wx-tx","trade_state":"SUCCESS","success_time":"2026-11-27T12:00:00+08:00","amount":{"total":7200,"currency":"CNY"}}`
	block, _ := aes.NewCipher([]byte(cfg.APIV3Key))
	aead, _ := cipher.NewGCM(block)
	ciphertext := aead.Seal(nil, []byte("123456789012"), []byte(plaintext), []byte("transaction"))
	body, _ := json.Marshal(map[string]interface{}{"id": "event", "event_type": "TRANSACTION.SUCCESS", "resource": map[string]string{"algorithm": "AEAD_AES_256_GCM", "nonce": "123456789012", "associated_data": "transaction", "ciphertext": base64.StdEncoding.EncodeToString(ciphertext)}})
	request := func(body []byte) *http.Request {
		r := httptest.NewRequest("POST", "/notify", bytes.NewReader(body))
		timestamp := fmt.Sprint(time.Now().Unix())
		r.Header.Set("Wechatpay-Timestamp", timestamp)
		r.Header.Set("Wechatpay-Nonce", "nonce")
		r.Header.Set("Wechatpay-Serial", cfg.PublicKeyID)
		r.Header.Set("Wechatpay-Signature", officialSign(t, key, timestamp+"\nnonce\n"+string(body)+"\n"))
		return r
	}
	receipt, err := ParseWechatReceipt(context.Background(), cfg, request(body))
	if err != nil || receipt.TransactionID != "wx-tx" || !receipt.Amount.Equal(decimal.NewFromInt(72)) {
		t.Fatal(receipt, err)
	}
	r := request(body)
	r.Body = io.NopCloser(strings.NewReader(strings.Replace(string(body), "TRANSACTION.SUCCESS", "TRANSACTION.FAIL", 1)))
	if _, err := ParseWechatReceipt(context.Background(), cfg, r); err == nil {
		t.Fatal("tampered callback accepted")
	}
	wrong := cfg
	wrong.APIV3Key = strings.Repeat("x", 32)
	if _, err := ParseWechatReceipt(context.Background(), wrong, request(body)); err == nil {
		t.Fatal("wrong decryption key accepted")
	}
	wrong = cfg
	wrong.MerchantID = "another"
	if _, err := ParseWechatReceipt(context.Background(), wrong, request(body)); err == nil {
		t.Fatal("wrong merchant accepted")
	}
}

type officialRoundTripper func(*http.Request) (*http.Response, error)

func (f officialRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOfficialOrderModesAndAmounts(t *testing.T) {
	db, order, receipt := officialTestDB(t)
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
	key, private, public := officialKeys(t)
	values := map[string]string{"site_url": "https://pay.example", "channel_alipay_official_enabled": "1", "channel_alipay_official_app_id": "app", "channel_alipay_official_merchant_id": "seller", "channel_alipay_official_private_key": private, "channel_alipay_official_public_key": public,
		"channel_wechat_official_enabled": "1", "channel_wechat_official_app_id": "app", "channel_wechat_official_merchant_id": "seller", "channel_wechat_official_private_key": private, "channel_wechat_official_public_key": public, "channel_wechat_official_public_key_id": "PUB_KEY_ID_TEST", "channel_wechat_official_serial_no": "serial", "channel_wechat_official_api_v3_key": strings.Repeat("k", 32)}
	for k, v := range values {
		if err := db.Create(&model.SystemConfig{Key: k, Value: v}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateOfficialSettings(values); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"page", "wap"} {
		order.PaymentMode = mode
		result, err := GetChannelService().CreateOfficialOrder(&order)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(result.PayURL)
		var biz map[string]interface{}
		json.Unmarshal([]byte(u.Query().Get("biz_content")), &biz)
		if biz["total_amount"] != "72.00" || u.Query().Get("notify_url") != "https://pay.example/channel/notify/alipay_official" || u.Query().Get("return_url") != "https://pay.example/cashier/official-order" || u.Query().Get("sign") == "" {
			t.Fatal("incorrect website payment", result.PayURL)
		}
	}
	oldHTTP := channelHTTPClient
	t.Cleanup(func() { channelHTTPClient = oldHTTP })
	settleDuringPrepay := false
	channelHTTPClient = &http.Client{Transport: officialRoundTripper(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if strings.Contains(req.URL.Host, "alipay") {
			form, _ := url.ParseQuery(string(body))
			var biz map[string]interface{}
			json.Unmarshal([]byte(form.Get("biz_content")), &biz)
			if biz["total_amount"] != "72.00" {
				t.Fatal("wrong f2f amount", string(body))
			}
			payload := `{"code":"10000","msg":"Success","out_trade_no":"official-order","qr_code":"https://qr.alipay.com/test"}`
			response := `{"alipay_trade_precreate_response":` + payload + `,"sign":"` + officialSign(t, key, payload) + `"}`
			if settleDuringPrepay {
				if _, _, err := ApplyOfficialReceipt(db, receipt); err != nil {
					t.Fatal(err)
				}
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
		}
		var request map[string]interface{}
		json.Unmarshal(body, &request)
		amount := request["amount"].(map[string]interface{})
		if amount["total"] != float64(7200) || amount["currency"] != "CNY" || request["out_trade_no"] != order.TradeNo {
			t.Fatal("wrong wechat request", string(body))
		}
		if req.Header.Get("Authorization") == "" {
			t.Fatal("unsigned request")
		}
		payload := `{"code_url":"weixin://wxpay/test"}`
		if strings.HasSuffix(req.URL.Path, "/h5") {
			payload = `{"h5_url":"https://wx.tenpay.com/pay?token=test"}`
		}
		timestamp := fmt.Sprint(time.Now().Unix())
		header := http.Header{"Wechatpay-Timestamp": {timestamp}, "Wechatpay-Nonce": {"nonce"}, "Wechatpay-Serial": {"PUB_KEY_ID_TEST"}, "Wechatpay-Signature": {officialSign(t, key, timestamp+"\nnonce\n"+payload+"\n")}}
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})}
	order.PaymentMode = "f2f"
	result, err := GetChannelService().CreateOfficialOrder(&order)
	if err != nil || result.QRCode != "https://qr.alipay.com/test" {
		t.Fatal(result, err)
	}
	settleDuringPrepay = true
	if err := GetOrderService().createUpstreamOrder(&order, order.Channel); err != nil {
		t.Fatal(err)
	}
	if order.Status != model.OrderStatusPaid || order.ChannelOrderID != receipt.TransactionID || order.NotifyStatus != model.NotifyStatusFailed {
		t.Fatal("prepay response overwrote concurrent callback settlement")
	}
	settleDuringPrepay = false
	order.Channel = string(ChannelTypeWechatOfficial)
	order.Chain = "wechat"
	order.ClientIP = "192.0.2.1"
	for _, mode := range []string{"native", "h5"} {
		order.PaymentMode = mode
		result, err := GetChannelService().CreateOfficialOrder(&order)
		if err != nil || (mode == "native" && result.QRCode == "") || (mode == "h5" && result.PayURL == "") {
			t.Fatal(mode, result, err)
		}
	}
	channelHTTPClient = &http.Client{Transport: officialRoundTripper(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, err := GetChannelService().CreateOfficialOrder(&order); err == nil {
		t.Fatal("provider outage ignored")
	}
	if err := GetChannelService().HandleUpstreamNotify(ChannelTypeVmq, order.TradeNo, "tx", order.PayAmount); err == nil {
		t.Fatal("vmq callback accepted official order")
	}
}

func TestOfficialCashierTemplate(t *testing.T) {
	tmpl, err := template.ParseFiles("../../web/templates/cashier.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"f2f", "page", "native", "h5"} {
		order := model.Order{Channel: "alipay_official", Chain: "alipay", QRCode: "https://qr.alipay.com/abc", PayAmount: decimal.NewFromInt(72)}
		if mode == "page" || mode == "h5" {
			order.QRCode = ""
			order.ChannelPayURL = "https://pay.example/official?x=1&y=2"
		}
		var output bytes.Buffer
		if err := tmpl.Execute(&output, map[string]interface{}{"order": order, "isOfficial": true, "expiredAt": time.Now().Add(time.Hour).UnixMilli()}); err != nil {
			t.Fatal(mode, err)
		}
		if strings.Contains(output.String(), "ZgotmplZ") {
			t.Fatal("unsafe template URL", mode)
		}
		if mode == "native" || mode == "f2f" {
			if !strings.Contains(output.String(), `const address = "https://qr.alipay.com/abc"`) {
				t.Fatal("missing official qr payload")
			}
		}
	}
}

func TestOfficialRoutingPreservesPaymentTypes(t *testing.T) {
	db, _, _ := officialTestDB(t)
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
	for k, v := range map[string]string{"payment_channel_alipay": "alipay_official", "payment_channel_wechat": "vmq", "channel_vmq_enabled": "1", "default_channel": "epay", "channel_epay_enabled": "1"} {
		if err := db.Create(&model.SystemConfig{Key: k, Value: v}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := GetOrderService()
	for _, test := range []struct{ requested, chain, want string }{
		{"", "alipay", "alipay_official"}, {"", "wechat", "vmq"}, {"", "trc20", "epay"},
		{"local", "alipay", "local"}, {"vmq", "alipay", "vmq"}, {"epay", "wechat", "epay"},
		{"wechat_official", "wechat", "wechat_official"},
	} {
		if got := svc.determineChannel(test.requested, test.chain); got != test.want {
			t.Fatalf("%+v: got %s", test, got)
		}
	}
	for _, value := range []string{"http://pay.example", "https://user:pass@pay.example", "https://pay.example?x=y", "/relative"} {
		if _, err := officialSiteURL(value); err == nil {
			t.Fatal("invalid site URL accepted", value)
		}
	}
}
