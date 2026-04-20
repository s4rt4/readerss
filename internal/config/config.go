package config

import (
	"flag"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr     string
	DBPath   string
	LogLevel slog.Level
}

func Load() Config {
	cfg := Config{
		Addr:     getEnv("READRESS_ADDR", ":8080"),
		DBPath:   getEnv("READRESS_DB", "data/readress.db"),
		LogLevel: parseLogLevel(getEnv("READRESS_LOG_LEVEL", "info")),
	}

	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	flag.Parse()

	return cfg
}

func (c Config) DatabaseURL() string {
	values := url.Values{}
	values.Set("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "journal_mode(WAL)")
	values.Add("_pragma", "foreign_keys(ON)")

	return "file:" + c.DBPath + "?" + values.Encode()
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		if parsed, err := strconv.Atoi(value); err == nil {
			return slog.Level(parsed)
		}
		return slog.LevelInfo
	}
}
