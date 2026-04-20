.PHONY: dev build test templ sqlc migrate

APP_NAME := readress
DB_PATH ?= data/readress.db

dev:
	air

build: templ sqlc
	go build -o bin/$(APP_NAME).exe ./cmd/server

test:
	go test ./...

templ:
	templ generate

sqlc:
	sqlc generate

migrate:
	goose -dir cmd/server/migrations sqlite3 "$(DB_PATH)" up
