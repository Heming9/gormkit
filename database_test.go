package gormkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Heming9/gormkit"
)

func TestDatabaseLifecycle(t *testing.T) {
	db := openTestDatabase(t)
	pool, err := db.SQLDB()
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if err := pool.PingContext(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if got := db.Client(nil); got == nil {
		t.Fatal("Client(nil) returned nil")
	}
}

func TestWrapRejectsNil(t *testing.T) {
	_, err := gormkit.Wrap(nil)
	if !errors.Is(err, gormkit.ErrNilDatabase) {
		t.Fatalf("expected ErrNilDatabase, got %v", err)
	}
}

func TestUseDefaultAndSQLClient(t *testing.T) {
	db := openTestDatabase(t)
	if err := gormkit.UseDefault(db); err != nil {
		t.Fatalf("use default: %v", err)
	}
	client, err := gormkit.SQLClient(context.Background())
	if err != nil {
		t.Fatalf("SQLClient: %v", err)
	}
	if client == nil || !gormkit.Connected() {
		t.Fatal("default database was not installed")
	}
}
