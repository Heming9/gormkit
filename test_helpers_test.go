package gormkit_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Heming9/gormkit"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testDatabaseID atomic.Uint64

type testUser struct {
	ID    uint `gorm:"primaryKey"`
	Name  string
	Email string
	Age   int
	Admin bool
}

type testPaging struct {
	offset int
	limit  int
	total  int64
}

func (paging *testPaging) Offset() int          { return paging.offset }
func (paging *testPaging) Limit() int           { return paging.limit }
func (paging *testPaging) SetTotal(total int64) { paging.total = total }

func openTestDatabase(t *testing.T, plugins ...gorm.Plugin) *gormkit.Database {
	t.Helper()
	dsn := fmt.Sprintf("file:gormkit-%d?mode=memory&cache=shared", testDatabaseID.Add(1))
	db, err := gormkit.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard}, plugins...)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return db
}

func migrateTestUsers(t *testing.T, db *gormkit.Database) {
	t.Helper()
	if err := db.Client(context.Background()).AutoMigrate(&testUser{}); err != nil {
		t.Fatalf("migrate test users: %v", err)
	}
}
