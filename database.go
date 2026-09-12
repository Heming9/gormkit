package gormkit

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"

	"gorm.io/gorm"
)

// Client is an alias for GORM's database handle.
type Client = *gorm.DB

var (
	// ErrNotInitialized indicates that no process-wide default database exists.
	ErrNotInitialized = errors.New("gormkit: default database is not initialized")
	// ErrNilDatabase indicates that a nil GORM database was supplied.
	ErrNilDatabase = errors.New("gormkit: database must not be nil")
)

// Database owns a configured GORM database handle.
type Database struct {
	client *gorm.DB
}

var defaultDatabase atomic.Pointer[Database]

// Open creates a Database from a GORM dialector and configuration.
// Force update support is registered on every database opened by this function.
func Open(dialector gorm.Dialector, config *gorm.Config, plugins ...gorm.Plugin) (*Database, error) {
	if dialector == nil {
		return nil, errors.New("gormkit: dialector must not be nil")
	}
	if config == nil {
		config = &gorm.Config{}
	}
	db, err := gorm.Open(dialector, config)
	if err != nil {
		return nil, err
	}
	if err := registerForceCallback(db); err != nil {
		closeGORM(db)
		return nil, err
	}
	for _, plugin := range plugins {
		if plugin == nil {
			closeGORM(db)
			return nil, errors.New("gormkit: plugin must not be nil")
		}
		if err := db.Use(plugin); err != nil {
			closeGORM(db)
			return nil, err
		}
	}
	return &Database{client: db}, nil
}

// Wrap creates a Database around an existing GORM handle.
// It does not register callbacks or plugins on the supplied handle.
func Wrap(db *gorm.DB) (*Database, error) {
	if db == nil {
		return nil, ErrNilDatabase
	}
	return &Database{client: db}, nil
}

// Client returns a session bound to ctx. A nil context is treated as
// context.Background().
func (d *Database) Client(ctx context.Context) Client {
	if d == nil || d.client == nil {
		panic(ErrNilDatabase)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if binding := transactionFromContext(ctx); binding != nil && binding.database == d {
		return binding.client.WithContext(ctx)
	}
	return d.client.WithContext(ctx)
}

// SQLDB exposes the underlying database/sql connection pool.
func (d *Database) SQLDB() (*sql.DB, error) {
	if d == nil || d.client == nil {
		return nil, ErrNilDatabase
	}
	return d.client.DB()
}

// Close closes the underlying database/sql connection pool.
func (d *Database) Close() error {
	pool, err := d.SQLDB()
	if err != nil {
		return err
	}
	defaultDatabase.CompareAndSwap(d, nil)
	return pool.Close()
}

// UseDefault installs db as the process-wide default database.
func UseDefault(db *Database) error {
	if db == nil || db.client == nil {
		return ErrNilDatabase
	}
	defaultDatabase.Store(db)
	return nil
}

// Default returns the process-wide default database.
func Default() (*Database, error) {
	db := defaultDatabase.Load()
	if db == nil {
		return nil, ErrNotInitialized
	}
	return db, nil
}

// Connected reports whether a process-wide default database is installed.
func Connected() bool {
	return defaultDatabase.Load() != nil
}

// SQLClient returns the default database session bound to ctx.
func SQLClient(ctx context.Context) (Client, error) {
	db, err := Default()
	if err != nil {
		return nil, err
	}
	return db.Client(ctx), nil
}

// MustSQLClient returns the default database session bound to ctx and panics
// when no default database has been installed.
func MustSQLClient(ctx context.Context) Client {
	db, err := SQLClient(ctx)
	if err != nil {
		panic(err)
	}
	return db
}

func closeGORM(db *gorm.DB) {
	if pool, err := db.DB(); err == nil {
		_ = pool.Close()
	}
}
