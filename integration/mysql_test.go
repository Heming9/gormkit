//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/Heming9/gormkit"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type user struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func (user) TableName() string { return "gormkit_integration_user" }

type tenantUser struct {
	ID       uint             `gorm:"primaryKey"`
	TenantID gormkit.TenantID `gorm:"column:tenant_id;index"`
	Name     string
}

func (tenantUser) TableName() string { return "gormkit_integration_tenant_user" }

func openMySQL(t *testing.T) *gormkit.Database {
	t.Helper()
	address := os.Getenv("MYSQL_ADDRESS")
	if address == "" {
		t.Skip("MYSQL_ADDRESS is not set")
	}
	database, err := gormkit.OpenMySQL(gormkit.MySQLConfig{
		Username: os.Getenv("MYSQL_USERNAME"),
		Password: os.Getenv("MYSQL_PASSWORD"),
		Address:  address,
		Database: os.Getenv("MYSQL_DATABASE"),
		Logger:   logger.Discard,
	})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Client(gormkit.WithDisableTenant(context.Background())).Migrator().DropTable(&user{}, &tenantUser{})
		if err := database.Close(); err != nil {
			t.Errorf("close MySQL: %v", err)
		}
	})
	return database
}

func TestMySQLTenantNestedTransaction(t *testing.T) {
	database := openMySQL(t)
	base := database.Client(gormkit.WithDisableTenant(context.Background()))
	if err := base.Migrator().DropTable(&tenantUser{}); err != nil {
		t.Fatal(err)
	}
	if err := base.AutoMigrate(&tenantUser{}); err != nil {
		t.Fatal(err)
	}

	ctx := gormkit.WithTenantID(context.Background(), 7)
	errRollback := errors.New("nested rollback")
	err := database.Client(ctx).Transaction(func(outer *gorm.DB) error {
		if err := outer.Create(&tenantUser{Name: "outer"}).Error; err != nil {
			return err
		}
		if err := outer.Transaction(func(nested *gorm.DB) error {
			return nested.Create(&tenantUser{Name: "nested committed"}).Error
		}); err != nil {
			return err
		}
		nestedErr := outer.Transaction(func(nested *gorm.DB) error {
			if err := nested.Create(&tenantUser{Name: "nested rolled back"}).Error; err != nil {
				return err
			}
			return errRollback
		})
		if !errors.Is(nestedErr, errRollback) {
			return nestedErr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var rows []tenantUser
	if err := database.Client(ctx).Order("name").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Name != "nested committed" || rows[1].Name != "outer" {
		t.Fatalf("unexpected MySQL tenant rows: %+v", rows)
	}
}

func TestMySQLRepositoryAndTransaction(t *testing.T) {
	database := openMySQL(t)
	client := database.Client(context.Background())
	if err := client.Migrator().DropTable(&user{}); err != nil {
		t.Fatal(err)
	}
	if err := client.AutoMigrate(&user{}); err != nil {
		t.Fatal(err)
	}

	committed := &user{Name: "committed"}
	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		repo := gormkit.NewRepo[*user](database.Client(ctx))
		return repo.Create(committed)
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := gormkit.NewRepo[*user](client)
	// MySQL commonly reports zero affected rows for an idempotent update. The
	// repository verifies existence and still treats it as success.
	if err := repo.Update(committed); err != nil {
		t.Fatalf("idempotent update: %v", err)
	}
	err = database.Transaction(context.Background(), func(ctx context.Context) error {
		if err := database.Client(ctx).Create(&user{Name: "rolled-back"}).Error; err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	var users []user
	if err := client.Find(&users).Error; err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Name != "committed" {
		t.Fatalf("unexpected MySQL rows: %+v", users)
	}
	if _, err := gormkit.Wrap((*gorm.DB)(nil)); !errors.Is(err, gormkit.ErrNilDatabase) {
		t.Fatalf("Wrap(nil): %v", err)
	}
}

func TestMySQLTransactionIsolation(t *testing.T) {
	database := openMySQL(t)
	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		var isolation string
		if err := database.Client(ctx).Raw(`
			SELECT transaction.ISOLATION_LEVEL
			FROM performance_schema.events_transactions_current AS transaction
			JOIN performance_schema.threads AS thread USING (THREAD_ID)
			WHERE thread.PROCESSLIST_ID = CONNECTION_ID()
			  AND transaction.STATE = 'ACTIVE'
		`).Scan(&isolation).Error; err != nil {
			return err
		}
		if isolation != "READ COMMITTED" {
			t.Fatalf("transaction isolation = %q, want READ COMMITTED", isolation)
		}
		return nil
	}, gormkit.WithTxOptions(&sql.TxOptions{Isolation: sql.LevelReadCommitted}))
	if err != nil {
		t.Fatal(err)
	}

	called := false
	err = database.Transaction(context.Background(), func(context.Context) error {
		called = true
		return nil
	}, gormkit.WithTxOptions(&sql.TxOptions{Isolation: sql.LevelSnapshot}))
	if err == nil {
		t.Fatal("unsupported isolation level succeeded")
	}
	if called {
		t.Fatal("callback ran with an unsupported isolation level")
	}
}
