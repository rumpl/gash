package commands

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rumpl/gash/internal/commandhelp"
)

func commandHelp(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		commands := append([]string(nil), c.Commands...)
		sort.Strings(commands)
		fmt.Fprintln(c.Stdout, "gash built-in commands:")
		for _, name := range commands {
			if info, ok := commandhelp.Lookup(name); ok && info.Summary != "" {
				fmt.Fprintf(c.Stdout, "  %-12s %s\n", name, info.Summary)
			} else {
				fmt.Fprintf(c.Stdout, "  %s\n", name)
			}
		}
		fmt.Fprintln(c.Stdout, "\nUse 'help NAME' for command-specific help when available.")
		return 0
	}
	code := 0
	for _, name := range args {
		info, ok := commandhelp.Lookup(name)
		if !ok {
			fmt.Fprintf(c.Stderr, "help: no help topics match '%s'\n", name)
			code = 1
			continue
		}
		if code := commandhelp.Show(c, info); code != 0 {
			return code
		}
	}
	return code
}

func commandWhich(_ context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "which: missing operand")
		return 1
	}
	available := map[string]bool{"bash": true, "sh": true}
	for _, name := range c.Commands {
		available[name] = true
	}
	code := 0
	for _, name := range args {
		if available[name] {
			fmt.Fprintf(c.Stdout, "%s: gash built-in\n", name)
		} else {
			code = 1
		}
	}
	return code
}

func commandHistory(_ context.Context, args []string, c *CommandContext) int {
	if len(args) > 0 {
		fmt.Fprintln(c.Stderr, "history: non-interactive command history is not persisted")
		return 1
	}
	return 0
}

func commandTimeout(ctx context.Context, args []string, c *CommandContext) int {
	if len(args) < 2 {
		fmt.Fprintln(c.Stderr, "timeout: missing duration or command")
		return 125
	}
	duration, err := parseCommandDuration(args[0])
	if err != nil {
		fmt.Fprintf(c.Stderr, "timeout: invalid time interval '%s'\n", args[0])
		return 125
	}
	if c.RunCommand == nil {
		fmt.Fprintln(c.Stderr, "timeout: command execution is unavailable")
		return 125
	}
	childCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	code := c.RunCommand(childCtx, args[1:], c)
	if childCtx.Err() == context.DeadlineExceeded && code == 0 {
		return 124
	}
	return code
}

func commandTime(ctx context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "time: missing command")
		return 1
	}
	if c.RunCommand == nil {
		fmt.Fprintln(c.Stderr, "time: command execution is unavailable")
		return 1
	}
	start := time.Now()
	code := c.RunCommand(ctx, args, c)
	elapsed := time.Since(start).Seconds()
	fmt.Fprintf(c.Stderr, "\nreal\t%.3fs\nuser\t0.000s\nsys\t0.000s\n", elapsed)
	return code
}

func commandDate(_ context.Context, args []string, c *CommandContext) int {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	clock := now()
	format := "Mon Jan _2 15:04:05 MST 2006"
	utc := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-u" || arg == "--utc":
			utc = true
		case arg == "+":
			format = ""
		case strings.HasPrefix(arg, "+"):
			format = strftimeLayout(arg[1:])
		case arg == "-d" || arg == "--date":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Stderr, "date: option requires an argument -- 'd'")
				return 1
			}
			i++
			parsed, err := parseDateString(args[i], clock)
			if err != nil {
				fmt.Fprintf(c.Stderr, "date: invalid date '%s'\n", args[i])
				return 1
			}
			clock = parsed
		case strings.HasPrefix(arg, "--date="):
			value := strings.TrimPrefix(arg, "--date=")
			parsed, err := parseDateString(value, clock)
			if err != nil {
				fmt.Fprintf(c.Stderr, "date: invalid date '%s'\n", value)
				return 1
			}
			clock = parsed
		default:
			fmt.Fprintf(c.Stderr, "date: invalid option '%s'\n", arg)
			return 1
		}
	}
	if utc {
		clock = clock.UTC()
	}
	output := clock.Format(format)
	output = strings.ReplaceAll(output, "__GASH_UNIX__", fmt.Sprintf("%d", clock.Unix()))
	fmt.Fprintln(c.Stdout, output)
	return 0
}

func parseCommandDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, fmt.Errorf("empty duration")
	}
	unit := time.Second
	number := value
	suffix := value[len(value)-1]
	switch suffix {
	case 's':
		unit, number = time.Second, value[:len(value)-1]
	case 'm':
		unit, number = time.Minute, value[:len(value)-1]
	case 'h':
		unit, number = time.Hour, value[:len(value)-1]
	case 'd':
		unit, number = 24*time.Hour, value[:len(value)-1]
	}
	seconds, err := strconvParseFloat(number)
	if err != nil || seconds < 0 {
		return 0, fmt.Errorf("invalid duration")
	}
	return time.Duration(seconds * float64(unit)), nil
}

func parseDateString(value string, base time.Time) (time.Time, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "now":
		return base, nil
	case "yesterday":
		return base.AddDate(0, 0, -1), nil
	case "tomorrow":
		return base.AddDate(0, 0, 1), nil
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05 -0700", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, base.Location()); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date")
}

func strftimeLayout(format string) string {
	var out strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			out.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			out.WriteString("2006")
		case 'y':
			out.WriteString("06")
		case 'm':
			out.WriteString("01")
		case 'd':
			out.WriteString("02")
		case 'e':
			out.WriteString("_2")
		case 'H':
			out.WriteString("15")
		case 'M':
			out.WriteString("04")
		case 'S':
			out.WriteString("05")
		case 'z':
			out.WriteString("-0700")
		case 'Z':
			out.WriteString("MST")
		case 'a':
			out.WriteString("Mon")
		case 'A':
			out.WriteString("Monday")
		case 'b', 'h':
			out.WriteString("Jan")
		case 'B':
			out.WriteString("January")
		case 'F':
			out.WriteString("2006-01-02")
		case 'T':
			out.WriteString("15:04:05")
		case 'R':
			out.WriteString("15:04")
		case 'c':
			out.WriteString("Mon Jan _2 15:04:05 2006")
		case 's':
			out.WriteString("__GASH_UNIX__")
		case '%':
			out.WriteByte('%')
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		default:
			out.WriteByte('%')
			out.WriteByte(format[i])
		}
	}
	return out.String()
}

func strconvParseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
