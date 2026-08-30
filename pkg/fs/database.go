package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// DB is the minimal database surface a Database filesystem needs. Query
// returns rows of driver values limited to nil, int64, float64, string and
// []byte, which is what both bundled backends produce. Implementations may be
// supplied by users to reach any other SQLite-compatible database.
type DB interface {
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) ([][]any, error)
	Close() error
}

const (
	defaultTablePrefix  = "gash_fs"
	defaultQueryTimeout = 30 * time.Second
	rootPath            = "."
)

type DatabaseOptions struct {
	// DB is the backend the filesystem stores its rows in.
	DB DB
	// Table is the prefix of the two tables owned by the filesystem. It
	// defaults to "gash_fs" and must contain only letters, digits and
	// underscores because it is interpolated into statements.
	Table string
	// Limit bounds the total number of file data bytes retained. Zero means
	// unbounded.
	Limit int64
	// Timeout bounds a single database statement. It defaults to 30 seconds.
	Timeout time.Duration
	// Mounts are copied into the database before the filesystem is returned.
	Mounts []CopyMount
}

// Database stores a complete virtual filesystem in two SQL tables. Paths are
// rows referencing node rows, so hard links, symbolic links, permissions and
// modification times survive process restarts. Every operation is serialized
// through one mutex, which keeps the read-modify-write sequences consistent
// without requiring the backend to support interactive transactions.
type Database struct {
	mu      sync.Mutex
	db      DB
	nodes   string
	paths   string
	limit   int64
	timeout time.Duration
}

func NewDatabase(options DatabaseOptions) (*Database, error) {
	if options.DB == nil {
		return nil, errors.New("database filesystem requires a DB")
	}
	prefix := options.Table
	if prefix == "" {
		prefix = defaultTablePrefix
	}
	if err := validTable(prefix); err != nil {
		return nil, err
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultQueryTimeout
	}
	d := &Database{
		db:      options.DB,
		nodes:   prefix + "_nodes",
		paths:   prefix + "_paths",
		limit:   options.Limit,
		timeout: timeout,
	}
	if err := d.init(); err != nil {
		return nil, err
	}
	if err := ApplyMounts(d, options.Mounts); err != nil {
		return nil, err
	}
	return d, nil
}

// Close releases the backing database handle.
func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.db.Close()
}

func validTable(name string) error {
	for _, r := range name {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !letter && !(r >= '0' && r <= '9') && r != '_' {
			return fmt.Errorf("invalid table prefix %q", name)
		}
	}
	if name == "" {
		return errors.New("empty table prefix")
	}
	return nil
}

func (d *Database) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d.timeout)
}

func (d *Database) init() error {
	ctx, cancel := d.context()
	defer cancel()
	statements := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind INTEGER NOT NULL,
			data BLOB NOT NULL,
			target TEXT NOT NULL,
			mode INTEGER NOT NULL,
			mtime INTEGER NOT NULL)`, d.nodes),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			path TEXT PRIMARY KEY,
			parent TEXT NOT NULL,
			node INTEGER NOT NULL)`, d.paths),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_parent ON %s(parent)`, d.paths, d.paths),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_node ON %s(node)`, d.paths, d.paths),
	}
	for _, statement := range statements {
		if err := d.db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	rows, err := d.db.Query(ctx, fmt.Sprintf("SELECT node FROM %s WHERE path = ?", d.paths), rootPath)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		return nil
	}
	id, err := d.insertNode(ctx, directory, nil, "", iofs.ModeDir|0o755, time.Now())
	if err != nil {
		return err
	}
	return d.db.Exec(ctx, fmt.Sprintf("INSERT INTO %s(path, parent, node) VALUES(?, ?, ?)", d.paths), rootPath, "", id)
}

type dbNode struct {
	id     int64
	kind   nodeKind
	target string
	mode   iofs.FileMode
	mtime  time.Time
	size   int64
}

func (d *Database) insertNode(ctx context.Context, kind nodeKind, data []byte, target string, mode iofs.FileMode, mtime time.Time) (int64, error) {
	if data == nil {
		data = []byte{}
	}
	query := fmt.Sprintf("INSERT INTO %s(kind, data, target, mode, mtime) VALUES(?, ?, ?, ?, ?) RETURNING id", d.nodes)
	rows, err := d.db.Query(ctx, query, int64(kind), data, target, int64(mode), mtime.UnixNano())
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0, errors.New("database did not return an inserted node id")
	}
	return asInt(rows[0][0]), nil
}

func (d *Database) lookup(ctx context.Context, names []string) (map[string]*dbNode, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	query := fmt.Sprintf(
		"SELECT p.path, n.id, n.kind, n.target, n.mode, n.mtime, LENGTH(n.data) FROM %s p JOIN %s n ON n.id = p.node WHERE p.path IN (%s)",
		d.paths, d.nodes, placeholders,
	)
	args := make([]any, 0, len(names))
	for _, name := range names {
		args = append(args, name)
	}
	rows, err := d.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*dbNode, len(rows))
	for _, row := range rows {
		if len(row) < 7 {
			return nil, errors.New("unexpected node row shape")
		}
		out[asString(row[0])] = &dbNode{
			id:     asInt(row[1]),
			kind:   nodeKind(asInt(row[2])),
			target: asString(row[3]),
			mode:   iofs.FileMode(uint32(asInt(row[4]))),
			mtime:  time.Unix(0, asInt(row[5])),
			size:   asInt(row[6]),
		}
	}
	return out, nil
}

// resolve walks name component by component, following symbolic links the way
// a kernel would. Every attempt fetches all ancestors in one query so a remote
// backend pays one round trip per link traversal rather than per component.
func (d *Database) resolve(ctx context.Context, name string, followFinal bool) (string, *dbNode, error) {
	for links := 0; links <= 40; links++ {
		if err := valid(name); err != nil {
			return "", nil, err
		}
		parts := strings.Split(name, "/")
		prefixes := []string{rootPath}
		cur := rootPath
		for _, part := range parts {
			if part == "." || part == "" {
				continue
			}
			cur = join(cur, part)
			prefixes = append(prefixes, cur)
		}
		found, err := d.lookup(ctx, prefixes)
		if err != nil {
			return "", nil, err
		}
		cur = rootPath
		restart := false
		for i, part := range parts {
			if part == "." || part == "" {
				continue
			}
			cur = join(cur, part)
			n := found[cur]
			if n == nil {
				return cur, nil, iofs.ErrNotExist
			}
			final := i == len(parts)-1
			if n.kind == symlink && (followFinal || !final) {
				rest := strings.Join(parts[i+1:], "/")
				target := n.target
				if strings.HasPrefix(target, "/") {
					target = Name(target)
				} else {
					target = path.Join(path.Dir(cur), target)
				}
				name = path.Clean(path.Join(target, rest))
				restart = true
				break
			}
			if !final && n.kind != directory {
				return cur, nil, ErrNotDir
			}
		}
		if !restart {
			return cur, found[cur], nil
		}
	}
	return "", nil, ErrLoop
}

// resolveParent resolves the parent directory of name and returns the resolved
// path the new entry would occupy.
func (d *Database) resolveParent(ctx context.Context, name string) (string, error) {
	resolvedParent, p, err := d.resolve(ctx, parent(name), true)
	if err != nil {
		return "", err
	}
	if p.kind != directory {
		return "", ErrNotDir
	}
	return join(resolvedParent, path.Base(name)), nil
}

// prepareCreate resolves where a new entry belongs and reports a conflicting
// entry that path resolution could not follow, such as a dangling symbolic
// link. Callers that replace an entry, like WriteFile, clear it first.
func (d *Database) prepareCreate(ctx context.Context, name string, replace bool) (string, error) {
	resolved, err := d.resolveParent(ctx, name)
	if err != nil {
		return "", err
	}
	existing, err := d.lookup(ctx, []string{resolved})
	if err != nil {
		return "", err
	}
	if existing[resolved] == nil {
		return resolved, nil
	}
	if !replace {
		return "", iofs.ErrExist
	}
	if err := d.deletePaths(ctx, []string{resolved}); err != nil {
		return "", err
	}
	return resolved, nil
}

func (d *Database) children(ctx context.Context, resolved string) ([]iofs.DirEntry, error) {
	query := fmt.Sprintf(
		"SELECT p.path, n.kind, n.mode, n.mtime, LENGTH(n.data) FROM %s p JOIN %s n ON n.id = p.node WHERE p.parent = ?",
		d.paths, d.nodes,
	)
	rows, err := d.db.Query(ctx, query, resolved)
	if err != nil {
		return nil, err
	}
	out := make([]iofs.DirEntry, 0, len(rows))
	for _, row := range rows {
		if len(row) < 5 {
			return nil, errors.New("unexpected directory row shape")
		}
		out = append(out, dbEntry{info: dbInfo{
			name:  path.Base(asString(row[0])),
			size:  asInt(row[4]),
			mode:  iofs.FileMode(uint32(asInt(row[2]))),
			mtime: time.Unix(0, asInt(row[3])),
		}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (d *Database) hasChildren(ctx context.Context, resolved string) (bool, error) {
	rows, err := d.db.Query(ctx, fmt.Sprintf("SELECT 1 FROM %s WHERE parent = ? LIMIT 1", d.paths), resolved)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (d *Database) readData(ctx context.Context, id int64) ([]byte, error) {
	rows, err := d.db.Query(ctx, fmt.Sprintf("SELECT data FROM %s WHERE id = ?", d.nodes), id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return nil, iofs.ErrNotExist
	}
	return asBytes(rows[0][0]), nil
}

func (d *Database) usedBytes(ctx context.Context) (int64, error) {
	rows, err := d.db.Query(ctx, fmt.Sprintf("SELECT COALESCE(SUM(LENGTH(data)), 0) FROM %s", d.nodes))
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0, nil
	}
	return asInt(rows[0][0]), nil
}

func (d *Database) checkQuota(ctx context.Context, delta int64) error {
	if d.limit <= 0 || delta <= 0 {
		return nil
	}
	used, err := d.usedBytes(ctx)
	if err != nil {
		return err
	}
	if used+delta > d.limit {
		return ErrQuota
	}
	return nil
}

// Used reports the total number of file data bytes stored in the database.
func (d *Database) Used() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	used, err := d.usedBytes(ctx)
	if err != nil {
		return 0
	}
	return used
}

func (d *Database) Open(name string) (iofs.File, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	resolved, n, err := d.resolve(ctx, name, true)
	if err != nil {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: err}
	}
	file := &dbFile{info: nodeInfo(path.Base(resolved), n)}
	if n.kind == directory {
		entries, err := d.children(ctx, resolved)
		if err != nil {
			return nil, &iofs.PathError{Op: "open", Path: name, Err: err}
		}
		file.entries = entries
		return file, nil
	}
	data, err := d.readData(ctx, n.id)
	if err != nil {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: err}
	}
	file.data = data
	return file, nil
}

func (d *Database) ReadFile(name string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	_, n, err := d.resolve(ctx, name, true)
	if err != nil {
		return nil, err
	}
	if n.kind == directory {
		return nil, ErrIsDir
	}
	return d.readData(ctx, n.id)
}

func (d *Database) ReadDir(name string) ([]iofs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	resolved, n, err := d.resolve(ctx, name, true)
	if err != nil {
		return nil, err
	}
	if n.kind != directory {
		return nil, ErrNotDir
	}
	return d.children(ctx, resolved)
}

func (d *Database) Stat(name string) (iofs.FileInfo, error) {
	return d.stat(name, true)
}

func (d *Database) Lstat(name string) (iofs.FileInfo, error) {
	return d.stat(name, false)
}

func (d *Database) stat(name string, followFinal bool) (iofs.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	resolved, n, err := d.resolve(ctx, name, followFinal)
	if err != nil {
		return nil, err
	}
	return nodeInfo(path.Base(resolved), n), nil
}

func (d *Database) CreateFile(name string, data []byte, perm iofs.FileMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(name); err != nil {
		return err
	}
	if _, _, err := d.resolve(ctx, name, false); err == nil {
		return iofs.ErrExist
	} else if !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	resolved, err := d.prepareCreate(ctx, name, false)
	if err != nil {
		return err
	}
	if err := d.checkQuota(ctx, int64(len(data))); err != nil {
		return err
	}
	return d.createEntry(ctx, resolved, regular, data, "", perm.Perm())
}

func (d *Database) createEntry(ctx context.Context, resolved string, kind nodeKind, data []byte, target string, mode iofs.FileMode) error {
	id, err := d.insertNode(ctx, kind, data, target, mode, time.Now())
	if err != nil {
		return err
	}
	query := fmt.Sprintf("INSERT INTO %s(path, parent, node) VALUES(?, ?, ?)", d.paths)
	return d.db.Exec(ctx, query, resolved, parent(resolved), id)
}

func (d *Database) WriteFile(name string, data []byte, perm iofs.FileMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(name); err != nil {
		return err
	}
	if data == nil {
		data = []byte{}
	}
	_, n, err := d.resolve(ctx, name, true)
	if err != nil && !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	if n == nil {
		resolved, err := d.prepareCreate(ctx, name, true)
		if err != nil {
			return err
		}
		if err := d.checkQuota(ctx, int64(len(data))); err != nil {
			return err
		}
		return d.createEntry(ctx, resolved, regular, data, "", perm.Perm())
	}
	if n.kind == directory {
		return ErrIsDir
	}
	if err := d.checkQuota(ctx, int64(len(data))-n.size); err != nil {
		return err
	}
	query := fmt.Sprintf("UPDATE %s SET data = ?, mode = ?, mtime = ? WHERE id = ?", d.nodes)
	return d.db.Exec(ctx, query, data, int64(n.mode.Type()|perm.Perm()), time.Now().UnixNano(), n.id)
}

func (d *Database) AppendFile(name string, data []byte, perm iofs.FileMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(name); err != nil {
		return err
	}
	if data == nil {
		data = []byte{}
	}
	_, n, err := d.resolve(ctx, name, true)
	if err != nil && !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	if err := d.checkQuota(ctx, int64(len(data))); err != nil {
		return err
	}
	if n == nil {
		resolved, err := d.prepareCreate(ctx, name, true)
		if err != nil {
			return err
		}
		return d.createEntry(ctx, resolved, regular, data, "", perm.Perm())
	}
	if n.kind != regular {
		return ErrIsDir
	}
	existing, err := d.readData(ctx, n.id)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("UPDATE %s SET data = ?, mtime = ? WHERE id = ?", d.nodes)
	return d.db.Exec(ctx, query, append(existing, data...), time.Now().UnixNano(), n.id)
}

func (d *Database) Mkdir(name string, perm iofs.FileMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(name); err != nil {
		return err
	}
	if _, _, err := d.resolve(ctx, name, true); err == nil {
		return iofs.ErrExist
	} else if !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	resolved, err := d.prepareCreate(ctx, name, false)
	if err != nil {
		return err
	}
	return d.createEntry(ctx, resolved, directory, nil, "", iofs.ModeDir|perm.Perm())
}

func (d *Database) MkdirAll(name string, perm iofs.FileMode) error {
	if err := valid(name); err != nil {
		return err
	}
	if name == rootPath {
		return nil
	}
	cur := ""
	for _, part := range strings.Split(name, "/") {
		if cur == "" {
			cur = part
		} else {
			cur = cur + "/" + part
		}
		err := d.Mkdir(cur, perm)
		if err != nil && !errors.Is(err, iofs.ErrExist) {
			return err
		}
	}
	return nil
}

func (d *Database) Remove(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(name); err != nil {
		return err
	}
	if name == rootPath {
		return errors.New("cannot remove root")
	}
	resolved, n, err := d.resolve(ctx, name, false)
	if err != nil {
		return err
	}
	if n.kind == directory {
		populated, err := d.hasChildren(ctx, resolved)
		if err != nil {
			return err
		}
		if populated {
			return ErrNotEmpty
		}
	}
	return d.deletePaths(ctx, []string{resolved})
}

func (d *Database) RemoveAll(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(name); err != nil {
		return err
	}
	if name == rootPath {
		return errors.New("cannot remove root")
	}
	resolved, _, err := d.resolve(ctx, name, false)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil
		}
		return err
	}
	return d.removeSubtree(ctx, resolved)
}

func (d *Database) removeSubtree(ctx context.Context, resolved string) error {
	prefix := resolved + "/"
	query := fmt.Sprintf("DELETE FROM %s WHERE path = ? OR substr(path, 1, ?) = ?", d.paths)
	if err := d.db.Exec(ctx, query, resolved, int64(len(prefix)), prefix); err != nil {
		return err
	}
	return d.collect(ctx)
}

func (d *Database) deletePaths(ctx context.Context, names []string) error {
	for _, name := range names {
		query := fmt.Sprintf("DELETE FROM %s WHERE path = ?", d.paths)
		if err := d.db.Exec(ctx, query, name); err != nil {
			return err
		}
	}
	return d.collect(ctx)
}

// collect drops node rows that no path references any more, which is how the
// filesystem releases the bytes of the last link to a file.
func (d *Database) collect(ctx context.Context) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id NOT IN (SELECT node FROM %s)", d.nodes, d.paths)
	return d.db.Exec(ctx, query)
}

func (d *Database) Rename(oldName, newName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(oldName); err != nil {
		return err
	}
	if err := valid(newName); err != nil {
		return err
	}
	oldResolved, n, err := d.resolve(ctx, oldName, false)
	if err != nil {
		return err
	}
	newResolved, err := d.resolveParent(ctx, newName)
	if err != nil {
		return err
	}
	if strings.HasPrefix(newResolved+"/", oldResolved+"/") {
		return errors.New("cannot move directory into itself")
	}
	existing, err := d.lookup(ctx, []string{newResolved})
	if err != nil {
		return err
	}
	if target := existing[newResolved]; target != nil {
		if target.kind == directory && n.kind != directory {
			return ErrIsDir
		}
		if target.kind != directory && n.kind == directory {
			return ErrNotDir
		}
		if target.kind == directory {
			populated, err := d.hasChildren(ctx, newResolved)
			if err != nil {
				return err
			}
			if populated {
				return ErrNotEmpty
			}
		}
		if err := d.deletePaths(ctx, []string{newResolved}); err != nil {
			return err
		}
	}
	return d.movePaths(ctx, oldResolved, newResolved)
}

func (d *Database) movePaths(ctx context.Context, oldResolved, newResolved string) error {
	prefix := oldResolved + "/"
	query := fmt.Sprintf("SELECT path FROM %s WHERE path = ? OR substr(path, 1, ?) = ?", d.paths)
	rows, err := d.db.Query(ctx, query, oldResolved, int64(len(prefix)), prefix)
	if err != nil {
		return err
	}
	moves := make([]string, 0, len(rows))
	for _, row := range rows {
		moves = append(moves, asString(row[0]))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(moves)))
	update := fmt.Sprintf("UPDATE %s SET path = ?, parent = ? WHERE path = ?", d.paths)
	for _, current := range moves {
		moved := newResolved + strings.TrimPrefix(current, oldResolved)
		if err := d.db.Exec(ctx, update, moved, parent(moved), current); err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) Symlink(target, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(name); err != nil {
		return err
	}
	if _, _, err := d.resolve(ctx, name, false); err == nil {
		return iofs.ErrExist
	} else if !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	resolved, err := d.prepareCreate(ctx, name, false)
	if err != nil {
		return err
	}
	return d.createEntry(ctx, resolved, symlink, nil, target, iofs.ModeSymlink|0o777)
}

func (d *Database) Readlink(name string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	_, n, err := d.resolve(ctx, name, false)
	if err != nil {
		return "", err
	}
	if n.kind != symlink {
		return "", errors.New("not a symbolic link")
	}
	return n.target, nil
}

func (d *Database) Link(oldName, newName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	if err := valid(newName); err != nil {
		return err
	}
	_, n, err := d.resolve(ctx, oldName, true)
	if err != nil {
		return err
	}
	if n.kind != regular {
		return errors.New("hard link source is not a regular file")
	}
	if _, _, err := d.resolve(ctx, newName, false); err == nil {
		return iofs.ErrExist
	} else if !errors.Is(err, iofs.ErrNotExist) {
		return err
	}
	resolved, err := d.prepareCreate(ctx, newName, false)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("INSERT INTO %s(path, parent, node) VALUES(?, ?, ?)", d.paths)
	return d.db.Exec(ctx, query, resolved, parent(resolved), n.id)
}

func (d *Database) Chmod(name string, mode iofs.FileMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	_, n, err := d.resolve(ctx, name, true)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("UPDATE %s SET mode = ?, mtime = ? WHERE id = ?", d.nodes)
	return d.db.Exec(ctx, query, int64(n.mode.Type()|mode.Perm()), time.Now().UnixNano(), n.id)
}

func (d *Database) Chtimes(name string, _ time.Time, mtime time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.context()
	defer cancel()
	_, n, err := d.resolve(ctx, name, true)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("UPDATE %s SET mtime = ? WHERE id = ?", d.nodes)
	return d.db.Exec(ctx, query, mtime.UnixNano(), n.id)
}

func nodeInfo(name string, n *dbNode) dbInfo {
	return dbInfo{name: name, size: n.size, mode: n.mode, mtime: n.mtime}
}

func asInt(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case []byte:
		return parseInt(string(v))
	case string:
		return parseInt(v)
	default:
		return 0
	}
}

func parseInt(value string) int64 {
	var out int64
	negative := strings.HasPrefix(value, "-")
	for _, r := range strings.TrimPrefix(value, "-") {
		if r < '0' || r > '9' {
			return 0
		}
		out = out*10 + int64(r-'0')
	}
	if negative {
		return -out
	}
	return out
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func asBytes(value any) []byte {
	switch v := value.(type) {
	case []byte:
		if v == nil {
			return []byte{}
		}
		return v
	case string:
		return []byte(v)
	case nil:
		return []byte{}
	default:
		return []byte(fmt.Sprint(v))
	}
}

type dbInfo struct {
	name  string
	size  int64
	mode  iofs.FileMode
	mtime time.Time
}

func (i dbInfo) Name() string {
	return i.name
}

func (i dbInfo) Size() int64 {
	return i.size
}

func (i dbInfo) Mode() iofs.FileMode {
	return i.mode
}

func (i dbInfo) ModTime() time.Time {
	return i.mtime
}

func (i dbInfo) IsDir() bool {
	return i.mode.IsDir()
}

func (i dbInfo) Sys() any {
	return nil
}

type dbEntry struct {
	info dbInfo
}

func (e dbEntry) Name() string {
	return e.info.name
}

func (e dbEntry) IsDir() bool {
	return e.info.IsDir()
}

func (e dbEntry) Type() iofs.FileMode {
	return e.info.mode.Type()
}

func (e dbEntry) Info() (iofs.FileInfo, error) {
	return e.info, nil
}

type dbFile struct {
	info      dbInfo
	data      []byte
	offset    int
	entries   []iofs.DirEntry
	dirOffset int
}

func (f *dbFile) Stat() (iofs.FileInfo, error) {
	return f.info, nil
}

func (*dbFile) Close() error {
	return nil
}

func (f *dbFile) Read(p []byte) (int, error) {
	if f.info.IsDir() {
		return 0, ErrIsDir
	}
	if f.offset >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.offset:])
	f.offset += n
	return n, nil
}

func (f *dbFile) ReadDir(n int) ([]iofs.DirEntry, error) {
	if !f.info.IsDir() {
		return nil, ErrNotDir
	}
	if f.dirOffset >= len(f.entries) {
		if n > 0 {
			return nil, io.EOF
		}
		return []iofs.DirEntry{}, nil
	}
	end := len(f.entries)
	if n > 0 && f.dirOffset+n < end {
		end = f.dirOffset + n
	}
	out := append([]iofs.DirEntry(nil), f.entries[f.dirOffset:end]...)
	f.dirOffset = end
	return out, nil
}

var (
	_ iofs.FS         = (*Database)(nil)
	_ iofs.ReadFileFS = (*Database)(nil)
	_ iofs.ReadDirFS  = (*Database)(nil)
	_ iofs.StatFS     = (*Database)(nil)
	_ LstatFS         = (*Database)(nil)
	_ CreateFileFS    = (*Database)(nil)
	_ WriteFileFS     = (*Database)(nil)
	_ AppendFileFS    = (*Database)(nil)
	_ MkdirFS         = (*Database)(nil)
	_ MkdirAllFS      = (*Database)(nil)
	_ RemoveFS        = (*Database)(nil)
	_ RemoveAllFS     = (*Database)(nil)
	_ RenameFS        = (*Database)(nil)
	_ SymlinkFS       = (*Database)(nil)
	_ ReadlinkFS      = (*Database)(nil)
	_ LinkFS          = (*Database)(nil)
	_ ChmodFS         = (*Database)(nil)
	_ ChtimesFS       = (*Database)(nil)
)
