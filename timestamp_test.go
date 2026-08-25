package gormkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/Heming9/gormkit"
)

type timestampRecord struct {
	ID    uint `gorm:"primaryKey"`
	Name  string
	CTime int64 `gorm:"column:ctime"`
	MTime int64 `gorm:"column:mtime"`
}

func TestTimestampPlugin(t *testing.T) {
	plugin := gormkit.NewTimestampPlugin()
	now := time.Unix(1_700_000_000, 0)
	plugin.Now = func() time.Time { return now }
	database := openTestDatabase(t, plugin)
	db := database.Client(context.Background())
	if err := db.AutoMigrate(&timestampRecord{}); err != nil {
		t.Fatal(err)
	}
	record := &timestampRecord{Name: "created"}
	if err := db.Create(record).Error; err != nil {
		t.Fatal(err)
	}
	if record.CTime != now.Unix() || record.MTime != now.Unix() {
		t.Fatalf("create timestamps: %+v", record)
	}
	now = now.Add(time.Minute)
	if err := db.Model(record).Update("name", "updated").Error; err != nil {
		t.Fatal(err)
	}
	if record.CTime != 1_700_000_000 || record.MTime != now.Unix() {
		t.Fatalf("update timestamps: %+v", record)
	}
}
