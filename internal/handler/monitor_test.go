package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ezpay/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func monitorTestDB(t *testing.T) (*gorm.DB, model.Merchant, time.Time) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "monitor.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.Merchant{}, &model.Order{}, &model.MonitorEvent{}); err != nil {
		t.Fatal(err)
	}
	merchant := model.Merchant{PID: "test", Name: "test", Key: "monitor-test-secret", Status: 1}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatal(err)
	}
	return db, merchant, time.Now().Truncate(time.Millisecond)
}

func monitorTestOrder(t *testing.T, db *gorm.DB, merchant uint, name string, amount string, created time.Time) model.Order {
	t.Helper()
	order := model.Order{TradeNo: name, OutTradeNo: name, MerchantID: merchant, Type: "wechat", Chain: "wechat",
		UniqueAmount: decimal.RequireFromString(amount), USDTAmount: decimal.RequireFromString("10.00"),
		PayCurrency: "CNY", SettlementAmount: decimal.NewFromInt(100), Fee: decimal.NewFromInt(2),
		Status: model.OrderStatusPending, Channel: "local", CreatedAt: created, ExpiredAt: created.Add(time.Hour)}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	return order
}

func testReceipt(id string, at time.Time, amount string) monitorReceipt {
	return monitorReceipt{EventID: strings.Repeat(id, 64), ReceivedAt: at, Type: "1", Price: amount, Amount: decimal.RequireFromString(amount)}
}

func monitorBalance(t *testing.T, db *gorm.DB, id uint) decimal.Decimal {
	t.Helper()
	var merchant model.Merchant
	if err := db.First(&merchant, id).Error; err != nil {
		t.Fatal(err)
	}
	return merchant.Balance
}

func TestMonitorReceiptReplayCannotPayReusedAmount(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	first := monitorTestOrder(t, db, merchant.ID, "first", "10.00", now.Add(-time.Minute))
	receipt := testReceipt("a", now, "10.00")
	event, created, err := applyMonitorReceipt(db, merchant.ID, receipt, now)
	if err != nil || !created || event.Code != 1 || event.OrderID != first.ID {
		t.Fatalf("first: %+v %v", event, err)
	}
	second := monitorTestOrder(t, db, merchant.ID, "second", "10.00", now.Add(time.Second))
	event, created, err = applyMonitorReceipt(db, merchant.ID, receipt, now.Add(48*time.Hour))
	if err != nil || created || event.OrderID != first.ID {
		t.Fatalf("replay: %+v %v", event, err)
	}
	if !monitorBalance(t, db, merchant.ID).Equal(decimal.NewFromInt(98)) {
		t.Fatal("duplicate credit")
	}
	db.First(&second, second.ID)
	if second.Status != model.OrderStatusPending {
		t.Fatal("replay paid new order")
	}
	changed := receipt
	changed.Price = "11.00"
	changed.Amount = decimal.NewFromInt(11)
	if _, _, err := applyMonitorReceipt(db, merchant.ID, changed, now); err != errMonitorConflict {
		t.Fatalf("expected identity conflict: %v", err)
	}
}

func TestMonitorUnmatchedEventsAndOriginalTimeArePreserved(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	receipt := testReceipt("b", now, "10.00")
	event, _, err := applyMonitorReceipt(db, merchant.ID, receipt, now)
	if err != nil || event.Code != 0 {
		t.Fatal(event, err)
	}
	order := monitorTestOrder(t, db, merchant.ID, "later", "10.00", now.Add(time.Second))
	event, created, err := applyMonitorReceipt(db, merchant.ID, receipt, now.Add(2*time.Second))
	if err != nil || created || event.Code != 0 {
		t.Fatal(event, err)
	}
	// Even an unseen old event cannot match an order created after its receipt time.
	event, _, err = applyMonitorReceipt(db, merchant.ID, testReceipt("c", now, "10.00"), now.Add(2*time.Second))
	if err != nil || event.Code != 0 {
		t.Fatal(event, err)
	}
	db.First(&order, order.ID)
	if order.Status != model.OrderStatusPending || !monitorBalance(t, db, merchant.ID).IsZero() {
		t.Fatal("old receipt paid later order")
	}
}

func TestMonitorConcurrentReplayCreditsOnce(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	monitorTestOrder(t, db, merchant.ID, "one", "10.00", now.Add(-time.Minute))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := applyMonitorReceipt(db, merchant.ID, testReceipt("d", now, "10.00"), now); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var count int64
	db.Model(&model.MonitorEvent{}).Count(&count)
	if count != 1 || !monitorBalance(t, db, merchant.ID).Equal(decimal.NewFromInt(98)) {
		t.Fatal("not idempotent", count)
	}
}

func TestMonitorTransactionRollsBackEventAndOrderWithFailedCredit(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	order := monitorTestOrder(t, db, merchant.ID, "rollback", "10.00", now.Add(-time.Minute))
	if err := db.Exec("CREATE TRIGGER reject_credit BEFORE UPDATE ON merchants BEGIN SELECT RAISE(ABORT, 'credit failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := applyMonitorReceipt(db, merchant.ID, testReceipt("e", now, "10.00"), now); err == nil {
		t.Fatal("expected failure")
	}
	var count int64
	db.Model(&model.MonitorEvent{}).Count(&count)
	db.First(&order, order.ID)
	if count != 0 || order.Status != model.OrderStatusPending {
		t.Fatal("partial transaction committed")
	}
	db.Exec("DROP TRIGGER reject_credit")
	event, _, err := applyMonitorReceipt(db, merchant.ID, testReceipt("e", now, "10.00"), now)
	if err != nil || event.Code != 1 {
		t.Fatal(event, err)
	}
}

func TestMonitorRejectsAmbiguityExpiredUpstreamAndWrongFallback(t *testing.T) {
	for _, scenario := range []string{"ambiguous", "ambiguous-legacy", "expired", "upstream", "fallback", "stale", "future"} {
		t.Run(scenario, func(t *testing.T) {
			db, merchant, now := monitorTestDB(t)
			order := monitorTestOrder(t, db, merchant.ID, "candidate", "10.00", now.Add(-time.Minute))
			receipt := testReceipt("f", now, "10.00")
			switch scenario {
			case "ambiguous":
				monitorTestOrder(t, db, merchant.ID, "duplicate", "10.00", now.Add(-time.Minute))
			case "ambiguous-legacy":
				monitorTestOrder(t, db, merchant.ID, "legacy", "0", now.Add(-time.Minute))
			case "expired":
				db.Model(&order).Update("expired_at", now.Add(-time.Second))
			case "upstream":
				db.Model(&order).Update("channel", "epay")
			case "fallback":
				db.Model(&order).Update("unique_amount", "10.01")
			case "stale":
				receipt.ReceivedAt = now.Add(-25 * time.Hour)
			case "future":
				receipt.ReceivedAt = now.Add(time.Minute)
			}
			event, _, err := applyMonitorReceipt(db, merchant.ID, receipt, now)
			if err != nil || event.Code != 0 || !monitorBalance(t, db, merchant.ID).IsZero() {
				t.Fatal(event, err)
			}
		})
	}
}

func TestMonitorCallbackRemainsRetryableIfProcessStopsAfterCommit(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	order := monitorTestOrder(t, db, merchant.ID, "callback", "10.00", now.Add(-time.Minute))
	if err := db.Model(&order).Update("notify_url", "https://merchant.example.com/notify").Error; err != nil {
		t.Fatal(err)
	}
	event, _, err := applyMonitorReceipt(db, merchant.ID, testReceipt("a", now, "10.00"), now)
	if err != nil || event.Code != 1 {
		t.Fatal(event, err)
	}
	var retryable []model.Order
	if err := db.Where("status = ? AND notify_status = ? AND notify_count < ?",
		model.OrderStatusPaid, model.NotifyStatusFailed, 15).Find(&retryable).Error; err != nil {
		t.Fatal(err)
	}
	if len(retryable) != 1 || retryable[0].ID != order.ID {
		t.Fatal("committed receipt cannot be recovered by callback retry scheduler", retryable)
	}
}

func TestMonitorHTTPAuthenticationAndParameterChecks(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewVmqHandler()
	router.GET("/appHeart", handler.AppHeart)
	router.GET("/appPush", handler.AppPush)
	call := func(path string, params url.Values) map[string]interface{} {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path+"?"+params.Encode(), nil))
		var body map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	timestamp := now.Unix()
	heart := url.Values{"v": {"2"}, "t": {fmt.Sprint(timestamp)}, "sign": {monitorSign(fmt.Sprintf("v2\nheart\n%d", timestamp), merchant.Key)}}
	if body := call("/appHeart", heart); body["code"] != float64(1) || body["authenticated"] != true {
		t.Fatal(body)
	}
	heart.Set("sign", "bad")
	if body := call("/appHeart", heart); body["code"] != float64(-1) {
		t.Fatal(body)
	}
	if body := call("/appHeart", url.Values{"t": {fmt.Sprint(timestamp)}}); body["authenticated"] != false {
		t.Fatal(body)
	}
	params := url.Values{"v": {"2"}, "t": {fmt.Sprint(timestamp)}, "event_id": {strings.Repeat("a", 64)},
		"received_at": {fmt.Sprint(now.UnixMilli())}, "type": {"1"}, "price": {"10.00"}, "currency": {"CNY"}}
	params.Set("sign", monitorSign(fmt.Sprintf("v2\npush\n%d\n%s\n%d\n1\n10.00\nCNY", timestamp, params.Get("event_id"), now.UnixMilli()), merchant.Key))
	if body := call("/appPush", params); body["code"] != float64(0) || body["event_id"] != params.Get("event_id") {
		t.Fatal(body)
	}
	for _, change := range []struct{ key, value string }{{"type", "3"}, {"t", fmt.Sprint(timestamp - 301)}, {"price", "-10.00"},
		{"price", "10.001"}, {"price", "10.01"}, {"currency", "USD"}, {"received_at", fmt.Sprint(now.UnixMilli() + 1)}, {"v", "1"}} {
		copy := url.Values{}
		for key, values := range params {
			copy[key] = append([]string(nil), values...)
		}
		copy.Set(change.key, change.value)
		if body := call("/appPush", copy); body["code"] != float64(-1) || body["retryable"] != (change.key == "t") {
			t.Fatal(change, body)
		}
	}
}

func TestMonitorProtocolGoldenVector(t *testing.T) {
	if got := monitorSign("v2\nheart\n1700000000", "test-key"); got != "1c2a24555948ede334a8d48b53387d62a2729531e18116f82e1bdaef19725db1" {
		t.Fatal("incompatible HMAC implementation", got)
	}
}

func TestMonitorDistinctConcurrentEventsCannotCreditSameOrder(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	monitorTestOrder(t, db, merchant.ID, "single", "10.00", now.Add(-time.Minute))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			receipt := testReceipt("a", now, "10.00")
			receipt.EventID = fmt.Sprintf("%064x", index+1)
			if _, _, err := applyMonitorReceipt(db, merchant.ID, receipt, now); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	var paid int64
	db.Model(&model.MonitorEvent{}).Where("code = 1").Count(&paid)
	if paid != 1 || !monitorBalance(t, db, merchant.ID).Equal(decimal.NewFromInt(98)) {
		t.Fatal("multiple credits", paid)
	}
}

func TestMonitorZeroNetSettlementCanCompleteOrder(t *testing.T) {
	db, merchant, now := monitorTestDB(t)
	order := monitorTestOrder(t, db, merchant.ID, "zero-net", "10.00", now.Add(-time.Minute))
	if err := db.Model(&order).Update("fee", 100).Error; err != nil {
		t.Fatal(err)
	}
	event, _, err := applyMonitorReceipt(db, merchant.ID, testReceipt("b", now, "10.00"), now)
	if err != nil || event.Code != 1 || !monitorBalance(t, db, merchant.ID).IsZero() {
		t.Fatal(event, err)
	}
}
