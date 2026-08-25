package gormkit

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

var ErrTenantRequired = errors.New("gormkit: tenant context is required")

type tenantKey struct{}

// TenantID is a numeric tenant identifier stored as int64.
type TenantID int64

func (id TenantID) String() string { return strconv.FormatInt(int64(id), 10) }
func (id TenantID) Int64() int64   { return int64(id) }

// WithTenantID returns a child context carrying id.
func WithTenantID(ctx context.Context, id TenantID) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantKey{}, id)
}

// GetTenantID returns the tenant stored in ctx, if any.
func GetTenantID(ctx context.Context) (TenantID, bool) {
	if ctx == nil {
		return 0, false
	}
	id, ok := ctx.Value(tenantKey{}).(TenantID)
	return id, ok
}

func (TenantID) QueryClauses(field *schema.Field) []clause.Interface {
	return []clause.Interface{tenantQueryClause{field: field}}
}

func (TenantID) UpdateClauses(field *schema.Field) []clause.Interface {
	return []clause.Interface{tenantUpdateClause{field: field}}
}

func (TenantID) DeleteClauses(field *schema.Field) []clause.Interface {
	return []clause.Interface{tenantDeleteClause{field: field}}
}

func (TenantID) CreateClauses(field *schema.Field) []clause.Interface {
	return []clause.Interface{tenantCreateClause{field: field}}
}

type tenantQueryClause struct {
	field *schema.Field
}

func (tenantQueryClause) Name() string               { return "" }
func (tenantQueryClause) Build(clause.Builder)       {}
func (tenantQueryClause) MergeClause(*clause.Clause) {}

func (tenant tenantQueryClause) ModifyStatement(statement *gorm.Statement) {
	if statement == nil || statement.Unscoped {
		return
	}
	if _, applied := statement.Clauses["gormkit:tenant_applied"]; applied {
		return
	}
	if current, ok := statement.Clauses["WHERE"]; ok {
		if where, ok := current.Expression.(clause.Where); ok {
			for _, expression := range where.Exprs {
				orConditions, isOR := expression.(clause.OrConditions)
				if isOR && len(orConditions.Exprs) == 1 {
					where.Exprs = []clause.Expression{clause.And(where.Exprs...)}
					current.Expression = where
					statement.Clauses["WHERE"] = current
					break
				}
			}
		}
	}
	id, ok := GetTenantID(statement.Context)
	if !ok {
		statement.AddError(ErrTenantRequired)
		return
	}
	statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{
			Column: clause.Column{Table: clause.CurrentTable, Name: tenant.field.DBName},
			Value:  id,
		},
	}})
	statement.Clauses["gormkit:tenant_applied"] = clause.Clause{}
}

type tenantUpdateClause struct {
	field *schema.Field
}

func (tenantUpdateClause) Name() string               { return "" }
func (tenantUpdateClause) Build(clause.Builder)       {}
func (tenantUpdateClause) MergeClause(*clause.Clause) {}

func (tenant tenantUpdateClause) ModifyStatement(statement *gorm.Statement) {
	if statement == nil || statement.SQL.Len() != 0 || statement.Unscoped {
		return
	}
	tenantQueryClause(tenant).ModifyStatement(statement)
	statement.Omit(tenant.field.DBName)
}

type tenantDeleteClause struct {
	field *schema.Field
}

func (tenantDeleteClause) Name() string               { return "" }
func (tenantDeleteClause) Build(clause.Builder)       {}
func (tenantDeleteClause) MergeClause(*clause.Clause) {}

func (tenant tenantDeleteClause) ModifyStatement(statement *gorm.Statement) {
	if statement == nil || statement.SQL.Len() != 0 || statement.Unscoped {
		return
	}
	tenantQueryClause(tenant).ModifyStatement(statement)
}

type tenantCreateClause struct {
	field *schema.Field
}

func (tenantCreateClause) Name() string               { return "" }
func (tenantCreateClause) Build(clause.Builder)       {}
func (tenantCreateClause) MergeClause(*clause.Clause) {}

func (tenant tenantCreateClause) ModifyStatement(statement *gorm.Statement) {
	if statement == nil || statement.SQL.Len() != 0 || statement.Unscoped {
		return
	}
	id, ok := GetTenantID(statement.Context)
	if !ok {
		statement.AddError(ErrTenantRequired)
		return
	}
	statement.SetColumn(tenant.field.DBName, id, true)
}
