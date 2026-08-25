package gormkit

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
)

// Scope is a reusable GORM scope.
type Scope = func(db Client) Client

type forceKey struct{}

func registerForceCallback(db *gorm.DB) error {
	return db.Callback().Update().Before("gorm:update").Register("gormkit:force_update", func(db *gorm.DB) {
		statement := db.Statement
		if statement == nil || statement.Schema == nil {
			return
		}
		if _, applied := statement.Clauses["gormkit:force_update_applied"]; applied {
			return
		}
		if len(statement.Omits) > 0 || len(statement.Selects) > 0 {
			return
		}
		fields, ok := statement.Context.Value(forceKey{}).([]string)
		if !ok || len(fields) == 0 {
			return
		}
		value := reflect.ValueOf(statement.Dest)
		for value.IsValid() && value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return
			}
			value = value.Elem()
		}
		if !value.IsValid() || value.Kind() != reflect.Struct {
			return
		}
		assignments := callbacks.ConvertToAssignments(statement)
		assigned := make(map[string]struct{}, len(assignments))
		for _, assignment := range assignments {
			assigned[assignment.Column.Name] = struct{}{}
		}
		for _, name := range fields {
			field := statement.Schema.LookUpField(name)
			if field == nil {
				_ = db.AddError(fmt.Errorf("gormkit: Force field %q does not exist", name))
				return
			}
			if _, exists := assigned[field.DBName]; exists {
				continue
			}
			fieldValue, _ := field.ValueOf(statement.Context, value)
			assignments = append(assignments, clause.Assignment{
				Column: clause.Column{Name: field.DBName},
				Value:  fieldValue,
			})
		}
		statement.AddClause(assignments)
		statement.Clauses["gormkit:force_update_applied"] = clause.Clause{}
	})
}

// Force includes the named zero-valued struct fields in an Updates operation.
// It has no effect when Select or Omit is already present.
func Force(fields ...string) Scope {
	owned := append([]string(nil), fields...)
	return func(db Client) Client {
		if len(owned) == 0 {
			return db
		}
		ctx := context.WithValue(db.Statement.Context, forceKey{}, owned)
		return db.WithContext(ctx)
	}
}

// LoadAllAssociations preloads all model associations.
func LoadAllAssociations() Scope {
	return func(db Client) Client {
		return db.Preload(clause.Associations)
	}
}

// LoadAssociations preloads the named associations.
func LoadAssociations(names ...string) Scope {
	owned := append([]string(nil), names...)
	return func(db Client) Client {
		for _, name := range owned {
			db = db.Preload(name)
		}
		return db
	}
}

// LoadAssociation preloads one association with optional GORM conditions.
func LoadAssociation(name string, conditions ...any) Scope {
	owned := append([]any(nil), conditions...)
	return func(db Client) Client {
		return db.Preload(name, owned...)
	}
}

// OrderBy applies structured order clauses. Callers must set Raw explicitly on
// a clause.Column when an unquoted database expression is intended.
func OrderBy(columns ...clause.OrderByColumn) Scope {
	owned := append([]clause.OrderByColumn(nil), columns...)
	return func(db Client) Client {
		return db.Order(clause.OrderBy{Columns: owned})
	}
}

// Asc returns an ascending, quoted order column.
func Asc(name string) clause.OrderByColumn {
	return clause.OrderByColumn{Column: clause.Column{Name: name}}
}

// Desc returns a descending, quoted order column.
func Desc(name string) clause.OrderByColumn {
	return clause.OrderByColumn{Column: clause.Column{Name: name}, Desc: true}
}

// RawOrder returns an explicitly raw order expression.
func RawOrder(expression string) (clause.OrderByColumn, error) {
	if expression == "" {
		return clause.OrderByColumn{}, errors.New("gormkit: raw order expression must not be empty")
	}
	return clause.OrderByColumn{Column: clause.Column{Name: expression, Raw: true}}, nil
}

// ApplyQuery combines Query implementations into one scope.
func ApplyQuery(queries ...Query) Scope {
	owned := append([]Query(nil), queries...)
	return func(db Client) Client {
		for _, query := range owned {
			if query != nil {
				db = query.Apply(db)
			}
		}
		return db
	}
}
