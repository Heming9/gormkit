package gormkit

import (
	"errors"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// MySQLConfig configures the optional MySQL convenience opener.
type MySQLConfig struct {
	Username                                  string
	Password                                  string
	Address                                   string
	Database                                  string
	Charset                                   string
	TablePrefix                               string
	Location                                  *time.Location
	Logger                                    logger.Interface
	Plugins                                   []gorm.Plugin
	SingularTable                             bool
	DisableForeignKeyConstraintsWhenMigrating bool
	MaxIdleConnections                        int
	MaxOpenConnections                        int
	ConnectionMaxIdleTime                     time.Duration
	ConnectionMaxLifetime                     time.Duration
}

// OpenMySQL opens a MySQL database without installing it as the process default.
func OpenMySQL(config MySQLConfig) (*Database, error) {
	if config.Address == "" {
		return nil, errors.New("gormkit: MySQL address must not be empty")
	}
	if config.Database == "" {
		return nil, errors.New("gormkit: MySQL database must not be empty")
	}
	charset := config.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	location := config.Location
	if location == nil {
		location = time.Local
	}
	dsn := mysqlDSN(config, charset, location)
	gormLogger := config.Logger
	if gormLogger == nil {
		gormLogger = logger.Discard
	}
	db, err := Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   config.TablePrefix,
			SingularTable: config.SingularTable,
		},
		Logger:                                   gormLogger,
		DisableForeignKeyConstraintWhenMigrating: config.DisableForeignKeyConstraintsWhenMigrating,
	}, config.Plugins...)
	if err != nil {
		return nil, err
	}
	pool, err := db.SQLDB()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if config.MaxIdleConnections > 0 {
		pool.SetMaxIdleConns(config.MaxIdleConnections)
	}
	if config.MaxOpenConnections > 0 {
		pool.SetMaxOpenConns(config.MaxOpenConnections)
	}
	if config.ConnectionMaxIdleTime > 0 {
		pool.SetConnMaxIdleTime(config.ConnectionMaxIdleTime)
	}
	if config.ConnectionMaxLifetime > 0 {
		pool.SetConnMaxLifetime(config.ConnectionMaxLifetime)
	}
	return db, nil
}

func mysqlDSN(config MySQLConfig, charset string, location *time.Location) string {
	driverConfig := mysqldriver.NewConfig()
	driverConfig.User = config.Username
	driverConfig.Passwd = config.Password
	driverConfig.Net = "tcp"
	driverConfig.Addr = config.Address
	driverConfig.DBName = config.Database
	driverConfig.ParseTime = true
	driverConfig.Loc = location
	driverConfig.Params = map[string]string{"charset": charset}
	return driverConfig.FormatDSN()
}

// ConnectMySQL opens MySQL and installs it as the process-wide default database.
func ConnectMySQL(config *MySQLConfig) error {
	if config == nil {
		return errors.New("gormkit: MySQL config must not be nil")
	}
	db, err := OpenMySQL(*config)
	if err != nil {
		return err
	}
	return UseDefault(db)
}

// Connect is retained for source compatibility.
// Deprecated: use ConnectMySQL.
func Connect(config *MySQLConfig) error {
	return ConnectMySQL(config)
}
