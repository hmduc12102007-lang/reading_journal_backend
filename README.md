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
- Completing a session sets its current page to the book's total pages. Completed library books display 100% progress; starting a reread resets the active session's progress.

## Book Cover Uploads

Create a book first, then upload its cover with the returned **book_id** (not user_book_id).

| Method | Route | Purpose |
| --- | --- | --- |
| POST | `/api/v1/books/{book_id}/cover` | Upload or replace a cover; requires the creator's Bearer token |
| GET | `/api/v1/books/{book_id}/cover` | Fetch image bytes; private books require the creator's Bearer token |
| DELETE | `/api/v1/books/{book_id}/cover` | Clear the cover, including an external URL; requires the creator's Bearer token |

POST accepts `multipart/form-data` with exactly one file field named `image`. Supported formats are JPEG, PNG, and GIF, up to **5 MiB** and **20 million pixels**. Images are checked by their actual contents; a filename or MIME header alone is not trusted. The request body limit is 6 MiB, including multipart overhead. Corrupt images return 400; oversized uploads return 413. Other API requests also have a 6 MiB body limit.

Windows example (replace the token, book ID, and file path):

```powershell
curl.exe -X POST "http://localhost:8080/api/v1/books/1/cover" -H "Authorization: Bearer YOUR_TOKEN" -F "image=@C:/Pictures/cover.jpg"
```

The upload returns the normal book response with `cover_url: "/api/v1/books/1/cover"`. Flutter must resolve relative URLs against the server origin and include the Bearer token when displaying a private uploaded cover:

```dart
final imageUri = Uri.parse(serverOrigin).resolve(book.coverUrl!);
Image.network(
  imageUri.toString(),
  headers: {'Authorization': 'Bearer $accessToken'},
);
```

Here `serverOrigin` is, for example, `http://10.0.2.2:8080`. Only attach your access token to images served by your backend; external HTTP(S) cover URLs do not need that token. A missing cover should display a placeholder. The local GET endpoint returns 404 for a book using an external cover URL; display that external URL directly.

`UPLOAD_DIR` defaults to `uploads/covers`. The server creates it on startup, stores random filenames there, and records an internal `books.cover_file` value through AutoMigrate. Filenames are hidden from JSON responses. Replacing/deleting a cover or clearing/replacing its URL removes the unused local file. Changing the URL to the same uploaded URL preserves its file. Files are served through permission checks; there is no public static uploads directory.

For deployment, mount `UPLOAD_DIR` on persistent writable storage and back it up together with PostgreSQL. All server instances must use the same storage directory if you run multiple instances. Restarts must preserve this directory. Avatar upload is outside the current API contract and has not been added.

## Verification

```powershell
go test ./...
go vet ./...
```

Unit tests validate image contents, corrupt files, pixel limits, path traversal, invalid cover URLs, and pagination overflow. To also run the API integration test, set `TEST_DATABASE_URL` to a PostgreSQL URL for a **test server** whose account can create schemas:

```powershell
$env:TEST_DATABASE_URL='postgres://test_user:test_password@localhost:5432/test_database?sslmode=disable'
go test ./... -count=1
```

The integration test creates a random schema, migrates and exercises the APIs there, and drops only that test schema. It covers authentication, cross-user access, cover upload/replacement/deletion, reading sessions, notes, reviews, goals, and password token revocation. Without `TEST_DATABASE_URL`, this test is skipped.

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
