# Changelog

All notable changes are documented in this file.

## [0.5.0] - 2026-09-12

### Added

- Add context-propagated `WithManualTenant` and `WithDisableTenant` modes.
- Reject raw SQL, explicit table names, and joins in automatic tenant mode.

### Changed

- `Unscoped()` now bypasses only soft-delete filtering and preserves tenant
  isolation. Cross-tenant access must use `WithDisableTenant` explicitly.
- `Wrap` now installs gormkit's required callbacks while continuing to leave
  optional plugins unchanged.

## [0.4.0] - 2026-09-12

### Changed

- Rename the explicit-client repository constructor from `NewRepository` to `NewRepo`.
- Remove the deprecated `SqlClient` compatibility spelling; use `SQLClient` or `MustSQLClient`.

## [0.3.0] - 2026-09-12

### Added

- Primary-key-based `Update` and transactional `UpdateAll` repository operations. Updates include zero-valued fields, require an existing record, and never fall back to an insert.

### Changed

- Remove the ambiguous repository `Save` and `SaveAll` operations. Callers now choose explicitly between `Create`/`CreateAll` and `Update`/`UpdateAll`.

### Fixed

- Reject conflict-updating inserts for tenant models, preventing GORM's `Save` fallback or `OnConflict{UpdateAll: true}` from overwriting a row owned by another tenant.

## [0.2.1] - 2026-09-12

### Fixed

- Always persist the context tenant during selected, omitted, and batch creates.
- Keep the tenant column immutable even when it is explicitly selected for an update.

## [0.2.0] - 2026-08-25

### Added

- Insert-only generic repository operations `Create` and `CreateAll`, including batch inserts and generated primary-key population.

### Changed

- Upgrade GORM from 1.31.1 to 1.31.2.

## [0.1.1] - 2026-08-25

### Fixed

- Preserve the MySQL driver's secure defaults when building a DSN, including support for `mysql_native_password` users.

## [0.1.0] - 2026-08-25

### Added

- Explicit database lifecycle management and optional process default.
- MySQL convenience configuration with connection-pool controls.
- Required and nested context-propagated transactions.
- Error-returning generic repository for pointer-to-struct models.
- Reusable query, ordering, preload, and forced update scopes.
- Tenant isolation clauses for create, query, update, and delete operations.
- Configurable Unix timestamp plugin.
- Pure-Go default unit tests and MySQL integration tests.
