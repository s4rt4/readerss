# ReadeRSS

ReadeRSS is a self-hosted RSS reader built with Go, HTMX, templ, Tailwind CSS, and SQLite.

## Development

Install the local tools first:

```powershell
go install github.com/air-verse/air@latest
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Generate code and run the server:

```powershell
templ generate
sqlc generate
go run ./cmd/server
```

The app listens on `http://localhost:8080` by default.
