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

func TestTenantCreateAlwaysPersistsContextTenant(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(context.Background())
	if err := base.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}

	ctx := gormkit.WithTenantID(context.Background(), 7)
	client := database.Client(ctx)
	selected := &tenantRecord{TenantID: 91, Name: "selected"}
	omitted := &tenantRecord{TenantID: 92, Name: "omitted"}
	omitAll := &tenantRecord{TenantID: 93, Name: "not-persisted"}

	if err := client.Select("Name").Create(selected).Error; err != nil {
		t.Fatalf("create with Select: %v", err)
	}
	if err := client.Omit("TenantID").Create(omitted).Error; err != nil {
		t.Fatalf("create with Omit: %v", err)
	}
	if err := client.Omit("*").Create(omitAll).Error; err != nil {
		t.Fatalf("create with Omit all: %v", err)
	}

	var rows []tenantRecord
	if err := base.Unscoped().Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("row count = %d, want 3", len(rows))
	}
	for _, row := range rows {
		if row.TenantID != 7 {
			t.Fatalf("persisted tenant = %d for row %+v, want 7", row.TenantID, row)
		}
	}
	if rows[0].Name != "selected" || rows[1].Name != "omitted" || rows[2].Name != "" {
		t.Fatalf("create selections were not preserved: %+v", rows)
	}
	if selected.TenantID != 7 || omitted.TenantID != 7 || omitAll.TenantID != 7 {
		t.Fatalf("models were not synchronized: selected=%d omitted=%d omitAll=%d",
			selected.TenantID, omitted.TenantID, omitAll.TenantID)
	}
}

func TestTenantBatchCreateOverwritesEveryTenant(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(context.Background())
	if err := base.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}

	ctx := gormkit.WithTenantID(context.Background(), 12)
	rows := []tenantRecord{
		{TenantID: 1, Name: "first"},
		{TenantID: 2, Name: "second"},
	}
	if err := database.Client(ctx).Select("Name").Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if rows[i].TenantID != 12 {
			t.Fatalf("row %d tenant = %d, want 12", i, rows[i].TenantID)
		}
	}

	var count int64
	if err := database.Client(ctx).Model(&tenantRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("tenant row count = %d, want 2", count)
	}
}

func TestTenantUpdateCannotSelectTenantColumn(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(context.Background())
	if err := base.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}

	ctx := gormkit.WithTenantID(context.Background(), 21)
	record := &tenantRecord{Name: "before"}
	client := database.Client(ctx)
	if err := client.Create(record).Error; err != nil {
		t.Fatal(err)
	}
	if err := client.Model(record).Select("Name", "TenantID").Updates(&tenantRecord{
		TenantID: 22,
		Name:     "after",
	}).Error; err != nil {
		t.Fatal(err)
	}

	var stored tenantRecord
	if err := base.Unscoped().First(&stored, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TenantID != 21 || stored.Name != "after" {
		t.Fatalf("tenant update was not isolated: %+v", stored)
	}
}
