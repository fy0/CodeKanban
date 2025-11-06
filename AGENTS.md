# Repository Guidelines

## Project Structure & Module Organization
The entry point `main.go` wires CLI flags, service installation, and HTTP startup via `server.go`. HTTP handlers live in `api/` with Huma schemas under `api/h`. Data access code resides in `model/`, where `table/` hosts sample GORM models and `sqlc_gen/` stores SQL schema plus codegen stubs. Shared helpers, configuration loaders, and logging adapters are kept in `utils/` (see `app_config.go` for defaults). Static assets and generated docs land under `static/` and `docs/`, both served via the embedded filesystem. Keep project-wide settings in `config.yaml`.

## Build, Test, and Development Commands
- `go mod tidy` syncs dependencies with the module definition.
- `go run .` starts the HTTP service; append `-m` to force migrations, `-i` or `--uninstall` for Windows service management.
- `go test ./...` runs all package tests with race detection optional via `-race`.
- `go generate -run "sqlc" ./model/sqlc_gen/...` regenerates SQLC outputs before committing schema changes.
- `go vet ./...` catches common anti-patterns during review.

## Coding Style & Naming Conventions
Target Go 1.24 and format every change with `gofmt` (or your editor's equivalent) before commit; gofmt's tab-based indentation is the single source of truth. Align imports with `goimports` to keep standard, third-party, and local packages grouped. Exported types and functions use PascalCase, private helpers use camelCase, and interface names should end in `er` when they describe behavior. Match file names to the package or domain (`config_service.go`, `user_handler.go`) to ease discovery. Keep log messages structured, using `zap` fields instead of printf-style strings.

## Testing Guidelines
Place `_test.go` files next to the code under test and mirror package names to avoid import cycles. Prefer table-driven tests for handlers and utilities; use `github.com/DATA-DOG/go-sqlmock` when exercising database logic. Validate HTTP routes with Fiber's test utilities, asserting both status codes and response envelopes defined in `api/h/response.go`. Before opening a PR, run `go test ./...` (add `-race` and `-count=1` for flake-sensitive code) and ensure migrations leave databases clean.

## Commit & Pull Request Guidelines
Write commits in the imperative mood (`Add user lookup endpoint`) and keep scopes focused. Reference related issues in the footer (`Refs #123`) and describe config or schema migrations in the body. Pull requests should outline the change, note any config toggles in `config.yaml`, and include screenshots or cURL snippets when API responses change. Flag manual steps (service install, migration flags) so reviewers can reproduce.

## Configuration & Deployment Notes
Never commit real credentials; `config.yaml` is a template and runtime secrets should come from environment variables or external stores. Update `utils/app_config.go` when adding new config keys to guarantee defaults and validation. For Windows deployments use the embedded service flow (`go run . -i`), and on other platforms deploy via your preferred process manager while ensuring the embedded `static/` directory is rebuilt if assets change.
