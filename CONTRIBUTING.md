# Contributing

Bug reports and focused pull requests are welcome.

Before submitting a change:

1. Keep public API changes small and document observable behavior.
2. Add or update tests for the changed behavior.
3. Run `go test ./...`, `go test -race ./...`, and `go vet ./...`.
4. Run MySQL integration tests when changing SQL, transaction, tenant, or driver behavior.

Do not include credentials, production data, or application-specific details in
issues, fixtures, logs, or pull requests.
