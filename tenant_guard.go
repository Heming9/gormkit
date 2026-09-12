package gormkit

import "gorm.io/gorm"

// registerTenantGuardCallbacks rejects GORM APIs that can bypass model clauses
// while an automatically scoped tenant context is active. Manual mode is an
// explicit acknowledgement that the caller supplies the tenant condition;
// disabled mode explicitly permits cross-tenant access.
func registerTenantGuardCallbacks(db *gorm.DB) error {
	guard := func(db *gorm.DB) {
		if db == nil || db.Statement == nil || db.Error != nil {
			return
		}
		if getTenantState(db.Statement.Context).mode != tenantModeAutomatic {
			return
		}

		statement := db.Statement
		if statement.SQL.Len() > 0 || statement.TableExpr != nil || len(statement.Joins) > 0 {
			db.AddError(ErrManualTenantRequired)
		}
	}

	if err := db.Callback().Create().Before("gorm:create").Register("gormkit:tenant_guard", guard); err != nil {
		return err
	}
	if err := db.Callback().Query().Before("gorm:query").Register("gormkit:tenant_guard", guard); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("gormkit:tenant_guard", guard); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("gormkit:tenant_guard", guard); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register("gormkit:tenant_guard", guard); err != nil {
		return err
	}
	return db.Callback().Raw().Before("gorm:raw").Register("gormkit:tenant_guard", guard)
}
