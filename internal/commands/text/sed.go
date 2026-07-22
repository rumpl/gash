package text

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
)

type sedOptions struct {
	scripts       []string
	scriptFiles   []string
	files         []string
	silent        bool
	inPlace       bool
	extendedRegex bool
}

type sedCommandType string

const (
	sedSubstitute      sedCommandType = "substitute"
	sedPrint           sedCommandType = "print"
	sedPrintFirstLine  sedCommandType = "printFirstLine"
	sedDelete          sedCommandType = "delete"
	sedDeleteFirstLine sedCommandType = "deleteFirstLine"
	sedAppend          sedCommandType = "append"
	sedInsert          sedCommandType = "insert"
	sedChange          sedCommandType = "change"
	sedHold            sedCommandType = "hold"
	sedHoldAppend      sedCommandType = "holdAppend"
	sedGet             sedCommandType = "get"
	sedGetAppend       sedCommandType = "getAppend"
	sedExchange        sedCommandType = "exchange"
	sedNext            sedCommandType = "next"
	sedNextAppend      sedCommandType = "nextAppend"
	sedQuit            sedCommandType = "quit"
	sedQuitSilent      sedCommandType = "quitSilent"
	sedZap             sedCommandType = "zap"
	sedLineNumber      sedCommandType = "lineNumber"
	sedList            sedCommandType = "list"
	sedFilename        sedCommandType = "filename"
	sedTransliterate   sedCommandType = "transliterate"
	sedReadFile        sedCommandType = "readFile"
	sedReadFileLine    sedCommandType = "readFileLine"
	sedWriteFile       sedCommandType = "writeFile"
	sedWriteFirstLine  sedCommandType = "writeFirstLine"
	sedExecute         sedCommandType = "execute"
	sedLabel           sedCommandType = "label"
	sedBranch          sedCommandType = "branch"
	sedBranchOnSubst   sedCommandType = "branchOnSubst"
	sedBranchOnNoSubst sedCommandType = "branchOnNoSubst"
	sedGroup           sedCommandType = "group"
	sedVersion         sedCommandType = "version"
)

type sedCommandDef struct {
	typ           sedCommandType
	address       *sedAddressRange
	pattern       string
	replacement   string
	global        bool
	ignoreCase    bool
	printOnMatch  bool
	nthOccurrence int
	extendedRegex bool
	text          string
	source        string
	dest          string
	filename      string
	label         string
	commands      []sedCommandDef
}

type sedAddressKind int

const (
	sedAddressLine sedAddressKind = iota
	sedAddressLast
	sedAddressRegex
	sedAddressStep
	sedAddressOffset
)

type sedAddress struct {
	kind    sedAddressKind
	line    int
	pattern string
	first   int
	step    int
	offset  int
}

type sedAddressRange struct {
	start   *sedAddress
	end     *sedAddress
	negated bool
}

type sedRangeState struct {
	active    bool
	completed bool
	startLine int
}

type sedState struct {
	patternSpace         string
	holdSpace            string
	lineNumber           int
	totalLines           int
	deleted              bool
	printed              bool
	quit                 bool
	quitSilent           bool
	exitCode             *int
	errorMessage         string
	appendBuffer         []string
	lineNumberOutput     []string
	nCommandOutput       []string
	substitutionMade     bool
	restartCycle         bool
	currentFilename      string
	changedText          *string
	lastPattern          string
	rangeStates          map[string]*sedRangeState
	linesConsumedInCycle int
	pendingReads         []sedFileRead
	pendingWrites        []sedFileWrite
	branchRequest        string
}

type sedFileRead struct {
	filename  string
	wholeFile bool
}

type sedFileWrite struct {
	filename string
	content  string
}

func commandSed(_ context.Context, args []string, c *CommandContext) int {
	if commandhelp.Requested(args) {
		if info, ok := commandhelp.Lookup("sed"); ok {
			return commandhelp.Show(c, info)
		}
	}

	opts, code := parseSedArgs(args, c)
	if code != 0 {
		return code
	}
	for _, scriptFile := range opts.scriptFiles {
		data, err := gfs.ReadFile(c.FS, abs(c, scriptFile))
		if err != nil {
			fmt.Fprintf(c.Stderr, "sed: couldn't open file %s: No such file or directory\n", scriptFile)
			return 1
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				opts.scripts = append(opts.scripts, trimmed)
			}
		}
	}
	if len(opts.scripts) == 0 {
		fmt.Fprint(c.Stderr, "sed: no script specified\n")
		return 1
	}

	commands, silentFromScript, err := parseSedScripts(opts.scripts, opts.extendedRegex)
	if err != nil {
		fmt.Fprintf(c.Stderr, "sed: %s\n", err)
		return 1
	}
	effectiveSilent := opts.silent || silentFromScript

	if opts.inPlace {
		if len(opts.files) == 0 {
			fmt.Fprint(c.Stderr, "sed: -i requires at least one file argument\n")
			return 1
		}
		for _, file := range opts.files {
			if file == "-" {
				continue
			}
			data, err := gfs.ReadFile(c.FS, abs(c, file))
			if err != nil {
				fmt.Fprintf(c.Stderr, "sed: %s: No such file or directory\n", file)
				return 1
			}
			out, exitCode, msg, err := processSedContent(string(data), commands, effectiveSilent, sedProcessOptions{filename: file, ctx: c})
			if err != nil {
				fmt.Fprintf(c.Stderr, "sed: %v\n", err)
				return 1
			}
			if msg != "" {
				fmt.Fprintln(c.Stderr, msg)
				if exitCode == 0 {
					exitCode = 1
				}
				return exitCode
			}
			if err := gfs.WriteFile(c.FS, abs(c, file), []byte(out), c.CreationMode(0o666)); err != nil {
				return report(c, "sed: "+file, err)
			}
		}
		return 0
	}

	content, errCode := readSedInput(opts.files, c)
	if errCode != 0 {
		return errCode
	}
	filename := ""
	if len(opts.files) == 1 {
		filename = opts.files[0]
	}
	out, exitCode, msg, err := processSedContent(content, commands, effectiveSilent, sedProcessOptions{filename: filename, ctx: c})
	if err != nil {
		fmt.Fprintf(c.Stderr, "sed: %v\n", err)
		return 1
	}
	if msg != "" {
		fmt.Fprintln(c.Stderr, msg)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	fmt.Fprint(c.Stdout, out)
	return exitCode
}

func parseSedArgs(args []string, c *CommandContext) (sedOptions, int) {
	var opts sedOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-n" || arg == "--quiet" || arg == "--silent":
			opts.silent = true
		case arg == "-i" || arg == "--in-place" || (strings.HasPrefix(arg, "-i") && arg != "-"):
			opts.inPlace = true
		case arg == "-E" || arg == "-r" || arg == "--regexp-extended":
			opts.extendedRegex = true
		case arg == "-e":
			if i+1 >= len(args) {
				return opts, commandhelp.UnknownOption(c, "sed", arg)
			}
			i++
			opts.scripts = append(opts.scripts, args[i])
		case arg == "-f":
			if i+1 >= len(args) {
				return opts, commandhelp.UnknownOption(c, "sed", arg)
			}
			i++
			opts.scriptFiles = append(opts.scriptFiles, args[i])
		case strings.HasPrefix(arg, "--"):
			return opts, commandhelp.UnknownOption(c, "sed", arg)
		case arg == "-":
			opts.files = append(opts.files, arg)
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			for _, r := range arg[1:] {
				switch r {
				case 'n':
					opts.silent = true
				case 'i':
					opts.inPlace = true
				case 'E', 'r':
					opts.extendedRegex = true
				case 'e':
					if i+1 >= len(args) {
						return opts, commandhelp.UnknownOption(c, "sed", "-e")
					}
					i++
					opts.scripts = append(opts.scripts, args[i])
				case 'f':
					if i+1 >= len(args) {
						return opts, commandhelp.UnknownOption(c, "sed", "-f")
					}
					i++
					opts.scriptFiles = append(opts.scriptFiles, args[i])
				default:
					return opts, commandhelp.UnknownOption(c, "sed", "-"+string(r))
				}
			}
		case len(opts.scripts) == 0 && len(opts.scriptFiles) == 0:
			opts.scripts = append(opts.scripts, arg)
		default:
			opts.files = append(opts.files, arg)
		}
	}
	return opts, 0
}

func readSedInput(files []string, c *CommandContext) (string, int) {
	if len(files) == 0 {
		data, err := io.ReadAll(c.Stdin)
		if err != nil {
			fmt.Fprintf(c.Stderr, "sed: %v\n", err)
			return "", 1
		}
		return string(data), 0
	}
	var b strings.Builder
	stdinConsumed := false
	for _, file := range files {
		var data []byte
		if file == "-" {
			if stdinConsumed {
				data = nil
			} else {
				var err error
				data, err = io.ReadAll(c.Stdin)
				if err != nil {
					fmt.Fprintf(c.Stderr, "sed: %v\n", err)
					return "", 1
				}
				stdinConsumed = true
			}
		} else {
			var err error
			data, err = gfs.ReadFile(c.FS, abs(c, file))
			if err != nil {
				fmt.Fprintf(c.Stderr, "sed: %s: No such file or directory\n", file)
				return "", 1
			}
		}
		if b.Len() > 0 && len(data) > 0 {
			s := b.String()
			if !strings.HasSuffix(s, "\n") {
				b.WriteByte('\n')
			}
		}
		b.Write(data)
	}
	return b.String(), 0
}

type sedParser struct {
	s        string
	pos      int
	extended bool
}

func parseSedScripts(scripts []string, extended bool) ([]sedCommandDef, bool, error) {
	var silent bool
	joined := make([]string, 0, len(scripts))
	for i, script := range scripts {
		if len(joined) == 0 && i == 0 {
			lower := strings.ToLower(script)
			if strings.HasPrefix(lower, "#n") && (len(script) == 2 || script[2] == '\n' || script[2] == ' ' || script[2] == '\t') {
				silent = true
				if idx := strings.IndexByte(script, '\n'); idx >= 0 {
					script = script[idx+1:]
				} else {
					script = ""
				}
			} else if strings.HasPrefix(lower, "#r") && (len(script) == 2 || script[2] == '\n' || script[2] == ' ' || script[2] == '\t') {
				extended = true
				if idx := strings.IndexByte(script, '\n'); idx >= 0 {
					script = script[idx+1:]
				} else {
					script = ""
				}
			}
		}
		if len(joined) > 0 && strings.HasSuffix(joined[len(joined)-1], "\\") {
			joined[len(joined)-1] += "\n" + script
		} else {
			joined = append(joined, script)
		}
	}
	p := &sedParser{s: strings.Join(joined, "\n"), extended: extended}
	cmds, err := p.parseCommands(false)
	if err != nil {
		return nil, silent, err
	}
	if label := findUndefinedSedLabel(cmds, collectSedLabels(cmds)); label != "" {
		return nil, silent, fmt.Errorf("undefined label '%s'", label)
	}
	return cmds, silent, nil
}

func (p *sedParser) parseCommands(inGroup bool) ([]sedCommandDef, error) {
	var cmds []sedCommandDef
	for {
		p.skipSeparators()
		if p.eof() {
			if inGroup {
				return nil, fmt.Errorf("unmatched brace in grouped commands")
			}
			return cmds, nil
		}
		if p.peek() == '}' {
			if !inGroup {
				return nil, fmt.Errorf("unknown command: }")
			}
			p.pos++
			return cmds, nil
		}
		cmd, err := p.parseCommand()
		if err != nil {
			return nil, err
		}
		if cmd != nil {
			cmds = append(cmds, *cmd)
		}
	}
}

func (p *sedParser) parseCommand() (*sedCommandDef, error) {
	addr, err := p.parseAddressRange()
	if err != nil {
		return nil, err
	}
	if p.peek() == '!' {
		p.pos++
		if addr == nil {
			addr = &sedAddressRange{}
		}
		addr.negated = true
	}
	p.skipInlineSpace()
	if p.eof() || p.peek() == '\n' || p.peek() == ';' || p.peek() == '}' {
		if addr != nil && addr.start != nil {
			return nil, fmt.Errorf("command expected")
		}
		return nil, nil
	}
	ch := p.next()
	simple := func(t sedCommandType) *sedCommandDef { return &sedCommandDef{typ: t, address: addr} }
	switch ch {
	case 'p':
		return simple(sedPrint), nil
	case 'P':
		return simple(sedPrintFirstLine), nil
	case 'd':
		return simple(sedDelete), nil
	case 'D':
		return simple(sedDeleteFirstLine), nil
	case 'h':
		return simple(sedHold), nil
	case 'H':
		return simple(sedHoldAppend), nil
	case 'g':
		return simple(sedGet), nil
	case 'G':
		return simple(sedGetAppend), nil
	case 'x':
		return simple(sedExchange), nil
	case 'n':
		return simple(sedNext), nil
	case 'N':
		return simple(sedNextAppend), nil
	case 'q':
		return simple(sedQuit), nil
	case 'Q':
		return simple(sedQuitSilent), nil
	case 'z':
		return simple(sedZap), nil
	case '=':
		return simple(sedLineNumber), nil
	case 'l':
		return simple(sedList), nil
	case 'F':
		return simple(sedFilename), nil
	case 's':
		return p.parseSubstitute(addr)
	case 'y':
		return p.parseTransliterate(addr)
	case 'a', 'i', 'c':
		return p.parseTextCommand(ch, addr), nil
	case 'r':
		return &sedCommandDef{typ: sedReadFile, address: addr, filename: strings.TrimSpace(p.readToCommandEnd())}, nil
	case 'R':
		return &sedCommandDef{typ: sedReadFileLine, address: addr, filename: strings.TrimSpace(p.readToCommandEnd())}, nil
	case 'w':
		return &sedCommandDef{typ: sedWriteFile, address: addr, filename: strings.TrimSpace(p.readToCommandEnd())}, nil
	case 'W':
		return &sedCommandDef{typ: sedWriteFirstLine, address: addr, filename: strings.TrimSpace(p.readToCommandEnd())}, nil
	case 'e':
		if p.eof() || p.peek() == ';' || p.peek() == '\n' || p.peek() == '}' {
			return &sedCommandDef{typ: sedExecute, address: addr}, nil
		}
		return &sedCommandDef{typ: sedExecute, address: addr, text: strings.TrimSpace(p.readToCommandEnd())}, nil
	case ':':
		return &sedCommandDef{typ: sedLabel, label: strings.TrimSpace(p.readToCommandEnd())}, nil
	case 'b':
		return &sedCommandDef{typ: sedBranch, address: addr, label: strings.TrimSpace(p.readToCommandEnd())}, nil
	case 't':
		return &sedCommandDef{typ: sedBranchOnSubst, address: addr, label: strings.TrimSpace(p.readToCommandEnd())}, nil
	case 'T':
		return &sedCommandDef{typ: sedBranchOnNoSubst, address: addr, label: strings.TrimSpace(p.readToCommandEnd())}, nil
	case 'v':
		return &sedCommandDef{typ: sedVersion, address: addr, text: strings.TrimSpace(p.readToCommandEnd())}, nil
	case '{':
		cmds, err := p.parseCommands(true)
		if err != nil {
			return nil, err
		}
		return &sedCommandDef{typ: sedGroup, address: addr, commands: cmds}, nil
	default:
		return nil, fmt.Errorf("unknown command: %c", ch)
	}
}

func (p *sedParser) parseAddressRange() (*sedAddressRange, error) {
	p.skipInlineSpace()
	if p.peek() == ',' {
		return nil, fmt.Errorf("expected context address")
	}
	start := p.parseAddress()
	if start == nil {
		return nil, nil
	}
	r := &sedAddressRange{start: start}
	if p.peek() == ',' {
		p.pos++
		if p.peek() == '+' {
			p.pos++
			n := p.readNumber()
			if n == "" {
				return nil, fmt.Errorf("expected context address")
			}
			off, _ := strconv.Atoi(n)
			r.end = &sedAddress{kind: sedAddressOffset, offset: off}
			return r, nil
		}
		end := p.parseAddress()
		if end == nil {
			return nil, fmt.Errorf("expected context address")
		}
		r.end = end
	}
	return r, nil
}

func (p *sedParser) parseAddress() *sedAddress {
	if p.eof() {
		return nil
	}
	ch := p.peek()
	if ch >= '0' && ch <= '9' {
		n, _ := strconv.Atoi(p.readNumber())
		if p.peek() == '~' {
			p.pos++
			stepS := p.readNumber()
			step, _ := strconv.Atoi(stepS)
			return &sedAddress{kind: sedAddressStep, first: n, step: step}
		}
		return &sedAddress{kind: sedAddressLine, line: n}
	}
	if ch == '$' {
		p.pos++
		return &sedAddress{kind: sedAddressLast}
	}
	if ch == '/' {
		p.pos++
		pat, ok := p.readDelimited('/')
		if !ok {
			return nil
		}
		return &sedAddress{kind: sedAddressRegex, pattern: pat}
	}
	return nil
}

func (p *sedParser) parseSubstitute(addr *sedAddressRange) (*sedCommandDef, error) {
	if p.eof() || p.peek() == '\n' {
		return nil, fmt.Errorf("unterminated substitute pattern")
	}
	delim := p.next()
	pat, ok := p.readDelimited(delim)
	if !ok {
		return nil, fmt.Errorf("unterminated substitute pattern")
	}
	repl, ok := p.readDelimited(delim)
	if !ok {
		return nil, fmt.Errorf("unterminated substitute replacement")
	}
	flags := p.readFlags()
	cmd := &sedCommandDef{typ: sedSubstitute, address: addr, pattern: pat, replacement: repl, extendedRegex: p.extended}
	for i := 0; i < len(flags); i++ {
		switch flags[i] {
		case 'g':
			cmd.global = true
		case 'i', 'I':
			cmd.ignoreCase = true
		case 'p':
			cmd.printOnMatch = true
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			j := i
			for j < len(flags) && flags[j] >= '0' && flags[j] <= '9' {
				j++
			}
			cmd.nthOccurrence, _ = strconv.Atoi(flags[i:j])
			i = j - 1
		}
	}
	return cmd, nil
}

func (p *sedParser) parseTransliterate(addr *sedAddressRange) (*sedCommandDef, error) {
	if p.eof() {
		return nil, fmt.Errorf("unterminated transliterate command")
	}
	delim := p.next()
	src, ok := p.readDelimited(delim)
	if !ok {
		return nil, fmt.Errorf("unterminated transliterate command")
	}
	dst, ok := p.readDelimited(delim)
	if !ok {
		return nil, fmt.Errorf("unterminated transliterate command")
	}
	if utf8.RuneCountInString(src) != utf8.RuneCountInString(dst) {
		return nil, fmt.Errorf("transliteration sets must have same length")
	}
	return &sedCommandDef{typ: sedTransliterate, address: addr, source: src, dest: dst}, nil
}

func (p *sedParser) parseTextCommand(ch byte, addr *sedAddressRange) *sedCommandDef {
	p.skipInlineSpace()
	if p.peek() == '\\' {
		p.pos++
		if p.peek() == '\n' {
			p.pos++
		} else if p.peek() == ' ' || p.peek() == '\t' {
			p.skipInlineSpace()
		}
	} else {
		p.skipInlineSpace()
	}
	text := p.readLineText()
	typ := sedAppend
	if ch == 'i' {
		typ = sedInsert
	} else if ch == 'c' {
		typ = sedChange
	}
	return &sedCommandDef{typ: typ, address: addr, text: text}
}

func (p *sedParser) eof() bool {
	return p.pos >= len(p.s)
}

func (p *sedParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.s[p.pos]
}

func (p *sedParser) next() byte {
	ch := p.s[p.pos]
	p.pos++
	return ch
}

func (p *sedParser) skipInlineSpace() {
	for !p.eof() && (p.peek() == ' ' || p.peek() == '\t') {
		p.pos++
	}
}

func (p *sedParser) skipSeparators() {
	for !p.eof() {
		if p.peek() == ';' || p.peek() == '\n' || p.peek() == ' ' || p.peek() == '\t' {
			p.pos++
			continue
		}
		break
	}
}

func (p *sedParser) readNumber() string {
	start := p.pos
	for !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
		p.pos++
	}
	return p.s[start:p.pos]
}

func (p *sedParser) readDelimited(delim byte) (string, bool) {
	var b strings.Builder
	esc := false
	for !p.eof() {
		ch := p.next()
		if esc {
			b.WriteByte('\\')
			b.WriteByte(ch)
			esc = false
			continue
		}
		if ch == '\\' {
			esc = true
			continue
		}
		if ch == delim {
			return b.String(), true
		}
		if ch == '\n' {
			return b.String(), false
		}
		b.WriteByte(ch)
	}
	return b.String(), false
}

func (p *sedParser) readFlags() string {
	start := p.pos
	for !p.eof() && p.peek() != ';' && p.peek() != '\n' && p.peek() != '}' {
		p.pos++
	}
	return strings.TrimSpace(p.s[start:p.pos])
}

func (p *sedParser) readToCommandEnd() string {
	start := p.pos
	for !p.eof() && p.peek() != ';' && p.peek() != '\n' && p.peek() != '}' {
		p.pos++
	}
	return p.s[start:p.pos]
}

func (p *sedParser) readLineText() string {
	start := p.pos
	for !p.eof() && p.peek() != '\n' {
		p.pos++
	}
	return p.s[start:p.pos]
}

func collectSedLabels(commands []sedCommandDef) map[string]bool {
	labels := map[string]bool{}
	var walk func([]sedCommandDef)
	walk = func(cmds []sedCommandDef) {
		for _, cmd := range cmds {
			if cmd.typ == sedLabel {
				labels[cmd.label] = true
			}
			if cmd.typ == sedGroup {
				walk(cmd.commands)
			}
		}
	}
	walk(commands)
	return labels
}

func findUndefinedSedLabel(commands []sedCommandDef, labels map[string]bool) string {
	for _, cmd := range commands {
		if (cmd.typ == sedBranch || cmd.typ == sedBranchOnSubst || cmd.typ == sedBranchOnNoSubst) && cmd.label != "" && !labels[cmd.label] {
			return cmd.label
		}
		if cmd.typ == sedGroup {
			if label := findUndefinedSedLabel(cmd.commands, labels); label != "" {
				return label
			}
		}
	}
	return ""
}

type sedProcessOptions struct {
	filename string
	ctx      *CommandContext
}

func processSedContent(content string, commands []sedCommandDef, silent bool, opts sedProcessOptions) (string, int, string, error) {
	inputEndsWithNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var output strings.Builder
	holdSpace := ""
	lastPattern := ""
	rangeStates := map[string]*sedRangeState{}
	fileLineCache := map[string][]string{}
	fileLinePos := map[string]int{}
	fileWrites := map[string]string{}
	lastOutputWasAutoPrint := false
	exitCode := 0

	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		state := &sedState{patternSpace: lines[lineIndex], holdSpace: holdSpace, lastPattern: lastPattern, lineNumber: lineIndex + 1, totalLines: len(lines), rangeStates: rangeStates, currentFilename: opts.filename}
		cycleIterations := 0
		for {
			cycleIterations++
			if cycleIterations > 10000 {
				break
			}
			state.restartCycle = false
			state.pendingReads = nil
			state.pendingWrites = nil
			executeSedCommands(commands, state, lines, lineIndex)
			if opts.ctx != nil {
				for _, read := range state.pendingReads {
					filePath := abs(opts.ctx, read.filename)
					data, err := gfs.ReadFile(opts.ctx.FS, filePath)
					if err != nil {
						continue
					}
					if read.wholeFile {
						state.appendBuffer = append(state.appendBuffer, strings.TrimSuffix(string(data), "\n"))
					} else {
						if _, ok := fileLineCache[filePath]; !ok {
							fileLineCache[filePath] = strings.Split(string(data), "\n")
						}
						pos := fileLinePos[filePath]
						if pos < len(fileLineCache[filePath]) {
							state.appendBuffer = append(state.appendBuffer, fileLineCache[filePath][pos])
							fileLinePos[filePath] = pos + 1
						}
					}
				}
				for _, write := range state.pendingWrites {
					fileWrites[abs(opts.ctx, write.filename)] += write.content
				}
			}
			if !(state.restartCycle && !state.deleted && !state.quit && !state.quitSilent) {
				break
			}
		}
		lineIndex += state.linesConsumedInCycle
		holdSpace = state.holdSpace
		lastPattern = state.lastPattern
		if !silent {
			for _, ln := range state.nCommandOutput {
				output.WriteString(ln)
				output.WriteByte('\n')
			}
		}
		hadLineOutput := len(state.lineNumberOutput) > 0
		for _, ln := range state.lineNumberOutput {
			output.WriteString(ln)
			output.WriteByte('\n')
		}
		var inserts, appends []string
		for _, item := range state.appendBuffer {
			if strings.HasPrefix(item, "__INSERT__") {
				inserts = append(inserts, strings.TrimPrefix(item, "__INSERT__"))
			} else {
				appends = append(appends, item)
			}
		}
		for _, text := range inserts {
			output.WriteString(text)
			output.WriteByte('\n')
		}
		hadPatternOutput := false
		if !state.deleted && !state.quitSilent {
			if silent {
				if state.printed {
					output.WriteString(state.patternSpace)
					output.WriteByte('\n')
					hadPatternOutput = true
				}
			} else {
				output.WriteString(state.patternSpace)
				output.WriteByte('\n')
				hadPatternOutput = true
			}
		} else if state.changedText != nil {
			output.WriteString(*state.changedText)
			output.WriteByte('\n')
			hadPatternOutput = true
		}
		for _, text := range appends {
			output.WriteString(text)
			output.WriteByte('\n')
		}
		lastOutputWasAutoPrint = (hadLineOutput || hadPatternOutput) && len(appends) == 0
		if state.exitCode != nil {
			exitCode = *state.exitCode
		}
		if state.errorMessage != "" {
			return "", exitCode, state.errorMessage, nil
		}
		if state.quit || state.quitSilent {
			break
		}
	}
	if opts.ctx != nil {
		for filePath, content := range fileWrites {
			_ = gfs.WriteFile(opts.ctx.FS, filePath, []byte(content), opts.ctx.CreationMode(0o666))
		}
	}
	out := output.String()
	if !inputEndsWithNewline && lastOutputWasAutoPrint && strings.HasSuffix(out, "\n") {
		out = strings.TrimSuffix(out, "\n")
	}
	return out, exitCode, "", nil
}

func executeSedCommands(commands []sedCommandDef, state *sedState, lines []string, currentLineIndex int) {
	labelIndex := map[string]int{}
	for i, cmd := range commands {
		if cmd.typ == sedLabel {
			labelIndex[cmd.label] = i
		}
	}
	iterations := 0
	for i := 0; i < len(commands); {
		iterations++
		if iterations > 10000 || state.deleted || state.quit || state.quitSilent || state.restartCycle {
			break
		}
		cmd := commands[i]
		switch cmd.typ {
		case sedNext:
			if sedInRange(cmd.address, state) {
				state.nCommandOutput = append(state.nCommandOutput, state.patternSpace)
				if currentLineIndex+state.linesConsumedInCycle+1 < len(lines) {
					state.linesConsumedInCycle++
					state.patternSpace = lines[currentLineIndex+state.linesConsumedInCycle]
					state.lineNumber = currentLineIndex + state.linesConsumedInCycle + 1
					state.substitutionMade = false
				} else {
					state.quit = true
					state.deleted = true
					break
				}
			}
			i++
		case sedNextAppend:
			if sedInRange(cmd.address, state) {
				if currentLineIndex+state.linesConsumedInCycle+1 < len(lines) {
					state.linesConsumedInCycle++
					state.patternSpace += "\n" + lines[currentLineIndex+state.linesConsumedInCycle]
					state.lineNumber = currentLineIndex + state.linesConsumedInCycle + 1
				} else {
					state.quit = true
					break
				}
			}
			i++
		case sedBranch:
			if sedInRange(cmd.address, state) {
				if cmd.label == "" {
					break
				}
				if target, ok := labelIndex[cmd.label]; ok {
					i = target
					continue
				}
				state.branchRequest = cmd.label
				break
			}
			i++
		case sedBranchOnSubst:
			if sedInRange(cmd.address, state) && state.substitutionMade {
				state.substitutionMade = false
				if cmd.label == "" {
					break
				}
				if target, ok := labelIndex[cmd.label]; ok {
					i = target
					continue
				}
				state.branchRequest = cmd.label
				break
			}
			i++
		case sedBranchOnNoSubst:
			if sedInRange(cmd.address, state) && !state.substitutionMade {
				if cmd.label == "" {
					break
				}
				if target, ok := labelIndex[cmd.label]; ok {
					i = target
					continue
				}
				state.branchRequest = cmd.label
				break
			}
			i++
		case sedGroup:
			if sedInRange(cmd.address, state) {
				executeSedCommands(cmd.commands, state, lines, currentLineIndex)
				if state.branchRequest != "" {
					if target, ok := labelIndex[state.branchRequest]; ok {
						state.branchRequest = ""
						i = target
						continue
					}
					break
				}
			}
			i++
		default:
			executeSedCommand(cmd, state)
			i++
		}
	}
}

func executeSedCommand(cmd sedCommandDef, state *sedState) {
	if cmd.typ == sedLabel {
		return
	}
	if !sedInRange(cmd.address, state) {
		return
	}
	switch cmd.typ {
	case sedSubstitute:
		executeSedSubstitute(cmd, state)
	case sedPrint:
		state.lineNumberOutput = append(state.lineNumberOutput, state.patternSpace)
	case sedPrintFirstLine:
		if idx := strings.IndexByte(state.patternSpace, '\n'); idx >= 0 {
			state.lineNumberOutput = append(state.lineNumberOutput, state.patternSpace[:idx])
		} else {
			state.lineNumberOutput = append(state.lineNumberOutput, state.patternSpace)
		}
	case sedDelete:
		state.deleted = true
	case sedDeleteFirstLine:
		if idx := strings.IndexByte(state.patternSpace, '\n'); idx >= 0 {
			state.patternSpace = state.patternSpace[idx+1:]
			state.restartCycle = true
		} else {
			state.deleted = true
		}
	case sedZap:
		state.patternSpace = ""
	case sedAppend:
		state.appendBuffer = append(state.appendBuffer, cmd.text)
	case sedInsert:
		state.appendBuffer = append([]string{"__INSERT__" + cmd.text}, state.appendBuffer...)
	case sedChange:
		state.deleted = true
		text := cmd.text
		state.changedText = &text
	case sedHold:
		state.holdSpace = state.patternSpace
	case sedHoldAppend:
		if state.holdSpace != "" {
			state.holdSpace += "\n" + state.patternSpace
		} else {
			state.holdSpace = state.patternSpace
		}
	case sedGet:
		state.patternSpace = state.holdSpace
	case sedGetAppend:
		state.patternSpace += "\n" + state.holdSpace
	case sedExchange:
		state.patternSpace, state.holdSpace = state.holdSpace, state.patternSpace
	case sedQuit:
		state.quit = true
	case sedQuitSilent:
		state.quit = true
		state.quitSilent = true
	case sedList:
		state.lineNumberOutput = append(state.lineNumberOutput, sedEscapeForList(state.patternSpace))
	case sedFilename:
		if state.currentFilename != "" {
			state.lineNumberOutput = append(state.lineNumberOutput, state.currentFilename)
		}
	case sedVersion:
		if cmd.text != "" && sedVersionTooNew(cmd.text) {
			state.quit = true
			code := 1
			state.exitCode = &code
			state.errorMessage = "sed: this is not GNU sed version " + cmd.text
		}
	case sedReadFile:
		state.pendingReads = append(state.pendingReads, sedFileRead{filename: cmd.filename, wholeFile: true})
	case sedReadFileLine:
		state.pendingReads = append(state.pendingReads, sedFileRead{filename: cmd.filename})
	case sedWriteFile:
		state.pendingWrites = append(state.pendingWrites, sedFileWrite{filename: cmd.filename, content: state.patternSpace + "\n"})
	case sedWriteFirstLine:
		first := state.patternSpace
		if idx := strings.IndexByte(first, '\n'); idx >= 0 {
			first = first[:idx]
		}
		state.pendingWrites = append(state.pendingWrites, sedFileWrite{filename: cmd.filename, content: first + "\n"})
	case sedExecute:
		state.errorMessage = "sed: e command (shell execution) is not supported in sandboxed environment"
		state.quit = true
	case sedTransliterate:
		state.patternSpace = executeSedTransliterate(state.patternSpace, cmd)
	case sedLineNumber:
		state.lineNumberOutput = append(state.lineNumberOutput, strconv.Itoa(state.lineNumber))
	}
}

func executeSedSubstitute(cmd sedCommandDef, state *sedState) {
	rawPattern := cmd.pattern
	if rawPattern == "" && state.lastPattern != "" {
		rawPattern = state.lastPattern
	} else if rawPattern != "" {
		state.lastPattern = rawPattern
	}
	pattern := rawPattern
	if !cmd.extendedRegex {
		pattern = sedBREToERE(pattern)
	}
	pattern = sedNormalizeRegex(pattern)
	flags := ""
	if cmd.ignoreCase {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return
	}
	matches := re.FindAllStringSubmatchIndex(state.patternSpace, -1)
	if len(matches) == 0 {
		return
	}
	state.substitutionMade = true
	replaceAt := map[int]bool{}
	if cmd.global {
		for i := range matches {
			replaceAt[i] = true
		}
	} else if cmd.nthOccurrence > 0 {
		if cmd.nthOccurrence <= len(matches) {
			replaceAt[cmd.nthOccurrence-1] = true
		}
	} else {
		replaceAt[0] = true
	}
	var out strings.Builder
	last := 0
	for i, m := range matches {
		if !replaceAt[i] {
			continue
		}
		start, end := m[0], m[1]
		out.WriteString(state.patternSpace[last:start])
		out.WriteString(processSedReplacement(cmd.replacement, state.patternSpace[start:end], m, state.patternSpace))
		last = end
		if start == end && last < len(state.patternSpace) {
			_, size := utf8.DecodeRuneInString(state.patternSpace[last:])
			out.WriteString(state.patternSpace[last : last+size])
			last += size
		}
	}
	out.WriteString(state.patternSpace[last:])
	state.patternSpace = out.String()
	if cmd.printOnMatch {
		state.lineNumberOutput = append(state.lineNumberOutput, state.patternSpace)
	}
}

func processSedReplacement(repl, match string, indexes []int, input string) string {
	var out strings.Builder
	for i := 0; i < len(repl); i++ {
		ch := repl[i]
		if ch == '&' {
			out.WriteString(match)
			continue
		}
		if ch == '\\' && i+1 < len(repl) {
			n := repl[i+1]
			i++
			if n == '&' {
				out.WriteByte('&')
			} else if n >= '1' && n <= '9' {
				gi := int(n - '0')
				if 2*gi+1 < len(indexes) && indexes[2*gi] >= 0 {
					out.WriteString(input[indexes[2*gi]:indexes[2*gi+1]])
				}
			} else if n == 'n' {
				out.WriteByte('\n')
			} else if n == 't' {
				out.WriteByte('\t')
			} else {
				out.WriteByte(n)
			}
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func executeSedTransliterate(input string, cmd sedCommandDef) string {
	src := []rune(cmd.source)
	dst := []rune(cmd.dest)
	m := map[rune]rune{}
	for i, r := range src {
		m[r] = dst[i]
	}
	return strings.Map(func(r rune) rune {
		if to, ok := m[r]; ok {
			return to
		}
		return r
	}, input)
}

func sedInRange(r *sedAddressRange, state *sedState) bool {
	res := sedInRangeInternal(r, state)
	if r != nil && r.negated {
		return !res
	}
	return res
}

func sedInRangeInternal(r *sedAddressRange, state *sedState) bool {
	if r == nil || r.start == nil {
		return true
	}
	if r.end == nil {
		return sedMatchesAddress(r.start, state)
	}
	start, end := r.start, r.end
	if end.kind == sedAddressOffset {
		key := sedRangeKey(r)
		rs := state.rangeStates[key]
		if rs == nil {
			rs = &sedRangeState{}
			state.rangeStates[key] = rs
		}
		if !rs.active {
			if sedMatchesAddress(start, state) {
				rs.active = true
				rs.startLine = state.lineNumber
				if end.offset == 0 {
					rs.active = false
				}
				return true
			}
			return false
		}
		if state.lineNumber >= rs.startLine+end.offset {
			rs.active = false
		}
		return true
	}
	if start.kind != sedAddressRegex && end.kind != sedAddressRegex {
		startNum := sedAddressLineNumber(start, state.totalLines)
		endNum := sedAddressLineNumber(end, state.totalLines)
		if startNum <= endNum {
			return state.lineNumber >= startNum && state.lineNumber <= endNum
		}
		key := sedRangeKey(r)
		rs := state.rangeStates[key]
		if rs == nil {
			rs = &sedRangeState{}
			state.rangeStates[key] = rs
		}
		if !rs.completed && state.lineNumber >= startNum {
			rs.completed = true
			return true
		}
		return false
	}
	key := sedRangeKey(r)
	rs := state.rangeStates[key]
	if rs == nil {
		rs = &sedRangeState{}
		state.rangeStates[key] = rs
	}
	if !rs.active {
		if rs.completed {
			return false
		}
		startMatches := false
		if start.kind == sedAddressLine {
			startMatches = state.lineNumber >= start.line
		} else {
			startMatches = sedMatchesAddress(start, state)
		}
		if startMatches {
			rs.active = true
			if sedMatchesAddress(end, state) {
				rs.active = false
				if start.kind == sedAddressLine {
					rs.completed = true
				}
			}
			return true
		}
		return false
	}
	if sedMatchesAddress(end, state) {
		rs.active = false
		if start.kind == sedAddressLine {
			rs.completed = true
		}
	}
	return true
}

func sedMatchesAddress(a *sedAddress, state *sedState) bool {
	switch a.kind {
	case sedAddressLine:
		return state.lineNumber == a.line
	case sedAddressLast:
		return state.lineNumber == state.totalLines
	case sedAddressStep:
		if a.step == 0 {
			return state.lineNumber == a.first
		}
		return state.lineNumber >= a.first && (state.lineNumber-a.first)%a.step == 0
	case sedAddressRegex:
		pat := a.pattern
		if pat == "" && state.lastPattern != "" {
			pat = state.lastPattern
		} else if pat != "" {
			state.lastPattern = pat
		}
		re, err := regexp.Compile(sedNormalizeRegex(sedBREToERE(pat)))
		return err == nil && re.MatchString(state.patternSpace)
	default:
		return false
	}
}

func sedAddressLineNumber(a *sedAddress, total int) int {
	if a.kind == sedAddressLast {
		return total
	}
	if a.kind == sedAddressLine {
		return a.line
	}
	return 1
}

func sedRangeKey(r *sedAddressRange) string {
	return sedAddressKey(r.start) + "," + sedAddressKey(r.end)
}

func sedAddressKey(a *sedAddress) string {
	if a == nil {
		return "nil"
	}
	return fmt.Sprintf("%d:%d:%s:%d:%d:%d", a.kind, a.line, a.pattern, a.first, a.step, a.offset)
}

func sedBREToERE(pattern string) string {
	var out strings.Builder
	inBracket := false
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '[' && !inBracket {
			if converted, n, ok := sedPOSIXClass(pattern[i:]); ok {
				out.WriteString(converted)
				i += n - 1
				continue
			}
			inBracket = true
			out.WriteByte(ch)
			continue
		}
		if inBracket {
			if ch == ']' {
				inBracket = false
			}
			out.WriteByte(ch)
			continue
		}
		if ch == '\\' && i+1 < len(pattern) {
			n := pattern[i+1]
			switch n {
			case '+', '?', '|', '(', ')', '{', '}':
				out.WriteByte(n)
			case 't':
				out.WriteByte('\t')
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			default:
				out.WriteByte('\\')
				out.WriteByte(n)
			}
			i++
			continue
		}
		if strings.ContainsRune("+?|()", rune(ch)) {
			out.WriteByte('\\')
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func sedPOSIXClass(s string) (string, int, bool) {
	classes := map[string]string{"alnum": "a-zA-Z0-9", "alpha": "a-zA-Z", "ascii": "\\x00-\\x7F", "blank": " \\t", "cntrl": "\\x00-\\x1F\\x7F", "digit": "0-9", "graph": "!-~", "lower": "a-z", "print": " -~", "punct": "!-/:-@\\[-`{-~", "space": " \\t\\n\\r\\f\\v", "upper": "A-Z", "word": "a-zA-Z0-9_", "xdigit": "0-9A-Fa-f"}
	neg := false
	prefix := "[[:"
	start := 3
	if strings.HasPrefix(s, "[^[:") {
		neg = true
		prefix = "[^[:"
		start = 4
	}
	if !strings.HasPrefix(s, prefix) {
		return "", 0, false
	}
	end := strings.Index(s[start:], ":]]")
	if end < 0 {
		return "", 0, false
	}
	name := s[start : start+end]
	cls, ok := classes[name]
	if !ok {
		return "", 0, false
	}
	if neg {
		return "[^" + cls + "]", start + end + 3, true
	}
	return "[" + cls + "]", start + end + 3, true
}

func sedNormalizeRegex(pattern string) string {
	return regexp.MustCompile(`\{,(\d+)\}`).ReplaceAllString(pattern, "{0,$1}")
}

func sedEscapeForList(input string) string {
	var out strings.Builder
	for _, r := range input {
		switch r {
		case '\\':
			out.WriteString("\\\\")
		case '\t':
			out.WriteString("\\t")
		case '\n':
			out.WriteString("$\n")
		case '\r':
			out.WriteString("\\r")
		case '\a':
			out.WriteString("\\a")
		case '\b':
			out.WriteString("\\b")
		case '\f':
			out.WriteString("\\f")
		case '\v':
			out.WriteString("\\v")
		default:
			if r < 32 || r >= 127 {
				out.WriteString(fmt.Sprintf("\\%03o", r))
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('$')
	return out.String()
}

func sedVersionTooNew(v string) bool {
	parts := strings.Split(v, ".")
	ours := []int{4, 8, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return true
		}
		if n > ours[i] {
			return true
		}
		if n < ours[i] {
			return false
		}
	}
	return false
}

// Deferred just-bash sed differences: gash does not expose configurable just-bash
// execution limits, and Go regexp compatibility is used instead of the upstream
// JavaScript regex sandbox. The sandboxed sed e command remains intentionally
// blocked and never invokes a host shell.
