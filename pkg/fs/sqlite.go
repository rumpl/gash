//go:build !(js && wasm)

package fs

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

type SQLiteOptions struct {
	// Path is the host path of the database file. An empty path or
	// ":memory:" selects a private in-memory database that is discarded when
	// the filesystem is closed.
	Path string
	// Table is the prefix of the tables owned by the filesystem.
	Table string
	// Limit bounds the total number of file data bytes retained.
	Limit int64
	// Timeout bounds a single database statement.
	Timeout time.Duration
	// Mounts are copied into the database before the filesystem is returned.
	Mounts []CopyMount
}

// NewSQLite returns a filesystem persisted in a SQLite database. It uses the
// CGO-free modernc.org/sqlite driver, so it keeps gash's single-binary policy,
// and it opens exactly one connection because every filesystem operation is
// already serialized by the filesystem itself.
func NewSQLite(options SQLiteOptions) (*Database, error) {
	database, err := sql.Open("sqlite", sqliteDSN(options.Path))
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.PingContext(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	filesystem, err := NewDatabase(DatabaseOptions{
		DB:      &sqlDB{db: database},
		Table:   options.Table,
		Limit:   options.Limit,
		Timeout: options.Timeout,
		Mounts:  options.Mounts,
	})
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return filesystem, nil
}

func sqliteDSN(path string) string {
	if path == "" || path == ":memory:" {
		return ":memory:"
	}
	if strings.HasPrefix(path, "file:") {
		return path
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	dsn := url.URL{
		Scheme:   "file",
		OmitHost: true,
		Path:     filepath.ToSlash(absolute),
		RawQuery: "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
	}
	return dsn.String()
}

// sqlDB adapts database/sql to the DB interface.
type sqlDB struct {
	db *sql.DB
}

func (s *sqlDB) Exec(ctx context.Context, query string, args ...any) error {
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("%s: %w", firstWords(query), err)
	}
	return nil
}

func (s *sqlDB) Query(ctx context.Context, query string, args ...any) ([][]any, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", firstWords(query), err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out [][]any
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		out = append(out, values)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *sqlDB) Close() error {
	return s.db.Close()
}

// firstWords keeps database errors readable without echoing whole statements.
func firstWords(query string) string {
	fields := strings.Fields(query)
	if len(fields) > 2 {
		fields = fields[:2]
	}
	if len(fields) == 0 {
		return "sql"
	}
	return strings.Join(fields, " ")
}
