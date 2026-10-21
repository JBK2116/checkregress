package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"github.com/JBK2116/checkregress/internal/models"
)

const (
	// maxOpenConns caps the database connection pool at one connection. SQLite
	// permits a single writer at a time and the Store already serializes writes
	// through a single goroutine, so one connection is sufficient.
	maxOpenConns = 1
	// busyTimeoutMS is how long a SQLite operation waits on a locked database
	// before returning SQLITE_BUSY.
	busyTimeoutMS = 5000
)

// schemaStatements is the SQLite schema, applied idempotently on startup so
// the database file is self-initializing.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS shadows (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    canonical_path   TEXT NOT NULL,
    legacy_url       TEXT NOT NULL,
    secondary_url    TEXT NOT NULL,
    candidate_url    TEXT NOT NULL,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    status_legacy    INTEGER NOT NULL,
    status_candidate INTEGER NOT NULL,
    status_match     INTEGER NOT NULL,

    headers          TEXT,
    body             TEXT,

    CONSTRAINT shadows_status_match_ck
        CHECK (status_match = (status_legacy = status_candidate)),
    CONSTRAINT shadows_row_kind_ck
        CHECK (
            (status_match AND headers IS NOT NULL AND body IS NOT NULL) OR
            (NOT status_match AND headers IS NULL AND body IS NULL)
        )
)`,
	`CREATE INDEX IF NOT EXISTS idx_shadows_path_created
    ON shadows (canonical_path, created_at DESC)`,
}

// insertShadowsStmt is the INSERT statement used to batch rows into shadows.
const insertShadowsStmt = `
INSERT INTO shadows
    (canonical_path, legacy_url, secondary_url, candidate_url,
     status_legacy, status_candidate, status_match, headers, body)
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?)`

// sqliteDB is the concrete [pool] implementation backed by a SQLite database.
type sqliteDB struct {
	db *sql.DB
}

// newDB opens the SQLite database at path, applies its connection pragmas,
// creates the schema if it does not yet exist, and returns a [pool] for the
// Store.
func newDB(ctx context.Context, path string) (pool, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	if err := applyPragmas(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := createSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(maxOpenConns)
	return &sqliteDB{db: db}, nil
}

// applyPragmas configures the SQLite connection for use as a single-writer
// store.
func applyPragmas(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = " + strconv.Itoa(busyTimeoutMS),
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("apply pragma: %w", err)
		}
	}
	return nil
}

// createSchema creates the shadows table and index if they do not yet exist.
func createSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range schemaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}
	return nil
}

// InsertRows inserts the given rows into the shadows table in a single
// transaction so a batch is applied atomically.
func (d *sqliteDB) InsertRows(ctx context.Context, rows []*models.ShadowRow) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // a committed transaction returns an error that is safe to ignore

	stmt, err := tx.PrepareContext(ctx, insertShadowsStmt)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }() //nolint:errcheck // close errors are safe to ignore

	for _, r := range rows {
		// Untyped nil becomes SQL NULL. The row-kind CHECK requires NULL, not
		// JSON 'null'.
		var headers, body any
		if r.Headers != nil {
			b, err := json.Marshal(r.Headers)
			if err != nil {
				return fmt.Errorf("marshal headers: %w", err)
			}
			headers = string(b)
		}
		if r.Body != nil {
			b, err := json.Marshal(r.Body)
			if err != nil {
				return fmt.Errorf("marshal body: %w", err)
			}
			body = string(b)
		}

		if _, err := stmt.ExecContext(ctx,
			r.CanonicalPath, r.LegacyURL, r.SecondaryURL, r.CandidateURL,
			r.StatusLegacy, r.StatusCandidate, boolToInt(r.StatusMatch),
			headers, body,
		); err != nil {
			return fmt.Errorf("insert shadow row: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Close closes the underlying database.
func (d *sqliteDB) Close() error {
	return d.db.Close()
}

// boolToInt converts a bool to the integer SQLite uses for booleans (0/1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
