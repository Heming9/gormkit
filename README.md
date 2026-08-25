# gormkit

`gormkit` is a small set of composable helpers for [GORM v2](https://gorm.io/):

- explicit database lifecycle management with an optional process default;
- context-propagated required and nested transactions;
- an error-returning generic repository for pointer-to-struct models;
- safe ordering, association, query, and forced-zero-value update scopes;
- opt-in tenant isolation through a model field type;
- an optional configurable Unix timestamp plugin.

The module currently targets Go 1.23 or newer. The API is released as `v0.x`
while real-world usage is used to validate its shape.

## Install

```bash
go get github.com/Heming9/gormkit@latest
```

## Open a database

The primary API accepts a GORM dialector and configuration, so callers retain
control over the driver, logger, naming strategy, and migration behavior.

```go
database, err := gormkit.Open(
    mysql.Open(dsn),
    &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)},
    gormkit.NewTimestampPlugin(),
)
if err != nil {
    return err
}
defer database.Close()

db := database.Client(ctx)
```

`OpenMySQL` is a convenience for common MySQL connection and pool settings. It
uses a silent GORM logger unless a logger is supplied.

## Default database

Libraries should prefer an explicit `*gormkit.Database`. Applications that use
a single process-wide database can install a default:

```go
database, err := gormkit.Open(mysql.Open(dsn), &gorm.Config{})
if err != nil {
    return err
}
if err := gormkit.UseDefault(database); err != nil {
    return err
}

db, err := gormkit.SQLClient(ctx)
```

`MustSQLClient` is available at application boundaries where initialization is
already guaranteed. The deprecated `SqlClient` spelling remains temporarily for
source compatibility.

## Transactions

```go
err := database.Transaction(ctx, func(txctx context.Context) error {
    db := database.Client(txctx)
    return db.Create(&record).Error
})
```

`TPRequired` is the default and reuses an existing transaction in the context.
`TPNested` asks GORM to create a nested transaction/savepoint when a transaction
already exists.

## Generic repository

Repositories accept pointer-to-struct models and return database errors. Record
not found errors remain compatible with `errors.Is(err, gorm.ErrRecordNotFound)`.

```go
repo := gormkit.NewRepository[*User](database.Client(ctx))

user := &User{Name: "example"}
if err := repo.Create(user); err != nil {
    // Create is insert-only; primary/unique conflicts are returned.
}

user, err := repo.FindByID(id)
if errors.Is(err, gorm.ErrRecordNotFound) {
    // handle absence
}

users, err := repo.
    WithScope(gormkit.OrderBy(gormkit.Desc("created_at"))).
    FindBy("status = ?", "active")
```

`RepoOf[T](ctx)` creates a repository from the process-wide default database.
Conditional `Save` calls should target columns protected by a database unique
constraint when concurrent writers are possible.

## Tenant isolation

Use `gormkit.TenantID` as a model field to opt a model into tenant clauses:

```go
type Project struct {
    ID       uint
    TenantID gormkit.TenantID `gorm:"column:tenant_id;index"`
    Name     string
}

ctx = gormkit.WithTenantID(ctx, 42)
err := database.Client(ctx).Create(&Project{Name: "example"}).Error
```

Create, query, update, and delete operations require a tenant context for these
models. The tenant field is overwritten on create and omitted from updates.

> `Unscoped()` deliberately bypasses tenant filtering as well as GORM soft-delete
> filtering. Treat it as a privileged operation and do not expose it to
> untrusted request paths.

## Timestamp plugin

`NewTimestampPlugin` maintains `ctime` and `mtime` Unix-second columns by
default. The column names and clock are configurable on `TimestampPlugin`.

```go
plugin := gormkit.NewTimestampPlugin()
plugin.CreatedColumn = "created_unix"
plugin.UpdatedColumn = "updated_unix"
```

## Tests

Default tests use a pure-Go in-memory SQLite driver:

```bash
go test ./...
go test -race ./...
go vet ./...
```

MySQL integration tests use the `integration` build tag and these environment
variables: `MYSQL_ADDRESS`, `MYSQL_USERNAME`, `MYSQL_PASSWORD`, and
`MYSQL_DATABASE`.

```bash
go test -tags integration ./integration
```

## Compatibility

Before `v1.0.0`, minor releases may adjust APIs when doing so fixes ambiguous or
unsafe behavior. Release notes document every user-visible change.

## License

[MIT](LICENSE)
