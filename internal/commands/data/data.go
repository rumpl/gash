package data

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
	nethtml "golang.org/x/net/html"
	"gopkg.in/yaml.v3"
)

const (
	maxStructuredInput  = 32 << 20
	maxStructuredOutput = 64 << 20
	maxStructuredRows   = 200000
)

type CommandContext = command.Context

func Commands() []command.Command {
	return []command.Command{
		{Name: "jq", Run: commandJQ},
		{Name: "yq", Run: commandYQ},
		{Name: "xan", Run: commandXan},
		{Name: "html-to-markdown", Run: commandHTMLToMarkdown},
	}
}

func abs(ctx *CommandContext, name string) string {
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean(path.Join(*ctx.Cwd, name))
}

func readAllLimited(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, io.LimitReader(r, maxStructuredInput+1))
	if err != nil {
		return nil, err
	}
	if buf.Len() > maxStructuredInput {
		return nil, fmt.Errorf("input exceeds %d bytes", maxStructuredInput)
	}
	return buf.Bytes(), nil
}

func readNamed(ctx *CommandContext, name string) ([]byte, error) {
	if name == "-" {
		return readAllLimited(ctx.Stdin)
	}
	data, err := gfs.ReadFile(ctx.FS, abs(ctx, name))
	if err != nil {
		return nil, err
	}
	if len(data) > maxStructuredInput {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, maxStructuredInput)
	}
	return data, nil
}

func writeBounded(ctx *CommandContext, command string, data []byte) int {
	if len(data) > maxStructuredOutput {
		fmt.Fprintf(ctx.Stderr, "%s: output exceeds %d bytes\n", command, maxStructuredOutput)
		return 1
	}
	_, _ = ctx.Stdout.Write(data)
	return 0
}

func commandJQ(ctx context.Context, args []string, c *CommandContext) int {
	opts, code := parseJQArgs(args, c)
	if code != 0 {
		return code
	}
	query, err := gojq.Parse(opts.filter)
	if err != nil {
		fmt.Fprintf(c.Stderr, "jq: %v\n", err)
		return 3
	}
	compilerOpts := []gojq.CompilerOption{gojq.WithEnvironLoader(func() []string { return envList(c.Env) })}
	varNames := make([]string, 0, len(opts.args))
	varVals := make([]any, 0, len(opts.args))
	for _, a := range opts.args {
		varNames = append(varNames, "$"+a.name)
		varVals = append(varVals, a.value)
	}
	if len(varNames) > 0 {
		compilerOpts = append(compilerOpts, gojq.WithVariables(varNames))
	}
	codeObj, err := gojq.Compile(query, compilerOpts...)
	if err != nil {
		fmt.Fprintf(c.Stderr, "jq: %v\n", err)
		return 3
	}
	inputs, err := jqInputs(opts, c)
	if err != nil {
		fmt.Fprintf(c.Stderr, "jq: %v\n", err)
		return 2
	}
	var out bytes.Buffer
	for _, in := range inputs {
		if ctx.Err() != nil {
			fmt.Fprintf(c.Stderr, "jq: %v\n", ctx.Err())
			return 124
		}
		iter := codeObj.RunWithContext(ctx, in, varVals...)
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				fmt.Fprintf(c.Stderr, "jq: %v\n", err)
				return 5
			}
			line, err := formatJQ(v, opts)
			if err != nil {
				fmt.Fprintf(c.Stderr, "jq: %v\n", err)
				return 5
			}
			out.Write(line)
			out.WriteByte('\n')
			if out.Len() > maxStructuredOutput {
				return writeBounded(c, "jq", out.Bytes())
			}
		}
	}
	return writeBounded(c, "jq", out.Bytes())
}

type jqOptions struct {
	filter     string
	files      []string
	rawInput   bool
	slurp      bool
	nullInput  bool
	rawOutput  bool
	compact    bool
	monochrome bool
	exitStatus bool
	args       []jqArg
}

type jqArg struct {
	name  string
	value any
}

func parseJQArgs(args []string, c *CommandContext) (jqOptions, int) {
	opts := jqOptions{}
	pos := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			return opts, showHelp(c, "jq")
		case a == "-r" || a == "--raw-output":
			opts.rawOutput = true
		case a == "-c" || a == "--compact-output":
			opts.compact = true
		case a == "-M" || a == "--monochrome-output":
			opts.monochrome = true
		case a == "-R" || a == "--raw-input":
			opts.rawInput = true
		case a == "-s" || a == "--slurp":
			opts.slurp = true
		case a == "-n" || a == "--null-input":
			opts.nullInput = true
		case a == "-e" || a == "--exit-status":
			opts.exitStatus = true
		case a == "--arg" || a == "--argjson":
			if i+2 >= len(args) {
				return opts, unknown(c, "jq", a)
			}
			name, val := args[i+1], any(args[i+2])
			if a == "--argjson" {
				if err := json.Unmarshal([]byte(args[i+2]), &val); err != nil {
					fmt.Fprintf(c.Stderr, "jq: invalid JSON for --argjson %s: %v\n", name, err)
					return opts, 2
				}
			}
			opts.args = append(opts.args, jqArg{name: name, value: normalizeJSONValue(val)})
			i += 2
		case strings.HasPrefix(a, "-") && a != "-":
			return opts, unknown(c, "jq", a)
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) == 0 {
		fmt.Fprint(c.Stderr, "jq: missing filter\n")
		return opts, 2
	}
	opts.filter = pos[0]
	opts.files = pos[1:]
	return opts, 0
}

func jqInputs(opts jqOptions, c *CommandContext) ([]any, error) {
	if opts.nullInput {
		return []any{nil}, nil
	}
	if opts.rawInput {
		data, err := readJQBytes(opts.files, c)
		if err != nil {
			return nil, err
		}
		text := strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		if opts.slurp {
			return []any{text}, nil
		}
		if text == "" {
			return nil, nil
		}
		lines := strings.Split(text, "\n")
		out := make([]any, len(lines))
		for i, line := range lines {
			out[i] = line
		}
		return out, nil
	}
	data, err := readJQBytes(opts.files, c)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var vals []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		vals = append(vals, normalizeJSONValue(v))
		if len(vals) > maxStructuredRows {
			return nil, fmt.Errorf("too many JSON values")
		}
	}
	if opts.slurp {
		return []any{vals}, nil
	}
	return vals, nil
}

func readJQBytes(files []string, c *CommandContext) ([]byte, error) {
	if len(files) == 0 {
		return readAllLimited(c.Stdin)
	}
	var buf bytes.Buffer
	for _, f := range files {
		data, err := readNamed(c, f)
		if err != nil {
			return nil, err
		}
		buf.Write(data)
		if len(files) > 1 && !bytes.HasSuffix(data, []byte("\n")) {
			buf.WriteByte('\n')
		}
		if buf.Len() > maxStructuredInput {
			return nil, fmt.Errorf("input exceeds %d bytes", maxStructuredInput)
		}
	}
	return buf.Bytes(), nil
}

func normalizeJSONValue(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := strconv.Atoi(x.String()); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(x.String(), 64); err == nil {
			return f
		}
		return x.String()
	case []any:
		for i := range x {
			x[i] = normalizeJSONValue(x[i])
		}
		return x
	case map[string]any:
		for k, v := range x {
			x[k] = normalizeJSONValue(v)
		}
		return x
	default:
		return v
	}
}

func formatJQ(v any, opts jqOptions) ([]byte, error) {
	if opts.rawOutput {
		if s, ok := v.(string); ok {
			return []byte(s), nil
		}
	}
	if opts.compact {
		return gojq.Marshal(v)
	}
	return json.MarshalIndent(toStdJSON(v), "", "  ")
}

func toStdJSON(v any) any {
	switch x := v.(type) {
	case []any:
		for i := range x {
			x[i] = toStdJSON(x[i])
		}
		return x
	case map[string]any:
		for k, v := range x {
			x[k] = toStdJSON(v)
		}
		return x
	default:
		return x
	}
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func commandYQ(ctx context.Context, args []string, c *CommandContext) int {
	opts, code := parseYQArgs(args, c)
	if code != 0 {
		return code
	}
	jqOpts := jqOptions{filter: opts.filter, rawOutput: opts.rawOutput, compact: opts.outputJSON}
	for _, f := range opts.files {
		if ctx.Err() != nil {
			fmt.Fprintf(c.Stderr, "yq: %v\n", ctx.Err())
			return 124
		}
		data, err := readNamed(c, f)
		if err != nil {
			fmt.Fprintf(c.Stderr, "yq: %v\n", err)
			return 1
		}
		values, err := parseYAMLDocuments(data)
		if err != nil {
			fmt.Fprintf(c.Stderr, "yq: %v\n", err)
			return 1
		}
		if len(values) == 0 {
			values = []any{nil}
		}
		var out bytes.Buffer
		for _, val := range values {
			query, err := gojq.Parse(opts.filter)
			if err != nil {
				fmt.Fprintf(c.Stderr, "yq: %v\n", err)
				return 1
			}
			codeObj, err := gojq.Compile(query, gojq.WithEnvironLoader(func() []string { return envList(c.Env) }))
			if err != nil {
				fmt.Fprintf(c.Stderr, "yq: %v\n", err)
				return 1
			}
			iter := codeObj.RunWithContext(ctx, val)
			for {
				v, ok := iter.Next()
				if !ok {
					break
				}
				if err, ok := v.(error); ok {
					fmt.Fprintf(c.Stderr, "yq: %v\n", err)
					return 1
				}
				var b []byte
				if opts.outputJSON || opts.rawOutput {
					b, err = formatJQ(v, jqOpts)
				} else {
					b, err = yaml.Marshal(v)
				}
				if err != nil {
					fmt.Fprintf(c.Stderr, "yq: %v\n", err)
					return 1
				}
				out.Write(bytes.TrimRight(b, "\n"))
				out.WriteByte('\n')
			}
		}
		if code := writeBounded(c, "yq", out.Bytes()); code != 0 {
			return code
		}
	}
	return 0
}

type yqOptions struct {
	filter                string
	files                 []string
	outputJSON, rawOutput bool
}

func parseYQArgs(args []string, c *CommandContext) (yqOptions, int) {
	opts := yqOptions{filter: "."}
	pos := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help":
			return opts, showHelp(c, "yq")
		case "-o=json", "--output-format=json", "-j":
			opts.outputJSON = true
		case "-r", "--raw-output":
			opts.rawOutput = true
		case "-y", "-P", "--prettyPrint":
			opts.outputJSON = false
		default:
			if strings.HasPrefix(a, "-") && a != "-" {
				return opts, unknown(c, "yq", a)
			}
			pos = append(pos, a)
		}
	}
	if len(pos) > 0 {
		opts.filter = pos[0]
		opts.files = pos[1:]
	}
	if len(opts.files) == 0 {
		opts.files = []string{"-"}
	}
	return opts, 0
}

func parseYAMLDocuments(data []byte) ([]any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var out []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, normalizeYAML(v))
		if len(out) > maxStructuredRows {
			return nil, fmt.Errorf("too many YAML documents")
		}
	}
	return out, nil
}

func normalizeYAML(v any) any {
	switch x := v.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, v := range x {
			m[fmt.Sprint(k)] = normalizeYAML(v)
		}
		return m
	case map[string]any:
		for k, v := range x {
			x[k] = normalizeYAML(v)
		}
		return x
	case []any:
		for i := range x {
			x[i] = normalizeYAML(x[i])
		}
		return x
	default:
		return x
	}
}

func commandXan(ctx context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 || args[0] == "--help" {
		return showHelp(c, "xan")
	}
	sub := args[0]
	opts, code := parseXanArgs(args[1:], c)
	if code != 0 {
		return code
	}
	if ctx.Err() != nil {
		fmt.Fprintf(c.Stderr, "xan: %v\n", ctx.Err())
		return 124
	}
	recs, err := readCSVRecords(opts, c)
	if err != nil {
		fmt.Fprintf(c.Stderr, "xan: %v\n", err)
		return 1
	}
	out, err := runXanSub(sub, opts, recs)
	if err != nil {
		fmt.Fprintf(c.Stderr, "xan: %v\n", err)
		return 1
	}
	return writeBounded(c, "xan", out)
}

type xanOptions struct {
	delimiter                                          rune
	noHeaders                                          bool
	selectExpr, filterExpr, mapExpr, aggExpr, groupKey string
	reverse                                            bool
	numeric                                            bool
	files                                              []string
}
type csvTable struct {
	header []string
	rows   []map[string]string
	order  [][]string
}

func parseXanArgs(args []string, c *CommandContext) (xanOptions, int) {
	opts := xanOptions{delimiter: ','}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-d", "--delimiter":
			if i+1 >= len(args) {
				return opts, unknown(c, "xan", a)
			}
			r := []rune(args[i+1])
			if len(r) > 0 {
				opts.delimiter = r[0]
			}
			i++
		case "--no-headers", "-n":
			opts.noHeaders = true
		case "--select", "-s":
			if i+1 >= len(args) {
				return opts, unknown(c, "xan", a)
			}
			opts.selectExpr = args[i+1]
			i++
		case "--filter", "-f":
			if i+1 >= len(args) {
				return opts, unknown(c, "xan", a)
			}
			opts.filterExpr = args[i+1]
			i++
		case "--map":
			if i+1 >= len(args) {
				return opts, unknown(c, "xan", a)
			}
			opts.mapExpr = args[i+1]
			i++
		case "--agg", "--aggregate":
			if i+1 >= len(args) {
				return opts, unknown(c, "xan", a)
			}
			opts.aggExpr = args[i+1]
			i++
		case "--groupby", "--group-by":
			if i+1 >= len(args) {
				return opts, unknown(c, "xan", a)
			}
			opts.groupKey = args[i+1]
			i++
		case "--reverse", "-R":
			opts.reverse = true
		case "--numeric", "-N":
			opts.numeric = true
		default:
			if strings.HasPrefix(a, "-") && a != "-" {
				return opts, unknown(c, "xan", a)
			}
			opts.files = append(opts.files, a)
		}
	}
	if len(opts.files) == 0 {
		opts.files = []string{"-"}
	}
	return opts, 0
}

func readCSVRecords(opts xanOptions, c *CommandContext) (csvTable, error) {
	var all csvTable
	for _, f := range opts.files {
		data, err := readNamed(c, f)
		if err != nil {
			return all, err
		}
		r := csv.NewReader(bytes.NewReader(data))
		r.Comma = opts.delimiter
		r.FieldsPerRecord = -1
		r.ReuseRecord = false
		records, err := r.ReadAll()
		if err != nil {
			return all, err
		}
		if len(records) == 0 {
			continue
		}
		start := 0
		header := records[0]
		if opts.noHeaders {
			header = make([]string, len(records[0]))
			for i := range header {
				header[i] = strconv.Itoa(i + 1)
			}
		} else {
			start = 1
		}
		if all.header == nil {
			all.header = append([]string{}, header...)
		}
		for _, rec := range records[start:] {
			if len(all.rows) >= maxStructuredRows {
				return all, fmt.Errorf("too many CSV rows")
			}
			row := map[string]string{}
			for i, h := range header {
				if i < len(rec) {
					row[h] = rec[i]
				} else {
					row[h] = ""
				}
			}
			all.rows = append(all.rows, row)
			all.order = append(all.order, rec)
		}
	}
	return all, nil
}

func runXanSub(sub string, opts xanOptions, t csvTable) ([]byte, error) {
	switch sub {
	case "headers", "columns":
		return []byte(strings.Join(t.header, "\n") + "\n"), nil
	case "select":
		cols := splitCSVList(firstNonempty(opts.selectExpr, opts.aggExpr))
		if len(cols) == 0 {
			return nil, fmt.Errorf("select requires --select columns")
		}
		return csvOutput(cols, filterRows(t.rows, opts.filterExpr), opts.delimiter)
	case "filter", "search":
		return csvOutput(t.header, filterRows(t.rows, firstNonempty(opts.filterExpr, opts.selectExpr)), opts.delimiter)
	case "sort":
		key := firstNonempty(opts.selectExpr, opts.aggExpr)
		if key == "" {
			key = t.header[0]
		}
		rows := append([]map[string]string{}, t.rows...)
		sort.SliceStable(rows, func(i, j int) bool {
			cmp := compareCSV(rows[i][key], rows[j][key], opts.numeric)
			if opts.reverse {
				return cmp > 0
			}
			return cmp < 0
		})
		return csvOutput(t.header, rows, opts.delimiter)
	case "map":
		name, expr, ok := strings.Cut(opts.mapExpr, "=")
		if !ok {
			return nil, fmt.Errorf("map requires --map name=expression")
		}
		header := append(append([]string{}, t.header...), strings.TrimSpace(name))
		rows := filterRows(t.rows, opts.filterExpr)
		for _, r := range rows {
			r[strings.TrimSpace(name)] = evalCSVExpr(expr, r)
		}
		return csvOutput(header, rows, opts.delimiter)
	case "agg", "aggregate":
		return xanAgg(t, opts)
	case "groupby", "group-by":
		return xanGroupBy(t, opts)
	case "view", "flatten":
		return xanView(t), nil
	default:
		return nil, fmt.Errorf("unsupported xan subcommand %q (supported: headers, select, filter, sort, map, agg, groupby, view)", sub)
	}
}

func csvOutput(header []string, rows []map[string]string, delim rune) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Comma = delim
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		rec := make([]string, len(header))
		for i, h := range header {
			rec[i] = row[h]
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func filterRows(rows []map[string]string, expr string) []map[string]string {
	if strings.TrimSpace(expr) == "" {
		return append([]map[string]string{}, rows...)
	}
	out := []map[string]string{}
	for _, r := range rows {
		if evalCSVBool(expr, r) {
			cp := map[string]string{}
			for k, v := range r {
				cp[k] = v
			}
			out = append(out, cp)
		}
	}
	return out
}

func evalCSVBool(expr string, row map[string]string) bool {
	expr = strings.TrimSpace(expr)
	ops := []string{"==", "!=", ">=", "<=", "~", ">", "<"}
	for _, op := range ops {
		if i := strings.Index(expr, op); i >= 0 {
			left := csvValue(strings.TrimSpace(expr[:i]), row)
			right := strings.Trim(strings.TrimSpace(expr[i+len(op):]), "'\"")
			switch op {
			case "==":
				return left == right
			case "!=":
				return left != right
			case "~":
				return strings.Contains(left, right)
			case ">":
				return compareCSV(left, right, true) > 0
			case "<":
				return compareCSV(left, right, true) < 0
			case ">=":
				return compareCSV(left, right, true) >= 0
			case "<=":
				return compareCSV(left, right, true) <= 0
			}
		}
	}
	return csvValue(expr, row) != ""
}

func evalCSVExpr(expr string, row map[string]string) string {
	parts := strings.Split(expr, "+")
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(csvValue(strings.TrimSpace(p), row))
	}
	return b.String()
}

func csvValue(token string, row map[string]string) string {
	token = strings.Trim(token, "'\"")
	if v, ok := row[token]; ok {
		return v
	}
	return token
}

func splitCSVList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonempty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func compareCSV(a, b string, numeric bool) int {
	if numeric {
		af, _ := strconv.ParseFloat(a, 64)
		bf, _ := strconv.ParseFloat(b, 64)
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	return strings.Compare(a, b)
}

func xanAgg(t csvTable, opts xanOptions) ([]byte, error) {
	expr := opts.aggExpr
	if expr == "" {
		expr = "count"
	}
	name := expr
	val := ""
	rows := filterRows(t.rows, opts.filterExpr)
	if strings.HasPrefix(expr, "count") {
		val = strconv.Itoa(len(rows))
	} else if strings.HasPrefix(expr, "sum(") {
		col := strings.TrimSuffix(strings.TrimPrefix(expr, "sum("), ")")
		sum := 0.0
		for _, r := range rows {
			f, _ := strconv.ParseFloat(r[col], 64)
			sum += f
		}
		name = "sum"
		val = strconv.FormatFloat(sum, 'f', -1, 64)
	} else {
		return nil, fmt.Errorf("unsupported aggregate %q", expr)
	}
	return csvOutput([]string{name}, []map[string]string{{name: val}}, opts.delimiter)
}

func xanGroupBy(t csvTable, opts xanOptions) ([]byte, error) {
	key := opts.groupKey
	if key == "" {
		key = opts.selectExpr
	}
	if key == "" {
		return nil, fmt.Errorf("groupby requires --groupby column")
	}
	counts := map[string]int{}
	keys := []string{}
	for _, r := range filterRows(t.rows, opts.filterExpr) {
		v := r[key]
		if _, ok := counts[v]; !ok {
			keys = append(keys, v)
		}
		counts[v]++
	}
	sort.Strings(keys)
	rows := []map[string]string{}
	for _, k := range keys {
		rows = append(rows, map[string]string{key: k, "count": strconv.Itoa(counts[k])})
	}
	return csvOutput([]string{key, "count"}, rows, opts.delimiter)
}

func xanView(t csvTable) []byte {
	widths := make([]int, len(t.header))
	for i, h := range t.header {
		widths[i] = len(h)
	}
	for _, r := range t.rows {
		for i, h := range t.header {
			if len(r[h]) > widths[i] {
				widths[i] = len(r[h])
			}
		}
	}
	var b strings.Builder
	for i, h := range t.header {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(pad(h, widths[i]))
	}
	b.WriteByte('\n')
	for _, r := range t.rows {
		for i, h := range t.header {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(pad(r[h], widths[i]))
		}
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func commandHTMLToMarkdown(_ context.Context, args []string, c *CommandContext) int {
	if len(args) > 0 && args[0] == "--help" {
		return showHelp(c, "html-to-markdown")
	}
	data, err := readHTMLInput(args, c)
	if err != nil {
		fmt.Fprintf(c.Stderr, "html-to-markdown: %v\n", err)
		return 1
	}
	node, err := nethtml.Parse(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(c.Stderr, "html-to-markdown: %v\n", err)
		return 1
	}
	md := strings.TrimSpace(renderMarkdown(node)) + "\n"
	return writeBounded(c, "html-to-markdown", []byte(md))
}

func readHTMLInput(args []string, c *CommandContext) ([]byte, error) {
	if len(args) == 0 {
		return readAllLimited(c.Stdin)
	}
	var b bytes.Buffer
	for _, f := range args {
		data, err := readNamed(c, f)
		if err != nil {
			return nil, err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.Bytes(), nil
}

func renderMarkdown(n *nethtml.Node) string {
	if n.Type == nethtml.TextNode {
		return collapseText(n.Data)
	}
	if n.Type != nethtml.ElementNode && n.Type != nethtml.DocumentNode {
		return ""
	}
	var child strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		child.WriteString(renderMarkdown(c))
	}
	content := child.String()
	if n.Type == nethtml.DocumentNode {
		return content
	}
	switch strings.ToLower(n.Data) {
	case "script", "style", "noscript":
		return ""
	case "p":
		return "\n\n" + strings.TrimSpace(content) + "\n\n"
	case "br":
		return "  \n"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		return "\n\n" + strings.Repeat("#", level) + " " + strings.TrimSpace(content) + "\n\n"
	case "strong", "b":
		return "**" + strings.TrimSpace(content) + "**"
	case "em", "i":
		return "_" + strings.TrimSpace(content) + "_"
	case "code":
		return "`" + strings.TrimSpace(content) + "`"
	case "pre":
		return "\n\n```\n" + strings.TrimSpace(textContent(n)) + "\n```\n\n"
	case "a":
		href := attr(n, "href")
		if href == "" || unsafeURL(href) {
			return content
		}
		return "[" + strings.TrimSpace(content) + "](" + href + ")"
	case "img":
		src := attr(n, "src")
		if unsafeURL(src) {
			src = ""
		}
		alt := attr(n, "alt")
		return "![" + alt + "](" + src + ")"
	case "ul":
		return "\n" + renderList(n, false) + "\n"
	case "ol":
		return "\n" + renderList(n, true) + "\n"
	case "blockquote":
		return "\n" + prefixLines(strings.TrimSpace(content), "> ") + "\n"
	case "li":
		return strings.TrimSpace(content)
	}
	return content
}

func renderList(n *nethtml.Node, ordered bool) string {
	var b strings.Builder
	i := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == nethtml.ElementNode && strings.EqualFold(c.Data, "li") {
			marker := "- "
			if ordered {
				marker = strconv.Itoa(i) + ". "
				i++
			}
			b.WriteString(marker + strings.TrimSpace(renderMarkdown(c)) + "\n")
		}
	}
	return b.String()
}

func textContent(n *nethtml.Node) string {
	if n.Type == nethtml.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textContent(c))
	}
	return b.String()
}

func attr(n *nethtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return stdhtml.UnescapeString(a.Val)
		}
	}
	return ""
}

func unsafeURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return true
	}
	return strings.EqualFold(u.Scheme, "javascript") || strings.EqualFold(u.Scheme, "data")
}

func prefixLines(s, p string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = p + strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "\n")
}

func collapseText(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
}

func showHelp(c *CommandContext, name string) int {
	if info, ok := commandhelp.Lookup(name); ok {
		return commandhelp.Show(c, info)
	}
	return 0
}

func unknown(c *CommandContext, name, option string) int {
	return commandhelp.UnknownOption(c, name, option)
}
