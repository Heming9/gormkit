package gormkit

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// TransactionPropagation controls how a transaction interacts with an
// existing transaction stored in the context.
type TransactionPropagation uint8

const (
	// TPRequired reuses an existing transaction or starts a new one.
	TPRequired TransactionPropagation = iota
	// TPNested creates a nested transaction when one already exists.
	TPNested
)

var ErrInvalidTransactionPropagation = errors.New("gormkit: invalid transaction propagation")

type transactionKey struct{}

type transactionBinding struct {
	database *Database
	client   Client
}

func transactionFromContext(ctx context.Context) *transactionBinding {
	if ctx == nil {
		return nil
	}
	binding, _ := ctx.Value(transactionKey{}).(*transactionBinding)
	return binding
}

func withTransaction(ctx context.Context, database *Database, tx Client) context.Context {
	return context.WithValue(ctx, transactionKey{}, &transactionBinding{
		database: database,
		client:   tx,
	})
}

// GetTransaction returns the transaction stored in ctx, if any.
func GetTransaction(ctx context.Context) Client {
	binding := transactionFromContext(ctx)
	if binding == nil {
		return nil
	}
	return binding.client
}

// Transaction executes fn using the process-wide default database.
func Transaction(ctx context.Context, fn func(context.Context) error, options ...TransactionPropagation) error {
	db, err := Default()
	if err != nil {
		return err
	}
	return db.Transaction(ctx, fn, options...)
}

// Transaction executes fn in a transaction and propagates the transaction
// through the callback context.
func (d *Database) Transaction(ctx context.Context, fn func(context.Context) error, options ...TransactionPropagation) error {
	if d == nil || d.client == nil {
		return ErrNilDatabase
	}
	if fn == nil {
		return errors.New("gormkit: transaction callback must not be nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(options) > 1 {
		return ErrInvalidTransactionPropagation
	}
	propagation := TPRequired
	if len(options) == 1 {
		propagation = options[0]
	}
	binding := transactionFromContext(ctx)
	var existing Client
	if binding != nil && binding.database == d {
		existing = binding.client
	}
	switch propagation {
	case TPRequired:
		if existing != nil {
			return fn(ctx)
		}
	case TPNested:
		if existing != nil {
			return existing.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return fn(withTransaction(ctx, d, tx))
			})
		}
	default:
		return ErrInvalidTransactionPropagation
	}
	return d.client.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTransaction(ctx, d, tx))
	})
}
