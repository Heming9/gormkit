package gormkit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

var (
	ErrInvalidTransactionPropagation = errors.New("gormkit: invalid transaction propagation")
	ErrInvalidTransactionOption      = errors.New("gormkit: invalid transaction option")
	ErrIncompatibleTxOptions         = errors.New("gormkit: transaction options are incompatible with the existing transaction")
)

// TransactionOption configures a transaction. TransactionPropagation values
// also implement TransactionOption, so calls that pass TPRequired or TPNested
// directly remain supported.
type TransactionOption interface {
	applyTransactionOption(*transactionConfig) error
}

type transactionConfig struct {
	propagation    TransactionPropagation
	propagationSet bool
	txOptions      sql.TxOptions
	txOptionsSet   bool
}

func (p TransactionPropagation) applyTransactionOption(config *transactionConfig) error {
	if p != TPRequired && p != TPNested {
		return ErrInvalidTransactionPropagation
	}
	if config.propagationSet {
		return ErrInvalidTransactionPropagation
	}
	config.propagation = p
	config.propagationSet = true
	return nil
}

// WithPropagation configures how the transaction interacts with a transaction
// already stored in ctx.
func WithPropagation(propagation TransactionPropagation) TransactionOption {
	return propagation
}

type txOptionsOption sql.TxOptions

func (option txOptionsOption) applyTransactionOption(config *transactionConfig) error {
	if config.txOptionsSet {
		return ErrInvalidTransactionOption
	}
	config.txOptions = sql.TxOptions(option)
	config.txOptionsSet = true
	return nil
}

// WithTxOptions passes options to the driver when a root transaction is
// started. The value is copied when WithTxOptions is called. A nil value means
// the default isolation level and a read-write transaction.
func WithTxOptions(options *sql.TxOptions) TransactionOption {
	if options == nil {
		return txOptionsOption(sql.TxOptions{})
	}
	return txOptionsOption(*options)
}

type transactionKey struct{}

type transactionBinding struct {
	database  *Database
	client    Client
	txOptions sql.TxOptions
}

func transactionFromContext(ctx context.Context) *transactionBinding {
	if ctx == nil {
		return nil
	}
	binding, _ := ctx.Value(transactionKey{}).(*transactionBinding)
	return binding
}

func withTransaction(ctx context.Context, database *Database, tx Client, options sql.TxOptions) context.Context {
	return context.WithValue(ctx, transactionKey{}, &transactionBinding{
		database:  database,
		client:    tx,
		txOptions: options,
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
func Transaction(ctx context.Context, fn func(context.Context) error, options ...TransactionOption) error {
	db, err := Default()
	if err != nil {
		return err
	}
	return db.Transaction(ctx, fn, options...)
}

// Transaction executes fn in a transaction and propagates the transaction
// through the callback context.
func (d *Database) Transaction(ctx context.Context, fn func(context.Context) error, options ...TransactionOption) error {
	if d == nil || d.client == nil {
		return ErrNilDatabase
	}
	if fn == nil {
		return errors.New("gormkit: transaction callback must not be nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	config := transactionConfig{propagation: TPRequired}
	for _, option := range options {
		if option == nil {
			return ErrInvalidTransactionOption
		}
		if err := option.applyTransactionOption(&config); err != nil {
			return err
		}
	}
	binding := transactionFromContext(ctx)
	var existing Client
	if binding != nil && binding.database == d {
		existing = binding.client
	}
	switch config.propagation {
	case TPRequired:
		if existing != nil {
			if err := validateExistingTxOptions(binding, config); err != nil {
				return err
			}
			return fn(ctx)
		}
	case TPNested:
		if existing != nil {
			if err := validateExistingTxOptions(binding, config); err != nil {
				return err
			}
			return existing.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return fn(withTransaction(ctx, d, tx, binding.txOptions))
			})
		}
	default:
		return ErrInvalidTransactionPropagation
	}
	callback := func(tx *gorm.DB) error {
		return fn(withTransaction(ctx, d, tx, config.txOptions))
	}
	if config.txOptionsSet {
		return d.client.WithContext(ctx).Transaction(callback, &config.txOptions)
	}
	return d.client.WithContext(ctx).Transaction(callback)
}

func validateExistingTxOptions(binding *transactionBinding, config transactionConfig) error {
	if !config.txOptionsSet || binding.txOptions == config.txOptions {
		return nil
	}
	return fmt.Errorf(
		"%w: existing isolation=%s read-only=%t; requested isolation=%s read-only=%t",
		ErrIncompatibleTxOptions,
		binding.txOptions.Isolation,
		binding.txOptions.ReadOnly,
		config.txOptions.Isolation,
		config.txOptions.ReadOnly,
	)
}
