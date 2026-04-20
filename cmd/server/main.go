package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"

	"readress/internal/config"
	"readress/internal/db"
	"readress/internal/handler"
	webassets "readress/web"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	conn, err := sql.Open("sqlite", cfg.DatabaseURL())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer conn.Close()

	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if err := db.Migrate(conn, migrationsFS); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	userID, err := db.EnsureDefaultData(context.Background(), conn)
	if err != nil {
		return fmt.Errorf("bootstrap default data: %w", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	staticFS, err := fs.Sub(webassets.Static, "static")
	if err != nil {
		return fmt.Errorf("load embedded static files: %w", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	app := handler.NewApp(conn, logger, userID)
	app.Routes(r)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.StartBackground(ctx)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.Addr, "db", cfg.DBPath)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
