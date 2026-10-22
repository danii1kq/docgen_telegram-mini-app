// Package store is the PostgreSQL persistence layer (pgx + hand-written SQL).
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// ErrNotFound is returned when a single row lookup finds nothing.
var ErrNotFound = errors.New("store: not found")

// ErrConflict is returned for unique-constraint violations (duplicate promo code, ...).
var ErrConflict = errors.New("store: conflict")

// Store wraps the connection pool.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// Open creates the pool and verifies connectivity.
func Open(ctx context.Context, databaseURL string, log *slog.Logger) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: parse DATABASE_URL: %w", err)
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnLifetime = time.Hour
	config.HealthCheckPeriod = 30 * time.Second
	// Server-side statement cache stays default; the API uses simple queries.

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{pool: pool, log: log}, nil
}

// Pool exposes the raw pool (used by the seed importer and health checks).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Close releases every connection.
func (s *Store) Close() { s.pool.Close() }

// Ping checks the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate applies migrations/*.sql in file-name order, once each.
func (s *Store) Migrate(ctx context.Context) error {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("store: read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("store: acquire for migrate: %w", err)
	}
	defer conn.Release()

	// The tracking table has to exist before it can be queried.
	if _, err := conn.Exec(ctx, `create table if not exists schema_migrations (
		version text primary key,
		applied_at timestamptz not null default now()
	)`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	for _, name := range names {
		var applied bool
		if err := conn.QueryRow(ctx,
			`select exists(select 1 from schema_migrations where version = $1)`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("store: check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		sqlBytes, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("store: read %s: %w", name, err)
		}

		// pgconn.Exec sends the file through the simple protocol, so multiple
		// statements and plain '$' quoting work without extra parsing.
		if _, err := conn.Conn().PgConn().Exec(ctx, string(sqlBytes)).ReadAll(); err != nil {
			return fmt.Errorf("store: apply %s: %w", name, err)
		}
		if _, err := conn.Exec(ctx,
			`insert into schema_migrations (version) values ($1)`, name,
		); err != nil {
			return fmt.Errorf("store: record migration %s: %w", name, err)
		}
		s.log.Info("migration applied", "file", name)
	}
	return nil
}

// IsEmpty reports whether the demo data has not been imported yet.
func (s *Store) IsEmpty(ctx context.Context) (bool, error) {
	var orders int
	if err := s.pool.QueryRow(ctx, `select count(*) from orders`).Scan(&orders); err != nil {
		return false, err
	}
	return orders == 0, nil
}

// withTx runs fn inside a transaction and commits on success.
func (s *Store) withTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// classify maps driver errors onto ErrConflict.
func classify(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.Detail)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.Detail)
		}
	}
	return err
}

// nullString converts an empty string to NULL.
func nullString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

// nullChatID converts the "no chat" zero value to SQL NULL.
func nullChatID(chatID int64) *int64 {
	if chatID == 0 {
		return nil
	}
	return &chatID
}

// utf8Time normalizes a stored timestamp to UTC before it reaches JSON.
func utc(t time.Time) time.Time { return t.UTC() }

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	normalized := t.UTC()
	return &normalized
}
