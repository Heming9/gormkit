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
			if isTenantControlStatement(statement) {
				return
			}
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

// isTenantControlStatement recognizes the exact transaction-control SQL that
// GORM's supported dialectors emit for nested transactions. These statements
// do not access tenant data and must not require manual tenant mode.
func isTenantControlStatement(statement *gorm.Statement) bool {
	if statement == nil || len(statement.Vars) != 0 || statement.TableExpr != nil || len(statement.Joins) != 0 {
		return false
	}

	sql := statement.SQL.String()
	for _, prefix := range []string{
		"SAVEPOINT ",
		"ROLLBACK TO SAVEPOINT ",
		"RELEASE SAVEPOINT ",
	} {
		if strings.HasPrefix(sql, prefix) {
			return isGORMGeneratedSavepointName(strings.TrimPrefix(sql, prefix))
		}
	}
	return false
}

func isGORMGeneratedSavepointName(name string) bool {
	if len(name) <= 2 || !strings.HasPrefix(name, "sp") {
		return false
	}
	for _, char := range name[2:] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
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
