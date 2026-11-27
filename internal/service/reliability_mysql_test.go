package service

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"ezpay/internal/model"
	mysqlclient "github.com/go-sql-driver/mysql"
	"github.com/shopspring/decimal"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestReliabilityMySQLPayments(t *testing.T) {
	socket := os.Getenv("EZPAY_MONITOR_TEST_MYSQL_SOCKET")
	if socket == "" {
		t.Skip("isolated MySQL socket not configured")
	}
	for _, foundRows := range []bool{false, true} {
		t.Run(fmt.Sprint("clientFoundRows=", foundRows), func(t *testing.T) {
			cfg := mysqlclient.NewConfig()
			cfg.User = "root"
			cfg.Net = "unix"
			cfg.Addr = socket
			cfg.ParseTime = true
			cfg.ClientFoundRows = foundRows
			cfg.Timeout = 5 * time.Second
			cfg.ReadTimeout = 15 * time.Second
			cfg.WriteTimeout = 15 * time.Second
			admin, err := sql.Open("mysql", cfg.FormatDSN())
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			name := fmt.Sprint("ezpay_review_", time.Now().UnixNano())
			if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); err != nil {
				t.Fatal(err)
			}
			defer admin.Exec("DROP DATABASE `" + name + "`")
			cfg.DBName = name
			db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			defer sqlDB.Close()
			sqlDB.SetMaxOpenConns(12)
			if err := db.AutoMigrate(&model.Merchant{}, &model.Order{}, &model.OfficialPayment{}, &model.UpstreamPayment{}, &model.AmountLock{}, &model.TransactionLog{}); err != nil {
				t.Fatal(err)
			}
			merchant := model.Merchant{PID: "mysql-review", Status: 1}
			if err := db.Create(&merchant).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Now().Truncate(time.Millisecond)
			order := model.Order{TradeNo: "official", OutTradeNo: "official", MerchantID: merchant.ID, Channel: "alipay_official", Chain: "alipay", OfficialAppID: "app", OfficialMerchantID: "seller", PayCurrency: "CNY", PayAmount: decimal.NewFromInt(72), UniqueAmount: decimal.NewFromInt(72), SettlementAmount: decimal.NewFromInt(10), Fee: decimal.NewFromInt(1), FeeType: model.FeeTypeDeduction, CreatedAt: now.Add(-time.Minute), ExpiredAt: now.Add(time.Hour)}
			if err := db.Create(&order).Error; err != nil {
				t.Fatal(err)
			}
			receipt := OfficialReceipt{Channel: order.Channel, TradeNo: order.TradeNo, TransactionID: "tx", AppID: "app", MerchantID: "seller", Currency: "CNY", Amount: order.PayAmount, PaidAt: now}
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
			var current model.Merchant
			db.First(&current, merchant.ID)
			if !current.Balance.Equal(decimal.NewFromInt(9)) {
				t.Fatal("duplicate official settlement", current.Balance)
			}
			zero := order
			zero.ID = 0
			zero.TradeNo = "zero"
			zero.OutTradeNo = "zero"
			zero.Fee = zero.SettlementAmount
			if err := db.Create(&zero).Error; err != nil {
				t.Fatal(err)
			}
			receipt.TradeNo = "zero"
			receipt.TransactionID = "zero"
			if _, applied, err := ApplyOfficialReceipt(db, receipt); err != nil || !applied {
				t.Fatal("zero-net settlement rejected", err)
			}
			if err := db.Create(&model.AmountLock{Chain: "trc20"}).Error; err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					err := db.Transaction(func(tx *gorm.DB) error {
						amount := GetRateService().GenerateUniqueAmountWithTx(tx, decimal.NewFromInt(10), "trc20")
						if !amount.IsPositive() {
							return fmt.Errorf("amount allocation failed")
						}
						return tx.Create(&model.Order{TradeNo: fmt.Sprint("local-", i), OutTradeNo: fmt.Sprint("local-", i), MerchantID: merchant.ID, Chain: "trc20", UniqueAmount: amount, ExpiredAt: now.Add(time.Hour)}).Error
					})
					if err != nil {
						t.Error(err)
					}
				}(i)
			}
			wg.Wait()
			var count int64
			db.Model(&model.Order{}).Where("chain = ?", "trc20").Distinct("unique_amount").Count(&count)
			if count != 16 {
				t.Fatal("concurrent allocation collision", count)
			}
		})
	}
}
