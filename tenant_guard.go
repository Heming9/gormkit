package gormkit

import (
	"strings"

	"gorm.io/gorm"
)

// registerTenantGuardCallbacks rejects GORM APIs that can bypass model clauses
// while an automatically scoped tenant context is active. Manual mode is an
// explicit acknowledgement that the caller supplies the tenant condition;
// disabled mode explicitly permits cross-tenant access.
func registerTenantGuardCallbacks(db *gorm.DB) error {
	guard := func(operation string) func(*gorm.DB) {
		return func(db *gorm.DB) {
			if db == nil || db.Statement == nil || db.Error != nil {
				return
			}
			if getTenantState(db.Statement.Context).mode != tenantModeAutomatic {
				return
			}

			statement := db.Statement
			reasons := tenantGuardReasons(operation, statement)
			if len(reasons) > 0 {
				db.AddError(&TenantGuardError{
					Operation: operation,
					Table:     statement.Table,
					Model:     tenantGuardModel(statement),
					Reasons:   reasons,
				})
			}
		}
	}

	if err := db.Callback().Create().Before("gorm:create").Register("gormkit:tenant_guard", guard("create")); err != nil {
		return err
	}
	if err := db.Callback().Query().Before("gorm:query").Register("gormkit:tenant_guard", guard("query")); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("gormkit:tenant_guard", guard("update")); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("gormkit:tenant_guard", guard("delete")); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register("gormkit:tenant_guard", guard("row")); err != nil {
		return err
	}
	return db.Callback().Raw().Before("gorm:raw").Register("gormkit:tenant_guard", guard("raw"))
}

func tenantGuardReasons(operation string, statement *gorm.Statement) []string {
	var reasons []string
	if operation == "raw" {
		reasons = append(reasons, "raw SQL")
	} else if statement.SQL.Len() > 0 {
		reasons = append(reasons, "prebuilt SQL")
	}
	if statement.TableExpr != nil {
		reasons = append(reasons, "Table")
	}
	if len(statement.Joins) > 0 {
		reasons = append(reasons, "Joins")
	}
	return reasons
}

func tenantGuardModel(statement *gorm.Statement) string {
	if statement.Schema == nil {
		return ""
	}
	return strings.TrimSpace(statement.Schema.Name)
}
