package gormkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Heming9/gormkit"
)

type tenantRecord struct {
	ID       uint             `gorm:"primaryKey"`
	TenantID gormkit.TenantID `gorm:"column:tenant_id;index"`
	Name     string
}

func TestTenantIsolationAcrossCRUD(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(context.Background())
	if err := base.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := base.Create(&tenantRecord{Name: "missing"}).Error; !errors.Is(err, gormkit.ErrTenantRequired) {
		t.Fatalf("create without tenant: %v", err)
	}

	tenantOne := gormkit.WithTenantID(context.Background(), 1)
	tenantTwo := gormkit.WithTenantID(context.Background(), 2)
	one := &tenantRecord{TenantID: 999, Name: "one"}
	if err := database.Client(tenantOne).Create(one).Error; err != nil {
		t.Fatal(err)
	}
	if one.TenantID != 1 {
		t.Fatalf("tenant was not overwritten: %d", one.TenantID)
	}
	if err := database.Client(tenantTwo).Create(&tenantRecord{Name: "two"}).Error; err != nil {
		t.Fatal(err)
	}

	var rows []tenantRecord
	if err := database.Client(tenantOne).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "one" {
		t.Fatalf("tenant query leaked rows: %+v", rows)
	}
	if err := database.Client(tenantOne).Model(&tenantRecord{}).Where("id = ?", one.ID).
		Updates(map[string]any{"name": "updated", "tenant_id": 2}).Error; err != nil {
		t.Fatal(err)
	}
	rows = nil
	if err := database.Client(tenantOne).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "updated" || rows[0].TenantID != 1 {
		t.Fatalf("tenant update escaped isolation: %+v", rows)
	}
	if err := database.Client(tenantTwo).Where("id = ?", one.ID).Delete(&tenantRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.Client(tenantOne).Model(&tenantRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("cross-tenant delete removed row; count=%d", count)
	}
	if err := database.Client(tenantOne).Where("id = ?", one.ID).Delete(&tenantRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := base.Unscoped().Model(&tenantRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unexpected unscoped row count: %d", count)
	}
}

func TestTenantQueryRequiresContext(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(context.Background())
	if err := base.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}
	var records []tenantRecord
	err := base.Find(&records).Error
	if !errors.Is(err, gormkit.ErrTenantRequired) {
		t.Fatalf("query without tenant: %v", err)
	}
}
