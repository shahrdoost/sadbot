// Command migrate applies /app/migrations to DATABASE_URL and exits.
// (The upstream migrate CLI image does not bundle the postgres driver.)
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	dir := env("MIGRATIONS_PATH", "/app/migrations")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fatal(errors.New("DATABASE_URL is required"))
	}
	m, err := migrate.New("file://"+dir, dbURL)
	if err != nil {
		fatal(err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		fatal(err)
	}
	fmt.Println("[migrate] up to date")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "[migrate] %v\n", err)
	os.Exit(1)
}
