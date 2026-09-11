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
	if err := repo.Create(alice); err != nil {
		t.Fatalf("create: %v", err)
	}
	found, err := repo.FindByID(alice.ID)
	if err != nil || found.Name != alice.Name {
		t.Fatalf("find by ID: found=%+v err=%v", found, err)
	}
	if _, err := repo.FindByID(uint(99999)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}

	alice.Age = 31
	if err := repo.Update(alice); err != nil {
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

func TestRepositoryCreateIsInsertOnly(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))

	created := &testUser{Name: "created"}
	if err := repo.Create(created); err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("Create did not populate the generated primary key")
	}

	duplicate := &testUser{ID: created.ID, Name: "duplicate"}
	err := repo.Create(duplicate)
	if err == nil {
		t.Fatal("Create must return a primary-key conflict instead of updating")
	}
	found, findErr := repo.FindByID(created.ID)
	if findErr != nil {
		t.Fatalf("find original row: %v", findErr)
	}
	if found.Name != created.Name {
		t.Fatalf("conflicting Create changed the row: got %q want %q", found.Name, created.Name)
	}
}

func TestRepositoryCreateAllUsesOneBatchAndPopulatesIDs(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))

	first := &testUser{Name: "first"}
	second := &testUser{Name: "second"}
	if err := repo.CreateAll(first, second); err != nil {
		t.Fatalf("create all: %v", err)
	}
	if first.ID == 0 || second.ID == 0 {
		t.Fatalf("CreateAll did not populate generated primary keys: first=%d second=%d", first.ID, second.ID)
	}
	count, err := repo.CountBy()
	if err != nil || count != 2 {
		t.Fatalf("count after CreateAll: count=%d err=%v", count, err)
	}
}

func TestRepositoryCreateAllEmptyIsNoOp(t *testing.T) {
	repo := gormkit.NewRepository[*testUser](nil)
	if err := repo.CreateAll(); err != nil {
		t.Fatalf("empty CreateAll: %v", err)
	}
}

func TestRepositoryUpdateIncludesZeroValues(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))

	user := &testUser{Name: "first", Email: "first@example.test", Age: 30, Admin: true}
	if err := repo.Create(user); err != nil {
		t.Fatal(err)
	}
	user.Name = ""
	user.Age = 0
	user.Admin = false
	if err := repo.Update(user); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := repo.FindByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "" || got.Age != 0 || got.Admin || got.Email != user.Email {
		t.Fatalf("Update did not include zero values: %+v", got)
	}
}

func TestRepositoryUpdateValidation(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))

	if err := repo.Update(nil); !errors.Is(err, gormkit.ErrNilEntity) {
		t.Fatalf("Update(nil): %v", err)
	}
	if err := repo.Update(&testUser{Name: "missing key"}); !errors.Is(err, gormkit.ErrPrimaryKeyRequired) {
		t.Fatalf("Update with zero primary key: %v", err)
	}
	if err := repo.Update(&testUser{ID: 99999, Name: "missing"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("Update missing row: %v", err)
	}
}

func TestRepositoryUpdateAllIsAtomic(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))

	first := &testUser{Name: "first"}
	second := &testUser{Name: "second"}
	if err := repo.CreateAll(first, second); err != nil {
		t.Fatal(err)
	}
	first.Name = "updated first"
	second.Name = "updated second"
	if err := repo.UpdateAll(first, second); err != nil {
		t.Fatalf("UpdateAll: %v", err)
	}

	first.Name = "must roll back"
	err := repo.UpdateAll(first, &testUser{ID: 99999, Name: "missing"})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("UpdateAll missing row: %v", err)
	}
	stored, err := repo.FindByID(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "updated first" {
		t.Fatalf("UpdateAll did not roll back: %+v", stored)
	}
}

func TestRepositoryUpdateAllEmptyIsNoOp(t *testing.T) {
	repo := gormkit.NewRepository[*testUser](nil)
	if err := repo.UpdateAll(); err != nil {
		t.Fatalf("empty UpdateAll: %v", err)
	}
}

func TestRepositoryPagingAndScopeIsolation(t *testing.T) {
	database := openTestDatabase(t)
	migrateTestUsers(t, database)
	repo := gormkit.NewRepository[*testUser](database.Client(context.Background()))
	if err := repo.CreateAll(
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
	if err := repo.Create(user); err != nil {
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
