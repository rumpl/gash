package text

import (
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

type grepOptions struct {
	ignoreCase         bool
	showLineNumbers    bool
	invertMatch        bool
	countOnly          bool
	filesWithMatches   bool
	filesWithoutMatch  bool
	recursive          bool
	wholeWord          bool
	lineRegexp         bool
	fixedStrings       bool
	onlyMatching       bool
	noFilename         bool
	quiet              bool
	maxCount           int
	beforeContext      int
	afterContext       int
	includePatterns    []string
	excludePatterns    []string
	excludeDirPatterns []string
	pattern            *string
	files              []string
}

type grepMatch struct {
	start int
	end   int
}

type grepSource struct {
	name     string
	content  string
	filename string
}

func commandGrep(_ context.Context, args []string, c *CommandContext) int {
	return runGrep(args, c)
}

func commandFGrep(_ context.Context, args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		if info, ok := commandhelp.Lookup("fgrep"); ok {
			return commandhelp.Show(c, info)
		}
	}
	fgrepArgs := make([]string, 0, len(args)+1)
	fgrepArgs = append(fgrepArgs, "-F")
	fgrepArgs = append(fgrepArgs, args...)
	return runGrep(fgrepArgs, c)
}

func runGrep(args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		if info, ok := commandhelp.Lookup("grep"); ok {
			return commandhelp.Show(c, info)
		}
	}

	opts, code := parseGrepArgs(args, c)
	if code != 0 {
		return code
	}
	if opts.pattern == nil {
		fmt.Fprint(c.Stderr, "grep: missing pattern\n")
		return 2
	}

	matcher, err := newGrepMatcher(*opts.pattern, opts)
	if err != nil {
		fmt.Fprintf(c.Stderr, "grep: invalid regular expression: %s\n", *opts.pattern)
		return 2
	}

	sources, anyError := grepSources(opts, c)
	if len(sources) == 0 && len(opts.files) == 0 && !anyError {
		fmt.Fprint(c.Stderr, "grep: no input files\n")
		return 2
	}

	showFilename := len(sources) > 1 && !opts.noFilename
	anyMatch := false
	filesWithoutMatchPrinted := false

	for _, source := range sources {
		result := searchGrepContent(source.content, matcher, opts, source.name, showFilename)
		if result.matched {
			anyMatch = true
			if opts.quiet {
				return 0
			}
		}

		if opts.quiet {
			continue
		}

		if opts.filesWithMatches {
			if result.matched && source.filename != "" {
				fmt.Fprintln(c.Stdout, source.filename)
			}
			continue
		}
		if opts.filesWithoutMatch {
			if !result.matched && source.filename != "" {
				filesWithoutMatchPrinted = true
				fmt.Fprintln(c.Stdout, source.filename)
			}
			continue
		}
		fmt.Fprint(c.Stdout, result.output)
	}

	if anyError {
		return 2
	}
	if opts.filesWithoutMatch {
		if filesWithoutMatchPrinted {
			return 0
		}
		return 1
	}
	if anyMatch {
		return 0
	}
	return 1
}

func parseGrepArgs(args []string, c *CommandContext) (grepOptions, int) {
	var opts grepOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") && arg != "-" {
			switch {
			case arg == "-e":
				if i+1 >= len(args) {
					return opts, commandhelp.UnknownOption(c, "grep", arg)
				}
				i++
				pattern := args[i]
				opts.pattern = &pattern
				continue
			case strings.HasPrefix(arg, "--include="):
				opts.includePatterns = append(opts.includePatterns, strings.TrimPrefix(arg, "--include="))
				continue
			case strings.HasPrefix(arg, "--exclude="):
				opts.excludePatterns = append(opts.excludePatterns, strings.TrimPrefix(arg, "--exclude="))
				continue
			case strings.HasPrefix(arg, "--exclude-dir="):
				opts.excludeDirPatterns = append(opts.excludeDirPatterns, strings.TrimPrefix(arg, "--exclude-dir="))
				continue
			case strings.HasPrefix(arg, "--max-count="):
				opts.maxCount, _ = strconv.Atoi(strings.TrimPrefix(arg, "--max-count="))
				continue
			}

			if m := regexp.MustCompile(`^-m(\d+)$`).FindStringSubmatch(arg); m != nil {
				opts.maxCount, _ = strconv.Atoi(m[1])
				continue
			}
			if arg == "-m" {
				if i+1 >= len(args) {
					return opts, commandhelp.UnknownOption(c, "grep", arg)
				}
				i++
				opts.maxCount, _ = strconv.Atoi(args[i])
				continue
			}
			if m := regexp.MustCompile(`^-([ABC])(\d+)$`).FindStringSubmatch(arg); m != nil {
				setGrepContext(&opts, m[1], m[2])
				continue
			}
			if arg == "-A" || arg == "-B" || arg == "-C" {
				if i+1 >= len(args) {
					return opts, commandhelp.UnknownOption(c, "grep", arg)
				}
				i++
				setGrepContext(&opts, strings.TrimPrefix(arg, "-"), args[i])
				continue
			}

			flags := []string{arg}
			if !strings.HasPrefix(arg, "--") {
				flags = strings.Split(strings.TrimPrefix(arg, "-"), "")
			}
			for _, flag := range flags {
				switch flag {
				case "i", "--ignore-case":
					opts.ignoreCase = true
				case "n", "--line-number":
					opts.showLineNumbers = true
				case "v", "--invert-match":
					opts.invertMatch = true
				case "c", "--count":
					opts.countOnly = true
				case "l", "--files-with-matches":
					opts.filesWithMatches = true
				case "L", "--files-without-match":
					opts.filesWithoutMatch = true
				case "r", "R", "--recursive":
					opts.recursive = true
				case "w", "--word-regexp":
					opts.wholeWord = true
				case "x", "--line-regexp":
					opts.lineRegexp = true
				case "E", "--extended-regexp", "P", "--perl-regexp":
					// Go's regexp engine is already closest to the extended mode used by gash.
				case "F", "--fixed-strings":
					opts.fixedStrings = true
				case "o", "--only-matching":
					opts.onlyMatching = true
				case "h", "--no-filename":
					opts.noFilename = true
				case "q", "--quiet", "--silent":
					opts.quiet = true
				default:
					if strings.HasPrefix(flag, "--") {
						return opts, commandhelp.UnknownOption(c, "grep", flag)
					}
					return opts, commandhelp.UnknownOption(c, "grep", "-"+flag)
				}
			}
			continue
		}

		if opts.pattern == nil {
			pattern := arg
			opts.pattern = &pattern
		} else {
			opts.files = append(opts.files, arg)
		}
	}
	return opts, 0
}

func setGrepContext(opts *grepOptions, flag, value string) {
	n, _ := strconv.Atoi(value)
	switch flag {
	case "A":
		opts.afterContext = n
	case "B":
		opts.beforeContext = n
	case "C":
		opts.beforeContext = n
		opts.afterContext = n
	}
}

type grepMatcher struct {
	pattern string
	re      *regexp.Regexp
	opts    grepOptions
}

func newGrepMatcher(pattern string, opts grepOptions) (grepMatcher, error) {
	matcher := grepMatcher{pattern: pattern, opts: opts}
	if opts.fixedStrings {
		return matcher, nil
	}
	rePattern := pattern
	if opts.wholeWord {
		rePattern = `\b(?:` + rePattern + `)\b`
	}
	if opts.lineRegexp {
		rePattern = `^` + rePattern + `$`
	}
	if opts.ignoreCase {
		rePattern = "(?i)" + rePattern
	}
	re, err := regexp.Compile(rePattern)
	matcher.re = re
	return matcher, err
}

func (m grepMatcher) findAll(line string) []grepMatch {
	if m.opts.fixedStrings {
		return m.findAllFixed(line)
	}
	indexes := m.re.FindAllStringIndex(line, -1)
	matches := make([]grepMatch, 0, len(indexes))
	for _, idx := range indexes {
		matches = append(matches, grepMatch{start: idx[0], end: idx[1]})
	}
	return matches
}

func (m grepMatcher) matches(line string) bool {
	return len(m.findAll(line)) > 0
}

func (m grepMatcher) findAllFixed(line string) []grepMatch {
	pattern := m.pattern
	haystack := line
	if m.opts.ignoreCase {
		pattern = strings.ToLower(pattern)
		haystack = strings.ToLower(line)
	}
	if m.opts.lineRegexp {
		if haystack == pattern {
			return []grepMatch{{start: 0, end: len(line)}}
		}
		return nil
	}
	if pattern == "" {
		return []grepMatch{{start: 0, end: 0}}
	}
	var matches []grepMatch
	start := 0
	for start <= len(haystack) {
		idx := strings.Index(haystack[start:], pattern)
		if idx < 0 {
			break
		}
		matchStart := start + idx
		matchEnd := matchStart + len(pattern)
		if !m.opts.wholeWord || fixedWordBoundary(line, matchStart, matchEnd) {
			matches = append(matches, grepMatch{start: matchStart, end: matchEnd})
		}
		if matchEnd == start {
			start++
		} else {
			start = matchEnd
		}
	}
	return matches
}

func fixedWordBoundary(line string, start, end int) bool {
	return (start == 0 || !grepWordByteBefore(line, start)) && (end == len(line) || !grepWordByteAt(line, end))
}

func grepWordByteBefore(s string, end int) bool {
	if end <= 0 || end > len(s) {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:end])
	return isGrepWordRune(r)
}

func grepWordByteAt(s string, start int) bool {
	if start < 0 || start >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[start:])
	return isGrepWordRune(r)
}

func isGrepWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

type grepSearchResult struct {
	output  string
	matched bool
	count   int
}

func searchGrepContent(content string, matcher grepMatcher, opts grepOptions, sourceName string, showFilename bool) grepSearchResult {
	lines := strings.Split(content, "\n")
	lastIdx := len(lines)
	if lastIdx > 0 && lines[lastIdx-1] == "" {
		lastIdx--
	}

	matching := make([]bool, lastIdx)
	lineMatches := make([][]grepMatch, lastIdx)
	matchCount := 0
	for i := 0; i < lastIdx; i++ {
		if opts.maxCount > 0 && matchCount >= opts.maxCount {
			break
		}
		matches := matcher.findAll(lines[i])
		isMatch := len(matches) > 0
		if opts.invertMatch {
			isMatch = !isMatch
		}
		if isMatch {
			matching[i] = true
			lineMatches[i] = matches
			matchCount++
		}
	}

	if opts.countOnly {
		return grepSearchResult{output: fmt.Sprintf("%s%d\n", grepPrefix(sourceName, showFilename, 0, false), matchCount), matched: matchCount > 0, count: matchCount}
	}

	if opts.onlyMatching && opts.beforeContext == 0 && opts.afterContext == 0 {
		var out strings.Builder
		for i := 0; i < lastIdx; i++ {
			if !matching[i] || opts.invertMatch {
				continue
			}
			for _, match := range lineMatches[i] {
				fmt.Fprintf(&out, "%s%s\n", grepPrefix(sourceName, showFilename, i+1, opts.showLineNumbers), lines[i][match.start:match.end])
			}
		}
		return grepSearchResult{output: out.String(), matched: matchCount > 0, count: matchCount}
	}

	var out strings.Builder
	if opts.beforeContext == 0 && opts.afterContext == 0 {
		for i := 0; i < lastIdx; i++ {
			if matching[i] {
				fmt.Fprintf(&out, "%s%s\n", grepPrefix(sourceName, showFilename, i+1, opts.showLineNumbers), lines[i])
			}
		}
		return grepSearchResult{output: out.String(), matched: matchCount > 0, count: matchCount}
	}

	printed := map[int]bool{}
	lastPrinted := -1
	for i := 0; i < lastIdx; i++ {
		if !matching[i] {
			continue
		}
		start := i - opts.beforeContext
		if start < 0 {
			start = 0
		}
		end := i + opts.afterContext
		if end >= lastIdx {
			end = lastIdx - 1
		}
		if lastPrinted >= 0 && start > lastPrinted+1 {
			out.WriteString("--\n")
		}
		for lineNo := start; lineNo <= end; lineNo++ {
			if printed[lineNo] {
				continue
			}
			printed[lineNo] = true
			lastPrinted = lineNo
			sep := ':'
			if lineNo != i {
				sep = '-'
			}
			fmt.Fprintf(&out, "%s%s\n", grepContextPrefix(sourceName, showFilename, lineNo+1, opts.showLineNumbers, sep), lines[lineNo])
		}
	}
	return grepSearchResult{output: out.String(), matched: matchCount > 0, count: matchCount}
}

func grepPrefix(filename string, showFilename bool, lineNumber int, showLineNumber bool) string {
	var prefix strings.Builder
	if showFilename && filename != "" {
		prefix.WriteString(filename)
		prefix.WriteByte(':')
	}
	if showLineNumber && lineNumber > 0 {
		fmt.Fprintf(&prefix, "%d:", lineNumber)
	}
	return prefix.String()
}

func grepContextPrefix(filename string, showFilename bool, lineNumber int, showLineNumber bool, sep rune) string {
	var prefix strings.Builder
	if showFilename && filename != "" {
		prefix.WriteString(filename)
		prefix.WriteRune(sep)
	}
	if showLineNumber {
		fmt.Fprintf(&prefix, "%d%c", lineNumber, sep)
	}
	return prefix.String()
}

func grepSources(opts grepOptions, c *CommandContext) ([]grepSource, bool) {
	if len(opts.files) == 0 {
		data, err := io.ReadAll(c.Stdin)
		if err != nil {
			fmt.Fprintf(c.Stderr, "grep: %v\n", err)
			return nil, true
		}
		return []grepSource{{content: string(data)}}, false
	}

	files := expandGrepFiles(opts, c)
	sources := make([]grepSource, 0, len(files))
	anyError := false
	for _, name := range files {
		if name == "-" {
			data, err := io.ReadAll(c.Stdin)
			if err != nil {
				fmt.Fprintf(c.Stderr, "grep: %v\n", err)
				anyError = true
				continue
			}
			sources = append(sources, grepSource{name: "-", filename: "-", content: string(data)})
			continue
		}
		absName := abs(c, name)
		if info, err := gfs.Stat(c.FS, absName); err == nil && info.IsDir() {
			fmt.Fprintf(c.Stderr, "grep: %s: Is a directory\n", name)
			continue
		}
		data, err := gfs.ReadFile(c.FS, absName)
		if err != nil {
			fmt.Fprintf(c.Stderr, "grep: %s: No such file or directory\n", name)
			anyError = true
			continue
		}
		sources = append(sources, grepSource{name: name, filename: name, content: string(data)})
	}
	return sources, anyError
}

func expandGrepFiles(opts grepOptions, c *CommandContext) []string {
	var files []string
	for _, name := range opts.files {
		if name == "-" {
			files = append(files, name)
			continue
		}
		if opts.recursive {
			files = append(files, expandGrepRecursive(c, name, 0, opts)...)
			continue
		}
		if hasGlobMeta(name) {
			files = append(files, expandGrepGlob(c, name)...)
			continue
		}
		if grepFileAllowed(name, opts) {
			files = append(files, name)
		}
	}
	return files
}

const maxGrepDepth = 256

func expandGrepRecursive(c *CommandContext, name string, depth int, opts grepOptions) []string {
	if depth >= maxGrepDepth || name == "-" {
		return nil
	}
	info, err := gfs.Stat(c.FS, abs(c, name))
	if err != nil {
		return []string{name}
	}
	if !info.IsDir() {
		if grepFileAllowed(name, opts) {
			return []string{name}
		}
		return nil
	}
	base := path.Base(name)
	if grepAnyGlobMatch(base, opts.excludeDirPatterns) {
		return nil
	}
	entries, err := gfs.ReadDir(c.FS, abs(c, name))
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		out = append(out, expandGrepRecursive(c, path.Join(name, entry.Name()), depth+1, opts)...)
	}
	return out
}

func expandGrepGlob(c *CommandContext, pattern string) []string {
	dir, glob := path.Split(pattern)
	if dir == "" {
		dir = "."
	} else {
		dir = strings.TrimSuffix(dir, "/")
	}
	entries, err := gfs.ReadDir(c.FS, abs(c, dir))
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		matched, err := path.Match(glob, entry.Name())
		if err == nil && matched {
			if dir == "." {
				out = append(out, entry.Name())
			} else {
				out = append(out, path.Join(dir, entry.Name()))
			}
		}
	}
	sort.Strings(out)
	return out
}

func grepFileAllowed(name string, opts grepOptions) bool {
	base := path.Base(name)
	if grepAnyGlobMatch(base, opts.excludePatterns) {
		return false
	}
	if len(opts.includePatterns) > 0 && !grepAnyGlobMatch(base, opts.includePatterns) {
		return false
	}
	return true
}

func grepAnyGlobMatch(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, err := path.Match(pattern, name); err == nil && matched {
			return true
		}
		if pattern == name {
			return true
		}
	}
	return false
}

func hasGlobMeta(name string) bool {
	return strings.ContainsAny(name, "*?[")
}
