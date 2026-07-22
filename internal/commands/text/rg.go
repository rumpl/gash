package text

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	iofs "io/fs"
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

type rgOptions struct {
	ignoreCase       bool
	caseSensitive    bool
	smartCase        bool
	fixedStrings     bool
	wordRegexp       bool
	lineRegexp       bool
	invertMatch      bool
	patterns         []string
	patternFiles     []string
	count            bool
	countMatches     bool
	files            bool
	filesWithMatches bool
	filesWithout     bool
	stats            bool
	onlyMatching     bool
	maxCount         int
	lineNumber       bool
	noFilename       bool
	withFilename     bool
	nullSeparator    bool
	byteOffset       bool
	column           bool
	vimgrep          bool
	json             bool
	quiet            bool
	afterContext     int
	beforeContext    int
	contextSeparator string
	globs            []string
	iglobs           []string
	globIgnoreCase   bool
	types            []string
	typesNot         []string
	typeAdd          []string
	typeClear        []string
	hidden           bool
	noIgnore         bool
	noIgnoreDot      bool
	noIgnoreVCS      bool
	ignoreFiles      []string
	maxDepth         int
	maxFilesize      int64
	followSymlinks   bool
	searchBinary     bool
	passthru         bool
	includeZero      bool
	heading          bool
	sort             string
	replace          *string
	searchZip        bool
	preprocessor     string
	preGlobs         []string
}

type rgParseResult struct {
	opts                rgOptions
	paths               []string
	explicitLineNumbers bool
	code                int
}

type rgFile struct {
	rel      string
	abs      string
	explicit bool
}

type rgLineMatch struct{ start, end int }

type rgFileResult struct {
	file       string
	output     string
	matched    bool
	matchCount int
	bytes      int
	content    string
}

func commandRg(ctx context.Context, args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		if info, ok := commandhelp.Lookup("rg"); ok {
			return commandhelp.Show(c, info)
		}
	}
	parsed := parseRgArgs(args, c)
	if parsed.code != 0 {
		return parsed.code
	}
	return runRg(ctx, parsed, c)
}

func defaultRgOptions() rgOptions {
	return rgOptions{smartCase: true, lineNumber: true, contextSeparator: "--", maxDepth: 256, maxFilesize: 512 * 1024 * 1024, sort: "path"}
}

func parseRgArgs(args []string, c *CommandContext) rgParseResult {
	opts := defaultRgOptions()
	var positionalPattern *string
	var paths []string
	explicitA, explicitB, explicitC := -1, -1, -1
	explicitLineNumbers := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") && arg != "-" {
			if m := regexp.MustCompile(`^-([ABC])(\d+)$`).FindStringSubmatch(arg); m != nil {
				v, _ := strconv.Atoi(m[2])
				switch m[1] {
				case "A":
					if v > explicitA {
						explicitA = v
					}
				case "B":
					if v > explicitB {
						explicitB = v
					}
				case "C":
					explicitC = v
				}
				continue
			}
			if arg == "-A" || arg == "-B" || arg == "-C" {
				if i+1 >= len(args) {
					return rgParseResult{code: commandhelp.UnknownOption(c, "rg", arg)}
				}
				i++
				v, _ := strconv.Atoi(args[i])
				switch arg {
				case "-A":
					if v > explicitA {
						explicitA = v
					}
				case "-B":
					if v > explicitB {
						explicitB = v
					}
				case "-C":
					explicitC = v
				}
				continue
			}
			if m := regexp.MustCompile(`^-m(\d+)$`).FindStringSubmatch(arg); m != nil {
				opts.maxCount, _ = strconv.Atoi(m[1])
				continue
			}
			if ok, ni, code := parseRgValueOption(args, i, &opts, c); ok {
				if code != 0 {
					return rgParseResult{code: code}
				}
				i = ni
				continue
			}
			if arg == "--sort" && i+1 < len(args) && (args[i+1] == "path" || args[i+1] == "none") {
				opts.sort = args[i+1]
				i++
				continue
			}
			if strings.HasPrefix(arg, "--sort=") {
				val := strings.TrimPrefix(arg, "--sort=")
				if val == "path" || val == "none" {
					opts.sort = val
					continue
				}
			}

			flags := []string{arg}
			if !strings.HasPrefix(arg, "--") {
				flags = strings.Split(strings.TrimPrefix(arg, "-"), "")
			}
			for _, flag := range flags {
				switch flag {
				case "n", "--line-number":
					opts.lineNumber = true
					explicitLineNumbers = true
				case "u", "--unrestricted":
					handleRgUnrestricted(&opts)
				case "P", "--pcre2":
					fmt.Fprint(c.Stderr, "rg: PCRE2 is not supported. Use standard regex syntax instead.\n")
					return rgParseResult{code: 1}
				case "i", "--ignore-case":
					opts.ignoreCase = true
					opts.caseSensitive = false
					opts.smartCase = false
				case "s", "--case-sensitive":
					opts.caseSensitive = true
					opts.ignoreCase = false
					opts.smartCase = false
				case "S", "--smart-case":
					opts.smartCase = true
					opts.ignoreCase = false
					opts.caseSensitive = false
				case "F", "--fixed-strings":
					opts.fixedStrings = true
				case "w", "--word-regexp":
					opts.wordRegexp = true
				case "x", "--line-regexp":
					opts.lineRegexp = true
				case "v", "--invert-match":
					opts.invertMatch = true
				case "c", "--count":
					opts.count = true
				case "--count-matches":
					opts.countMatches = true
				case "l", "--files-with-matches":
					opts.filesWithMatches = true
				case "--files-without-match":
					opts.filesWithout = true
				case "--files":
					opts.files = true
				case "--stats":
					opts.stats = true
				case "o", "--only-matching":
					opts.onlyMatching = true
				case "q", "--quiet":
					opts.quiet = true
				case "N", "--no-line-number":
					opts.lineNumber = false
				case "H", "--with-filename":
					opts.withFilename = true
				case "I", "--no-filename":
					opts.noFilename = true
				case "0", "--null":
					opts.nullSeparator = true
				case "b", "--byte-offset":
					opts.byteOffset = true
				case "--column":
					opts.column = true
					opts.lineNumber = true
				case "--no-column":
					opts.column = false
				case "--vimgrep":
					opts.vimgrep = true
					opts.column = true
					opts.lineNumber = true
				case "--json":
					opts.json = true
				case "--hidden":
					opts.hidden = true
				case "--no-ignore":
					opts.noIgnore = true
				case "--no-ignore-dot":
					opts.noIgnoreDot = true
				case "--no-ignore-vcs":
					opts.noIgnoreVCS = true
				case "L", "--follow":
					opts.followSymlinks = true
				case "z", "--search-zip":
					opts.searchZip = true
				case "a", "--text":
					opts.searchBinary = true
				case "--heading":
					opts.heading = true
				case "--passthru":
					opts.passthru = true
				case "--include-zero":
					opts.includeZero = true
				case "--glob-case-insensitive":
					opts.globIgnoreCase = true
				default:
					if len(flag) == 1 {
						return rgParseResult{code: commandhelp.UnknownOption(c, "rg", "-"+flag)}
					}
					return rgParseResult{code: commandhelp.UnknownOption(c, "rg", flag)}
				}
			}
			continue
		}
		if positionalPattern == nil && len(opts.patterns) == 0 && len(opts.patternFiles) == 0 {
			p := arg
			positionalPattern = &p
		} else {
			paths = append(paths, arg)
		}
	}
	if explicitA >= 0 || explicitC >= 0 {
		opts.afterContext = maxInt(ifNonneg(explicitA), ifNonneg(explicitC))
	}
	if explicitB >= 0 || explicitC >= 0 {
		opts.beforeContext = maxInt(ifNonneg(explicitB), ifNonneg(explicitC))
	}
	if positionalPattern != nil {
		opts.patterns = append(opts.patterns, *positionalPattern)
	}
	if opts.column || opts.vimgrep {
		explicitLineNumbers = true
	}
	return rgParseResult{opts: opts, paths: paths, explicitLineNumbers: explicitLineNumbers}
}

func parseRgValueOption(args []string, i int, opts *rgOptions, c *CommandContext) (bool, int, int) {
	arg := args[i]
	type def struct {
		short, long string
		apply       func(string) int
	}
	defs := []def{
		{"g", "glob", func(v string) int { opts.globs = append(opts.globs, v); return 0 }},
		{"", "iglob", func(v string) int { opts.iglobs = append(opts.iglobs, v); return 0 }},
		{"t", "type", func(v string) int { opts.types = append(opts.types, v); return 0 }},
		{"T", "type-not", func(v string) int { opts.typesNot = append(opts.typesNot, v); return 0 }},
		{"", "type-add", func(v string) int { opts.typeAdd = append(opts.typeAdd, v); return 0 }},
		{"", "type-clear", func(v string) int { opts.typeClear = append(opts.typeClear, v); return 0 }},
		{"m", "max-count", func(v string) int { opts.maxCount, _ = strconv.Atoi(v); return 0 }},
		{"e", "regexp", func(v string) int { opts.patterns = append(opts.patterns, v); return 0 }},
		{"f", "file", func(v string) int { opts.patternFiles = append(opts.patternFiles, v); return 0 }},
		{"r", "replace", func(v string) int { opts.replace = &v; return 0 }},
		{"d", "max-depth", func(v string) int {
			if !regexp.MustCompile(`^\d+$`).MatchString(v) {
				fmt.Fprintf(c.Stderr, "rg: invalid --max-depth value: %s\n", v)
				return 1
			}
			opts.maxDepth, _ = strconv.Atoi(v)
			return 0
		}},
		{"", "max-filesize", func(v string) int {
			n, ok := parseRgFilesize(v)
			if !ok {
				fmt.Fprintf(c.Stderr, "rg: invalid --max-filesize value: %s\n", v)
				return 1
			}
			opts.maxFilesize = n
			return 0
		}},
		{"", "context-separator", func(v string) int { opts.contextSeparator = v; return 0 }},
		{"j", "threads", func(string) int { return 0 }},
		{"", "ignore-file", func(v string) int { opts.ignoreFiles = append(opts.ignoreFiles, v); return 0 }},
		{"", "pre", func(v string) int { opts.preprocessor = v; return 0 }},
		{"", "pre-glob", func(v string) int { opts.preGlobs = append(opts.preGlobs, v); return 0 }},
	}
	for _, d := range defs {
		if d.long != "" && strings.HasPrefix(arg, "--"+d.long+"=") {
			return true, i, d.apply(strings.TrimPrefix(arg, "--"+d.long+"="))
		}
		if d.short != "" && strings.HasPrefix(arg, "-"+d.short) && len(arg) > 2 {
			return true, i, d.apply(arg[2:])
		}
		if (d.short != "" && arg == "-"+d.short) || (d.long != "" && arg == "--"+d.long) {
			if i+1 >= len(args) {
				return true, i, commandhelp.UnknownOption(c, "rg", arg)
			}
			return true, i + 1, d.apply(args[i+1])
		}
	}
	return false, i, 0
}

func runRg(ctx context.Context, parsed rgParseResult, c *CommandContext) int {
	opts := parsed.opts
	for _, glob := range opts.globs {
		if err := validateRgGlob(strings.TrimPrefix(glob, "!")); err != "" {
			fmt.Fprintln(c.Stderr, err)
			return 1
		}
	}
	if opts.files {
		return rgListFiles(ctx, append(append([]string{}, opts.patterns...), parsed.paths...), opts, c)
	}
	patterns := append([]string{}, opts.patterns...)
	for _, pf := range opts.patternFiles {
		data, err := rgReadNamedInput(pf, c)
		if err != nil {
			fmt.Fprintf(c.Stderr, "rg: %s: No such file or directory\n", pf)
			return 2
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				patterns = append(patterns, line)
			}
		}
	}
	if len(patterns) == 0 {
		if len(opts.patternFiles) > 0 {
			return 1
		}
		fmt.Fprint(c.Stderr, "rg: no pattern given\n")
		return 2
	}
	matcher, err := newRgMatcher(patterns, opts)
	if err != nil {
		fmt.Fprintf(c.Stderr, "rg: invalid regex: %s\n", strings.Join(patterns, ", "))
		return 2
	}

	if len(parsed.paths) == 0 && len(opts.patternFiles) == 0 {
		if data, _ := io.ReadAll(c.Stdin); len(data) > 0 {
			return rgSearchStdin(string(data), matcher, opts, c)
		}
	}
	paths := parsed.paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	files, singleExplicit := collectRgFiles(ctx, paths, opts, c)
	if len(files) == 0 {
		return 1
	}
	showFilename := !opts.noFilename && (opts.withFilename || !singleExplicit || len(files) > 1)
	effectiveLineNumbers := opts.lineNumber
	if !parsed.explicitLineNumbers {
		if singleExplicit && len(files) == 1 {
			effectiveLineNumbers = false
		}
		if opts.onlyMatching {
			effectiveLineNumbers = false
		}
	}
	return rgSearchFiles(ctx, files, matcher, opts, showFilename, effectiveLineNumbers, c)
}

type rgMatcher struct {
	patterns []string
	re       *regexp.Regexp
	opts     rgOptions
}

func newRgMatcher(patterns []string, opts rgOptions) (rgMatcher, error) {
	ignore := opts.ignoreCase
	if opts.caseSensitive {
		ignore = false
	} else if opts.smartCase && !opts.ignoreCase {
		ignore = true
		for _, p := range patterns {
			if hasUpperASCII(p) {
				ignore = false
				break
			}
		}
	}
	parts := make([]string, len(patterns))
	for i, p := range patterns {
		if opts.fixedStrings {
			p = regexp.QuoteMeta(p)
		}
		parts[i] = "(?:" + p + ")"
	}
	pat := strings.Join(parts, "|")
	if opts.wordRegexp {
		pat = `\b(?:` + pat + `)\b`
	}
	if opts.lineRegexp {
		pat = `^(?:` + pat + `)$`
	}
	if ignore {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	return rgMatcher{patterns: patterns, re: re, opts: opts}, err
}

func (m rgMatcher) findAll(line string) []rgLineMatch {
	idxs := m.re.FindAllStringIndex(line, -1)
	out := make([]rgLineMatch, 0, len(idxs))
	for _, idx := range idxs {
		out = append(out, rgLineMatch{idx[0], idx[1]})
	}
	return out
}

func rgSearchStdin(content string, matcher rgMatcher, opts rgOptions, c *CommandContext) int {
	res := searchRgContent(content, matcher, opts, "", false, opts.lineNumber)
	if opts.quiet {
		if res.matched {
			return 0
		}
		return 1
	}
	if opts.filesWithMatches {
		if res.matched {
			fmt.Fprint(c.Stdout, "(standard input)\n")
			return 0
		}
		return 1
	}
	if opts.filesWithout {
		if !res.matched {
			fmt.Fprint(c.Stdout, "(standard input)\n")
			return 0
		}
		return 1
	}
	fmt.Fprint(c.Stdout, res.output)
	if res.matched {
		return 0
	}
	return 1
}

func rgSearchFiles(ctx context.Context, files []rgFile, matcher rgMatcher, opts rgOptions, showFilename, showLineNumbers bool, c *CommandContext) int {
	anyMatch := false
	filesWithMatch := 0
	totalMatches := 0
	bytesSearched := 0
	var out strings.Builder
	var jsonLines []string
	for _, file := range files {
		if ctx.Err() != nil {
			fmt.Fprintf(c.Stderr, "rg: %v\n", ctx.Err())
			return 2
		}
		data, err := gfs.ReadFile(c.FS, file.abs)
		if err != nil {
			continue
		}
		content := string(data)
		bytesSearched += len(content)
		if isRgBinary(data) && !opts.searchBinary {
			continue
		}
		res := searchRgContent(content, matcher, opts, filenameIf(showFilename && !opts.heading, file.rel), showFilename && !opts.heading, showLineNumbers)
		res.file = file.rel
		res.content = content
		res.bytes = len(content)
		if res.matched {
			anyMatch = true
			filesWithMatch++
			totalMatches += res.matchCount
			if opts.quiet && !opts.json {
				break
			}
			if opts.json && !opts.quiet {
				appendRgJSON(&jsonLines, res, matcher, opts)
			} else if opts.filesWithMatches {
				out.WriteString(file.rel)
				out.WriteString(rgSep(opts))
			} else if !opts.filesWithout {
				if opts.heading && !opts.noFilename {
					fmt.Fprintf(&out, "%s\n", file.rel)
				}
				out.WriteString(res.output)
			}
		} else if opts.filesWithout {
			out.WriteString(file.rel)
			out.WriteString(rgSep(opts))
		} else if opts.includeZero && (opts.count || opts.countMatches) {
			out.WriteString(res.output)
		}
	}
	if opts.json {
		jsonLines = append(jsonLines, rgSummaryJSON(len(files), filesWithMatch, bytesSearched, totalMatches))
		out.Reset()
		out.WriteString(strings.Join(jsonLines, "\n"))
		out.WriteByte('\n')
	}
	final := out.String()
	if opts.quiet && !opts.json {
		final = ""
	}
	if opts.stats && !opts.json {
		final += fmt.Sprintf("\n%d matches\n%d matched lines\n%d files contained matches\n%d files searched\n%d bytes searched\n", totalMatches, totalMatches, filesWithMatch, len(files), bytesSearched)
	}
	fmt.Fprint(c.Stdout, final)
	if opts.filesWithout {
		if out.Len() > 0 {
			return 0
		}
		return 1
	}
	if anyMatch {
		return 0
	}
	return 1
}

func searchRgContent(content string, matcher rgMatcher, opts rgOptions, filename string, showFilename, showLineNumbers bool) rgFileResult {
	lines := strings.Split(content, "\n")
	last := len(lines)
	if last > 0 && lines[last-1] == "" {
		last--
	}
	matching := make([]bool, last)
	lineMatches := make([][]rgLineMatch, last)
	matchLines := 0
	matchCount := 0
	for i := 0; i < last; i++ {
		if opts.maxCount > 0 && matchLines >= opts.maxCount {
			break
		}
		ms := matcher.findAll(lines[i])
		is := len(ms) > 0
		if opts.invertMatch {
			is = !is
		}
		if is {
			matching[i] = true
			lineMatches[i] = ms
			matchLines++
			if opts.invertMatch {
				matchCount++
			} else {
				matchCount += len(ms)
			}
		}
	}
	if opts.count || opts.countMatches {
		n := matchLines
		if opts.countMatches {
			n = matchCount
		}
		if !opts.includeZero && n == 0 {
			return rgFileResult{matched: false, matchCount: n}
		}
		return rgFileResult{output: fmt.Sprintf("%s%d\n", rgPrefix(filename, showFilename, 0, false, false, false, 0, 0), n), matched: n > 0, matchCount: n}
	}
	if opts.onlyMatching && opts.beforeContext == 0 && opts.afterContext == 0 {
		var out strings.Builder
		for i := 0; i < last; i++ {
			if !matching[i] || opts.invertMatch {
				continue
			}
			for _, m := range lineMatches[i] {
				fmt.Fprintf(&out, "%s%s\n", rgPrefix(filename, showFilename, i+1, showLineNumbers, opts.column, opts.byteOffset, lineByteOffset(lines, i)+m.start, rgColumn(lines[i], m.start)), lines[i][m.start:m.end])
			}
		}
		return rgFileResult{output: out.String(), matched: matchLines > 0, matchCount: matchCount}
	}
	if opts.beforeContext == 0 && opts.afterContext == 0 {
		var out strings.Builder
		for i := 0; i < last; i++ {
			if matching[i] {
				text := lines[i]
				if opts.replace != nil && !opts.invertMatch {
					text = matcher.re.ReplaceAllString(text, *opts.replace)
				}
				fmt.Fprintf(&out, "%s%s\n", rgPrefix(filename, showFilename, i+1, showLineNumbers, opts.column, opts.byteOffset, lineByteOffset(lines, i), rgFirstColumn(lines[i], lineMatches[i])), text)
			} else if opts.passthru {
				fmt.Fprintf(&out, "%s\n", lines[i])
			}
		}
		return rgFileResult{output: out.String(), matched: matchLines > 0, matchCount: matchCount}
	}
	var out strings.Builder
	printed := map[int]bool{}
	lastPrinted := -1
	for i := 0; i < last; i++ {
		if !matching[i] {
			continue
		}
		start := maxInt(0, i-opts.beforeContext)
		end := minInt(last-1, i+opts.afterContext)
		if lastPrinted >= 0 && start > lastPrinted+1 {
			out.WriteString(opts.contextSeparator + "\n")
		}
		for ln := start; ln <= end; ln++ {
			if printed[ln] {
				continue
			}
			printed[ln] = true
			lastPrinted = ln
			sep := ':'
			if ln != i {
				sep = '-'
			}
			fmt.Fprintf(&out, "%s%s\n", rgContextPrefix(filename, showFilename, ln+1, showLineNumbers, sep), lines[ln])
		}
	}
	return rgFileResult{output: out.String(), matched: matchLines > 0, matchCount: matchCount}
}

func collectRgFiles(ctx context.Context, paths []string, opts rgOptions, c *CommandContext) ([]rgFile, bool) {
	var files []rgFile
	explicitFiles, dirs := 0, 0
	ignores := loadRgIgnores(paths, opts, c)
	types := newRgTypeRegistry(opts)
	for _, p := range paths {
		if ctx.Err() != nil {
			break
		}
		ap := abs(c, p)
		info, err := gfs.Stat(c.FS, ap)
		if err != nil {
			continue
		}
		if info.IsDir() {
			dirs++
			walkRgDir(ctx, p, ap, 0, opts, ignores, types, &files, c)
		} else {
			explicitFiles++
			if opts.maxFilesize > 0 && info.Size() > opts.maxFilesize {
				continue
			}
			if shouldIncludeRgFile(p, opts, ignores, types, false) {
				files = append(files, rgFile{rel: p, abs: ap, explicit: true})
			}
		}
	}
	if opts.sort == "path" {
		sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	}
	return files, explicitFiles == 1 && dirs == 0
}

func walkRgDir(ctx context.Context, rel, ap string, depth int, opts rgOptions, ignores []rgIgnorePattern, types rgTypeRegistry, files *[]rgFile, c *CommandContext) {
	if ctx.Err() != nil || depth >= opts.maxDepth {
		return
	}
	entries, err := gfs.ReadDir(c.FS, ap)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if commonRgIgnored(name) && !opts.noIgnore {
			continue
		}
		erel := path.Join(rel, name)
		if rel == "." {
			erel = name
		}
		eabs := path.Join(ap, name)
		info, err := rgEntryInfo(c, eabs, e, opts.followSymlinks)
		if err != nil {
			continue
		}
		isDir := info.IsDir()
		if rgIgnored(erel, isDir, ignores) {
			continue
		}
		if strings.HasPrefix(name, ".") && !opts.hidden && !rgWhitelisted(erel, isDir, ignores) {
			continue
		}
		if isDir {
			walkRgDir(ctx, erel, eabs, depth+1, opts, ignores, types, files, c)
			continue
		}
		if opts.maxFilesize > 0 && info.Size() > opts.maxFilesize {
			continue
		}
		if shouldIncludeRgFile(erel, opts, ignores, types, false) {
			*files = append(*files, rgFile{rel: erel, abs: eabs})
		}
	}
}

func rgEntryInfo(c *CommandContext, p string, e iofs.DirEntry, follow bool) (iofs.FileInfo, error) {
	if e.Type()&iofs.ModeSymlink != 0 && !follow {
		return nil, fmt.Errorf("symlink")
	}
	return gfs.Stat(c.FS, p)
}

func shouldIncludeRgFile(rel string, opts rgOptions, ignores []rgIgnorePattern, types rgTypeRegistry, isDir bool) bool {
	if rgIgnored(rel, isDir, ignores) {
		return false
	}
	filename := path.Base(rel)
	if len(opts.types) > 0 && !types.matches(filename, opts.types) {
		return false
	}
	if len(opts.typesNot) > 0 && types.matches(filename, opts.typesNot) {
		return false
	}
	if !matchRgGlobFilters(filename, rel, opts.globs, opts.globIgnoreCase) {
		return false
	}
	if !matchRgGlobFilters(filename, rel, opts.iglobs, true) {
		return false
	}
	return true
}

func rgListFiles(ctx context.Context, paths []string, opts rgOptions, c *CommandContext) int {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	files, _ := collectRgFiles(ctx, paths, opts, c)
	if len(files) == 0 {
		return 1
	}
	if opts.quiet {
		return 0
	}
	sep := rgSep(opts)
	for _, f := range files {
		fmt.Fprint(c.Stdout, f.rel, sep)
	}
	return 0
}

func rgReadNamedInput(name string, c *CommandContext) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(c.Stdin)
	}
	return gfs.ReadFile(c.FS, abs(c, name))
}

func parseRgFilesize(v string) (int64, bool) {
	m := regexp.MustCompile(`^(\d+)([KMGkmg])?$`).FindStringSubmatch(v)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	switch strings.ToUpper(m[2]) {
	case "K":
		n *= 1024
	case "M":
		n *= 1024 * 1024
	case "G":
		n *= 1024 * 1024 * 1024
	}
	return n, true
}

func validateRgGlob(g string) string {
	in := false
	for _, r := range g {
		if r == '[' && !in {
			in = true
		} else if r == ']' && in {
			in = false
		}
	}
	if in {
		return fmt.Sprintf("rg: glob '%s' has an unclosed character class", g)
	}
	return ""
}

func handleRgUnrestricted(o *rgOptions) {
	if o.hidden {
		o.searchBinary = true
	} else if o.noIgnore {
		o.hidden = true
	} else {
		o.noIgnore = true
	}
}

func hasUpperASCII(s string) bool {
	for _, r := range s {
		if 'A' <= r && r <= 'Z' {
			return true
		}
	}
	return false
}

func isRgBinary(data []byte) bool {
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	return false
}

func rgSep(o rgOptions) string {
	if o.nullSeparator {
		return "\x00"
	}
	return "\n"
}

func filenameIf(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}

func ifNonneg(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func lineByteOffset(lines []string, idx int) int {
	off := 0
	for i := 0; i < idx; i++ {
		off += len(lines[i]) + 1
	}
	return off
}

func rgPrefix(filename string, showFilename bool, lineNo int, showLine, showColumn, showByte bool, byteOff, column int) string {
	var b strings.Builder
	if showFilename && filename != "" {
		b.WriteString(filename)
		b.WriteByte(':')
	}
	if showLine && lineNo > 0 {
		fmt.Fprintf(&b, "%d:", lineNo)
	}
	if showColumn && lineNo > 0 {
		if column <= 0 {
			column = 1
		}
		fmt.Fprintf(&b, "%d:", column)
	}
	if showByte {
		fmt.Fprintf(&b, "%d:", byteOff)
	}
	return b.String()
}

func rgContextPrefix(filename string, showFilename bool, lineNo int, showLine bool, sep rune) string {
	var b strings.Builder
	if showFilename && filename != "" {
		b.WriteString(filename)
		b.WriteRune(sep)
	}
	if showLine {
		fmt.Fprintf(&b, "%d%c", lineNo, sep)
	}
	return b.String()
}

// Deferred just-bash rg differences: gzip decompression (-z), multiline matching,
// external preprocessors (--pre), and full ripgrep JSON/stat metadata are not
// implemented; gash keeps searching within its virtual filesystem with bounded,
// deterministic recursive traversal and Go regexp syntax.

type rgIgnorePattern struct {
	pattern                  string
	negated, dirOnly, rooted bool
}

func loadRgIgnores(paths []string, opts rgOptions, c *CommandContext) []rgIgnorePattern {
	if opts.noIgnore {
		return nil
	}
	var out []rgIgnorePattern
	roots := []string{"."}
	if len(paths) > 0 {
		roots = paths
	}
	seen := map[string]bool{}
	for _, r := range roots {
		ap := abs(c, r)
		info, err := gfs.Stat(c.FS, ap)
		if err == nil && !info.IsDir() {
			ap = path.Dir(ap)
		}
		collectRgIgnoreFiles(c, ap, opts, seen, &out)
	}
	for _, custom := range opts.ignoreFiles {
		if data, err := gfs.ReadFile(c.FS, abs(c, custom)); err == nil {
			out = append(out, parseRgIgnore(string(data))...)
		}
	}
	return out
}

func collectRgIgnoreFiles(c *CommandContext, dir string, opts rgOptions, seen map[string]bool, out *[]rgIgnorePattern) {
	if seen[dir] {
		return
	}
	seen[dir] = true
	if !opts.noIgnoreVCS {
		if data, err := gfs.ReadFile(c.FS, path.Join(dir, ".gitignore")); err == nil {
			*out = append(*out, parseRgIgnore(string(data))...)
		}
	}
	if !opts.noIgnoreDot {
		if data, err := gfs.ReadFile(c.FS, path.Join(dir, ".ignore")); err == nil {
			*out = append(*out, parseRgIgnore(string(data))...)
		}
	}
	entries, err := gfs.ReadDir(c.FS, dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			collectRgIgnoreFiles(c, path.Join(dir, e.Name()), opts, seen, out)
		}
	}
}

func parseRgIgnore(content string) []rgIgnorePattern {
	var out []rgIgnorePattern
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		p := rgIgnorePattern{pattern: trimmed}
		if strings.HasPrefix(trimmed, "!") {
			p.negated = true
			trimmed = trimmed[1:]
		}
		if strings.HasSuffix(trimmed, "/") {
			p.dirOnly = true
			trimmed = strings.TrimSuffix(trimmed, "/")
		}
		if strings.HasPrefix(trimmed, "/") {
			p.rooted = true
			trimmed = trimmed[1:]
		} else if strings.Contains(trimmed, "/") && !strings.HasPrefix(trimmed, "**/") {
			p.rooted = true
		}
		p.pattern = trimmed
		out = append(out, p)
	}
	return out
}

func rgIgnored(rel string, isDir bool, patterns []rgIgnorePattern) bool {
	ignored := false
	for _, p := range patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if rgGitignoreMatch(rel, p) {
			ignored = !p.negated
		}
	}
	return ignored
}

func rgWhitelisted(rel string, isDir bool, patterns []rgIgnorePattern) bool {
	ok := false
	for _, p := range patterns {
		if p.negated && (!p.dirOnly || isDir) && rgGitignoreMatch(rel, p) {
			ok = true
		}
	}
	return ok
}

func rgGitignoreMatch(rel string, p rgIgnorePattern) bool {
	rel = strings.TrimPrefix(rel, "./")
	if p.rooted {
		return matchRgGlob(rel, p.pattern, false)
	}
	base := path.Base(rel)
	return matchRgGlob(base, p.pattern, false) || matchRgGlob(rel, p.pattern, false)
}

func commonRgIgnored(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor":
		return true
	}
	return false
}

func matchRgGlobFilters(filename, rel string, globs []string, ignoreCase bool) bool {
	if len(globs) == 0 {
		return true
	}
	pos := []string{}
	neg := []string{}
	for _, g := range globs {
		if strings.HasPrefix(g, "!") {
			neg = append(neg, g[1:])
		} else {
			pos = append(pos, g)
		}
	}
	if len(pos) > 0 {
		matched := false
		for _, g := range pos {
			if matchRgGlob(filename, g, ignoreCase) || matchRgGlob(rel, g, ignoreCase) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, g := range neg {
		if strings.HasPrefix(g, "/") {
			if matchRgGlob(rel, g[1:], ignoreCase) {
				return false
			}
		} else if matchRgGlob(filename, g, ignoreCase) || matchRgGlob(rel, g, ignoreCase) {
			return false
		}
	}
	return true
}

func matchRgGlob(s, pattern string, ignoreCase bool) bool {
	if ignoreCase {
		s = strings.ToLower(s)
		pattern = strings.ToLower(pattern)
	}
	var re strings.Builder
	re.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				re.WriteString(".*")
				i++
			} else {
				re.WriteString("[^/]*")
			}
		case '?':
			re.WriteString("[^/]")
		case '[':
			j := i + 1
			if j < len(pattern) && pattern[j] == '!' {
				j++
			}
			if j < len(pattern) && pattern[j] == ']' {
				j++
			}
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j < len(pattern) {
				cc := pattern[i : j+1]
				if strings.HasPrefix(cc, "[!") {
					cc = "[^" + cc[2:]
				}
				re.WriteString(cc)
				i = j
			} else {
				re.WriteString(`\[`)
			}
		default:
			re.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	re.WriteByte('$')
	ok, _ := regexp.MatchString(re.String(), s)
	return ok
}

type (
	rgTypeRegistry map[string]rgType
	rgType         struct{ exts, globs []string }
)

func newRgTypeRegistry(opts rgOptions) rgTypeRegistry {
	r := rgBuiltinTypes()
	for _, n := range opts.typeClear {
		delete(r, n)
	}
	for _, spec := range opts.typeAdd {
		r.add(spec)
	}
	return r
}

func rgBuiltinTypes() rgTypeRegistry {
	return rgTypeRegistry{
		"js": {[]string{".js", ".mjs", ".cjs", ".jsx"}, nil}, "ts": {[]string{".ts", ".tsx", ".mts", ".cts"}, nil}, "go": {[]string{".go"}, nil}, "py": {[]string{".py", ".pyi", ".pyw"}, nil}, "rs": {[]string{".rs"}, nil}, "rust": {[]string{".rs"}, nil}, "json": {[]string{".json", ".jsonc", ".json5"}, nil}, "md": {[]string{".md", ".mdx", ".markdown", ".mdown", ".mkd"}, nil}, "markdown": {[]string{".md", ".mdx", ".markdown", ".mdown", ".mkd"}, nil}, "txt": {[]string{".txt", ".text"}, nil}, "sh": {[]string{".sh", ".bash", ".zsh", ".fish"}, []string{".bashrc", ".zshrc", ".profile"}}, "yaml": {[]string{".yaml", ".yml"}, nil}, "toml": {[]string{".toml"}, []string{"Cargo.toml", "pyproject.toml"}}, "html": {[]string{".html", ".htm", ".xhtml"}, nil}, "css": {[]string{".css", ".scss", ".sass", ".less"}, nil}, "c": {[]string{".c", ".h"}, nil}, "cpp": {[]string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx", ".h"}, nil}, "make": {[]string{".mk", ".mak"}, []string{"Makefile", "GNUmakefile", "makefile"}}, "docker": {nil, []string{"Dockerfile", "Dockerfile.*", "*.dockerfile"}},
	}
}

func (r rgTypeRegistry) add(spec string) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return
	}
	name, pat := parts[0], parts[1]
	if strings.HasPrefix(pat, "include:") {
		if t, ok := r[strings.TrimPrefix(pat, "include:")]; ok {
			cur := r[name]
			cur.exts = append(cur.exts, t.exts...)
			cur.globs = append(cur.globs, t.globs...)
			r[name] = cur
		}
		return
	}
	cur := r[name]
	if strings.HasPrefix(pat, "*.") {
		cur.exts = append(cur.exts, strings.TrimPrefix(pat, "*"))
	} else {
		cur.globs = append(cur.globs, pat)
	}
	r[name] = cur
}

func (r rgTypeRegistry) matches(filename string, names []string) bool {
	for _, n := range names {
		if t, ok := r[n]; ok {
			ext := path.Ext(filename)
			for _, e := range t.exts {
				if ext == e {
					return true
				}
			}
			for _, g := range t.globs {
				if matchRgGlob(filename, g, false) {
					return true
				}
			}
		}
	}
	return false
}

func appendRgJSON(lines *[]string, res rgFileResult, matcher rgMatcher, opts rgOptions) {
	*lines = append(*lines, mustJSON(map[string]any{"type": "begin", "data": map[string]any{"path": map[string]string{"text": res.file}}}))
	off := 0
	for i, line := range strings.Split(res.content, "\n") {
		subs := []map[string]any{}
		for _, m := range matcher.findAll(line) {
			sub := map[string]any{"match": map[string]string{"text": line[m.start:m.end]}, "start": m.start, "end": m.end}
			if opts.replace != nil {
				sub["replacement"] = map[string]string{"text": *opts.replace}
			}
			subs = append(subs, sub)
		}
		if len(subs) > 0 {
			*lines = append(*lines, mustJSON(map[string]any{"type": "match", "data": map[string]any{"path": map[string]string{"text": res.file}, "lines": map[string]string{"text": line + "\n"}, "line_number": i + 1, "absolute_offset": off, "submatches": subs}}))
		}
		off += len(line) + 1
	}
	*lines = append(*lines, mustJSON(map[string]any{"type": "end", "data": map[string]any{"path": map[string]string{"text": res.file}, "binary_offset": nil, "stats": map[string]any{"elapsed": map[string]any{"secs": 0, "nanos": 0, "human": "0s"}, "searches": 1, "searches_with_match": 1, "bytes_searched": len(res.content), "bytes_printed": 0, "matched_lines": res.matchCount, "matches": res.matchCount}}}))
}

func rgSummaryJSON(searches, withMatch, bytes, matches int) string {
	return mustJSON(map[string]any{"type": "summary", "data": map[string]any{"elapsed_total": map[string]any{"secs": 0, "nanos": 0, "human": "0s"}, "stats": map[string]any{"elapsed": map[string]any{"secs": 0, "nanos": 0, "human": "0s"}, "searches": searches, "searches_with_match": withMatch, "bytes_searched": bytes, "bytes_printed": 0, "matched_lines": matches, "matches": matches}}})
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func rgColumn(line string, byteStart int) int { return utf8.RuneCountInString(line[:byteStart]) + 1 }
func rgFirstColumn(line string, matches []rgLineMatch) int {
	if len(matches) == 0 {
		return 1
	}
	return rgColumn(line, matches[0].start)
}
