package model

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"testing"
)

func TestReliabilityPaymentSchemaMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	if err := autoMigrate(); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropColumn(&Order{}, "notify_lease_until"); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasIndex(&Order{}, "idx_orders_merchant_request") {
		if err := db.Migrator().DropIndex(&Order{}, "idx_orders_merchant_request"); err != nil {
			t.Fatal(err)
		}
	}
	if err := autoMigrate(); err != nil {
		t.Fatal("upgrade failed", err)
	}
	if err := autoMigrate(); err != nil {
		t.Fatal("repeat startup failed", err)
	}
	for _, column := range []string{"notify_lease_until", "provider_checked_at", "payment_mode", "official_app_id", "official_merchant_id"} {
		if !db.Migrator().HasColumn(&Order{}, column) {
			t.Fatal("missing required column", column)
		}
	}
	if !db.Migrator().HasIndex(&Order{}, "idx_orders_merchant_request") {
		t.Fatal("payment idempotency constraint missing")
	}
}

func TestReliabilityMigrationRejectsDuplicateRequests(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "duplicates.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	if err := db.AutoMigrate(&Order{}); err != nil {
		t.Fatal(err)
	}
	db.Migrator().DropIndex(&Order{}, "idx_orders_merchant_request")
	for _, trade := range []string{"first", "second"} {
		if err := db.Create(&Order{TradeNo: trade, OutTradeNo: "same", MerchantID: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := autoMigrate(); err == nil {
		t.Fatal("service would start without payment uniqueness")
	}
}
