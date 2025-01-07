package db

import (
	"database/sql"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

func Connect() *sql.DB {
	db, err := sql.Open("postgres", os.Getenv("DATABASE_PUBLIC_URL"))
	if err != nil {
		panic(err)
	}

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	db.SetMaxOpenConns(10)

	slog.Info("Successfully connected!")
	return db
}

func Migrate() {
	m, err := migrate.New("file://db/migrations", os.Getenv("DATABASE_PUBLIC_URL"))
	if err != nil {
		slog.Error("Failed to run migrations", "error", err)
	}
	if err := m.Up(); err != nil {
		slog.Warn("Failed to call up() on migrations", "error", err)
	}
	slog.Info("Migrations successfully ran")
}
