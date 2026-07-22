package text

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

// Deferred just-bash awk differences: this implementation intentionally keeps to
// a deterministic, in-process AWK subset suitable for gash. It supports BEGIN/END,
// pattern-action programs, records/fields, scalar variables, associative arrays,
// user-defined functions, common expressions/operators, print/printf, getline,
// next/nextfile, and common built-ins. Advanced POSIX/GNU awk features such as
// multi-dimensional SUBSEP arrays, locale-specific formatting, pipes/co-processes,
// system(), close(), fflush(), dynamic extension loading, and full awk regexp
// replacement back-reference semantics are rejected or omitted rather than using
// host process/filesystem access.

const (
	awkMaxLoopIterations = 100000
	awkMaxCallDepth      = 100
)

func commandAwk(ctx context.Context, args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		if info, ok := commandhelp.Lookup("awk"); ok {
			return commandhelp.Show(c, info)
		}
	}
	opts, code := parseAwkArgs(args, c)
	if code != 0 {
		return code
	}
	if strings.TrimSpace(opts.program) == "" {
		fmt.Fprint(c.Stderr, "awk: missing program\n")
		return 2
	}
	lx := newAwkLexer(opts.program)
	toks, err := lx.tokens()
	if err != nil {
		fmt.Fprintf(c.Stderr, "awk: %v\n", err)
		return 2
	}
	prog, err := newAwkParser(toks).parseProgram()
	if err != nil {
		fmt.Fprintf(c.Stderr, "awk: %v\n", err)
		return 2
	}
	vm := newAwkVM(ctx, c)
	for k, v := range opts.vars {
		vm.setVar(k, awkValue{str: v, isStr: true})
	}
	if opts.fieldSep != nil {
		vm.setVar("FS", awkValue{str: *opts.fieldSep, isStr: true})
	}
	if err := vm.run(prog, opts.files); err != nil {
		fmt.Fprintf(c.Stderr, "awk: %v\n", err)
		return 1
	}
	if vm.runtimeErr != nil {
		fmt.Fprintf(c.Stderr, "awk: %v\n", vm.runtimeErr)
		return 1
	}
	return 0
}

type awkOptions struct {
	program  string
	files    []string
	vars     map[string]string
	fieldSep *string
}

func parseAwkArgs(args []string, c *CommandContext) (awkOptions, int) {
	opts := awkOptions{vars: map[string]string{}}
	var programParts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			if opts.program == "" && len(programParts) == 0 && i+1 < len(args) {
				i++
				opts.program = args[i]
			}
			opts.files = append(opts.files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			switch {
			case a == "-F":
				if i+1 >= len(args) {
					return opts, commandhelp.UnknownOption(c, "awk", a)
				}
				i++
				fs := args[i]
				opts.fieldSep = &fs
				continue
			case strings.HasPrefix(a, "-F") && len(a) > 2:
				fs := a[2:]
				opts.fieldSep = &fs
				continue
			case a == "-v":
				if i+1 >= len(args) {
					return opts, commandhelp.UnknownOption(c, "awk", a)
				}
				i++
				setAwkVarArg(opts.vars, args[i])
				continue
			case strings.HasPrefix(a, "-v") && len(a) > 2:
				setAwkVarArg(opts.vars, a[2:])
				continue
			case a == "-f":
				if i+1 >= len(args) {
					return opts, commandhelp.UnknownOption(c, "awk", a)
				}
				i++
				data, err := gfs.ReadFile(c.FS, abs(c, args[i]))
				if err != nil {
					fmt.Fprintf(c.Stderr, "awk: %s: No such file or directory\n", args[i])
					return opts, 1
				}
				programParts = append(programParts, string(data))
				continue
			default:
				return opts, commandhelp.UnknownOption(c, "awk", a)
			}
		}
		if opts.program == "" && len(programParts) == 0 {
			opts.program = a
		} else {
			opts.files = append(opts.files, a)
		}
	}
	if len(programParts) > 0 {
		if opts.program != "" {
			programParts = append(programParts, opts.program)
		}
		opts.program = strings.Join(programParts, "\n")
	}
	return opts, 0
}

func setAwkVarArg(vars map[string]string, s string) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) == 2 && parts[0] != "" {
		vars[parts[0]] = parts[1]
	}
}

type awkTokKind int

const (
	awkEOF awkTokKind = iota
	awkIdent
	awkNumber
	awkString
	awkRegex
	awkNewline
	awkOp
)

type awkToken struct {
	kind awkTokKind
	lit  string
	pos  int
}

type awkLexer struct {
	src      string
	pos      int
	canRegex bool
}

func newAwkLexer(s string) *awkLexer {
	return &awkLexer{src: s, canRegex: true}
}

func (l *awkLexer) tokens() ([]awkToken, error) {
	var out []awkToken
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		if t.kind == awkEOF {
			return out, nil
		}
	}
}

func (l *awkLexer) next() (awkToken, error) {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.pos++
			continue
		}
		if ch == '#' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
			continue
		}
		break
	}
	start := l.pos
	if start >= len(l.src) {
		return awkToken{kind: awkEOF, pos: start}, nil
	}
	ch := l.src[l.pos]
	if ch == '\n' {
		l.pos++
		l.canRegex = true
		return awkToken{kind: awkNewline, lit: "\n", pos: start}, nil
	}
	if ch == '"' {
		s, err := l.scanString()
		l.canRegex = false
		return awkToken{kind: awkString, lit: s, pos: start}, err
	}
	if ch == '/' && l.canRegex && !l.starts("//") {
		s, err := l.scanRegex()
		l.canRegex = false
		return awkToken{kind: awkRegex, lit: s, pos: start}, err
	}
	if isAwkIdentStart(rune(ch)) {
		for l.pos < len(l.src) {
			r, sz := utf8.DecodeRuneInString(l.src[l.pos:])
			if !isAwkIdentPart(r) {
				break
			}
			l.pos += sz
		}
		lit := l.src[start:l.pos]
		l.canRegex = lit == "BEGIN" || lit == "END" || lit == "if" || lit == "while" || lit == "for" || lit == "print" || lit == "printf" || lit == "return" || lit == "function" || lit == "delete" || lit == "in"
		return awkToken{kind: awkIdent, lit: lit, pos: start}, nil
	}
	if unicode.IsDigit(rune(ch)) || (ch == '.' && l.pos+1 < len(l.src) && unicode.IsDigit(rune(l.src[l.pos+1]))) {
		if ch == '0' && l.pos+1 < len(l.src) && (l.src[l.pos+1] == 'x' || l.src[l.pos+1] == 'X') {
			l.pos += 2
		} else {
			l.pos++
		}
		for l.pos < len(l.src) {
			c := l.src[l.pos]
			if !(unicode.IsDigit(rune(c)) || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-') {
				break
			}
			prev := l.src[l.pos-1]
			if (c == '+' || c == '-') && prev != 'e' && prev != 'E' {
				break
			}
			l.pos++
		}
		l.canRegex = false
		return awkToken{kind: awkNumber, lit: l.src[start:l.pos], pos: start}, nil
	}
	ops2 := []string{"++", "--", "+=", "-=", "*=", "/=", "%=", "==", "!=", "<=", ">=", "&&", "||", "!~"}
	for _, op := range ops2 {
		if l.starts(op) {
			l.pos += 2
			l.canRegex = op != "++" && op != "--"
			return awkToken{kind: awkOp, lit: op, pos: start}, nil
		}
	}
	l.pos++
	lit := string(ch)
	l.canRegex = strings.Contains("({[,;?:=~!+-*/%<>$", lit)
	return awkToken{kind: awkOp, lit: lit, pos: start}, nil
}

func (l *awkLexer) starts(s string) bool {
	return strings.HasPrefix(l.src[l.pos:], s)
}

func (l *awkLexer) scanString() (string, error) {
	return scanAwkQuoted(l.src, &l.pos, '"')
}

func (l *awkLexer) scanRegex() (string, error) {
	return scanAwkQuoted(l.src, &l.pos, '/')
}

func scanAwkQuoted(src string, pos *int, quote byte) (string, error) {
	(*pos)++
	var b strings.Builder
	for *pos < len(src) {
		ch := src[*pos]
		(*pos)++
		if ch == quote {
			return b.String(), nil
		}
		if ch == '\\' && *pos < len(src) {
			n := src[*pos]
			(*pos)++
			switch n {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			default:
				b.WriteByte(n)
			}
		} else {
			b.WriteByte(ch)
		}
	}
	return b.String(), fmt.Errorf("unterminated %q", quote)
}

func isAwkIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isAwkIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

type awkProgram struct {
	funcs map[string]*awkFunction
	rules []awkRule
}
type awkFunction struct {
	name   string
	params []string
	body   []awkStmt
}
type awkRule struct {
	begin, end  bool
	pat, patEnd awkExpr
	action      []awkStmt
	inRange     bool
}

type (
	awkStmt      interface{ awkStmt() }
	awkPrintStmt struct {
		exprs    []awkExpr
		printf   bool
		redirect string
	}
)

type (
	awkExprStmt struct{ expr awkExpr }
	awkIfStmt   struct {
		cond                 awkExpr
		thenStmts, elseStmts []awkStmt
	}
)

type awkWhileStmt struct {
	cond awkExpr
	body []awkStmt
}
type awkForStmt struct {
	init, cond, post awkExpr
	body             []awkStmt
}
type awkForInStmt struct {
	name, array string
	body        []awkStmt
}
type awkDeleteStmt struct {
	name  string
	index awkExpr
}
type (
	awkReturnStmt struct{ expr awkExpr }
	awkNextStmt   struct{ file bool }
)

func (awkPrintStmt) awkStmt() {
}

func (awkExprStmt) awkStmt() {
}

func (awkIfStmt) awkStmt() {
}

func (awkWhileStmt) awkStmt() {
}

func (awkForStmt) awkStmt() {
}

func (awkForInStmt) awkStmt() {
}

func (awkDeleteStmt) awkStmt() {
}

func (awkReturnStmt) awkStmt() {
}

func (awkNextStmt) awkStmt() {
}

type (
	awkExpr        interface{ awkExpr() }
	awkLiteralExpr struct{ val awkValue }
	awkVarExpr     struct{ name string }
	awkArrayExpr   struct {
		name  string
		index awkExpr
	}
)

type (
	awkFieldExpr  struct{ index awkExpr }
	awkAssignExpr struct {
		left  awkExpr
		op    string
		right awkExpr
	}
)

type awkBinaryExpr struct {
	op          string
	left, right awkExpr
}
type awkUnaryExpr struct {
	op   string
	expr awkExpr
}
type awkCallExpr struct {
	name string
	args []awkExpr
}
type awkPostExpr struct {
	op   string
	expr awkExpr
}
type (
	awkTernaryExpr struct{ cond, yes, no awkExpr }
	awkRegexExpr   struct{ pattern string }
)

func (awkLiteralExpr) awkExpr() {
}

func (awkVarExpr) awkExpr() {
}

func (awkArrayExpr) awkExpr() {
}

func (awkFieldExpr) awkExpr() {
}

func (awkAssignExpr) awkExpr() {
}

func (awkBinaryExpr) awkExpr() {
}

func (awkUnaryExpr) awkExpr() {
}

func (awkCallExpr) awkExpr() {
}

func (awkPostExpr) awkExpr() {
}

func (awkTernaryExpr) awkExpr() {
}

func (awkRegexExpr) awkExpr() {
}

type awkParser struct {
	toks []awkToken
	pos  int
}

func newAwkParser(t []awkToken) *awkParser {
	return &awkParser{toks: t}
}

func (p *awkParser) parseProgram() (*awkProgram, error) {
	prog := &awkProgram{funcs: map[string]*awkFunction{}}
	for {
		p.skipNL()
		if p.peek().kind == awkEOF {
			return prog, nil
		}
		if p.matchIdent("function") {
			fn, err := p.parseFunction()
			if err != nil {
				return nil, err
			}
			prog.funcs[fn.name] = fn
			continue
		}
		r, err := p.parseRule()
		if err != nil {
			return nil, err
		}
		prog.rules = append(prog.rules, r)
	}
}

func (p *awkParser) parseFunction() (*awkFunction, error) {
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if !p.matchOp("(") {
		return nil, p.err("expected ( after function name")
	}
	var params []string
	if !p.matchOp(")") {
		for {
			id, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			params = append(params, id)
			if p.matchOp(")") {
				break
			}
			if !p.matchOp(",") {
				return nil, p.err("expected , or )")
			}
		}
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &awkFunction{name: name, params: params, body: body}, nil
}

func (p *awkParser) parseRule() (awkRule, error) {
	var r awkRule
	if p.matchIdent("BEGIN") {
		r.begin = true
		b, err := p.parseBlock()
		r.action = b
		return r, err
	}
	if p.matchIdent("END") {
		r.end = true
		b, err := p.parseBlock()
		r.action = b
		return r, err
	}
	if p.peekOp("{") {
		b, err := p.parseBlock()
		r.action = b
		return r, err
	}
	pat, err := p.parseExpr()
	if err != nil {
		return r, err
	}
	r.pat = pat
	if p.matchOp(",") {
		end, err := p.parseExpr()
		if err != nil {
			return r, err
		}
		r.patEnd = end
	}
	if p.peekOp("{") {
		b, err := p.parseBlock()
		r.action = b
		return r, err
	}
	r.action = []awkStmt{awkPrintStmt{exprs: []awkExpr{awkFieldExpr{index: awkLiteralExpr{val: awkNumberVal(0)}}}}}
	return r, nil
}

func (p *awkParser) parseBlock() ([]awkStmt, error) {
	if !p.matchOp("{") {
		return nil, p.err("expected action block")
	}
	var out []awkStmt
	for {
		p.skipSep()
		if p.matchOp("}") {
			return out, nil
		}
		if p.peek().kind == awkEOF {
			return nil, p.err("unterminated block")
		}
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		p.skipSep()
	}
}

func (p *awkParser) parseStmtListOrStmt() ([]awkStmt, error) {
	if p.peekOp("{") {
		return p.parseBlock()
	}
	s, err := p.parseStmt()
	if err != nil {
		return nil, err
	}
	return []awkStmt{s}, nil
}

func (p *awkParser) parseStmt() (awkStmt, error) {
	if p.matchIdent("print") {
		ex := p.parseExprListUntilStmtEnd()
		return awkPrintStmt{exprs: ex}, nil
	}
	if p.matchIdent("printf") {
		ex := p.parseExprListUntilStmtEnd()
		return awkPrintStmt{exprs: ex, printf: true}, nil
	}
	if p.matchIdent("if") {
		if !p.matchOp("(") {
			return nil, p.err("expected ( after if")
		}
		c, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.matchOp(")") {
			return nil, p.err("expected )")
		}
		th, err := p.parseStmtListOrStmt()
		if err != nil {
			return nil, err
		}
		var el []awkStmt
		if p.matchIdent("else") {
			el, err = p.parseStmtListOrStmt()
			if err != nil {
				return nil, err
			}
		}
		return awkIfStmt{c, th, el}, nil
	}
	if p.matchIdent("while") {
		if !p.matchOp("(") {
			return nil, p.err("expected ( after while")
		}
		c, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.matchOp(")") {
			return nil, p.err("expected )")
		}
		b, err := p.parseStmtListOrStmt()
		return awkWhileStmt{c, b}, err
	}
	if p.matchIdent("for") {
		return p.parseFor()
	}
	if p.matchIdent("delete") {
		n, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		var idx awkExpr
		if p.matchOp("[") {
			idx, err = p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp("]") {
				return nil, p.err("expected ]")
			}
		}
		return awkDeleteStmt{name: n, index: idx}, nil
	}
	if p.matchIdent("return") {
		if p.atStmtEnd() {
			return awkReturnStmt{}, nil
		}
		e, err := p.parseExpr()
		return awkReturnStmt{e}, err
	}
	if p.matchIdent("next") {
		return awkNextStmt{}, nil
	}
	if p.matchIdent("nextfile") {
		return awkNextStmt{file: true}, nil
	}
	e, err := p.parseExpr()
	return awkExprStmt{e}, err
}

func (p *awkParser) parseFor() (awkStmt, error) {
	if !p.matchOp("(") {
		return nil, p.err("expected ( after for")
	}
	if p.peek().kind == awkIdent && p.pos+2 < len(p.toks) && p.toks[p.pos+1].lit == "in" {
		name := p.next().lit
		p.next()
		arr, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		if !p.matchOp(")") {
			return nil, p.err("expected )")
		}
		b, err := p.parseStmtListOrStmt()
		return awkForInStmt{name: name, array: arr, body: b}, err
	}
	var init, cond, post awkExpr
	var err error
	if !p.peekOp(";") {
		init, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	if !p.matchOp(";") {
		return nil, p.err("expected ;")
	}
	if !p.peekOp(";") {
		cond, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	if !p.matchOp(";") {
		return nil, p.err("expected ;")
	}
	if !p.peekOp(")") {
		post, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}
	if !p.matchOp(")") {
		return nil, p.err("expected )")
	}
	b, err := p.parseStmtListOrStmt()
	return awkForStmt{init, cond, post, b}, err
}

func (p *awkParser) parseExprListUntilStmtEnd() []awkExpr {
	var out []awkExpr
	for !p.atStmtEnd() && !p.peekOp("}") && p.peek().kind != awkEOF {
		e, err := p.parseExpr()
		if err != nil {
			break
		}
		out = append(out, e)
		if !p.matchOp(",") {
			break
		}
	}
	return out
}

func (p *awkParser) parseExpr() (awkExpr, error) {
	return p.parseAssign()
}

func (p *awkParser) parseAssign() (awkExpr, error) {
	left, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if p.peek().kind == awkOp {
		op := p.peek().lit
		if op == "=" || op == "+=" || op == "-=" || op == "*=" || op == "/=" || op == "%=" {
			p.next()
			r, err := p.parseAssign()
			return awkAssignExpr{left, op, r}, err
		}
	}
	return left, nil
}

func (p *awkParser) parseTernary() (awkExpr, error) {
	c, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.matchOp("?") {
		y, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.matchOp(":") {
			return nil, p.err("expected :")
		}
		n, err := p.parseExpr()
		return awkTernaryExpr{c, y, n}, err
	}
	return c, nil
}

func (p *awkParser) parseOr() (awkExpr, error) {
	return p.parseLeft(p.parseAnd, "||")
}

func (p *awkParser) parseAnd() (awkExpr, error) {
	return p.parseLeft(p.parseCompare, "&&")
}

func (p *awkParser) parseCompare() (awkExpr, error) {
	left, err := p.parseConcat()
	if err != nil {
		return nil, err
	}
	for {
		op := p.peek().lit
		if op == "==" || op == "!=" || op == "<" || op == "<=" || op == ">" || op == ">=" || op == "~" || op == "!~" || op == "in" {
			p.next()
			r, err := p.parseConcat()
			if err != nil {
				return nil, err
			}
			left = awkBinaryExpr{op, left, r}
			continue
		}
		return left, nil
	}
}

func (p *awkParser) parseConcat() (awkExpr, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for p.startsPrimary() {
		r, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		left = awkBinaryExpr{"concat", left, r}
	}
	return left, nil
}

func (p *awkParser) parseAdd() (awkExpr, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for p.peekOp("+") || p.peekOp("-") {
		op := p.next().lit
		r, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		left = awkBinaryExpr{op, left, r}
	}
	return left, nil
}

func (p *awkParser) parseMul() (awkExpr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peekOp("*") || p.peekOp("/") || p.peekOp("%") {
		op := p.next().lit
		r, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = awkBinaryExpr{op, left, r}
	}
	return left, nil
}

func (p *awkParser) parseUnary() (awkExpr, error) {
	if p.peekOp("!") || p.peekOp("-") || p.peekOp("+") || p.peekOp("++") || p.peekOp("--") {
		op := p.next().lit
		e, err := p.parseUnary()
		return awkUnaryExpr{op, e}, err
	}
	return p.parsePostfix()
}

func (p *awkParser) parsePostfix() (awkExpr, error) {
	e, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.peekOp("++") || p.peekOp("--") {
		op := p.next().lit
		e = awkPostExpr{op, e}
	}
	return e, nil
}

func (p *awkParser) parsePrimary() (awkExpr, error) {
	t := p.next()
	switch t.kind {
	case awkNumber:
		f, _ := strconv.ParseFloat(t.lit, 64)
		return awkLiteralExpr{awkNumberVal(f)}, nil
	case awkString:
		return awkLiteralExpr{awkValue{str: t.lit, isStr: true}}, nil
	case awkRegex:
		return awkRegexExpr{t.lit}, nil
	case awkIdent:
		if t.lit == "getline" {
			if p.startsPrimary() {
				a, err := p.parseUnary()
				if err != nil {
					return nil, err
				}
				return awkCallExpr{t.lit, []awkExpr{a}}, nil
			}
			return awkCallExpr{t.lit, nil}, nil
		}
		if p.matchOp("(") {
			var args []awkExpr
			if !p.matchOp(")") {
				for {
					a, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if p.matchOp(")") {
						break
					}
					if !p.matchOp(",") {
						return nil, p.err("expected , or )")
					}
				}
			}
			return awkCallExpr{t.lit, args}, nil
		}
		if p.matchOp("[") {
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp("]") {
				return nil, p.err("expected ]")
			}
			return awkArrayExpr{t.lit, idx}, nil
		}
		return awkVarExpr{t.lit}, nil
	case awkOp:
		if t.lit == "(" {
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp(")") {
				return nil, p.err("expected )")
			}
			return e, nil
		}
		if t.lit == "$" {
			idx, err := p.parseUnary()
			return awkFieldExpr{idx}, err
		}
	}
	return nil, p.err("unexpected token " + t.lit)
}

func (p *awkParser) parseLeft(next func() (awkExpr, error), ops ...string) (awkExpr, error) {
	left, err := next()
	if err != nil {
		return nil, err
	}
	for {
		matched := false
		for _, op := range ops {
			if p.peekOp(op) {
				p.next()
				r, err := next()
				if err != nil {
					return nil, err
				}
				left = awkBinaryExpr{op, left, r}
				matched = true
				break
			}
		}
		if !matched {
			return left, nil
		}
	}
}

func (p *awkParser) startsPrimary() bool {
	t := p.peek()
	if t.kind == awkNumber || t.kind == awkString || t.kind == awkRegex || t.kind == awkIdent {
		return t.lit != "in" && t.lit != "else"
	}
	return t.kind == awkOp && (t.lit == "(" || t.lit == "$")
}

func (p *awkParser) skipNL() {
	for p.peek().kind == awkNewline {
		p.pos++
	}
}

func (p *awkParser) skipSep() {
	for p.peek().kind == awkNewline || p.peekOp(";") {
		p.pos++
	}
}

func (p *awkParser) atStmtEnd() bool {
	return p.peek().kind == awkNewline || p.peekOp(";") || p.peekOp("}") || p.peek().kind == awkEOF
}

func (p *awkParser) peek() awkToken {
	if p.pos >= len(p.toks) {
		return awkToken{kind: awkEOF}
	}
	return p.toks[p.pos]
}

func (p *awkParser) next() awkToken {
	t := p.peek()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

func (p *awkParser) peekOp(op string) bool {
	t := p.peek()
	return t.kind == awkOp && t.lit == op
}

func (p *awkParser) matchOp(op string) bool {
	if p.peekOp(op) {
		p.pos++
		return true
	}
	return false
}

func (p *awkParser) matchIdent(id string) bool {
	t := p.peek()
	if t.kind == awkIdent && t.lit == id {
		p.pos++
		return true
	}
	return false
}

func (p *awkParser) expectIdent() (string, error) {
	t := p.next()
	if t.kind != awkIdent {
		return "", p.err("expected identifier")
	}
	return t.lit, nil
}

func (p *awkParser) err(s string) error {
	return fmt.Errorf("parse error near byte %d: %s", p.peek().pos, s)
}

type awkValue struct {
	str   string
	num   float64
	isStr bool
}

func awkNumberVal(f float64) awkValue {
	return awkValue{num: f}
}

func (v awkValue) String() string {
	if v.isStr {
		return v.str
	}
	if math.IsNaN(v.num) || math.IsInf(v.num, 0) {
		return fmt.Sprintf("%g", v.num)
	}
	if math.Trunc(v.num) == v.num {
		return strconv.FormatInt(int64(v.num), 10)
	}
	return strconv.FormatFloat(v.num, 'g', -1, 64)
}

func (v awkValue) Number() float64 {
	if !v.isStr {
		return v.num
	}
	f, _ := strconv.ParseFloat(strings.TrimSpace(v.str), 64)
	return f
}

func (v awkValue) Bool() bool {
	if v.isStr {
		return v.str != "" && v.Number() != 0
	}
	return v.num != 0
}

type awkVM struct {
	ctx              context.Context
	c                *CommandContext
	globals          map[string]awkValue
	arrays           map[string]map[string]awkValue
	funcs            map[string]*awkFunction
	fields           []string
	record           string
	files            []awkInputFile
	fileIdx, lineIdx int
	rng              *rand.Rand
	callDepth        int
	runtimeErr       error
}
type awkInputFile struct {
	name  string
	lines []string
}

func newAwkVM(ctx context.Context, c *CommandContext) *awkVM {
	vm := &awkVM{ctx: ctx, c: c, globals: map[string]awkValue{}, arrays: map[string]map[string]awkValue{}, rng: rand.New(rand.NewSource(1))}
	vm.setVar("FS", awkValue{str: " ", isStr: true})
	vm.setVar("OFS", awkValue{str: " ", isStr: true})
	vm.setVar("ORS", awkValue{str: "\n", isStr: true})
	vm.setVar("OFMT", awkValue{str: "%.6g", isStr: true})
	vm.setVar("CONVFMT", awkValue{str: "%.6g", isStr: true})
	return vm
}

func (vm *awkVM) run(prog *awkProgram, files []string) error {
	vm.funcs = prog.funcs
	if err := vm.loadInputs(files); err != nil {
		return err
	}
	for i := range prog.rules {
		if prog.rules[i].begin {
			if _, err := vm.execStmts(prog.rules[i].action, nil); err != nil {
				return err
			}
		}
	}
	for {
		if vm.ctx.Err() != nil {
			return vm.ctx.Err()
		}
		ok := vm.nextRecord()
		if !ok {
			break
		}
		skipFile := false
		for i := range prog.rules {
			r := &prog.rules[i]
			if r.begin || r.end {
				continue
			}
			match := true
			if r.pat != nil {
				match = vm.evalPattern(r)
			}
			if match {
				sig, err := vm.execStmts(r.action, nil)
				if err != nil {
					return err
				}
				if sig == "next" {
					break
				}
				if sig == "nextfile" {
					skipFile = true
					break
				}
			}
			if vm.runtimeErr != nil {
				return vm.runtimeErr
			}
		}
		if skipFile {
			vm.skipCurrentFile()
		}
		if vm.runtimeErr != nil {
			return vm.runtimeErr
		}
	}
	for i := range prog.rules {
		if prog.rules[i].end {
			if _, err := vm.execStmts(prog.rules[i].action, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (vm *awkVM) loadInputs(files []string) error {
	if len(files) == 0 {
		data, err := io.ReadAll(vm.c.Stdin)
		if err != nil {
			return err
		}
		vm.files = []awkInputFile{{name: "-", lines: splitAwkRecords(string(data))}}
		return nil
	}
	for _, name := range files {
		var data []byte
		var err error
		if name == "-" {
			data, err = io.ReadAll(vm.c.Stdin)
		} else {
			data, err = gfs.ReadFile(vm.c.FS, abs(vm.c, name))
		}
		if err != nil {
			return fmt.Errorf("%s: No such file or directory", name)
		}
		vm.files = append(vm.files, awkInputFile{name: name, lines: splitAwkRecords(string(data))})
	}
	return nil
}

func splitAwkRecords(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func (vm *awkVM) nextRecord() bool {
	for vm.fileIdx < len(vm.files) {
		f := vm.files[vm.fileIdx]
		if vm.lineIdx < len(f.lines) {
			vm.setRecord(f.name, f.lines[vm.lineIdx])
			vm.lineIdx++
			return true
		}
		vm.fileIdx++
		vm.lineIdx = 0
		vm.setVar("FNR", awkNumberVal(0))
	}
	return false
}

func (vm *awkVM) skipCurrentFile() {
	if vm.fileIdx < len(vm.files) {
		vm.fileIdx++
		vm.lineIdx = 0
		vm.setVar("FNR", awkNumberVal(0))
	}
}

func (vm *awkVM) setRecord(filename, rec string) {
	vm.record = rec
	vm.setVar("FILENAME", awkValue{str: filename, isStr: true})
	vm.setVar("NR", awkNumberVal(vm.getVar("NR").Number()+1))
	vm.setVar("FNR", awkNumberVal(vm.getVar("FNR").Number()+1))
	vm.splitFields()
}

func (vm *awkVM) splitFields() {
	fs := vm.getVar("FS").String()
	if fs == " " {
		vm.fields = strings.Fields(vm.record)
	} else if fs == "" {
		vm.fields = []string{}
		for _, r := range vm.record {
			vm.fields = append(vm.fields, string(r))
		}
	} else if re, err := regexp.Compile(fs); err == nil {
		vm.fields = re.Split(vm.record, -1)
	} else {
		vm.fields = strings.Split(vm.record, fs)
	}
	vm.setVar("NF", awkNumberVal(float64(len(vm.fields))))
}

func (vm *awkVM) rebuildRecord() {
	vm.record = strings.Join(vm.fields, vm.getVar("OFS").String())
}

func (vm *awkVM) evalPattern(r *awkRule) bool {
	m := vm.eval(r.pat, nil).Bool()
	if r.patEnd == nil {
		return m
	}
	if r.inRange {
		if vm.eval(r.patEnd, nil).Bool() {
			r.inRange = false
		}
		return true
	}
	if m {
		r.inRange = true
		if vm.eval(r.patEnd, nil).Bool() {
			r.inRange = false
		}
		return true
	}
	return false
}

func (vm *awkVM) execStmts(stmts []awkStmt, locals map[string]awkValue) (string, error) {
	for _, s := range stmts {
		sig, err := vm.execStmt(s, locals)
		if err != nil || sig != "" {
			return sig, err
		}
	}
	return "", nil
}

func (vm *awkVM) execStmt(s awkStmt, locals map[string]awkValue) (string, error) {
	switch st := s.(type) {
	case awkPrintStmt:
		return "", vm.execPrint(st, locals)
	case awkExprStmt:
		vm.eval(st.expr, locals)
		return "", nil
	case awkIfStmt:
		if vm.eval(st.cond, locals).Bool() {
			return vm.execStmts(st.thenStmts, locals)
		}
		return vm.execStmts(st.elseStmts, locals)
	case awkWhileStmt:
		for i := 0; vm.eval(st.cond, locals).Bool(); i++ {
			if i > awkMaxLoopIterations {
				return "", fmt.Errorf("loop iteration limit exceeded")
			}
			sig, err := vm.execStmts(st.body, locals)
			if sig != "" || err != nil {
				return sig, err
			}
		}
		return "", nil
	case awkForStmt:
		if st.init != nil {
			vm.eval(st.init, locals)
		}
		for i := 0; ; i++ {
			if i > awkMaxLoopIterations {
				return "", fmt.Errorf("loop iteration limit exceeded")
			}
			if st.cond != nil && !vm.eval(st.cond, locals).Bool() {
				break
			}
			sig, err := vm.execStmts(st.body, locals)
			if sig != "" || err != nil {
				return sig, err
			}
			if st.post != nil {
				vm.eval(st.post, locals)
			}
		}
		return "", nil
	case awkForInStmt:
		arr := vm.arrays[st.array]
		keys := make([]string, 0, len(arr))
		for k := range arr {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			vm.setVarScoped(st.name, awkValue{str: k, isStr: true}, locals)
			sig, err := vm.execStmts(st.body, locals)
			if sig != "" || err != nil {
				return sig, err
			}
		}
		return "", nil
	case awkDeleteStmt:
		if st.index == nil {
			delete(vm.arrays, st.name)
		} else {
			delete(vm.array(st.name), vm.eval(st.index, locals).String())
		}
		return "", nil
	case awkReturnStmt:
		if st.expr != nil {
			vm.setVarScoped("return", vm.eval(st.expr, locals), locals)
		}
		return "return", nil
	case awkNextStmt:
		if st.file {
			return "nextfile", nil
		}
		return "next", nil
	}
	return "", nil
}

func (vm *awkVM) execPrint(st awkPrintStmt, locals map[string]awkValue) error {
	vals := make([]awkValue, 0, len(st.exprs))
	for _, e := range st.exprs {
		vals = append(vals, vm.eval(e, locals))
	}
	if st.printf {
		if len(vals) == 0 {
			return nil
		}
		fmt.Fprint(vm.c.Stdout, awkSprintf(vals))
		return nil
	}
	if len(vals) == 0 {
		fmt.Fprint(vm.c.Stdout, vm.record, vm.getVar("ORS").String())
		return nil
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = v.String()
	}
	fmt.Fprint(vm.c.Stdout, strings.Join(parts, vm.getVar("OFS").String()), vm.getVar("ORS").String())
	return nil
}

func (vm *awkVM) eval(e awkExpr, locals map[string]awkValue) awkValue {
	switch ex := e.(type) {
	case nil:
		return awkValue{}
	case awkLiteralExpr:
		return ex.val
	case awkVarExpr:
		return vm.getVarScoped(ex.name, locals)
	case awkArrayExpr:
		return vm.array(ex.name)[vm.eval(ex.index, locals).String()]
	case awkFieldExpr:
		n := int(vm.eval(ex.index, locals).Number())
		if n == 0 {
			return awkValue{str: vm.record, isStr: true}
		}
		if n > 0 && n <= len(vm.fields) {
			return awkValue{str: vm.fields[n-1], isStr: true}
		}
		return awkValue{str: "", isStr: true}
	case awkRegexExpr:
		re, err := regexp.Compile(ex.pattern)
		if err != nil {
			return awkValue{}
		}
		return awkNumberVal(boolNum(re.MatchString(vm.record)))
	case awkAssignExpr:
		return vm.evalAssign(ex, locals)
	case awkBinaryExpr:
		return vm.evalBinary(ex, locals)
	case awkUnaryExpr:
		return vm.evalUnary(ex, locals)
	case awkPostExpr:
		old := vm.eval(ex.expr, locals)
		delta := 1.0
		if ex.op == "--" {
			delta = -1
		}
		vm.assign(ex.expr, awkNumberVal(old.Number()+delta), locals)
		return old
	case awkTernaryExpr:
		if vm.eval(ex.cond, locals).Bool() {
			return vm.eval(ex.yes, locals)
		}
		return vm.eval(ex.no, locals)
	case awkCallExpr:
		return vm.call(ex, locals)
	}
	return awkValue{}
}

func (vm *awkVM) evalAssign(ex awkAssignExpr, locals map[string]awkValue) awkValue {
	r := vm.eval(ex.right, locals)
	if ex.op != "=" {
		l := vm.eval(ex.left, locals)
		switch ex.op {
		case "+=":
			r = awkNumberVal(l.Number() + r.Number())
		case "-=":
			r = awkNumberVal(l.Number() - r.Number())
		case "*=":
			r = awkNumberVal(l.Number() * r.Number())
		case "/=":
			r = awkNumberVal(l.Number() / r.Number())
		case "%=":
			r = awkNumberVal(math.Mod(l.Number(), r.Number()))
		}
	}
	vm.assign(ex.left, r, locals)
	return r
}

func (vm *awkVM) evalBinary(ex awkBinaryExpr, locals map[string]awkValue) awkValue {
	if ex.op == "&&" {
		if !vm.eval(ex.left, locals).Bool() {
			return awkNumberVal(0)
		}
		return awkNumberVal(boolNum(vm.eval(ex.right, locals).Bool()))
	}
	if ex.op == "||" {
		if vm.eval(ex.left, locals).Bool() {
			return awkNumberVal(1)
		}
		return awkNumberVal(boolNum(vm.eval(ex.right, locals).Bool()))
	}
	l := vm.eval(ex.left, locals)
	r := vm.eval(ex.right, locals)
	switch ex.op {
	case "concat":
		return awkValue{str: l.String() + r.String(), isStr: true}
	case "+":
		return awkNumberVal(l.Number() + r.Number())
	case "-":
		return awkNumberVal(l.Number() - r.Number())
	case "*":
		return awkNumberVal(l.Number() * r.Number())
	case "/":
		return awkNumberVal(l.Number() / r.Number())
	case "%":
		return awkNumberVal(math.Mod(l.Number(), r.Number()))
	case "==":
		return awkNumberVal(boolNum(compareAwk(l, r) == 0))
	case "!=":
		return awkNumberVal(boolNum(compareAwk(l, r) != 0))
	case "<":
		return awkNumberVal(boolNum(compareAwk(l, r) < 0))
	case "<=":
		return awkNumberVal(boolNum(compareAwk(l, r) <= 0))
	case ">":
		return awkNumberVal(boolNum(compareAwk(l, r) > 0))
	case ">=":
		return awkNumberVal(boolNum(compareAwk(l, r) >= 0))
	case "~", "!~":
		re, err := regexp.Compile(r.String())
		ok := err == nil && re.MatchString(l.String())
		if ex.op == "!~" {
			ok = !ok
		}
		return awkNumberVal(boolNum(ok))
	case "in":
		if av, ok := ex.right.(awkVarExpr); ok {
			_, ok := vm.arrays[av.name][l.String()]
			return awkNumberVal(boolNum(ok))
		}
	}
	return awkValue{}
}

func compareAwk(a, b awkValue) int {
	if af, aok := awkNumericString(a); aok {
		if bf, bok := awkNumericString(b); bok {
			if af < bf {
				return -1
			}
			if af > bf {
				return 1
			}
			return 0
		}
	}
	as, bs := a.String(), b.String()
	if as < bs {
		return -1
	}
	if as > bs {
		return 1
	}
	return 0
}

func awkNumericString(v awkValue) (float64, bool) {
	if !v.isStr {
		return v.num, true
	}
	s := strings.TrimSpace(v.str)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

func boolNum(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (vm *awkVM) evalUnary(ex awkUnaryExpr, locals map[string]awkValue) awkValue {
	v := vm.eval(ex.expr, locals)
	switch ex.op {
	case "!":
		return awkNumberVal(boolNum(!v.Bool()))
	case "-":
		return awkNumberVal(-v.Number())
	case "+":
		return awkNumberVal(v.Number())
	case "++":
		nv := awkNumberVal(v.Number() + 1)
		vm.assign(ex.expr, nv, locals)
		return nv
	case "--":
		nv := awkNumberVal(v.Number() - 1)
		vm.assign(ex.expr, nv, locals)
		return nv
	}
	return v
}

func (vm *awkVM) assign(left awkExpr, v awkValue, locals map[string]awkValue) {
	switch l := left.(type) {
	case awkVarExpr:
		vm.setVarScoped(l.name, v, locals)
	case awkArrayExpr:
		vm.array(l.name)[vm.eval(l.index, locals).String()] = v
	case awkFieldExpr:
		n := int(vm.eval(l.index, locals).Number())
		if n == 0 {
			vm.record = v.String()
			vm.splitFields()
		} else if n > 0 {
			for len(vm.fields) < n {
				vm.fields = append(vm.fields, "")
			}
			vm.fields[n-1] = v.String()
			vm.setVar("NF", awkNumberVal(float64(len(vm.fields))))
			vm.rebuildRecord()
		}
	}
}

func (vm *awkVM) call(ex awkCallExpr, locals map[string]awkValue) awkValue {
	args := make([]awkValue, len(ex.args))
	for i, a := range ex.args {
		args[i] = vm.eval(a, locals)
	}
	switch ex.name {
	case "getline":
		if len(ex.args) > 0 {
			return vm.getline(ex.args[0], locals)
		}
		return vm.getline(nil, locals)
	case "length":
		if len(args) == 0 {
			return awkNumberVal(float64(len([]rune(vm.record))))
		}
		return awkNumberVal(float64(len([]rune(args[0].String()))))
	case "substr":
		s := []rune(args[0].String())
		start := int(args[1].Number()) - 1
		if start < 0 {
			start = 0
		}
		end := len(s)
		if len(args) > 2 {
			end = start + int(args[2].Number())
			if end > len(s) {
				end = len(s)
			}
		}
		if start > len(s) {
			return awkValue{str: "", isStr: true}
		}
		return awkValue{str: string(s[start:end]), isStr: true}
	case "index":
		return awkNumberVal(float64(strings.Index(args[0].String(), args[1].String()) + 1))
	case "tolower":
		return awkValue{str: strings.ToLower(args[0].String()), isStr: true}
	case "toupper":
		return awkValue{str: strings.ToUpper(args[0].String()), isStr: true}
	case "int":
		return awkNumberVal(math.Trunc(args[0].Number()))
	case "sqrt":
		return awkNumberVal(math.Sqrt(args[0].Number()))
	case "sin":
		return awkNumberVal(math.Sin(args[0].Number()))
	case "cos":
		return awkNumberVal(math.Cos(args[0].Number()))
	case "atan2":
		return awkNumberVal(math.Atan2(args[0].Number(), args[1].Number()))
	case "log":
		return awkNumberVal(math.Log(args[0].Number()))
	case "exp":
		return awkNumberVal(math.Exp(args[0].Number()))
	case "rand":
		return awkNumberVal(vm.rng.Float64())
	case "srand":
		seed := time.Now().UnixNano()
		if len(args) > 0 {
			seed = int64(args[0].Number())
		}
		vm.rng.Seed(seed)
		return awkNumberVal(float64(seed))
	case "sprintf":
		return awkValue{str: awkSprintf(args), isStr: true}
	case "split":
		return vm.builtinSplit(ex, args, locals)
	case "match":
		re, _ := regexp.Compile(args[1].String())
		idx := re.FindStringIndex(args[0].String())
		if idx == nil {
			return awkNumberVal(0)
		}
		vm.setVar("RSTART", awkNumberVal(float64(idx[0]+1)))
		vm.setVar("RLENGTH", awkNumberVal(float64(idx[1]-idx[0])))
		return awkNumberVal(float64(idx[0] + 1))
	case "sub", "gsub":
		return vm.builtinSub(ex, args, locals)
	}
	if fn := vm.funcs[ex.name]; fn != nil {
		return vm.callUser(fn, args)
	}
	vm.runtimeErr = fmt.Errorf("unsupported awk function %s", ex.name)
	return awkValue{}
}

func (vm *awkVM) builtinSplit(ex awkCallExpr, args []awkValue, locals map[string]awkValue) awkValue {
	if len(ex.args) < 2 {
		return awkNumberVal(0)
	}
	arrExpr, ok := ex.args[1].(awkVarExpr)
	if !ok {
		return awkNumberVal(0)
	}
	sep := vm.getVar("FS").String()
	if len(args) > 2 {
		sep = args[2].String()
	}
	var parts []string
	if sep == " " {
		parts = strings.Fields(args[0].String())
	} else if re, err := regexp.Compile(sep); err == nil {
		parts = re.Split(args[0].String(), -1)
	} else {
		parts = strings.Split(args[0].String(), sep)
	}
	arr := map[string]awkValue{}
	for i, p := range parts {
		arr[strconv.Itoa(i+1)] = awkValue{str: p, isStr: true}
	}
	vm.arrays[arrExpr.name] = arr
	return awkNumberVal(float64(len(parts)))
}

func (vm *awkVM) builtinSub(ex awkCallExpr, args []awkValue, locals map[string]awkValue) awkValue {
	if len(args) < 2 {
		return awkNumberVal(0)
	}
	var target awkExpr = awkFieldExpr{index: awkLiteralExpr{awkNumberVal(0)}}
	text := vm.record
	if len(ex.args) > 2 {
		target = ex.args[2]
		text = vm.eval(target, locals).String()
	}
	re, err := regexp.Compile(args[0].String())
	if err != nil {
		return awkNumberVal(0)
	}
	n := 0
	repl := args[1].String()
	out := re.ReplaceAllStringFunc(text, func(m string) string {
		if ex.name == "sub" && n > 0 {
			return m
		}
		n++
		return repl
	})
	vm.assign(target, awkValue{str: out, isStr: true}, locals)
	return awkNumberVal(float64(n))
}

func awkSprintf(vals []awkValue) string {
	if len(vals) == 0 {
		return ""
	}
	format := vals[0].String()
	args := make([]any, 0, len(vals)-1)
	verbs := awkFormatVerbs(format)
	for i, v := range vals[1:] {
		verb := byte('g')
		if i < len(verbs) {
			verb = verbs[i]
		}
		switch verb {
		case 's', 'q':
			args = append(args, v.String())
		case 'd', 'b', 'o', 'x', 'X', 'c', 'U':
			args = append(args, int64(v.Number()))
		default:
			args = append(args, v.Number())
		}
	}
	return fmt.Sprintf(format, args...)
}

func awkFormatVerbs(format string) []byte {
	var verbs []byte
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			continue
		}
		for i < len(format) && strings.ContainsRune("#0+- '0123456789.*[]", rune(format[i])) {
			i++
		}
		if i < len(format) {
			verbs = append(verbs, format[i])
		}
	}
	return verbs
}

func (vm *awkVM) callUser(fn *awkFunction, args []awkValue) awkValue {
	if vm.callDepth > awkMaxCallDepth {
		return awkValue{}
	}
	vm.callDepth++
	defer func() { vm.callDepth-- }()
	loc := map[string]awkValue{}
	for i, p := range fn.params {
		if i < len(args) {
			loc[p] = args[i]
		} else {
			loc[p] = awkValue{}
		}
	}
	sig, _ := vm.execStmts(fn.body, loc)
	if sig == "return" {
		return loc["return"]
	}
	return awkValue{}
}

func (vm *awkVM) array(name string) map[string]awkValue {
	if vm.arrays[name] == nil {
		vm.arrays[name] = map[string]awkValue{}
	}
	return vm.arrays[name]
}

func (vm *awkVM) getVar(name string) awkValue {
	return vm.globals[name]
}

func (vm *awkVM) setVar(name string, v awkValue) {
	vm.globals[name] = v
}

func (vm *awkVM) getVarScoped(name string, locals map[string]awkValue) awkValue {
	if locals != nil {
		if v, ok := locals[name]; ok {
			return v
		}
	}
	return vm.getVar(name)
}

func (vm *awkVM) setVarScoped(name string, v awkValue, locals map[string]awkValue) {
	if locals != nil {
		if _, ok := locals[name]; ok || name == "return" {
			locals[name] = v
			return
		}
	}
	vm.setVar(name, v)
}

func (vm *awkVM) getline(into awkExpr, locals map[string]awkValue) awkValue {
	if vm.nextRecord() {
		if into != nil {
			vm.assign(into, awkValue{str: vm.record, isStr: true}, locals)
		}
		return awkNumberVal(1)
	}
	return awkNumberVal(0)
}

// Keep bufio imported in this file's package builds when future getline-from-file
// support is extended; current command input uses io.ReadAll for deterministic VFS reads.
var _ = bufio.ErrInvalidUnreadByte
