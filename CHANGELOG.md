# Changelog

All notable changes are documented in this file.

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
