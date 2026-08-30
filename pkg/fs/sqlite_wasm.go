//go:build js && wasm

package fs

import (
	"errors"
	"time"
)

// modernc.org/sqlite does not support Go's js/wasm target, so the SQLite
// backend is unavailable there. Turso-backed filesystems still work because
// they only need net/http.
type SQLiteOptions struct {
	Path    string
	Table   string
	Limit   int64
	Timeout time.Duration
	Mounts  []CopyMount
}

var ErrNoSQLite = errors.New("sqlite-backed filesystem is unavailable on js/wasm")

func NewSQLite(SQLiteOptions) (*Database, error) {
	return nil, ErrNoSQLite
}
