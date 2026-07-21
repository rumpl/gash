package files

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	gfs "github.com/rumpl/gash/pkg/fs"
)

type findExpr interface{ isFindExpr() }

type findNameExpr struct {
	pattern    string
	ignoreCase bool
}
type findPathExpr struct {
	pattern    string
	ignoreCase bool
}
type findRegexExpr struct {
	pattern    string
	ignoreCase bool
}
type findTypeExpr struct{ fileType string }
type findEmptyExpr struct{}
type findMtimeExpr struct {
	days       int
	comparison string
}
type findNewerExpr struct{ refPath string }
type findSizeExpr struct {
	value      int64
	unit       byte
	comparison string
}
type findPermExpr struct {
	mode      iofs.FileMode
	matchType string
}
type findPruneExpr struct{}
type findActionExpr struct{ action findAction }
type findNotExpr struct{ expr findExpr }
type findAndExpr struct{ left, right findExpr }
type findOrExpr struct{ left, right findExpr }

func (findNameExpr) isFindExpr()   {}
func (findPathExpr) isFindExpr()   {}
func (findRegexExpr) isFindExpr()  {}
func (findTypeExpr) isFindExpr()   {}
func (findEmptyExpr) isFindExpr()  {}
func (findMtimeExpr) isFindExpr()  {}
func (findNewerExpr) isFindExpr()  {}
func (findSizeExpr) isFindExpr()   {}
func (findPermExpr) isFindExpr()   {}
func (findPruneExpr) isFindExpr()  {}
func (findActionExpr) isFindExpr() {}
func (findNotExpr) isFindExpr()    {}
func (findAndExpr) isFindExpr()    {}
func (findOrExpr) isFindExpr()     {}

type findAction struct {
	typ       string
	format    string
	command   []string
	batchMode bool
}

type findToken struct {
	typ  string
	op   string
	expr findExpr
}

type findNode struct {
	path          string
	name          string
	info          iofs.FileInfo
	isFile        bool
	isDir         bool
	isEmpty       bool
	depth         int
	startingPoint string
}

type findEvalContext struct {
	findNode
	newerRefs map[string]time.Time
}

type findEvalResult struct {
	matches bool
	pruned  bool
	actions []findAction
}

type findEffect struct {
	action findAction
	node   findNode
}

func commandFind(_ context.Context, args []string, c *CommandContext) int {
	searchPaths := []string{}
	exprStart := len(args)
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") || arg == "(" || arg == "\\(" || arg == ")" || arg == "\\)" || arg == "!" {
			exprStart = i
			break
		}
		searchPaths = append(searchPaths, arg)
	}
	if len(searchPaths) == 0 {
		searchPaths = []string{"."}
	}

	maxDepth, minDepth := -1, -1
	depthFirst := false
	for i := exprStart; i < len(args); i++ {
		switch args[i] {
		case "-exec":
			i++
			for i < len(args) && args[i] != ";" && args[i] != "+" {
				i++
			}
		case "-maxdepth", "-mindepth":
			pred := args[i]
			if i+1 >= len(args) {
				fmt.Fprintf(c.Stderr, "find: missing argument to `%s'\n", pred)
				return 1
			}
			value, err := parseNonNegativeInt(args[i+1])
			if err != nil {
				fmt.Fprintf(c.Stderr, "find: invalid argument `%s' to `%s'\n", args[i+1], pred)
				return 1
			}
			if pred == "-maxdepth" {
				maxDepth = value
			} else {
				minDepth = value
			}
			i++
		case "-depth":
			depthFirst = true
		}
	}

	expr, errText := parseFindExpressions(args, exprStart)
	if errText != "" {
		fmt.Fprint(c.Stderr, errText)
		return 1
	}
	allActions := collectFindActions(expr)
	hasAction := len(allActions) > 0
	for _, a := range allActions {
		if a.typ == "delete" {
			depthFirst = true
		}
	}

	newerRefs := map[string]time.Time{}
	for _, ref := range collectFindNewerRefs(expr) {
		if info, err := gfs.Stat(c.FS, abs(c, ref)); err == nil {
			newerRefs[ref] = info.ModTime()
		}
	}

	code := 0
	var effects []findEffect
	for _, originalSearchPath := range searchPaths {
		searchPath := strings.TrimRight(originalSearchPath, "/")
		if searchPath == "" {
			searchPath = originalSearchPath
		}
		if searchPath == "" {
			searchPath = "/"
		}
		base := abs(c, searchPath)
		if _, err := gfs.Lstat(c.FS, base); err != nil {
			fmt.Fprintf(c.Stderr, "find: %s: No such file or directory\n", originalSearchPath)
			code = 1
			continue
		}
		walkEffects, walkCode := findWalk(c, base, searchPath, searchPath, 0, maxDepth, minDepth, depthFirst, expr, hasAction, newerRefs)
		if walkCode != 0 {
			code = walkCode
		}
		effects = append(effects, walkEffects...)
	}

	for _, effect := range effects {
		switch effect.action.typ {
		case "print":
			fmt.Fprintln(c.Stdout, effect.node.path)
		case "print0":
			fmt.Fprint(c.Stdout, effect.node.path, "\x00")
		case "printf":
			fmt.Fprint(c.Stdout, formatFindPrintf(effect.action.format, effect.node))
		case "delete":
			if err := gfs.Remove(c.FS, abs(c, effect.node.path)); err != nil {
				fmt.Fprintf(c.Stderr, "find: cannot delete '%s': %v\n", effect.node.path, err)
				code = 1
			}
		case "exec":
			// just-bash delegates to its sandboxed runtime executor. gash's file
			// command context intentionally has no command registry, so fail closed
			// instead of invoking the host PATH.
			fmt.Fprint(c.Stderr, "find: -exec is not supported by gash find (host PATH execution disabled)\n")
			return 1
		}
	}
	return code
}

func parseNonNegativeInt(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("invalid")
		}
	}
	n, err := strconv.ParseInt(s, 10, 0)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func parseFindExpressions(args []string, start int) (findExpr, string) {
	tokens := []findToken{}
	missing := func(pred string) (findExpr, string) {
		return nil, fmt.Sprintf("find: missing argument to `%s'\n", pred)
	}
	invalid := func(pred, val string) (findExpr, string) {
		return nil, fmt.Sprintf("find: invalid argument `%s' to `%s'\n", val, pred)
	}
	for i := start; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "(", "\\(":
			tokens = append(tokens, findToken{typ: "lparen"})
		case ")", "\\)":
			tokens = append(tokens, findToken{typ: "rparen"})
		case "-name", "-iname", "-path", "-ipath", "-regex", "-iregex":
			if i+1 >= len(args) {
				return missing(arg)
			}
			i++
			pat := args[i]
			ignore := strings.Contains(arg, "i")
			if strings.Contains(arg, "path") {
				tokens = append(tokens, findToken{typ: "expr", expr: findPathExpr{pat, ignore}})
			} else if strings.Contains(arg, "regex") {
				tokens = append(tokens, findToken{typ: "expr", expr: findRegexExpr{pat, ignore}})
			} else {
				tokens = append(tokens, findToken{typ: "expr", expr: findNameExpr{pat, ignore}})
			}
		case "-type":
			if i+1 >= len(args) {
				return missing(arg)
			}
			i++
			ft := args[i]
			if ft != "f" && ft != "d" {
				return nil, fmt.Sprintf("find: Unknown argument to -type: %s\n", ft)
			}
			tokens = append(tokens, findToken{typ: "expr", expr: findTypeExpr{ft}})
		case "-empty":
			tokens = append(tokens, findToken{typ: "expr", expr: findEmptyExpr{}})
		case "-mtime":
			if i+1 >= len(args) {
				return missing(arg)
			}
			i++
			cmp, num := findComparison(args[i])
			n, err := parseNonNegativeInt(num)
			if err != nil {
				return invalid(arg, args[i])
			}
			tokens = append(tokens, findToken{typ: "expr", expr: findMtimeExpr{n, cmp}})
		case "-newer":
			if i+1 >= len(args) {
				return missing(arg)
			}
			i++
			tokens = append(tokens, findToken{typ: "expr", expr: findNewerExpr{args[i]}})
		case "-size":
			if i+1 >= len(args) {
				return missing(arg)
			}
			i++
			cmp, spec := findComparison(args[i])
			multUnit := byte('b')
			if spec == "" {
				return invalid(arg, args[i])
			}
			last := spec[len(spec)-1]
			if strings.ContainsRune("ckMGb", rune(last)) {
				multUnit = last
				spec = spec[:len(spec)-1]
			}
			if spec == "" {
				return invalid(arg, args[i])
			}
			n, err := strconv.ParseInt(spec, 10, 64)
			if err != nil || n < 0 {
				return invalid(arg, args[i])
			}
			tokens = append(tokens, findToken{typ: "expr", expr: findSizeExpr{n, multUnit, cmp}})
		case "-perm":
			if i+1 >= len(args) {
				return missing(arg)
			}
			i++
			spec := args[i]
			mt := "exact"
			if strings.HasPrefix(spec, "-") {
				mt = "all"
				spec = spec[1:]
			} else if strings.HasPrefix(spec, "/") {
				mt = "any"
				spec = spec[1:]
			}
			if spec == "" || len(spec) > 4 {
				return invalid(arg, args[i])
			}
			for _, r := range spec {
				if r < '0' || r > '7' {
					return invalid(arg, args[i])
				}
			}
			n, _ := strconv.ParseUint(spec, 8, 32)
			tokens = append(tokens, findToken{typ: "expr", expr: findPermExpr{iofs.FileMode(n), mt}})
		case "-prune":
			tokens = append(tokens, findToken{typ: "expr", expr: findPruneExpr{}})
		case "-not", "!":
			tokens = append(tokens, findToken{typ: "not"})
		case "-o", "-or":
			tokens = append(tokens, findToken{typ: "op", op: "or"})
		case "-a", "-and":
			tokens = append(tokens, findToken{typ: "op", op: "and"})
		case "-maxdepth", "-mindepth":
			if i+1 >= len(args) {
				return missing(arg)
			}
			i++
		case "-depth":
		case "-exec":
			parts := []string{}
			i++
			for i < len(args) && args[i] != ";" && args[i] != "+" {
				parts = append(parts, args[i])
				i++
			}
			if i >= len(args) {
				return nil, "find: missing argument to `-exec'\n"
			}
			if len(parts) == 0 {
				return missing("-exec")
			}
			batch := args[i] == "+"
			if batch {
				count := 0
				for _, p := range parts {
					if p == "{}" {
						count++
					}
				}
				if count != 1 || parts[len(parts)-1] != "{}" {
					return invalid("-exec", "+")
				}
			}
			tokens = append(tokens, findToken{typ: "expr", expr: findActionExpr{findAction{typ: "exec", command: parts, batchMode: batch}}})
		case "-print", "-print0", "-delete":
			typ := strings.TrimPrefix(arg, "-")
			tokens = append(tokens, findToken{typ: "expr", expr: findActionExpr{findAction{typ: typ}}})
		case "-printf":
			if i+1 >= len(args) {
				return missing(arg)
			}
			i++
			tokens = append(tokens, findToken{typ: "expr", expr: findActionExpr{findAction{typ: "printf", format: args[i]}}})
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, fmt.Sprintf("find: unknown predicate '%s'\n", arg)
			}
			return nil, fmt.Sprintf("find: paths must precede expression: `%s'\n", arg)
		}
	}
	if len(tokens) == 0 {
		return nil, ""
	}
	expr, err := buildFindExpressionTree(tokens)
	return expr, err
}

func findComparison(s string) (string, string) {
	if strings.HasPrefix(s, "+") {
		return "more", s[1:]
	}
	if strings.HasPrefix(s, "-") {
		return "less", s[1:]
	}
	return "exact", s
}

func buildFindExpressionTree(tokens []findToken) (findExpr, string) {
	pos := 0
	var parseOr func() findExpr
	var parseAnd func() findExpr
	var parseNot func() findExpr
	var parsePrimary func() findExpr
	errText := ""
	parseOr = func() findExpr {
		left := parseAnd()
		if left == nil {
			return nil
		}
		for pos < len(tokens) && tokens[pos].typ == "op" && tokens[pos].op == "or" {
			pos++
			right := parseAnd()
			if right == nil {
				errText = "find: expected an expression after `-o'\n"
				return nil
			}
			left = findOrExpr{left, right}
		}
		return left
	}
	parseAnd = func() findExpr {
		left := parseNot()
		if left == nil {
			return nil
		}
		for pos < len(tokens) {
			t := tokens[pos]
			if t.typ == "op" && t.op == "and" {
				pos++
				right := parseNot()
				if right == nil {
					errText = "find: expected an expression after `-a'\n"
					return nil
				}
				left = findAndExpr{left, right}
			} else if t.typ == "expr" || t.typ == "not" || t.typ == "lparen" {
				right := parseNot()
				if right == nil {
					return left
				}
				left = findAndExpr{left, right}
			} else {
				break
			}
		}
		return left
	}
	parseNot = func() findExpr {
		if pos < len(tokens) && tokens[pos].typ == "not" {
			pos++
			e := parseNot()
			if e == nil {
				errText = "find: expected an expression after `!'\n"
				return nil
			}
			return findNotExpr{e}
		}
		return parsePrimary()
	}
	parsePrimary = func() findExpr {
		if pos >= len(tokens) {
			return nil
		}
		t := tokens[pos]
		if t.typ == "lparen" {
			pos++
			e := parseOr()
			if e == nil || pos >= len(tokens) || tokens[pos].typ != "rparen" {
				errText = "find: missing closing `)'\n"
				return nil
			}
			pos++
			return e
		}
		if t.typ == "expr" {
			pos++
			return t.expr
		}
		return nil
	}
	expr := parseOr()
	if errText == "" && pos < len(tokens) {
		if tokens[pos].typ == "rparen" {
			errText = "find: unexpected `)'\n"
		} else {
			errText = "find: invalid expression\n"
		}
	}
	if errText == "" && containsNegatedFindDelete(expr, false) {
		errText = "find: refusing to evaluate `-delete' under negation\n"
	}
	if errText != "" {
		return nil, errText
	}
	return expr, ""
}

func containsNegatedFindDelete(expr findExpr, neg bool) bool {
	switch e := expr.(type) {
	case findActionExpr:
		return neg && e.action.typ == "delete"
	case findNotExpr:
		return containsNegatedFindDelete(e.expr, !neg)
	case findAndExpr:
		return containsNegatedFindDelete(e.left, neg) || containsNegatedFindDelete(e.right, neg)
	case findOrExpr:
		return containsNegatedFindDelete(e.left, neg) || containsNegatedFindDelete(e.right, neg)
	}
	return false
}

func findWalk(c *CommandContext, absPath, displayPath, start string, depth, maxDepth, minDepth int, depthFirst bool, expr findExpr, hasAction bool, refs map[string]time.Time) ([]findEffect, int) {
	if maxDepth >= 0 && depth > maxDepth {
		return nil, 0
	}
	info, err := gfs.Lstat(c.FS, absPath)
	if err != nil {
		return nil, 0
	}
	node := findNode{path: displayPath, name: path.Base(displayPath), info: info, isFile: info.Mode().IsRegular(), isDir: info.IsDir(), depth: depth, startingPoint: start}
	if displayPath == "/" {
		node.name = "/"
	}
	needsEmpty := expressionNeedsFindEmpty(expr)
	var children []iofs.DirEntry
	if node.isDir && (maxDepth < 0 || depth < maxDepth || needsEmpty) {
		children, err = gfs.ReadDir(c.FS, absPath)
		if err != nil {
			return nil, 0
		}
		node.isEmpty = len(children) == 0
	} else if node.isFile {
		node.isEmpty = info.Size() == 0
	}

	pruned := false
	if !depthFirst && expr != nil && expressionHasFindPrune(expr) {
		res := evalFindExpression(expr, findEvalContext{findNode: node, newerRefs: refs})
		pruned = res.pruned
	}

	var out []findEffect
	code := 0
	if !depthFirst && (minDepth < 0 || depth >= minDepth) {
		out = append(out, findNodeEffects(expr, node, hasAction, refs)...)
	}
	if node.isDir && !pruned && (maxDepth < 0 || depth < maxDepth) {
		if children == nil {
			children, err = gfs.ReadDir(c.FS, absPath)
			if err != nil {
				return out, 0
			}
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			childAbs := path.Join(absPath, child.Name())
			childDisplay := child.Name()
			if displayPath == "/" {
				childDisplay = "/" + child.Name()
			} else {
				childDisplay = path.Join(displayPath, child.Name())
			}
			sub, subCode := findWalk(c, childAbs, childDisplay, start, depth+1, maxDepth, minDepth, depthFirst, expr, hasAction, refs)
			out = append(out, sub...)
			if subCode != 0 {
				code = subCode
			}
		}
	}
	if depthFirst && (minDepth < 0 || depth >= minDepth) {
		out = append(out, findNodeEffects(expr, node, hasAction, refs)...)
	}
	return out, code
}

func findNodeEffects(expr findExpr, node findNode, hasAction bool, refs map[string]time.Time) []findEffect {
	matches := true
	actions := []findAction{}
	if expr != nil {
		res := evalFindExpression(expr, findEvalContext{findNode: node, newerRefs: refs})
		matches = res.matches
		actions = res.actions
	}
	if !hasAction && matches {
		actions = []findAction{{typ: "print"}}
	}
	out := []findEffect{}
	for _, a := range actions {
		out = append(out, findEffect{action: a, node: node})
	}
	return out
}

func evalFindExpression(expr findExpr, ctx findEvalContext) findEvalResult {
	switch e := expr.(type) {
	case findNameExpr:
		return findEvalResult{matches: matchFindGlob(ctx.name, e.pattern, e.ignoreCase)}
	case findPathExpr:
		return findEvalResult{matches: matchFindGlob(ctx.path, e.pattern, e.ignoreCase)}
	case findRegexExpr:
		pat := e.pattern
		if e.ignoreCase {
			pat = "(?i)" + pat
		}
		re, err := regexp.Compile(pat)
		return findEvalResult{matches: err == nil && re.MatchString(ctx.path)}
	case findTypeExpr:
		return findEvalResult{matches: (e.fileType == "f" && ctx.isFile) || (e.fileType == "d" && ctx.isDir)}
	case findEmptyExpr:
		return findEvalResult{matches: ctx.isEmpty}
	case findMtimeExpr:
		days := time.Since(ctx.info.ModTime()).Hours() / 24
		m := false
		if e.comparison == "more" {
			m = days > float64(e.days)
		} else if e.comparison == "less" {
			m = days < float64(e.days)
		} else {
			m = int(days) == e.days
		}
		return findEvalResult{matches: m}
	case findNewerExpr:
		ref, ok := ctx.newerRefs[e.refPath]
		return findEvalResult{matches: ok && ctx.info.ModTime().After(ref)}
	case findSizeExpr:
		target := e.value
		switch e.unit {
		case 'k':
			target *= 1024
		case 'M':
			target *= 1024 * 1024
		case 'G':
			target *= 1024 * 1024 * 1024
		case 'b':
			target *= 512
		}
		m := false
		if e.comparison == "more" {
			m = ctx.info.Size() > target
		} else if e.comparison == "less" {
			m = ctx.info.Size() < target
		} else if e.unit == 'b' {
			m = (ctx.info.Size()+511)/512 == e.value
		} else {
			m = ctx.info.Size() == target
		}
		return findEvalResult{matches: m}
	case findPermExpr:
		fm := ctx.info.Mode().Perm()
		tm := e.mode.Perm()
		m := false
		if e.matchType == "exact" {
			m = fm == tm
		} else if e.matchType == "all" {
			m = (fm & tm) == tm
		} else {
			m = (fm & tm) != 0
		}
		return findEvalResult{matches: m}
	case findPruneExpr:
		return findEvalResult{matches: true, pruned: true}
	case findActionExpr:
		return findEvalResult{matches: true, actions: []findAction{e.action}}
	case findNotExpr:
		inner := evalFindExpression(e.expr, ctx)
		return findEvalResult{matches: !inner.matches, pruned: inner.pruned, actions: inner.actions}
	case findAndExpr:
		left := evalFindExpression(e.left, ctx)
		if !left.matches {
			return findEvalResult{matches: false, pruned: left.pruned, actions: left.actions}
		}
		right := evalFindExpression(e.right, ctx)
		return findEvalResult{matches: right.matches, pruned: left.pruned || right.pruned, actions: append(left.actions, right.actions...)}
	case findOrExpr:
		left := evalFindExpression(e.left, ctx)
		if left.matches {
			return left
		}
		right := evalFindExpression(e.right, ctx)
		return findEvalResult{matches: right.matches, pruned: left.pruned || right.pruned, actions: append(left.actions, right.actions...)}
	}
	return findEvalResult{}
}

func matchFindGlob(value, pattern string, ignoreCase bool) bool {
	if ignoreCase {
		value = strings.ToLower(value)
		pattern = strings.ToLower(pattern)
	}
	re := "^"
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			re += ".*"
		case '?':
			re += "."
		case '[':
			j := i + 1
			if j < len(pattern) && (pattern[j] == '!' || pattern[j] == '^') {
				j++
			}
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j < len(pattern) {
				cls := pattern[i+1 : j]
				if strings.HasPrefix(cls, "!") {
					cls = "^" + cls[1:]
				}
				re += "[" + cls + "]"
				i = j
			} else {
				re += regexp.QuoteMeta(string(pattern[i]))
			}
		default:
			re += regexp.QuoteMeta(string(pattern[i]))
		}
	}
	re += "$"
	ok, err := regexp.MatchString(re, value)
	return err == nil && ok
}

func collectFindActions(expr findExpr) []findAction {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case findActionExpr:
		return []findAction{e.action}
	case findNotExpr:
		return collectFindActions(e.expr)
	case findAndExpr:
		return append(collectFindActions(e.left), collectFindActions(e.right)...)
	case findOrExpr:
		return append(collectFindActions(e.left), collectFindActions(e.right)...)
	}
	return nil
}
func collectFindNewerRefs(expr findExpr) []string {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case findNewerExpr:
		return []string{e.refPath}
	case findNotExpr:
		return collectFindNewerRefs(e.expr)
	case findAndExpr:
		return append(collectFindNewerRefs(e.left), collectFindNewerRefs(e.right)...)
	case findOrExpr:
		return append(collectFindNewerRefs(e.left), collectFindNewerRefs(e.right)...)
	}
	return nil
}
func expressionNeedsFindEmpty(expr findExpr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case findEmptyExpr:
		return true
	case findNotExpr:
		return expressionNeedsFindEmpty(e.expr)
	case findAndExpr:
		return expressionNeedsFindEmpty(e.left) || expressionNeedsFindEmpty(e.right)
	case findOrExpr:
		return expressionNeedsFindEmpty(e.left) || expressionNeedsFindEmpty(e.right)
	}
	return false
}
func expressionHasFindPrune(expr findExpr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case findPruneExpr:
		return true
	case findNotExpr:
		return expressionHasFindPrune(e.expr)
	case findAndExpr:
		return expressionHasFindPrune(e.left) || expressionHasFindPrune(e.right)
	case findOrExpr:
		return expressionHasFindPrune(e.left) || expressionHasFindPrune(e.right)
	}
	return false
}

func formatFindPrintf(format string, node findNode) string {
	format = strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\0`, "\x00", `\\`, `\`).Replace(format)
	out := strings.Builder{}
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			out.WriteByte(format[i])
			continue
		}
		i++
		if format[i] == '%' {
			out.WriteByte('%')
			continue
		}
		for i < len(format) && (format[i] == '-' || format[i] == '+' || format[i] == '.' || (format[i] >= '0' && format[i] <= '9')) {
			i++
		}
		if i >= len(format) {
			out.WriteByte('%')
			break
		}
		switch format[i] {
		case 'f':
			out.WriteString(node.name)
		case 'h':
			d := path.Dir(node.path)
			if d == "/" && !strings.HasPrefix(node.path, "/") {
				d = "."
			}
			out.WriteString(d)
		case 'p':
			out.WriteString(node.path)
		case 'P':
			out.WriteString(findStripStartingPoint(node.path, node.startingPoint))
		case 's':
			out.WriteString(strconv.FormatInt(node.info.Size(), 10))
		case 'd':
			out.WriteString(strconv.Itoa(node.depth))
		case 'm':
			out.WriteString(strconv.FormatUint(uint64(node.info.Mode().Perm()), 8))
		case 'M':
			out.WriteString(formatFindMode(node.info.Mode()))
		case 't':
			out.WriteString(node.info.ModTime().Format("Mon Jan _2 15:04:05 2006"))
		case 'T':
			if i+1 < len(format) {
				i++
				out.WriteString(formatFindTime(node.info.ModTime(), format[i]))
			} else {
				out.WriteString("%T")
			}
		default:
			out.WriteByte('%')
			out.WriteByte(format[i])
		}
	}
	return out.String()
}

func findStripStartingPoint(p, sp string) string {
	if p == sp {
		return ""
	}
	if strings.HasPrefix(p, sp+"/") {
		return strings.TrimPrefix(p, sp+"/")
	}
	if sp == "." && strings.HasPrefix(p, "./") {
		return strings.TrimPrefix(p, "./")
	}
	return p
}
func formatFindMode(mode iofs.FileMode) string {
	b := []byte("-rwxrwxrwx")
	if mode.IsDir() {
		b[0] = 'd'
	} else if mode&iofs.ModeSymlink != 0 {
		b[0] = 'l'
	}
	perms := []iofs.FileMode{0400, 0200, 0100, 0040, 0020, 0010, 0004, 0002, 0001}
	chars := []byte("rwxrwxrwx")
	for i, p := range perms {
		if mode&p == 0 {
			b[i+1] = '-'
		} else {
			b[i+1] = chars[i]
		}
	}
	return string(b)
}
func formatFindTime(t time.Time, d byte) string {
	switch d {
	case '@':
		return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', -1, 64)
	case 'Y':
		return t.Format("2006")
	case 'm':
		return t.Format("01")
	case 'd':
		return t.Format("02")
	case 'H':
		return t.Format("15")
	case 'M':
		return t.Format("04")
	case 'S':
		return t.Format("05")
	case 'T':
		return t.Format("15:04:05")
	case 'F':
		return t.Format("2006-01-02")
	}
	return "%T" + string(d)
}
