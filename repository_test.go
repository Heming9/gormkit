package gormkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Heming9/gormkit"
	"gorm.io/gorm"
)

func TestRepositoryCRUDAndErrors(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))

	alice := &testUser{Name: "Alice", Email: "alice@example.test", Age: 30}
	if err := repo.Save(alice); err != nil {
		t.Fatalf("save: %v", err)
	}
	found, err := repo.FindByID(alice.ID)
	if err != nil || found.Name != alice.Name {
		t.Fatalf("find by ID: found=%+v err=%v", found, err)
	}
	if _, err := repo.FindByID(uint(99999)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}

	alice.Age = 31
	if err := repo.Save(alice); err != nil {
		t.Fatalf("update: %v", err)
	}
	exists, err := repo.Exists("email = ?", alice.Email)
	if err != nil || !exists {
		t.Fatalf("exists: exists=%v err=%v", exists, err)
	}
	if err := repo.DeleteBy("id = ?", alice.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	exists, err = repo.Exists("id = ?", alice.ID)
	if err != nil || exists {
		t.Fatalf("exists after delete: exists=%v err=%v", exists, err)
	}
}

func TestRepositorySaveWithUniqueCondition(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))

	first := &testUser{Name: "first", Email: "same@example.test"}
	if err := repo.Save(first); err != nil {
		t.Fatal(err)
	}
	update := &testUser{Name: "updated"}
	if err := repo.Save(update, "email = ?", first.Email); err != nil {
		t.Fatalf("conditional update: %v", err)
	}
	if update.ID != first.ID {
		t.Fatalf("primary key was not copied: got %d want %d", update.ID, first.ID)
	}

	if err := repo.Save(&testUser{Name: "second", Email: first.Email}); err != nil {
		t.Fatal(err)
	}
	err := repo.Save(&testUser{Name: "ambiguous"}, "email = ?", first.Email)
	if !errors.Is(err, gormkit.ErrMultipleRows) {
		t.Fatalf("expected ErrMultipleRows, got %v", err)
	}
}

func TestRepositoryPagingAndScopeIsolation(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))
	if err := repo.SaveAll(
		&testUser{Name: "Charlie", Age: 40},
		&testUser{Name: "Alice", Age: 20},
		&testUser{Name: "Bob", Age: 30},
	); err != nil {
		t.Fatal(err)
	}
	paging := &testPaging{offset: 1, limit: 1}
	ordered := repo.WithScope(gormkit.OrderBy(gormkit.Asc("name"))).WithPaging(paging)
	users, err := ordered.FindAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Name != "Bob" || paging.total != 3 {
		t.Fatalf("unexpected page: users=%+v total=%d", users, paging.total)
	}
	all, err := repo.FindAll()
	if err != nil || len(all) != 3 {
		t.Fatalf("scope leaked into base repository: count=%d err=%v", len(all), err)
	}

	invalid := &testPaging{offset: -1, limit: 10}
	if _, err := repo.WithPaging(invalid).FindAll(); !errors.Is(err, gormkit.ErrInvalidPaging) {
		t.Fatalf("expected ErrInvalidPaging, got %v", err)
	}
}

func TestForceUpdatesZeroValues(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))
	user := &testUser{Name: "Alice", Age: 30, Admin: true}
	if err := repo.Save(user); err != nil {
		t.Fatal(err)
	}
	forced := repo.WithScope(gormkit.Force("Name", "Admin"))
	if err := forced.UpdateBy(&testUser{Name: "", Admin: false}, "id = ?", user.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "" || got.Admin || got.Age != 30 {
		t.Fatalf("Force result: %+v", got)
	}
	if err := repo.WithScope(gormkit.Force("Missing")).UpdateBy(&testUser{}, "id = ?", user.ID); err == nil {
		t.Fatal("expected invalid Force field error")
	}
}
