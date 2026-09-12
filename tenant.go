package gormkit

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

var (
	ErrTenantRequired       = errors.New("gormkit: tenant context is required")
	ErrManualTenantRequired = errors.New("gormkit: operation may bypass automatic tenant isolation; use WithManualTenant or WithDisableTenant")
	ErrUnsafeTenantUpsert   = errors.New("gormkit: updating conflicts is unsafe for tenant models")
)

type tenantKey struct{}

type tenantMode uint8

const (
	tenantModeUnset tenantMode = iota
	tenantModeAutomatic
	tenantModeManual
	tenantModeDisabled
)

type tenantState struct {
	id   TenantID
	mode tenantMode
}

// TenantID is a numeric tenant identifier stored as int64.
type TenantID int64

func (id TenantID) String() string { return strconv.FormatInt(int64(id), 10) }
func (id TenantID) Int64() int64   { return int64(id) }

// WithTenantID returns a child context carrying id.
func WithTenantID(ctx context.Context, id TenantID) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantKey{}, tenantState{id: id, mode: tenantModeAutomatic})
}

// WithManualTenant returns a child context that retains the current tenant but
// permits APIs whose tenant isolation cannot be applied automatically, such as
// raw SQL. The caller is responsible for applying the tenant condition in
// those operations. It does not disable automatic clauses for regular model
// operations.
func WithManualTenant(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	state := getTenantState(ctx)
	if state.mode == tenantModeAutomatic || state.mode == tenantModeManual {
		state.mode = tenantModeManual
	}
	return context.WithValue(ctx, tenantKey{}, state)
}

// WithDisableTenant returns a child context that explicitly disables tenant
// isolation. It is intended for cross-tenant operations.
func WithDisableTenant(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantKey{}, tenantState{mode: tenantModeDisabled})
}

// GetTenantID returns the tenant stored in ctx, if any.
func GetTenantID(ctx context.Context) (TenantID, bool) {
	state := getTenantState(ctx)
	if state.mode != tenantModeAutomatic && state.mode != tenantModeManual {
		return 0, false
	}
	return state.id, true
}

func getTenantState(ctx context.Context) tenantState {
	if ctx == nil {
		return tenantState{}
	}
	state, _ := ctx.Value(tenantKey{}).(tenantState)
	return state
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
	if statement == nil {
		return
	}
	state := getTenantState(statement.Context)
	if state.mode == tenantModeDisabled {
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
	if state.mode != tenantModeAutomatic && state.mode != tenantModeManual {
		statement.AddError(ErrTenantRequired)
		return
	}
	statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{
			Column: clause.Column{Table: clause.CurrentTable, Name: tenant.field.DBName},
			Value:  state.id,
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
	if statement == nil || statement.SQL.Len() != 0 {
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
	if statement == nil || statement.SQL.Len() != 0 {
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
	if statement == nil || statement.SQL.Len() != 0 {
		return
	}
	state := getTenantState(statement.Context)
	if state.mode == tenantModeDisabled {
		return
	}
	if state.mode != tenantModeAutomatic && state.mode != tenantModeManual {
		statement.AddError(ErrTenantRequired)
		return
	}
	if current, exists := statement.Clauses["ON CONFLICT"]; exists {
		if tenantConflictUpdates(current.Expression) {
			statement.AddError(ErrUnsafeTenantUpsert)
			return
		}
	}
	// Tenant isolation must not depend on the caller remembering to include the
	// tenant column in Select, or avoiding it in Omit. In particular,
	// Select("Name").Create(...) used to leave tenant_id at its database zero
	// value even though SetColumn updated the in-memory model.
	includeTenantOnCreate(statement, tenant.field)
	statement.SetColumn(tenant.field.DBName, state.id, true)
}

func tenantConflictUpdates(expression clause.Expression) bool {
	switch conflict := expression.(type) {
	case clause.OnConflict:
		return conflict.UpdateAll || len(conflict.DoUpdates) > 0
	case *clause.OnConflict:
		return conflict != nil && (conflict.UpdateAll || len(conflict.DoUpdates) > 0)
	default:
		return false
	}
}

// includeTenantOnCreate makes the tenant field mandatory for INSERTs while
// preserving all other Select/Omit choices made by the caller.
func includeTenantOnCreate(statement *gorm.Statement, field *schema.Field) {
	if statement == nil || field == nil {
		return
	}

	isTenant := func(name string) bool {
		name = strings.Trim(name, "`\"[]")
		if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
			name = strings.Trim(name[dot+1:], "`\"[]")
		}
		if name == field.Name || name == field.DBName {
			return true
		}
		return statement.Schema != nil && statement.Schema.LookUpField(name) == field
	}

	selectsTenant := false
	for _, name := range statement.Selects {
		if name == "*" || isTenant(name) {
			selectsTenant = true
			break
		}
	}
	if len(statement.Selects) > 0 && !selectsTenant {
		statement.Selects = append(statement.Selects, field.DBName)
	}

	omits := statement.Omits[:0]
	for _, name := range statement.Omits {
		switch {
		case isTenant(name):
			// The tenant column is mandatory on create.
		case name == "*" && statement.Schema != nil:
			// Expand Omit("*") so every regular field except the tenant remains
			// omitted. Simply dropping it would unexpectedly insert all fields.
			for _, dbName := range statement.Schema.DBNames {
				if !isTenant(dbName) {
					omits = append(omits, dbName)
				}
			}
		default:
			omits = append(omits, name)
		}
	}
	statement.Omits = omits
}
