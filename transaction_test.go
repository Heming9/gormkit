package gormkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Heming9/gormkit"
)

var errRollback = errors.New("rollback")

func TestTransactionCommitAndRollback(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)

	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		return gormkit.NewRepository[*testUser](database.Client(ctx)).Save(&testUser{Name: "committed"})
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	err = database.Transaction(context.Background(), func(ctx context.Context) error {
		if err := gormkit.NewRepository[*testUser](database.Client(ctx)).Save(&testUser{Name: "rolled back"}); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("rollback error: %v", err)
	}

	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))
	users, err := repo.FindAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Name != "committed" {
		t.Fatalf("unexpected committed rows: %+v", users)
	}
}

func TestTransactionPropagation(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)

	err := database.Transaction(context.Background(), func(outer context.Context) error {
		outerClient := database.Client(outer)
		if gormkit.GetTransaction(outer) == nil {
			t.Fatal("outer transaction missing from context")
		}
		if err := outerClient.Create(&testUser{Name: "outer"}).Error; err != nil {
			return err
		}
		if err := database.Transaction(outer, func(required context.Context) error {
			if gormkit.GetTransaction(required) != gormkit.GetTransaction(outer) {
				t.Fatal("TPRequired did not reuse transaction")
			}
			return database.Client(required).Create(&testUser{Name: "required"}).Error
		}); err != nil {
			return err
		}
		nestedErr := database.Transaction(outer, func(nested context.Context) error {
			if gormkit.GetTransaction(nested) == gormkit.GetTransaction(outer) {
				t.Fatal("TPNested reused outer handle")
			}
			if err := database.Client(nested).Create(&testUser{Name: "nested"}).Error; err != nil {
				return err
			}
			return errRollback
		}, gormkit.TPNested)
		if !errors.Is(nestedErr, errRollback) {
			return nestedErr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))
	users, err := repo.WithScope(func(db gormkit.Client) gormkit.Client {
		return db.Order("name")
	}).FindAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].Name != "outer" || users[1].Name != "required" {
		t.Fatalf("nested transaction was not rolled back: %+v", users)
	}
}

func TestTransactionValidation(t *testing.T) {
	database := openTestDatabase(t)
	noop := func(context.Context) error { return nil }
	if err := database.Transaction(context.Background(), noop, gormkit.TransactionPropagation(99)); !errors.Is(err, gormkit.ErrInvalidTransactionPropagation) {
		t.Fatalf("invalid propagation: %v", err)
	}
	if err := database.Transaction(context.Background(), noop, gormkit.TPRequired, gormkit.TPNested); !errors.Is(err, gormkit.ErrInvalidTransactionPropagation) {
		t.Fatalf("multiple propagation options: %v", err)
	}
	if err := database.Transaction(context.Background(), nil); err == nil {
		t.Fatal("expected nil callback error")
	}
}

func TestTransactionContextDoesNotCrossDatabaseInstances(t *testing.T) {
	first := openTestDatabase(t)
	second := openTestDatabase(t)
	migrateTestUsers(t, first)
	migrateTestUsers(t, second)

	err := first.Transaction(context.Background(), func(ctx context.Context) error {
		if err := first.Client(ctx).Create(&testUser{Name: "first"}).Error; err != nil {
			return err
		}
		return second.Transaction(ctx, func(secondContext context.Context) error {
			return second.Client(secondContext).Create(&testUser{Name: "second"}).Error
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	var firstCount, secondCount int64
	if err := first.Client(nil).Model(&testUser{}).Count(&firstCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := second.Client(nil).Model(&testUser{}).Count(&secondCount).Error; err != nil {
		t.Fatal(err)
	}
	if firstCount != 1 || secondCount != 1 {
		t.Fatalf("transaction crossed database instances: first=%d second=%d", firstCount, secondCount)
	}
}

func TestTransactionRollsBackPanic(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("expected transaction panic")
			}
		}()
		_ = database.Transaction(context.Background(), func(ctx context.Context) error {
			if err := database.Client(ctx).Create(&testUser{Name: "panic"}).Error; err != nil {
				return err
			}
			panic("boom")
		})
	}()

	var count int64
	if err := database.Client(context.Background()).Model(&testUser{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("panic transaction committed %d rows", count)
	}
}
