package gormkit

import (
	"context"
	"errors"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidModel       = errors.New("gormkit: repository model must be a concrete type")
	ErrInvalidPaging      = errors.New("gormkit: paging offset and limit must not be negative")
	ErrNilEntity          = errors.New("gormkit: entity must not be nil")
	ErrPrimaryKeyRequired = errors.New("gormkit: a non-zero primary key is required for update")
)

// Repository provides common CRUD operations for pointer-to-struct model T.
// Query methods return database errors instead of panicking.
type Repository[T any] interface {
	FindByID(id any) (T, error)
	FindOneBy(where any, args ...any) (T, error)
	FindLastOneBy(where any, args ...any) (T, error)
	FindByIDs(ids ...any) ([]T, error)
	FindBy(where any, args ...any) ([]T, error)
	FindAll() ([]T, error)
	Create(entity T) error
	CreateAll(entities ...T) error
	Update(entity T) error
	UpdateAll(entities ...T) error
	UpdateBy(data any, where any, args ...any) error
	DeleteBy(where any, args ...any) error
	CountBy(where ...any) (int64, error)
	Exists(where any, args ...any) (bool, error)
	WithPaging(paging Paging) Repository[T]
	WithScope(scopes ...Scope) Repository[T]
	Unscoped() Repository[T]
	DB() (Client, error)
}

type repository[T any] struct {
	db       Client
	err      error
	paging   Paging
	scopes   []Scope
	unscoped bool
}

// NewRepository creates a repository from an explicit GORM database handle.
func NewRepository[T any](db Client) Repository[T] {
	if db == nil {
		return &repository[T]{err: ErrNilDatabase}
	}
	if err := validateModelType[T](); err != nil {
		return &repository[T]{db: db, err: err}
	}
	return &repository[T]{db: db}
}

// RepoOf creates a repository using the process-wide default database.
func RepoOf[T any](ctx context.Context) Repository[T] {
	db, err := SQLClient(ctx)
	if err != nil {
		return &repository[T]{err: err}
	}
	return NewRepository[T](db)
}

func validateModelType[T any]() error {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	if typ.Kind() != reflect.Pointer || typ.Elem().Kind() != reflect.Struct {
		return ErrInvalidModel
	}
	return nil
}

func newModel[T any]() T {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	if typ.Kind() == reflect.Pointer {
		return reflect.New(typ.Elem()).Interface().(T)
	}
	return reflect.Zero(typ).Interface().(T)
}

func zero[T any]() T {
	var value T
	return value
}

func (r *repository[T]) session() (Client, error) {
	if r.err != nil {
		return nil, r.err
	}
	db := r.db.Session(&gorm.Session{NewDB: true, Context: r.db.Statement.Context})
	if r.unscoped {
		db = db.Unscoped()
	}
	if len(r.scopes) > 0 {
		db = db.Scopes(r.scopes...)
	}
	return db, nil
}

func (r *repository[T]) list(scope Scope) ([]T, error) {
	result := make([]T, 0)
	db, err := r.session()
	if err != nil {
		return result, err
	}
	if scope != nil {
		db = db.Scopes(scope)
	}
	if r.paging != nil {
		if r.paging.Offset() < 0 || r.paging.Limit() < 0 {
			return result, ErrInvalidPaging
		}
		var total int64
		if err := db.Model(newModel[T]()).Count(&total).Error; err != nil {
			return result, err
		}
		r.paging.SetTotal(total)
		if total == 0 {
			return result, nil
		}
		db = db.Offset(r.paging.Offset()).Limit(r.paging.Limit())
	}
	return result, db.Find(&result).Error
}

func (r *repository[T]) FindByID(id any) (T, error) {
	return r.FindOneBy(clause.Eq{Column: clause.Column{Name: clause.PrimaryKey}, Value: id})
}

func (r *repository[T]) FindOneBy(where any, args ...any) (T, error) {
	result := newModel[T]()
	db, err := r.session()
	if err != nil {
		return zero[T](), err
	}
	err = db.Where(where, args...).First(result).Error
	if err != nil {
		return zero[T](), err
	}
	return result, nil
}

func (r *repository[T]) FindLastOneBy(where any, args ...any) (T, error) {
	result := newModel[T]()
	db, err := r.session()
	if err != nil {
		return zero[T](), err
	}
	err = db.Where(where, args...).Last(result).Error
	if err != nil {
		return zero[T](), err
	}
	return result, nil
}

func (r *repository[T]) FindByIDs(ids ...any) ([]T, error) {
	if len(ids) == 0 {
		return []T{}, nil
	}
	return r.list(func(db Client) Client {
		return db.Clauses(clause.IN{
			Column: clause.Column{Name: clause.PrimaryKey},
			Values: ids,
		})
	})
}

func (r *repository[T]) FindBy(where any, args ...any) ([]T, error) {
	return r.list(whereScope(where, args...))
}

func (r *repository[T]) FindAll() ([]T, error) {
	return r.list(nil)
}

func (r *repository[T]) Create(entity T) error {
	db, err := r.session()
	if err != nil {
		return err
	}
	return db.Create(entity).Error
}

func (r *repository[T]) CreateAll(entities ...T) error {
	if len(entities) == 0 {
		return nil
	}
	db, err := r.session()
	if err != nil {
		return err
	}
	return db.Create(entities).Error
}

// Update replaces the persisted fields of entity, identified by its primary
// key. It never inserts a missing record. Zero-valued fields are included.
func (r *repository[T]) Update(entity T) error {
	db, err := r.session()
	if err != nil {
		return err
	}
	return updateEntity(db, entity)
}

// UpdateAll updates every entity in one transaction. Every entity must have a
// non-zero primary key, and the transaction is rolled back when one is absent.
func (r *repository[T]) UpdateAll(entities ...T) error {
	if len(entities) == 0 {
		return nil
	}
	db, err := r.session()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, entity := range entities {
			if err := updateEntity(tx, entity); err != nil {
				return err
			}
		}
		return nil
	})
}

func updateEntity[T any](db Client, entity T) error {
	value := reflect.ValueOf(entity)
	if !value.IsValid() || (value.Kind() == reflect.Pointer && value.IsNil()) {
		return ErrNilEntity
	}

	statement := &gorm.Statement{DB: db, Context: db.Statement.Context}
	if err := statement.Parse(entity); err != nil {
		return err
	}
	if statement.Schema == nil || len(statement.Schema.PrimaryFields) == 0 {
		return ErrPrimaryKeyRequired
	}

	primaryValues := make(map[string]any, len(statement.Schema.PrimaryFields))
	for _, field := range statement.Schema.PrimaryFields {
		fieldValue, zero := field.ValueOf(statement.Context, value)
		if zero {
			return ErrPrimaryKeyRequired
		}
		primaryValues[field.DBName] = fieldValue
	}

	result := db.Model(entity).Select("*").Updates(entity)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	// MySQL reports zero affected rows when every stored value already equals
	// the update. Distinguish that idempotent success from a missing record
	// without falling back to an unsafe upsert.
	var count int64
	if err := db.Model(newModel[T]()).Where(primaryValues).Limit(1).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *repository[T]) UpdateBy(data any, where any, args ...any) error {
	db, err := r.session()
	if err != nil {
		return err
	}
	return db.Model(newModel[T]()).Scopes(whereScope(where, args...)).Updates(data).Error
}

func (r *repository[T]) DeleteBy(where any, args ...any) error {
	db, err := r.session()
	if err != nil {
		return err
	}
	return db.Scopes(whereScope(where, args...)).Delete(newModel[T]()).Error
}

func (r *repository[T]) CountBy(where ...any) (int64, error) {
	db, err := r.session()
	if err != nil {
		return 0, err
	}
	if len(where) > 0 {
		db = db.Scopes(whereScope(where[0], where[1:]...))
	}
	var count int64
	err = db.Model(newModel[T]()).Count(&count).Error
	return count, err
}

func (r *repository[T]) Exists(where any, args ...any) (bool, error) {
	count, err := r.CountBy(append([]any{where}, args...)...)
	return count > 0, err
}

func (r *repository[T]) WithPaging(paging Paging) Repository[T] {
	clone := *r
	clone.paging = paging
	return &clone
}

func (r *repository[T]) WithScope(scopes ...Scope) Repository[T] {
	clone := *r
	clone.scopes = append(append([]Scope(nil), r.scopes...), scopes...)
	return &clone
}

func (r *repository[T]) Unscoped() Repository[T] {
	clone := *r
	clone.unscoped = true
	return &clone
}

func (r *repository[T]) DB() (Client, error) {
	return r.session()
}

func whereScope(where any, args ...any) Scope {
	return func(db Client) Client {
		if len(args) == 0 {
			return db.Where(where)
		}
		return db.Where(where, args...)
	}
}
