//go:build integration

package integration_test

import (
	"context"
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
		_ = database.Client(context.Background()).Migrator().DropTable(&user{})
		if err := database.Close(); err != nil {
			t.Errorf("close MySQL: %v", err)
		}
	})
	return database
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

	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		repo := gormkit.NewRepository[*user](database.Client(ctx))
		return repo.Save(&user{Name: "committed"})
	})
	if err != nil {
		t.Fatal(err)
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
