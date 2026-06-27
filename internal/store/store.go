// Package store wraps the SQLite database: connection, embedded migrations and
// seed data. modernc.org/sqlite is used (pure Go, no CGo) so the cross-compile
// in the Makefile keeps working.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	_ "modernc.org/sqlite"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store owns the database connection.
type Store struct {
	DB     *sql.DB
	logger *zap.Logger
}

// Open opens (or creates) the SQLite database at path, enables foreign keys,
// runs pending migrations and seeds baseline data.
func Open(path string, logger *zap.Logger) (*Store, error) {
	// _pragma busy_timeout avoids "database is locked" under concurrent access;
	// foreign_keys enforces the references declared in the schema.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &Store{DB: db, logger: logger}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	logger.Info("database ready", zap.String("path", path))
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.DB.Close()
}

// migrate applies every embedded migration that has not yet been recorded in
// the schema_migrations table, in lexical filename order, each in its own
// transaction.
func (s *Store) migrate() error {
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		if err := s.DB.QueryRow(
			`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, name,
		).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}

		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		s.logger.Info("applied migration", zap.String("version", name))
	}
	return nil
}
