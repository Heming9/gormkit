package gormkit

import (
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestMySQLDSNPreservesDriverDefaults(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load test location: %v", err)
	}
	dsn := mysqlDSN(MySQLConfig{
		Username: "user",
		Password: "password",
		Address:  "127.0.0.1:3306",
		Database: "app",
	}, "utf8mb4", location)

	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse generated DSN: %v", err)
	}
	if !config.AllowNativePasswords {
		t.Fatal("generated DSN must allow mysql_native_password authentication")
	}
	if !config.ParseTime {
		t.Fatal("generated DSN must enable parseTime")
	}
	if config.Loc.String() != location.String() {
		t.Fatalf("location = %q, want %q", config.Loc, location)
	}
	if config.Params["charset"] != "utf8mb4" {
		t.Fatalf("charset = %q, want utf8mb4", config.Params["charset"])
	}
}
