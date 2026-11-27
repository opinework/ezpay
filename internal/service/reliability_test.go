package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ezpay/internal/model"
	"ezpay/internal/util"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func useReliabilityDB(t *testing.T) (*gorm.DB, model.Order) {
	t.Helper()
	db, order, _ := officialTestDB(t)
	if err := db.AutoMigrate(&model.UpstreamPayment{}, &model.TransactionLog{}, &model.Withdrawal{}, &model.WithdrawAddress{}, &model.AmountLock{}, &model.Wallet{}); err != nil {
		t.Fatal(err)
	}
	old := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = old })
	return db, order
}

func TestReliabilityTerminalOrderFeeAtomicity(t *testing.T) {
	for _, status := range []model.OrderStatus{model.OrderStatusCancelled, model.OrderStatusExpired} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			db, order := useReliabilityDB(t)
			db.Model(&order).Updates(map[string]interface{}{"channel": "local", "fee_type": model.FeeTypeBalance})
			db.Model(&model.Merchant{}).Where("id = ?", order.MerchantID).Update("frozen_balance", order.Fee)
			if err := db.Exec("CREATE TRIGGER reject_release BEFORE UPDATE OF frozen_balance ON merchants BEGIN SELECT RAISE(ABORT,'release failed'); END").Error; err != nil {
				t.Fatal(err)
			}
			if _, err := finishUnpaidOrder(db, order.ID, status); err == nil {
				t.Fatal("fee release failure ignored")
			}
			var check model.Order
			db.First(&check, order.ID)
			if check.Status != model.OrderStatusPending {
				t.Fatal("status committed without fee release")
			}
			db.Exec("DROP TRIGGER reject_release")
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := finishUnpaidOrder(db, order.ID, status); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			var merchant model.Merchant
			db.First(&merchant, order.MerchantID)
			if !merchant.FrozenBalance.IsZero() {
				t.Fatal("fee released more or less than once", merchant.FrozenBalance)
			}
		})
	}
}

func TestReliabilityUpstreamSettlement(t *testing.T) {
	for _, channel := range []ChannelType{ChannelTypeVmq, ChannelTypeEpay} {
		t.Run(string(channel), func(t *testing.T) {
			db, order := useReliabilityDB(t)
			db.Model(&order).Updates(map[string]interface{}{"channel": channel, "channel_order_id": "", "notify_url": "https://merchant.example/notify"})
			expected := order.UniqueAmount
			if channel == ChannelTypeEpay {
				expected = order.Money
			}
			if _, _, err := applyUpstreamPayment(db, channel, order.TradeNo, "upstream", expected.Add(decimal.NewFromInt(1))); err == nil {
				t.Fatal("wrong amount accepted")
			}
			if _, _, err := applyUpstreamPayment(db, channel, order.TradeNo, "upstream", decimal.Zero); err == nil {
				t.Fatal("zero accepted")
			}
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, _, err := applyUpstreamPayment(db, channel, order.TradeNo, "upstream", expected); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			var merchant model.Merchant
			db.First(&merchant, order.MerchantID)
			if !merchant.Balance.Equal(decimal.NewFromInt(19)) {
				t.Fatal("duplicate or missing upstream credit", merchant.Balance)
			}
			var paid model.Order
			db.First(&paid, order.ID)
			if paid.NotifyStatus != model.NotifyStatusFailed {
				t.Fatal("outbox state missing")
			}
			if _, _, err := applyUpstreamPayment(db, channel, order.TradeNo, "another", expected); err == nil {
				t.Fatal("transaction replacement accepted")
			}
		})
	}
}

func TestReliabilityBlockchainRetryAfterRollback(t *testing.T) {
	db, order := useReliabilityDB(t)
	db.Model(&order).Updates(map[string]interface{}{"channel": "local", "chain": "trc20", "to_address": "T-test", "unique_amount": decimal.NewFromInt(72)})
	transfer := Transfer{TxHash: "tx-retry", Chain: "trc20", To: "T-test", Amount: decimal.NewFromInt(72)}
	if err := db.Exec("CREATE TRIGGER reject_credit BEFORE UPDATE OF balance ON merchants BEGIN SELECT RAISE(ABORT,'credit failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := GetBlockchainService().processTransfer(transfer); err == nil {
		t.Fatal("credit failure ignored")
	}
	var count int64
	db.Model(&model.TransactionLog{}).Count(&count)
	if count != 0 {
		t.Fatal("failed receipt permanently consumed")
	}
	db.Exec("DROP TRIGGER reject_credit")
	// Avoid asynchronous notification work in this transaction regression.
	db.Model(&order).Update("notify_url", "")
	if err := GetBlockchainService().processTransfer(transfer); err != nil {
		t.Fatal(err)
	}
	if err := GetBlockchainService().processTransfer(transfer); err != nil {
		t.Fatal(err)
	}
	var merchant model.Merchant
	db.First(&merchant, order.MerchantID)
	if !merchant.Balance.Equal(decimal.NewFromInt(19)) {
		t.Fatal("blockchain replay credited twice")
	}
}

func TestReliabilityBlockchainMatchingIsolation(t *testing.T) {
	db, order := useReliabilityDB(t)
	db.Model(&order).Updates(map[string]interface{}{"channel": "local", "chain": "trc20", "to_address": "T-test", "unique_amount": decimal.NewFromInt(73), "usdt_amount": decimal.NewFromInt(72)})
	transfer := Transfer{TxHash: "tx", Chain: "trc20", To: "T-test", Amount: decimal.NewFromInt(72)}
	if got := GetBlockchainService().matchOrder(transfer); got != nil {
		t.Fatal("legacy amount overrode unique amount")
	}
	db.Model(&order).Updates(map[string]interface{}{"channel": "epay", "unique_amount": decimal.NewFromInt(72)})
	if got := GetBlockchainService().matchOrder(transfer); got != nil {
		t.Fatal("local scanner matched upstream order")
	}
}

func TestReliabilityNotificationLeaseAndRetry(t *testing.T) {
	db, order := useReliabilityDB(t)
	db.Model(&order).Updates(map[string]interface{}{"status": model.OrderStatusPaid, "notify_url": "https://8.8.8.8/notify", "notify_status": model.NotifyStatusPending})
	db.Create(&model.SystemConfig{Key: model.ConfigKeyNotifyRetry, Value: "1"})
	var calls atomic.Int32
	client := &http.Client{Transport: officialRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("success")), Header: http.Header{}}, nil
	})}
	svc := &NotifyService{sem: make(chan struct{}, 20), client: client}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); svc.NotifyOrder(order.ID) }()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("concurrent duplicate notification", calls.Load())
	}
	var current model.Order
	db.First(&current, order.ID)
	if current.NotifyStatus != model.NotifyStatusSuccess || current.NotifyCount != 1 || current.NotifyLeaseUntil != nil {
		t.Fatal("notification did not finish durably")
	}
	client.Transport = officialRoundTripper(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 502, Body: io.NopCloser(strings.NewReader("fail")), Header: http.Header{}}, nil
	})
	db.Model(&order).Updates(map[string]interface{}{"notify_status": model.NotifyStatusFailed, "notify_count": 0})
	for i := 0; i < 5; i++ {
		svc.NotifyOrder(order.ID)
	}
	if calls.Load() != 4 {
		t.Fatal("retry ceiling ignored", calls.Load())
	}
	db.Model(&order).Updates(map[string]interface{}{"notify_status": model.NotifyStatusPending, "notify_count": 0, "notify_lease_until": time.Now().Add(time.Hour)})
	svc.NotifyOrder(order.ID)
	if calls.Load() != 4 {
		t.Fatal("active lease stolen")
	}
	db.Model(&order).Update("notify_lease_until", time.Now().Add(-time.Hour))
	svc.NotifyOrder(order.ID)
	if calls.Load() != 5 {
		t.Fatal("expired lease did not recover")
	}
	db.Model(&order).Updates(map[string]interface{}{"status": model.OrderStatusPending, "notify_status": model.NotifyStatusPending})
	svc.NotifyOrder(order.ID)
	if calls.Load() != 5 {
		t.Fatal("unpaid order sent payment notification")
	}
}

func TestReliabilityQueryMerging(t *testing.T) {
	target := mergePaymentQuery("https://merchant.example/callback?money=0&other=x#result", map[string]string{"money": "10", "sign": "signed"})
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if u.Fragment != "result" || len(u.Query()["money"]) != 1 || u.Query().Get("money") != "10" || u.Query().Get("other") != "x" {
		t.Fatal(target)
	}
	for _, raw := range []string{"javascript:alert(1)", "/relative", "https://user:pass@example.com"} {
		if mergePaymentQuery(raw, nil) != "" {
			t.Fatal("unsafe URL accepted", raw)
		}
	}
}

func TestReliabilityMerchantRequestUniqueness(t *testing.T) {
	db, order := useReliabilityDB(t)
	duplicate := order
	duplicate.ID = 0
	duplicate.TradeNo = "new-trade"
	if db.Create(&duplicate).Error == nil {
		t.Fatal("same merchant request inserted twice")
	}
	db.Delete(&order)
	if db.Create(&duplicate).Error == nil {
		t.Fatal("deleted merchant request identity was reused")
	}
}

func TestReliabilityOrderInputConflict(t *testing.T) {
	_, order := useReliabilityDB(t)
	for _, req := range []CreateOrderRequest{
		{MerchantPID: "official-test", Type: "alipay", OutTradeNo: order.OutTradeNo, Money: "11", Currency: "USD"},
		{MerchantPID: "official-test", Type: "wechat", OutTradeNo: order.OutTradeNo, Money: "10", Currency: "USD"},
		{MerchantPID: "official-test", Type: "alipay", OutTradeNo: order.OutTradeNo, Money: "10", Currency: "CNY"},
	} {
		if _, err := GetOrderService().CreateOrder(&req); err == nil {
			t.Fatal("changed idempotent order accepted")
		}
	}
}

func TestReliabilityOfficialQueryVerification(t *testing.T) {
	db, order := useReliabilityDB(t)
	key, private, public := officialKeys(t)
	for k, v := range map[string]string{"app_id": "app", "merchant_id": "seller", "private_key": private, "public_key": public} {
		db.Create(&model.SystemConfig{Key: "channel_alipay_official_" + k, Value: v})
	}
	old := channelHTTPClient
	t.Cleanup(func() { channelHTTPClient = old })
	channelHTTPClient = &http.Client{Transport: officialRoundTripper(func(*http.Request) (*http.Response, error) {
		payload := fmt.Sprintf(`{"code":"10000","out_trade_no":"%s","trade_no":"query-tx","trade_status":"TRADE_SUCCESS","total_amount":"72.00","send_pay_date":"%s"}`, order.TradeNo, time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05"))
		body := `{"alipay_trade_query_response":` + payload + `,"sign":"` + officialSign(t, key, payload) + `"}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	receipt, err := QueryOfficialOrder(context.Background(), &order)
	if err != nil || receipt == nil || receipt.TransactionID != "query-tx" {
		t.Fatal(receipt, err)
	}
	if _, applied, err := ApplyOfficialReceipt(db, *receipt); err != nil || !applied {
		t.Fatal("query could not recover lost callback", err)
	}
	channelHTTPClient = &http.Client{Transport: officialRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"alipay_trade_query_response":{"code":"10000"},"sign":"forged"}`))}, nil
	})}
	if _, err := QueryOfficialOrder(context.Background(), &order); err == nil {
		t.Fatal("unsigned provider query accepted")
	}
}

func TestReliabilityOperationalConfiguration(t *testing.T) {
	for _, values := range []map[string]string{{"notify_retry": "0"}, {"notify_retry": "1000000"}, {"order_expire": "-1"}, {"system_wallet_fee_rate": "1.01"}, {"personal_wallet_fee_rate": "-0.01"}} {
		if ValidateOperationalSettings(values) == nil {
			t.Fatal("invalid setting accepted", values)
		}
	}
	if err := ValidateOperationalSettings(map[string]string{"notify_retry": "3", "order_expire": "30", "system_wallet_fee_rate": "0.02"}); err != nil {
		t.Fatal(err)
	}
}

func TestReliabilityWithdrawalGuards(t *testing.T) {
	db, order := useReliabilityDB(t)
	withdrawal := model.Withdrawal{MerchantID: order.MerchantID, Amount: decimal.NewFromInt(50), Status: model.WithdrawStatusApproved}
	if err := db.Create(&withdrawal).Error; err != nil {
		t.Fatal(err)
	}
	if err := GetWithdrawService().CompleteWithdrawal(withdrawal.ID, "test"); err == nil {
		t.Fatal("withdrawal without funds completed")
	}
	var current model.Withdrawal
	db.First(&current, withdrawal.ID)
	if current.Status != model.WithdrawStatusApproved {
		t.Fatal("failed withdrawal committed status")
	}
	db.Model(&model.Merchant{}).Where("id = ?", order.MerchantID).Updates(map[string]interface{}{"balance": 100, "frozen_balance": 50})
	if err := GetWithdrawService().CompleteWithdrawal(withdrawal.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if err := GetWithdrawService().CompleteWithdrawal(withdrawal.ID, "test"); err == nil {
		t.Fatal("withdrawal debited twice")
	}
	var merchant model.Merchant
	db.First(&merchant, order.MerchantID)
	if !merchant.Balance.Equal(decimal.NewFromInt(50)) || !merchant.FrozenBalance.IsZero() {
		t.Fatal("incorrect withdrawal balance")
	}
}

func TestReliabilityBlockchainCheckpointOnRPCFailure(t *testing.T) {
	rpc := NewRPCClient([]string{"https://rpc.invalid"})
	rpc.maxRetries = 0
	rpc.SetCustomRateLimit(100000)
	block := "0x100"
	logs := `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"rate limited"}}`
	rpc.client = &http.Client{Transport: officialRoundTripper(func(r *http.Request) (*http.Response, error) {
		var req struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		body := logs
		if req.Method == "eth_blockNumber" {
			body = `{"jsonrpc":"2.0","id":1,"result":"` + block + `"}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	svc := &BlockchainService{rpcClients: map[string]*RPCClient{"bep20": rpc}, metrics: NewBlockchainMetrics()}
	listener := &ChainListener{chain: "bep20", confirmations: 1, lastBlock: 100, maxBatchSize: 1}
	if _, err := svc.scanEVMImproved(listener, map[string]bool{"0x1111111111111111111111111111111111111111": true}); err == nil {
		t.Fatal("RPC error accepted as empty scan")
	}
	if listener.lastBlock != 100 {
		t.Fatal("checkpoint skipped failed block range")
	}
	logs = `{"jsonrpc":"2.0","id":1,"result":"malformed logs"}`
	if _, err := svc.scanEVMImproved(listener, map[string]bool{"0x1111111111111111111111111111111111111111": true}); err == nil {
		t.Fatal("malformed logs accepted")
	}
	block = "0x1"
	listener.confirmations = 12
	listener.lastBlock = 0
	if _, err := svc.scanEVMImproved(listener, map[string]bool{}); err != nil || listener.lastBlock != 0 {
		t.Fatal("young chain underflow", err, listener.lastBlock)
	}
}

func TestReliabilityWalletAddressCase(t *testing.T) {
	db, _ := useReliabilityDB(t)
	address := "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	if err := db.Create(&model.Wallet{Chain: "trc20", Address: address, Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	cache := NewWalletCache(time.Minute)
	if !cache.GetAddresses("trc20")[address] || normalizeAddress(address, "trx") != address {
		t.Fatal("case-sensitive TRON address modified")
	}
	if normalizeAddress("0xAbCd", "erc20") != "0xabcd" {
		t.Fatal("EVM normalization changed")
	}
}

func TestReliabilityBlockchainMultipleEvents(t *testing.T) {
	db, order := useReliabilityDB(t)
	db.Model(&order).Updates(map[string]interface{}{"channel": "local", "chain": "erc20", "to_address": "0xtest", "notify_url": ""})
	var first model.Order
	db.First(&first, order.ID)
	second := first
	second.ID = 0
	second.TradeNo = "second"
	second.OutTradeNo = "second"
	second.UniqueAmount = decimal.RequireFromString("72.000001")
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	for i, amount := range []decimal.Decimal{first.UniqueAmount, second.UniqueAmount} {
		transfer := Transfer{Chain: "erc20", To: "0xtest", TxHash: "same-chain-transaction", EventID: fmt.Sprintf("same-chain-transaction:0x%d", i), Amount: amount, Timestamp: time.Now()}
		if err := GetBlockchainService().processTransfer(transfer); err != nil {
			t.Fatal(err)
		}
		if err := GetBlockchainService().processTransfer(transfer); err != nil {
			t.Fatal(err)
		}
	}
	var merchant model.Merchant
	db.First(&merchant, order.MerchantID)
	if !merchant.Balance.Equal(decimal.NewFromInt(28)) {
		t.Fatal("transaction events lost or replayed", merchant.Balance)
	}
	var count int64
	db.Model(&model.TransactionLog{}).Where("tx_hash = ?", "same-chain-transaction").Count(&count)
	if count != 2 {
		t.Fatal("transaction events collapsed", count)
	}
}

func TestReliabilityBlockchainOriginalTime(t *testing.T) {
	db, order := useReliabilityDB(t)
	db.Model(&order).Updates(map[string]interface{}{"channel": "local", "chain": "trc20", "to_address": "T-test", "notify_url": ""})
	transfer := Transfer{Chain: "trc20", To: "T-test", TxHash: "historical", Amount: order.UniqueAmount, Timestamp: order.CreatedAt.Add(-time.Hour)}
	if match := GetBlockchainService().matchOrder(transfer); match != nil {
		t.Fatal("historical receipt matched newer order")
	}
	db.Model(&order).Updates(map[string]interface{}{"status": model.OrderStatusExpired, "created_at": time.Now().Add(-time.Hour), "expired_at": time.Now().Add(-30 * time.Minute), "fee_type": model.FeeTypeBalance})
	transfer.Timestamp = time.Now().Add(-40 * time.Minute)
	if err := GetBlockchainService().processTransfer(transfer); err != nil {
		t.Fatal(err)
	}
	var merchant model.Merchant
	db.First(&merchant, order.MerchantID)
	if !merchant.Balance.Equal(decimal.NewFromInt(19)) || !merchant.FrozenBalance.IsZero() {
		t.Fatal("late observed timely payment mishandled", merchant.Balance, merchant.FrozenBalance)
	}
}

func TestReliabilityConcurrentAmountAllocation(t *testing.T) {
	db, order := useReliabilityDB(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := db.Transaction(func(tx *gorm.DB) error {
				amount := GetRateService().GenerateUniqueAmountWithTx(tx, decimal.NewFromInt(10), "trc20")
				if !amount.IsPositive() {
					return fmt.Errorf("amount allocation failed")
				}
				return tx.Create(&model.Order{TradeNo: fmt.Sprint("allocated-", i), OutTradeNo: fmt.Sprint("allocated-", i), MerchantID: order.MerchantID, Chain: "trc20", UniqueAmount: amount, Status: model.OrderStatusPending}).Error
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	var count int64
	db.Model(&model.Order{}).Where("chain = ?", "trc20").Distinct("unique_amount").Count(&count)
	if count != 20 {
		t.Fatal("allocated duplicate amounts", count)
	}
}

func TestReliabilityTronPagination(t *testing.T) {
	rpc := NewRPCClient([]string{"https://tron.invalid"})
	rpc.maxRetries = 0
	rpc.SetCustomRateLimit(100000)
	calls := 0
	rpc.client = &http.Client{Transport: officialRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"data":[{"transaction_id":"first"}],"meta":{"links":{"next":"https://tron.invalid/v1/accounts/T-test/transactions?fingerprint=next"}}}`
		if r.URL.Query().Get("fingerprint") == "next" {
			body = `{"data":[{"transaction_id":"second"}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	body, err := fetchTronData(rpc, "/v1/accounts/T-test/transactions?limit=200")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(string(body), "first") || !strings.Contains(string(body), "second") {
		t.Fatal("pagination truncated", string(body), calls)
	}
}

func TestReliabilityOfficialEndToEnd(t *testing.T) {
	db, existing := useReliabilityDB(t)
	key, private, public := officialKeys(t)
	for k, v := range map[string]string{"site_url": "https://pay.example", "rate_mode": "manual", "manual_rate": "7.2", "payment_channel_alipay": "alipay_official", "channel_alipay_official_enabled": "1", "channel_alipay_official_app_id": "app", "channel_alipay_official_merchant_id": "seller", "channel_alipay_official_private_key": private, "channel_alipay_official_public_key": public} {
		if err := db.Create(&model.SystemConfig{Key: k, Value: v}).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Model(&model.Merchant{}).Where("id = ?", existing.MerchantID).Update("key", "flow-key")
	var prepayCalls atomic.Int32
	oldHTTP := channelHTTPClient
	t.Cleanup(func() { channelHTTPClient = oldHTTP })
	channelHTTPClient = &http.Client{Transport: officialRoundTripper(func(r *http.Request) (*http.Response, error) {
		prepayCalls.Add(1)
		r.ParseForm()
		var biz map[string]interface{}
		json.Unmarshal([]byte(r.Form.Get("biz_content")), &biz)
		if biz["total_amount"] != "72.00" {
			t.Error("wrong converted payable amount", biz)
		}
		payload := fmt.Sprintf(`{"code":"10000","out_trade_no":"%s","qr_code":"https://qr.alipay.com/flow"}`, biz["out_trade_no"])
		body := `{"alipay_trade_precreate_response":` + payload + `,"sign":"` + officialSign(t, key, payload) + `"}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	req := &CreateOrderRequest{MerchantPID: "official-test", Type: "alipay", OutTradeNo: "closed-loop", Name: "test", Money: "10", Currency: "USD", NotifyURL: "https://8.8.8.8/notify"}
	response, err := GetOrderService().CreateOrder(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.PayAmount != "72" || !strings.Contains(response.QRCode, "/qrcode?text=") {
		t.Fatal(response)
	}
	if _, err := GetOrderService().CreateOrder(req); err != nil || prepayCalls.Load() != 1 {
		t.Fatal("retry created another provider order", err, prepayCalls.Load())
	}
	if paid, _, err := GetOrderService().CheckOrderPaid(response.TradeNo); err != nil || paid {
		t.Fatal("new order already paid")
	}
	form := url.Values{"app_id": {"app"}, "seller_id": {"seller"}, "out_trade_no": {response.TradeNo}, "trade_no": {"full-flow-tx"}, "total_amount": {"72.00"}, "trade_status": {"TRADE_SUCCESS"}, "gmt_payment": {time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")}}
	var keys []string
	for key := range form {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var fields []string
	for _, key := range keys {
		fields = append(fields, key+"="+form.Get(key))
	}
	form.Set("sign", officialSign(t, key, strings.Join(fields, "&")))
	form.Set("sign_type", "RSA2")
	cfg, err := GetOfficialConfig("alipay_official")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ParseAlipayReceipt(context.Background(), cfg, form)
	if err != nil {
		t.Fatal(err)
	}
	notifier := GetNotifyService()
	oldClient := notifier.client
	t.Cleanup(func() { notifier.client = oldClient })
	var notifyCalls atomic.Int32
	notifier.client = &http.Client{Transport: officialRoundTripper(func(r *http.Request) (*http.Response, error) {
		notifyCalls.Add(1)
		values := r.URL.Query()
		params := make(map[string]string)
		for key, values := range values {
			params[key] = values[0]
		}
		if values.Get("out_trade_no") != "closed-loop" || values.Get("money") != "10" || values.Get("pay_amount") != "72" || !util.VerifySign(params, "flow-key", values.Get("sign")) {
			t.Error("downstream protocol/signature mismatch", values)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("success"))}, nil
	})}
	if err := CompleteOfficialReceipt(*receipt); err != nil {
		t.Fatal(err)
	}
	var paidOrder model.Order
	for i := 0; i < 200; i++ {
		paidOrder = model.Order{}
		db.Where("trade_no = ?", response.TradeNo).First(&paidOrder)
		if paidOrder.NotifyStatus == model.NotifyStatusSuccess && paidOrder.NotifyLeaseUntil == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if paidOrder.NotifyStatus != model.NotifyStatusSuccess || paidOrder.NotifyLeaseUntil != nil {
		t.Fatal("notification did not close", paidOrder.NotifyStatus)
	}
	if err := CompleteOfficialReceipt(*receipt); err != nil {
		t.Fatal(err)
	}
	if paid, _, err := GetOrderService().CheckOrderPaid(response.TradeNo); err != nil || !paid {
		t.Fatal("cashier cache remained unpaid", err)
	}
	var merchant model.Merchant
	db.First(&merchant, existing.MerchantID)
	if !merchant.Balance.Equal(decimal.RequireFromString("19.8")) || notifyCalls.Load() != 1 {
		t.Fatal("end-to-end settlement/replay failed", merchant.Balance, notifyCalls.Load())
	}
}
