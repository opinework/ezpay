package handler

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

func TestMonitorMySQLTransactions(t *testing.T) {
	socket := os.Getenv("EZPAY_MONITOR_TEST_MYSQL_SOCKET")
	if socket == "" {
		t.Skip("run scripts/check-monitor-mysql.sh for isolated MySQL verification")
	}
	for _, foundRows := range []bool{false, true} {
		t.Run(fmt.Sprintf("clientFoundRows=%t", foundRows), func(t *testing.T) {
			config := mysqlclient.NewConfig()
			config.User = "root"
			config.Net = "unix"
			config.Addr = socket
			config.ParseTime = true
			config.ClientFoundRows = foundRows
			config.Timeout = 5 * time.Second
			config.ReadTimeout = 15 * time.Second
			config.WriteTimeout = 15 * time.Second
			admin, err := sql.Open("mysql", config.FormatDSN())
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			name := fmt.Sprintf("monitor_test_%d", time.Now().UnixNano())
			if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); err != nil {
				t.Fatal(err)
			}
			defer admin.Exec("DROP DATABASE `" + name + "`")
			config.DBName = name
			db, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			sqlDB.SetMaxOpenConns(8)
			if err := db.AutoMigrate(&model.Merchant{}, &model.Order{}, &model.MonitorEvent{}); err != nil {
				t.Fatal(err)
			}
			merchant := model.Merchant{PID: "test", Key: "test-secret", Status: 1}
			if err := db.Create(&merchant).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Now().Truncate(time.Millisecond)
			first := monitorTestOrder(t, db, merchant.ID, "first", "10.00", now.Add(-time.Minute))
			receipt := testReceipt("a", now, "10.00")
			var wait sync.WaitGroup
			for i := 0; i < 16; i++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					event, _, err := applyMonitorReceipt(db, merchant.ID, receipt, now)
					if err != nil || event.OrderID != first.ID {
						t.Errorf("concurrent replay: %+v %v", event, err)
					}
				}()
			}
			wait.Wait()
			if !monitorBalance(t, db, merchant.ID).Equal(decimal.NewFromInt(98)) {
				t.Fatal("duplicate credit")
			}
			second := monitorTestOrder(t, db, merchant.ID, "second", "10.00", now.Add(time.Second))
			if _, created, err := applyMonitorReceipt(db, merchant.ID, receipt, now.Add(2*time.Second)); err != nil || created {
				t.Fatal("replay was not immutable", err)
			}
			db.First(&second, second.ID)
			if second.Status != model.OrderStatusPending {
				t.Fatal("replay paid a new order")
			}
			zero := monitorTestOrder(t, db, merchant.ID, "zero", "11.00", now.Add(-time.Minute))
			if err := db.Model(&zero).Update("fee", 100).Error; err != nil {
				t.Fatal(err)
			}
			if event, _, err := applyMonitorReceipt(db, merchant.ID, testReceipt("b", now, "11.00"), now); err != nil || event.Code != 1 {
				t.Fatal("zero net receipt failed", event, err)
			}
			rollback := monitorTestOrder(t, db, merchant.ID, "rollback", "12.00", now.Add(-time.Minute))
			if err := db.Exec("CREATE TRIGGER reject_monitor_credit BEFORE UPDATE ON merchants FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'test rejection'").Error; err != nil {
				t.Fatal(err)
			}
			if _, _, err := applyMonitorReceipt(db, merchant.ID, testReceipt("c", now, "12.00"), now); err == nil {
				t.Fatal("expected credit failure")
			}
			var count int64
			db.Model(&model.MonitorEvent{}).Where("event_id = ?", testReceipt("c", now, "12.00").EventID).Count(&count)
			db.First(&rollback, rollback.ID)
			if count != 0 || rollback.Status != model.OrderStatusPending {
				t.Fatal("partial transaction committed")
			}
			if err := db.Exec("DROP TRIGGER reject_monitor_credit").Error; err != nil {
				t.Fatal(err)
			}
			if event, _, err := applyMonitorReceipt(db, merchant.ID, testReceipt("c", now, "12.00"), now); err != nil || event.Code != 1 {
				t.Fatal("recovery after rollback failed", event, err)
			}
		})
	}
}
