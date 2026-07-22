package sqlite

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	iofs "io/fs"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rumpl/gash/internal/command"
	gfs "github.com/rumpl/gash/pkg/fs"
	modernsqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	name = "sqlite3"

	maxDatabaseBytes = 128 << 20
	maxResultRows    = 100000
	maxResultBytes   = 64 << 20
	maxSQLBytes      = 16 << 20
	queryTimeout     = 30 * time.Second
)

// sqlite3 uses modernc.org/sqlite rather than host sqlite3. It is a pure-Go
// (CGO-free) port of upstream SQLite, which preserves gash's cross-platform
// single-binary policy on modernc's supported darwin/freebsd/linux/windows
// GOOS/GOARCH set. Database bytes are read from gash's virtual filesystem into
// a private in-memory SQLite connection and serialized back only through
// pkg/fs.WriteFile, so SQLite never opens host paths. This differs from
// just-bash's sql.js worker protocol (no JS worker, protocol token, or V8 heap
// limits); gash relies on Go context cancellation, SQLite interrupts/limits,
// result/database byte ceilings, and a per-virtual-file lock. Extension loading
// and host filesystem escape features are unavailable because no SQLite file
// path is exposed and the CLI intentionally rejects meta-commands.

var locks sync.Map // map[iofs.FS]*lockSet; scoped per FS identity

type lockSet struct {
	mu    sync.Mutex
	locks map[string]*databaseLock
}

type databaseLock struct {
	held    bool
	waiters []chan struct{}
}

type options struct {
	mode                 outputMode
	header               bool
	separator, newline   string
	nullValue            string
	readonly, bail, echo bool
	cmd                  []string
}

type parsedArgs struct {
	options     options
	database    string
	sql         string
	showVersion bool
}

type outputMode string

const (
	modeList     outputMode = "list"
	modeCSV      outputMode = "csv"
	modeJSON     outputMode = "json"
	modeLine     outputMode = "line"
	modeColumn   outputMode = "column"
	modeTable    outputMode = "table"
	modeMarkdown outputMode = "markdown"
	modeTabs     outputMode = "tabs"
	modeBox      outputMode = "box"
	modeQuote    outputMode = "quote"
	modeHTML     outputMode = "html"
	modeASCII    outputMode = "ascii"
)

type statementResult struct {
	columns []string
	rows    [][]any
	err     error
}

func commandSQLite3(ctx context.Context, args []string, c *command.Context) int {
	parsed, parseErr := parseArgs(args)
	if parseErr != nil {
		fmt.Fprint(c.Stderr, parseErr.Error())
		return 1
	}
	if parsed.showVersion {
		fmt.Fprintln(c.Stdout, modernsqliteVersion())
		return 0
	}
	if parsed.database == "" {
		fmt.Fprintln(c.Stderr, "sqlite3: missing database argument")
		return 1
	}
	sqlText := parsed.sql
	if sqlText == "" {
		data, err := io.ReadAll(c.Stdin)
		if err != nil {
			fmt.Fprintf(c.Stderr, "sqlite3: failed to read stdin: %v\n", err)
			return 1
		}
		sqlText = strings.TrimSpace(string(data))
	}
	if len(parsed.options.cmd) > 0 {
		parts := append([]string{}, parsed.options.cmd...)
		if sqlText != "" {
			parts = append(parts, sqlText)
		}
		sqlText = strings.Join(parts, "; ")
	}
	if sqlText == "" {
		fmt.Fprintln(c.Stderr, "sqlite3: no SQL provided")
		return 1
	}
	if len(sqlText) > maxSQLBytes {
		fmt.Fprintf(c.Stderr, "sqlite3: SQL exceeds %d byte limit\n", maxSQLBytes)
		return 1
	}

	isMemory := parsed.database == ":memory:"
	dbPath := ""
	var dbBuffer []byte
	var release func()
	if !isMemory {
		dbPath = resolve(*c.Cwd, parsed.database)
		release, _ = acquireLock(ctx, c.FS, dbPath)
		if release == nil {
			fmt.Fprintf(c.Stderr, "sqlite3: unable to open database %q: database lock wait aborted\n", parsed.database)
			return 1
		}
		defer release()

		if info, err := gfs.Stat(c.FS, dbPath); err == nil {
			if info.IsDir() {
				fmt.Fprintf(c.Stderr, "sqlite3: unable to open database %q: is a directory\n", parsed.database)
				return 1
			}
			if err := assertDatabaseSize(info.Size()); err != nil {
				fmt.Fprintf(c.Stderr, "sqlite3: unable to open database %q: %v\n", parsed.database, err)
				return 1
			}
			data, err := gfs.ReadFile(c.FS, dbPath)
			if err != nil {
				fmt.Fprintf(c.Stderr, "sqlite3: unable to open database %q: %v\n", parsed.database, err)
				return 1
			}
			if err := assertDatabaseSize(int64(len(data))); err != nil {
				fmt.Fprintf(c.Stderr, "sqlite3: unable to open database %q: %v\n", parsed.database, err)
				return 1
			}
			dbBuffer = data
		} else if !errors.Is(err, iofs.ErrNotExist) {
			fmt.Fprintf(c.Stderr, "sqlite3: unable to open database %q: %v\n", parsed.database, err)
			return 1
		}
	}

	result, modified, serialized, err := execute(ctx, dbBuffer, sqlText, parsed.options, !isMemory)
	if err != nil {
		fmt.Fprintf(c.Stderr, "sqlite3: %v\n", err)
		return 1
	}

	stdout := &budgetBuilder{limit: maxResultBytes}
	if parsed.options.echo {
		if err := stdout.writeString(sqlText + "\n"); err != nil {
			fmt.Fprintf(c.Stderr, "sqlite3: %v\n", err)
			return 1
		}
	}
	hadError := false
	var bailError error
	for _, r := range result {
		if r.err != nil {
			hadError = true
			if parsed.options.bail {
				bailError = r.err
				break
			}
			if err := stdout.writeString("Error: " + sanitizeError(r.err) + "\n"); err != nil {
				fmt.Fprintf(c.Stderr, "sqlite3: %v\n", err)
				return 1
			}
			continue
		}
		if len(r.rows) == 0 && !parsed.options.header {
			continue
		}
		formatted, err := formatOutput(r.columns, r.rows, parsed.options, stdout.remaining())
		if err != nil {
			fmt.Fprintf(c.Stderr, "sqlite3: %v\n", err)
			return 1
		}
		if err := stdout.writeString(formatted); err != nil {
			fmt.Fprintf(c.Stderr, "sqlite3: %v\n", err)
			return 1
		}
	}

	if modified && !parsed.options.readonly && !isMemory {
		if err := assertDatabaseSize(int64(len(serialized))); err != nil {
			fmt.Fprintf(c.Stderr, "sqlite3: failed to write database: %v\n", err)
			return 1
		}
		if err := gfs.MkdirAll(c.FS, path.Dir(dbPath), 0o755); err != nil && !errors.Is(err, gfs.ErrReadOnly) {
			fmt.Fprintf(c.Stderr, "sqlite3: failed to write database: %v\n", err)
			return 1
		}
		if err := gfs.WriteFile(c.FS, dbPath, serialized, 0o644); err != nil {
			fmt.Fprintf(c.Stderr, "sqlite3: failed to write database: %v\n", err)
			return 1
		}
	}
	fmt.Fprint(c.Stdout, stdout.String())
	if bailError != nil {
		fmt.Fprintf(c.Stderr, "Error: %s\n", sanitizeError(bailError))
		return 1
	}
	_ = hadError // non-bail sqlite3 keeps exit status zero for statement errors, matching just-bash.
	return 0
}

func parseArgs(args []string) (parsedArgs, error) {
	p := parsedArgs{options: options{mode: modeList, separator: "|", newline: "\n"}}
	end := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if end {
			if p.database == "" {
				p.database = arg
			} else if p.sql == "" {
				p.sql = arg
			}
			continue
		}
		switch arg {
		case "--":
			end = true
		case "-version":
			p.showVersion = true
		case "-list":
			p.options.mode = modeList
		case "-csv":
			p.options.mode = modeCSV
		case "-json":
			p.options.mode = modeJSON
		case "-line":
			p.options.mode = modeLine
		case "-column":
			p.options.mode = modeColumn
		case "-table":
			p.options.mode = modeTable
		case "-markdown":
			p.options.mode = modeMarkdown
		case "-tabs":
			p.options.mode = modeTabs
		case "-box":
			p.options.mode = modeBox
		case "-quote":
			p.options.mode = modeQuote
		case "-html":
			p.options.mode = modeHTML
		case "-ascii":
			p.options.mode = modeASCII
		case "-header":
			p.options.header = true
		case "-noheader":
			p.options.header = false
		case "-readonly":
			p.options.readonly = true
		case "-bail":
			p.options.bail = true
		case "-echo":
			p.options.echo = true
		case "-separator", "-newline", "-nullvalue", "-cmd":
			if i+1 >= len(args) {
				return p, fmt.Errorf("sqlite3: Error: missing argument to %s\n", arg)
			}
			i++
			switch arg {
			case "-separator":
				p.options.separator = args[i]
			case "-newline":
				p.options.newline = args[i]
			case "-nullvalue":
				p.options.nullValue = args[i]
			case "-cmd":
				p.options.cmd = append(p.options.cmd, args[i])
			}
		default:
			if strings.HasPrefix(arg, "-") {
				opt := arg
				if strings.HasPrefix(arg, "--") {
					opt = arg[1:]
				}
				return p, fmt.Errorf("sqlite3: Error: unknown option: %s\nUse -help for a list of options.\n", opt)
			}
			if p.database == "" {
				p.database = arg
			} else if p.sql == "" {
				p.sql = arg
			}
		}
	}
	return p, nil
}

func execute(parent context.Context, dbBuffer []byte, sqlText string, opts options, persistent bool) ([]statementResult, bool, []byte, error) {
	ctx, cancel := context.WithTimeout(parent, queryTimeout)
	defer cancel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, false, nil, err
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, false, nil, err
	}

	if err := setupLimits(conn); err != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, false, nil, err
	}
	if persistent && len(dbBuffer) > 0 {
		if err := conn.Raw(func(driverConn any) error { return driverConn.(deserializer).Deserialize(dbBuffer) }); err != nil {
			_ = conn.Close()
			_ = db.Close()
			return nil, false, nil, err
		}
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA trusted_schema=OFF; PRAGMA foreign_keys=ON; PRAGMA temp_store=MEMORY; PRAGMA query_only=OFF", nil); err != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, false, nil, err
	}

	var results []statementResult
	modified := false
	remaining := sqlText
	for strings.TrimSpace(remaining) != "" {
		stmtText, tail, ok := nextStatement(remaining)
		if !ok {
			results = append(results, statementResult{err: errors.New("incomplete input")})
			break
		}
		remaining = tail
		if strings.TrimSpace(stmtText) == "" {
			continue
		}
		if isForbidden(stmtText) {
			results = append(results, statementResult{err: errors.New("disabled sqlite3 meta-command or host filesystem access")})
			if opts.bail {
				break
			}
			continue
		}
		cols, rows, err := queryStatement(ctx, conn, stmtText)
		if err != nil {
			results = append(results, statementResult{err: err})
			if opts.bail {
				break
			}
			continue
		}
		results = append(results, statementResult{columns: cols, rows: rows})
		if !isReadOnly(stmtText) {
			modified = true
		}
	}
	if ctx.Err() != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, false, nil, fmt.Errorf("Query timed out after %s", queryTimeout)
	}
	var serialized []byte
	if modified && persistent {
		if err := conn.Raw(func(driverConn any) error {
			var err error
			serialized, err = driverConn.(serializer).Serialize()
			return err
		}); err != nil {
			_ = conn.Close()
			_ = db.Close()
			return nil, false, nil, err
		}
	}
	closeErr := conn.Close()
	if dbErr := db.Close(); closeErr == nil {
		closeErr = dbErr
	}
	if closeErr != nil {
		return nil, false, nil, closeErr
	}
	return results, modified, serialized, nil
}

type (
	serializer   interface{ Serialize() ([]byte, error) }
	deserializer interface{ Deserialize([]byte) error }
)

func setupLimits(conn *sql.Conn) error {
	limits := [][2]int{
		{sqlite3.SQLITE_LIMIT_LENGTH, maxResultBytes},
		{sqlite3.SQLITE_LIMIT_SQL_LENGTH, maxSQLBytes},
		{sqlite3.SQLITE_LIMIT_COLUMN, 2000},
		{sqlite3.SQLITE_LIMIT_EXPR_DEPTH, 1000},
		{sqlite3.SQLITE_LIMIT_COMPOUND_SELECT, 500},
		{sqlite3.SQLITE_LIMIT_VDBE_OP, 250000},
		{sqlite3.SQLITE_LIMIT_ATTACHED, 0},
		{sqlite3.SQLITE_LIMIT_LIKE_PATTERN_LENGTH, 50000},
		{sqlite3.SQLITE_LIMIT_VARIABLE_NUMBER, 32766},
		{sqlite3.SQLITE_LIMIT_TRIGGER_DEPTH, 1000},
		{sqlite3.SQLITE_LIMIT_WORKER_THREADS, 0},
	}
	for _, l := range limits {
		if _, err := modernsqlite.Limit(conn, l[0], l[1]); err != nil {
			return err
		}
	}
	return nil
}

func queryStatement(ctx context.Context, conn *sql.Conn, sqlText string) ([]string, [][]any, error) {
	rows, err := conn.QueryContext(ctx, sqlText, nil)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out [][]any
	bytesUsed := 0
	for rows.Next() {
		if len(out) >= maxResultRows {
			return nil, nil, fmt.Errorf("query result exceeds %d row limit", maxResultRows)
		}
		vals := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range dest {
			dest[i] = &vals[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, nil, err
		}
		for i, v := range vals {
			vals[i] = normalizeValue(v)
			bytesUsed += valueByteLength(vals[i]) + 8
		}
		if bytesUsed > maxResultBytes {
			return nil, nil, fmt.Errorf("query result exceeds %d byte limit", maxResultBytes)
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return cols, out, nil
}

func normalizeValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return append([]byte(nil), x...)
	case int64:
		return x
	case float64:
		return x
	case string, nil:
		return x
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	default:
		return fmt.Sprint(x)
	}
}

func valueByteLength(v any) int {
	if v == nil {
		return 4
	}
	switch x := v.(type) {
	case []byte:
		return len(x)
	case string:
		return len(x)
	default:
		return len(fmt.Sprint(x))
	}
}

func nextStatement(s string) (string, string, bool) {
	quote := rune(0)
	lineComment, blockComment := false, false
	for i, r := range s {
		if lineComment {
			if r == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if r == '/' && i > 0 && s[i-1] == '*' {
				blockComment = false
			}
			continue
		}
		if quote != 0 {
			if r == quote || (quote == ']' && r == ']') {
				_, size := utf8.DecodeRuneInString(s[i+len(string(r)):])
				_ = size
				if quote != ']' && i+1 < len(s) && rune(s[i+1]) == quote {
					continue
				}
				quote = 0
			}
			continue
		}
		if r == '-' && i+1 < len(s) && s[i+1] == '-' {
			lineComment = true
			continue
		}
		if r == '/' && i+1 < len(s) && s[i+1] == '*' {
			blockComment = true
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			quote = r
			continue
		}
		if r == '[' {
			quote = ']'
			continue
		}
		if r == ';' {
			return s[:i+1], s[i+1:], true
		}
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", "", true
	}
	return s, "", true
}

func isReadOnly(sqlText string) bool {
	s := strings.ToUpper(stripLeadingNoise(sqlText))
	return strings.HasPrefix(s, "SELECT") || strings.HasPrefix(s, "EXPLAIN") || strings.HasPrefix(s, "VALUES")
}

func isForbidden(sqlText string) bool {
	s := strings.TrimSpace(strings.ToUpper(sqlText))
	return strings.HasPrefix(s, ".") || strings.Contains(s, "LOAD_EXTENSION") || strings.HasPrefix(s, "ATTACH") || strings.Contains(s, "ATTACH DATABASE") || strings.Contains(s, "VACUUM INTO")
}

func stripLeadingNoise(s string) string {
	for {
		before := s
		s = strings.TrimLeft(s, " \t\r\n")
		if strings.HasPrefix(s, "--") {
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = s[i+1:]
			} else {
				s = ""
			}
		} else if strings.HasPrefix(s, "/*") {
			if i := strings.Index(s, "*/"); i >= 0 {
				s = s[i+2:]
			} else {
				s = ""
			}
		}
		if s == before {
			return s
		}
	}
}

func acquireLock(ctx context.Context, fsys iofs.FS, p string) (func(), error) {
	v, _ := locks.LoadOrStore(fsys, &lockSet{locks: map[string]*databaseLock{}})
	set := v.(*lockSet)
	p = path.Clean(p)
	wait := make(chan struct{})
	set.mu.Lock()
	l := set.locks[p]
	if l == nil {
		l = &databaseLock{}
		set.locks[p] = l
	}
	if !l.held {
		l.held = true
		set.mu.Unlock()
		return func() { releaseLock(set, p) }, nil
	}
	l.waiters = append(l.waiters, wait)
	set.mu.Unlock()
	select {
	case <-wait:
		return func() { releaseLock(set, p) }, nil
	case <-ctx.Done():
		set.mu.Lock()
		if l := set.locks[p]; l != nil {
			for i, ch := range l.waiters {
				if ch == wait {
					l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
					break
				}
			}
		}
		set.mu.Unlock()
		return nil, ctx.Err()
	}
}

func releaseLock(set *lockSet, p string) {
	set.mu.Lock()
	defer set.mu.Unlock()
	l := set.locks[p]
	if l == nil {
		return
	}
	if len(l.waiters) > 0 {
		next := l.waiters[0]
		l.waiters = l.waiters[1:]
		close(next)
		return
	}
	delete(set.locks, p)
}

func assertDatabaseSize(size int64) error {
	if size < 0 || size > maxDatabaseBytes {
		return fmt.Errorf("database exceeds %d byte limit", maxDatabaseBytes)
	}
	return nil
}

func modernsqliteVersion() string {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return "unknown"
	}
	defer db.Close()
	var v string
	if err := db.QueryRow("SELECT sqlite_version()").Scan(&v); err != nil {
		return "unknown"
	}
	return v
}

func resolve(base, p string) string {
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	return path.Clean(path.Join(base, p))
}

func sanitizeError(err error) string {
	msg := err.Error()
	msg = strings.ReplaceAll(msg, os.TempDir(), "[host-temp]")
	return msg
}

func formatOutput(columns []string, rows [][]any, opts options, budget int) (string, error) {
	if budget < 0 {
		budget = 0
	}
	switch opts.mode {
	case modeList:
		return formatSeparated(columns, rows, opts, opts.separator, opts.newline, false, budget)
	case modeTabs:
		return formatSeparated(columns, rows, opts, "\t", opts.newline, false, budget)
	case modeASCII:
		return formatSeparated(columns, rows, opts, string(rune(0x1f)), string(rune(0x1e)), false, budget)
	case modeCSV:
		return formatCSV(columns, rows, opts, budget)
	case modeJSON:
		return formatJSON(columns, rows, budget)
	case modeLine:
		return formatLine(columns, rows, opts, budget)
	case modeColumn:
		return formatColumn(columns, rows, opts, budget)
	case modeTable:
		return formatTable(columns, rows, opts, "+", "+", "+", "+", "-", "|", budget)
	case modeMarkdown:
		return formatMarkdown(columns, rows, opts, budget)
	case modeBox:
		return formatBox(columns, rows, opts, budget)
	case modeQuote:
		return formatQuote(columns, rows, opts, budget)
	case modeHTML:
		return formatHTML(columns, rows, opts, budget)
	default:
		return "", fmt.Errorf("unknown output mode %q", opts.mode)
	}
}

func formatSeparated(columns []string, rows [][]any, opts options, sep, nl string, _ bool, budget int) (string, error) {
	var lines []string
	if opts.header && len(columns) > 0 {
		lines = append(lines, strings.Join(columns, sep))
	}
	for _, row := range rows {
		parts := make([]string, len(row))
		for i, v := range row {
			parts[i] = valueToString(v, opts.nullValue)
		}
		lines = append(lines, strings.Join(parts, sep))
	}
	out := ""
	if len(lines) > 0 {
		out = strings.Join(lines, nl) + nl
	}
	return checkBudget(out, budget)
}

func formatCSV(columns []string, rows [][]any, opts options, budget int) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	w.UseCRLF = false
	if opts.header && len(columns) > 0 {
		_ = w.Write(columns)
	}
	for _, row := range rows {
		rec := make([]string, len(row))
		for i, v := range row {
			rec[i] = valueToString(v, opts.nullValue)
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return checkBudget(b.String(), budget)
}

func formatJSON(columns []string, rows [][]any, budget int) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}
	objects := make([]map[string]any, len(rows))
	for i, row := range rows {
		m := map[string]any{}
		for j, col := range columns {
			m[col] = jsonValue(row[j])
		}
		objects[i] = m
	}
	data, err := json.Marshal(objects)
	if err != nil {
		return "", err
	}
	out := strings.ReplaceAll(string(data), "},{", "},\n{") + "\n"
	return checkBudget(out, budget)
}

func jsonValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

func formatLine(columns []string, rows [][]any, opts options, budget int) (string, error) {
	if len(columns) == 0 || len(rows) == 0 {
		return "", nil
	}
	maxLen := 5
	for _, c := range columns {
		if len(c) > maxLen {
			maxLen = len(c)
		}
	}
	var lines []string
	for _, row := range rows {
		for i, c := range columns {
			lines = append(lines, fmt.Sprintf("%*s = %s", maxLen, c, valueToString(row[i], opts.nullValue)))
		}
	}
	return checkBudget(strings.Join(lines, "\n")+"\n", budget)
}

func widths(columns []string, rows [][]any, opts options) []int {
	w := make([]int, len(columns))
	for i, c := range columns {
		w[i] = utf8.RuneCountInString(c)
	}
	for _, row := range rows {
		for i, v := range row {
			if l := utf8.RuneCountInString(valueToString(v, opts.nullValue)); l > w[i] {
				w[i] = l
			}
		}
	}
	return w
}

func padRight(s string, n int) string {
	if d := n - utf8.RuneCountInString(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func formatColumn(columns []string, rows [][]any, opts options, budget int) (string, error) {
	if len(columns) == 0 {
		return "", nil
	}
	w := widths(columns, rows, opts)
	var lines []string
	if opts.header {
		cells := make([]string, len(columns))
		sep := make([]string, len(columns))
		for i, c := range columns {
			cells[i] = padRight(c, w[i])
			sep[i] = strings.Repeat("-", w[i])
		}
		lines = append(lines, strings.Join(cells, "  "), strings.Join(sep, "  "))
	}
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = padRight(valueToString(v, opts.nullValue), w[i])
		}
		lines = append(lines, strings.Join(cells, "  "))
	}
	return checkBudget(joinLines(lines), budget)
}

func formatTable(columns []string, rows [][]any, opts options, tl, tm, tr, cross, h, v string, budget int) (string, error) {
	if len(columns) == 0 {
		return "", nil
	}
	w := widths(columns, rows, opts)
	border := tl
	for i, width := range w {
		if i > 0 {
			border += tm
		}
		border += strings.Repeat(h, width+2)
	}
	border += tr
	var lines []string
	lines = append(lines, border)
	if opts.header {
		lines = append(lines, rowLine(columns, w, v))
		lines = append(lines, strings.ReplaceAll(strings.ReplaceAll(border, tl, cross), tr, cross))
	}
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, value := range row {
			cells[i] = valueToString(value, opts.nullValue)
		}
		lines = append(lines, rowLine(cells, w, v))
	}
	lines = append(lines, border)
	return checkBudget(joinLines(lines), budget)
}

func rowLine(cells []string, w []int, v string) string {
	padded := make([]string, len(cells))
	for i, c := range cells {
		padded[i] = " " + padRight(c, w[i]) + " "
	}
	return v + strings.Join(padded, v) + v
}

func formatMarkdown(columns []string, rows [][]any, opts options, budget int) (string, error) {
	if len(columns) == 0 {
		return "", nil
	}
	var lines []string
	if opts.header {
		lines = append(lines, "| "+strings.Join(columns, " | ")+" |", "|"+strings.Repeat("---|", len(columns)))
	}
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = valueToString(v, opts.nullValue)
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return checkBudget(joinLines(lines), budget)
}

func formatBox(columns []string, rows [][]any, opts options, budget int) (string, error) {
	if len(columns) == 0 {
		return "", nil
	}
	w := widths(columns, rows, opts)
	makeBorder := func(left, mid, right string) string {
		parts := make([]string, len(w))
		for i, width := range w {
			parts[i] = strings.Repeat("─", width+2)
		}
		return left + strings.Join(parts, mid) + right
	}
	var lines []string
	lines = append(lines, makeBorder("┌", "┬", "┐"), rowLine(columns, w, "│"), makeBorder("├", "┼", "┤"))
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = valueToString(v, opts.nullValue)
		}
		lines = append(lines, rowLine(cells, w, "│"))
	}
	lines = append(lines, makeBorder("└", "┴", "┘"))
	return checkBudget(joinLines(lines), budget)
}

func formatQuote(columns []string, rows [][]any, opts options, budget int) (string, error) {
	var lines []string
	if opts.header && len(columns) > 0 {
		quoted := make([]string, len(columns))
		for i, c := range columns {
			quoted[i] = sqlQuote(c)
		}
		lines = append(lines, strings.Join(quoted, ","))
	}
	for _, row := range rows {
		quoted := make([]string, len(row))
		for i, v := range row {
			quoted[i] = sqlQuote(v)
		}
		lines = append(lines, strings.Join(quoted, ","))
	}
	return checkBudget(joinWithNewline(lines, opts.newline), budget)
}

func formatHTML(columns []string, rows [][]any, opts options, budget int) (string, error) {
	var lines []string
	if opts.header && len(columns) > 0 {
		ths := make([]string, len(columns))
		for i, c := range columns {
			ths[i] = "<TH>" + html.EscapeString(c) + "</TH>"
		}
		lines = append(lines, "<TR>"+strings.Join(ths, ""), "</TR>")
	}
	for _, row := range rows {
		tds := make([]string, len(row))
		for i, v := range row {
			tds[i] = "<TD>" + html.EscapeString(valueToString(v, opts.nullValue)) + "</TD>"
		}
		lines = append(lines, "<TR>"+strings.Join(tds, ""), "</TR>")
	}
	return checkBudget(joinLines(lines), budget)
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func joinWithNewline(lines []string, nl string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, nl) + nl
}

func checkBudget(out string, budget int) (string, error) {
	if len(out) > budget {
		return "", fmt.Errorf("formatted output exceeds %d byte limit", budget)
	}
	return out, nil
}

func valueToString(v any, nullValue string) string {
	if v == nil {
		return nullValue
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Sprint(x)
		}
		return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(x, 'g', 17, 64), "0"), ".")
	default:
		return fmt.Sprint(x)
	}
}

func sqlQuote(v any) string {
	if v == nil {
		return "NULL"
	}
	switch x := v.(type) {
	case []byte:
		return "X'" + strings.ToUpper(hex.EncodeToString(x)) + "'"
	case int64, int, int32:
		return fmt.Sprint(x)
	case float64:
		return valueToString(x, "")
	default:
		return "'" + strings.ReplaceAll(fmt.Sprint(x), "'", "''") + "'"
	}
}

type budgetBuilder struct {
	strings.Builder
	limit int
}

func (b *budgetBuilder) writeString(s string) error {
	if b.Len()+len(s) > b.limit {
		return fmt.Errorf("formatted output exceeds %d byte limit", b.limit)
	}
	_, _ = b.Builder.WriteString(s)
	return nil
}

func (b *budgetBuilder) remaining() int { return b.limit - b.Len() }
