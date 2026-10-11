# Reading Journal Backend

REST API for the Reading Journal Flutter app, implemented with Go, Echo, GORM, and PostgreSQL.

## Requirements

- Go 1.23 or newer
- PostgreSQL

## Run Locally

```powershell
Copy-Item .env.example .env
go mod tidy
go run ./cmd/server
```

The server listens on `http://localhost:8080`. Configure `DB_CONNECTION_STRING` as a PostgreSQL URL, for example:

```text
postgres://postgres:postgres@localhost:5432/reading_journal?sslmode=disable
```

Set `JWT_SECRET` to a secret of at least 32 characters. The server runs GORM AutoMigrate for its eight tables when it starts. `category_name` is an optional free-form string on each book; there is no category CRUD API or active categories table. On an older database, migration copies existing category names into `books.category_name` when possible and leaves legacy tables/columns untouched. AutoMigrate does not convert unrelated data from the earlier scaffold schema, so review an existing database before using it.

For Android Emulator, use `http://10.0.2.2:8080/api/v1`. A physical device should use the development computer's LAN address. The API is under `/api/v1`; `GET /health` is outside that prefix.

## Apidog

Import [`docs/reading_journal_apidog.yaml`](docs/reading_journal_apidog.yaml) into Apidog as an OpenAPI 3.0 definition. The document includes request/response schemas, examples, pagination, bearer authentication, and local/Android Emulator server URLs. Select the appropriate server, then use **Bearer Token** auth with the `access_token` returned by register or login.

## API Groups

- Auth and profile: register, login, change password, and `/users/me`
- Catalog and personal library: books with optional `category_name`, favorites, and library membership
- Reading: sessions, progress, completion, and reading logs
- Notes and reviews: per-library-book notes and one review per book
- Goals and insights: reading goals, dashboard, and statistics

All successful JSON responses use a `success: true` envelope. Errors use `success: false` with `error.code` and `error.message`. List endpoints return `data` plus `pagination`. Personal resources require `Authorization: Bearer <access_token>` and are scoped to the authenticated user. Passwords are stored as bcrypt hashes; changing a password invalidates existing access tokens.

## MVP Decisions

- Books created by a user are private to that user; shared catalog rows have no `created_by`.
- Removing a library book with sessions, notes, or a review returns `409 BOOK_HAS_HISTORY`.
- Reading logs do not change the active session's `current_page`.
- `BOOKS` goals count distinct books completed in the goal date range.
- `PAGES` goals and page statistics sum logged page ranges, including overlaps and rereads.
- Empty nullable values in PATCH requests can be cleared with `null`; nullable text can also be cleared with an empty string.

## Project Structure

```text
cmd/server         Application entrypoint
internal/auth      JWT creation and authentication middleware
internal/config    Environment configuration
internal/database  PostgreSQL connection and migrations
internal/handlers  HTTP handlers and response helpers
internal/models    GORM models for the contract tables
internal/routes    API route registration
docs               Product contract and Apidog OpenAPI document
```
