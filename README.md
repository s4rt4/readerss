# ReadeRSS

ReadeRSS is a self-hosted RSS reader built with Go, templ, SQLite, and a small amount of local JavaScript. It is designed to run locally as a single binary, stay fast with SQLite, and provide a keyboard-friendly reading workflow.

## Current Features

- Single-user login with signed session cookies.
- Feed management: add, edit, delete, refresh, and mark a feed as read.
- RSS/Atom parsing with homepage feed discovery.
- SQLite storage with migrations powered by Goose and typed queries from sqlc.
- Article reader with unread/all/starred filters, detail pane, mobile flow, and readable text cleanup.
- Article actions: mark read/unread, star/unstar, and mark all read.
- Category and feed unread counts in the sidebar.
- Background scheduler with worker pool for due feed refreshes.
- HTTP caching support for feeds with ETag and Last-Modified.
- FTS5 search with snippet highlighting and a LIKE fallback.
- Settings page for fetch interval, retention, theme, density, and OPML.
- OPML import/export.
- Feed health dashboard backed by real feed status data.
- Filter rules for incoming articles: mark read, star, or delete on title/URL/content match.
- Server-Sent Events endpoint for live unread count updates.
- PWA basics with manifest and service worker.
- Embedded static assets for single-binary builds.
- Lucide-style inline SVG icons, including ReadeRSS logo and favicon assets.

## Default Local Login

The development bootstrap creates a default local user:

```text
username: sarta
password: readerss
```

Change `READRESS_SESSION_KEY` before using the app anywhere outside local development.

## Development Requirements

Install the local tools first:

```powershell
go install github.com/air-verse/air@latest
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Verify:

```powershell
go version
templ version
air -v
sqlc version
goose -version
```

## Run Locally

Generate code and start the server:

```powershell
templ generate
sqlc generate
go run ./cmd/server
```

Or run with hot reload:

```powershell
air
```

The app listens on:

```text
http://localhost:8080
```

## Build

```powershell
go build -o bin\readress.exe ./cmd/server
.\bin\readress.exe
```

Useful environment variables:

```text
READRESS_ADDR=:8080
READRESS_DB=data/readress.db
READRESS_LOG_LEVEL=info
READRESS_SESSION_KEY=change-this-local-secret
```

## Project Structure

```text
cmd/server              App entry point and migrations
internal/config         Runtime config
internal/db             Bootstrap, sqlc queries, generated DB code
internal/handler        HTTP handlers, auth, background scheduler
internal/service        Feed fetching, content cleanup, rule application
internal/view           templ components and generated templates
web/static              CSS, JS, images, manifest, service worker
web/assets.go           Embedded static file system
```

## Validation

Run:

```powershell
templ generate
sqlc generate
go test ./...
go build -o bin\readress.exe ./cmd/server
```

## Roadmap

Remaining polish before a comfortable v1:

- Replace the minimal local HTMX helper with the full vendored HTMX build.
- Add Dockerfile and docker-compose for self-host deployment.
- Add integration tests with in-memory SQLite and fixture feeds.
- Add richer article extraction and feed favicon caching.
- Improve filter rules with regex support and a dedicated management page.
- Add README screenshots and production hardening notes.
