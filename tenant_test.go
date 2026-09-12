package gormkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Heming9/gormkit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if err := database.Client(gormkit.WithDisableTenant(context.Background())).
		Unscoped().Model(&tenantRecord{}).Count(&count).Error; err != nil {
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
	if err := database.Client(gormkit.WithDisableTenant(context.Background())).
		Unscoped().Order("id").Find(&rows).Error; err != nil {
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
	if err := database.Client(gormkit.WithDisableTenant(context.Background())).
		Unscoped().First(&stored, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TenantID != 21 || stored.Name != "after" {
		t.Fatalf("tenant update was not isolated: %+v", stored)
	}
}

func TestTenantRepositoryUpdateRejectsAnotherTenantsPrimaryKey(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(context.Background())
	if err := base.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}

	tenantOne := gormkit.WithTenantID(context.Background(), 1)
	tenantTwo := gormkit.WithTenantID(context.Background(), 2)
	victim := &tenantRecord{Name: "victim"}
	if err := database.Client(tenantOne).Create(victim).Error; err != nil {
		t.Fatal(err)
	}

	repo := gormkit.NewRepo[*tenantRecord](database.Client(tenantTwo))
	err := repo.Update(&tenantRecord{ID: victim.ID, Name: "overwritten"})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-tenant Update: %v", err)
	}

	var stored tenantRecord
	if err := database.Client(gormkit.WithDisableTenant(context.Background())).
		Unscoped().First(&stored, victim.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TenantID != 1 || stored.Name != "victim" {
		t.Fatalf("cross-tenant Update changed row: %+v", stored)
	}
}

func TestTenantRejectsConflictUpdatingUpsert(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(context.Background())
	if err := base.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}

	tenantOne := gormkit.WithTenantID(context.Background(), 1)
	tenantTwo := gormkit.WithTenantID(context.Background(), 2)
	victim := &tenantRecord{Name: "victim"}
	if err := database.Client(tenantOne).Create(victim).Error; err != nil {
		t.Fatal(err)
	}

	err := database.Client(tenantTwo).Save(&tenantRecord{
		ID:   victim.ID,
		Name: "overwritten",
	}).Error
	if !errors.Is(err, gormkit.ErrUnsafeTenantUpsert) {
		t.Fatalf("cross-tenant GORM Save: %v", err)
	}
	err = database.Client(tenantTwo).Clauses(clause.OnConflict{
		DoUpdates: clause.AssignmentColumns([]string{"name"}),
	}).Create(&tenantRecord{ID: victim.ID, Name: "also overwritten"}).Error
	if !errors.Is(err, gormkit.ErrUnsafeTenantUpsert) {
		t.Fatalf("cross-tenant conflict update: %v", err)
	}
	if err := database.Client(tenantTwo).Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(&tenantRecord{ID: victim.ID, Name: "ignored"}).Error; err != nil {
		t.Fatalf("conflict do nothing: %v", err)
	}

	var stored tenantRecord
	if err := database.Client(gormkit.WithDisableTenant(context.Background())).
		Unscoped().First(&stored, victim.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TenantID != 1 || stored.Name != "victim" {
		t.Fatalf("unsafe upsert changed row: %+v", stored)
	}
}

type tenantSoftRecord struct {
	ID        uint             `gorm:"primaryKey"`
	TenantID  gormkit.TenantID `gorm:"column:tenant_id;index"`
	Name      string
	DeletedAt gorm.DeletedAt
}

func TestTenantUnscopedOnlyDisablesSoftDelete(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(gormkit.WithDisableTenant(context.Background()))
	if err := base.AutoMigrate(&tenantSoftRecord{}); err != nil {
		t.Fatal(err)
	}

	tenantOne := gormkit.WithTenantID(context.Background(), 1)
	tenantTwo := gormkit.WithTenantID(context.Background(), 2)
	one := &tenantSoftRecord{Name: "one"}
	two := &tenantSoftRecord{Name: "two"}
	if err := database.Client(tenantOne).Create(one).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Client(tenantTwo).Create(two).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Client(tenantOne).Delete(one).Error; err != nil {
		t.Fatal(err)
	}

	var rows []tenantSoftRecord
	if err := database.Client(tenantOne).Unscoped().Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != one.ID {
		t.Fatalf("Unscoped crossed tenant boundary: %+v", rows)
	}

	rows = nil
	if err := database.Client(gormkit.WithDisableTenant(tenantOne)).Unscoped().Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("disabled tenant row count = %d, want 2", len(rows))
	}
}

func TestTenantManualAndDisabledModes(t *testing.T) {
	database := openTestDatabase(t)
	base := database.Client(gormkit.WithDisableTenant(context.Background()))
	if err := base.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}

	tenantOne := gormkit.WithTenantID(context.Background(), 1)
	tenantTwo := gormkit.WithTenantID(context.Background(), 2)
	if err := database.Client(tenantOne).Create(&tenantRecord{Name: "one"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Client(tenantTwo).Create(&tenantRecord{Name: "two"}).Error; err != nil {
		t.Fatal(err)
	}

	var rows []tenantRecord
	err := database.Client(tenantOne).
		Raw("SELECT * FROM tenant_records WHERE tenant_id = ?", 1).
		Scan(&rows).Error
	if !errors.Is(err, gormkit.ErrManualTenantRequired) {
		t.Fatalf("automatic tenant Raw: %v", err)
	}

	manual := gormkit.WithManualTenant(tenantOne)
	manualTenantID, ok := gormkit.GetTenantID(manual)
	if !ok || manualTenantID != 1 {
		t.Fatalf("manual tenant identity = (%d, %v), want (1, true)", manualTenantID, ok)
	}
	rows = nil
	if err := database.Client(manual).
		Raw("SELECT * FROM tenant_records WHERE tenant_id = ?", manualTenantID).
		Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "one" {
		t.Fatalf("manual tenant rows: %+v", rows)
	}
	if err := database.Client(tenantOne).
		Exec("UPDATE tenant_records SET name = ? WHERE tenant_id = ?", "blocked", 1).Error; !errors.Is(err, gormkit.ErrManualTenantRequired) {
		t.Fatalf("automatic tenant Exec: %v", err)
	}
	if err := database.Client(manual).
		Exec("UPDATE tenant_records SET name = ? WHERE tenant_id = ?", "updated", manualTenantID).Error; err != nil {
		t.Fatal(err)
	}

	rows = nil
	if err := database.Client(manual).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "updated" {
		t.Fatalf("manual mode regular CRUD lost automatic isolation: %+v", rows)
	}

	rows = nil
	if err := database.Client(tenantOne).Table("tenant_records").Find(&rows).Error; !errors.Is(err, gormkit.ErrManualTenantRequired) {
		t.Fatalf("automatic tenant Table: %v", err)
	}

	rows = nil
	disabled := gormkit.WithDisableTenant(tenantOne)
	if _, ok := gormkit.GetTenantID(disabled); ok {
		t.Fatal("disabled tenant retained a tenant ID")
	}
	if err := database.Client(disabled).Raw("SELECT * FROM tenant_records").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("disabled tenant rows = %d, want 2", len(rows))
	}

	missing := gormkit.WithManualTenant(context.Background())
	if err := database.Client(missing).Find(&rows).Error; !errors.Is(err, gormkit.ErrTenantRequired) {
		t.Fatalf("manual mode without tenant: %v", err)
	}
}
