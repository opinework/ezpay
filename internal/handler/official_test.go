package handler

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ezpay/internal/model"
	"ezpay/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func TestOfficialSettingsSecretsAndAtomicValidation(t *testing.T) {
	db, _, _ := monitorTestDB(t)
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
	for key, value := range map[string]string{"channel_alipay_official_private_key": "secret-private-key", "channel_wechat_official_api_v3_key": "secret-api-key", "site_url": "https://pay.example"} {
		if err := db.Create(&model.SystemConfig{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := NewAdminHandler(nil)
	r.GET("/configs", handler.GetConfigs)
	r.POST("/configs", handler.UpdateConfigs)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/configs", nil))
	if strings.Contains(response.Body.String(), "secret-private-key") || strings.Contains(response.Body.String(), "secret-api-key") {
		t.Fatal("official credentials leaked")
	}
	if !strings.Contains(response.Body.String(), `"channel_alipay_official_private_key_configured":"1"`) {
		t.Fatal("missing credential state")
	}
	post := func(body string) map[string]interface{} {
		req := httptest.NewRequest("POST", "/configs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		var result map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := post(`{"channel_alipay_official_private_key":"","channel_wechat_official_api_v3_key":""}`); result["code"] != float64(1) {
		t.Fatal(result)
	}
	var secret model.SystemConfig
	db.Where("`key` = ?", "channel_alipay_official_private_key").First(&secret)
	if secret.Value != "secret-private-key" {
		t.Fatal("blank credential erased saved key")
	}
	if result := post(`{"channel_alipay_official_enabled":"1","site_url":"http://invalid.example"}`); result["code"] != float64(-1) {
		t.Fatal("invalid configuration accepted", result)
	}
	var site model.SystemConfig
	db.Where("`key` = ?", "site_url").First(&site)
	if site.Value != "https://pay.example" {
		t.Fatal("validation failure partially saved configuration")
	}
}

func TestLegacyPaymentProtocolsReturnOfficialOrders(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
	r := gin.New()
	r.GET("/mapi.php", NewEpayHandler().MAPISubmit)
	r.GET("/createOrder", NewVmqHandler().CreateOrder)
	for _, test := range []struct{ chain, epayType, vmqType string }{{"alipay", "alipay", "2"}, {"wechat", "wxpay", "1"}} {
		order := monitorTestOrder(t, db, merchant.ID, "legacy-"+test.chain, "10.00", now)
		if err := db.Model(&order).Updates(map[string]interface{}{"channel": test.chain + "_official", "chain": test.chain, "money": decimal.NewFromInt(10), "qr_code": "https://pay.example/dynamic-qr", "pay_amount": decimal.NewFromInt(10)}).Error; err != nil {
			t.Fatal(err)
		}
		params := map[string]string{"pid": merchant.PID, "type": test.epayType, "out_trade_no": order.OutTradeNo, "name": "test", "money": "10.00"}
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		values.Set("sign", util.GenerateSign(params, merchant.Key))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("GET", "/mapi.php?"+values.Encode(), nil))
		var result map[string]interface{}
		json.Unmarshal(response.Body.Bytes(), &result)
		if result["code"] != float64(1) || result["trade_no"] != order.TradeNo || !strings.Contains(result["qrcode"].(string), "/qrcode?text=") {
			t.Fatal("legacy epay failed", result)
		}
		values = url.Values{"payId": {order.OutTradeNo}, "type": {test.vmqType}, "price": {"10.00"}}
		values.Set("sign", util.MD5(order.OutTradeNo+test.vmqType+"10.00"+merchant.Key))
		response = httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("GET", "/createOrder?"+values.Encode(), nil))
		json.Unmarshal(response.Body.Bytes(), &result)
		if result["code"] != float64(1) || result["orderId"] != order.TradeNo || result["payType"] != test.vmqType || result["payUrl"] != "/cashier/"+order.TradeNo {
			t.Fatal("legacy vmq failed", result)
		}
	}
}
