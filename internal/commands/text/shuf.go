package text

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/rumpl/gash/internal/commandhelp"
)

type shufRandom interface {
	Intn(int) (int, error)
}

type cryptoShufRandom struct{}

func (cryptoShufRandom) Intn(limit int) (int, error) {
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

type shufOptions struct {
	count    int
	countSet bool
	repeat   bool
	rangeSet bool
	low      int
	high     int
	files    []string
}

var shufRangePattern = regexp.MustCompile(`^([+-]?\d+)-([+-]?\d+)$`)

const maxShufRangeItems = 1_000_000

func commandShuf(ctx context.Context, args []string, c *CommandContext) int {
	return commandShufWithRandom(ctx, args, c, cryptoShufRandom{})
}

func commandShufWithRandom(ctx context.Context, args []string, c *CommandContext, random shufRandom) int {
	options, code := parseShufOptions(args, c)
	if code != 0 {
		return code
	}
	if options.repeat && !options.countSet {
		fmt.Fprintln(c.Stderr, "shuf: --repeat requires --head-count to keep output bounded")
		return 1
	}

	var lines []string
	if options.rangeSet {
		if len(options.files) != 0 {
			fmt.Fprintln(c.Stderr, "shuf: extra operand with --input-range")
			return 1
		}
		lines = make([]string, 0, options.high-options.low+1)
		for value := options.low; ; value++ {
			lines = append(lines, strconv.Itoa(value))
			if value == options.high {
				break
			}
		}
	} else {
		data, err := readInputs(options.files, c)
		if err != nil {
			return report(c, "shuf", err)
		}
		input := strings.TrimSuffix(string(data), "\n")
		if len(data) != 0 {
			lines = strings.Split(input, "\n")
		}
	}

	count := len(lines)
	if options.countSet {
		count = options.count
	}
	if !options.repeat && count > len(lines) {
		count = len(lines)
	}
	if count == 0 {
		return 0
	}
	if len(lines) == 0 {
		fmt.Fprintln(c.Stderr, "shuf: no lines to repeat")
		return 1
	}

	if options.repeat {
		for index := 0; index < count; index++ {
			selected, err := random.Intn(len(lines))
			if err != nil {
				return report(c, "shuf", fmt.Errorf("random source: %w", err))
			}
			if _, err := fmt.Fprintln(c.Stdout, lines[selected]); err != nil {
				return 1
			}
			if err := ctx.Err(); err != nil {
				return 1
			}
		}
		return 0
	}

	for index := 0; index < count; index++ {
		offset, err := random.Intn(len(lines) - index)
		if err != nil {
			return report(c, "shuf", fmt.Errorf("random source: %w", err))
		}
		selected := index + offset
		lines[index], lines[selected] = lines[selected], lines[index]
		if _, err := fmt.Fprintln(c.Stdout, lines[index]); err != nil {
			return 1
		}
		if err := ctx.Err(); err != nil {
			return 1
		}
	}
	return 0
}

func parseShufOptions(args []string, c *CommandContext) (shufOptions, int) {
	var options shufOptions
	parsingOptions := true
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if parsingOptions && arg == "--" {
			parsingOptions = false
			continue
		}
		if !parsingOptions || arg == "-" || !strings.HasPrefix(arg, "-") {
			options.files = append(options.files, arg)
			continue
		}
		switch {
		case arg == "-r" || arg == "--repeat":
			options.repeat = true
		case arg == "-n" || arg == "--head-count":
			if index+1 >= len(args) {
				fmt.Fprintf(c.Stderr, "shuf: option %q requires an argument\n", arg)
				return options, 1
			}
			index++
			if !setShufCount(args[index], &options, c) {
				return options, 1
			}
		case strings.HasPrefix(arg, "--head-count="):
			if !setShufCount(strings.TrimPrefix(arg, "--head-count="), &options, c) {
				return options, 1
			}
		case arg == "-i" || arg == "--input-range":
			if index+1 >= len(args) {
				fmt.Fprintf(c.Stderr, "shuf: option %q requires an argument\n", arg)
				return options, 1
			}
			index++
			if !setShufRange(args[index], &options, c) {
				return options, 1
			}
		case strings.HasPrefix(arg, "--input-range="):
			if !setShufRange(strings.TrimPrefix(arg, "--input-range="), &options, c) {
				return options, 1
			}
		default:
			return options, commandhelp.UnknownOption(c, "shuf", arg)
		}
	}
	return options, 0
}

func setShufCount(value string, options *shufOptions, c *CommandContext) bool {
	count, err := strconv.Atoi(value)
	if err != nil || count < 0 {
		fmt.Fprintf(c.Stderr, "shuf: invalid line count %q\n", value)
		return false
	}
	options.count = count
	options.countSet = true
	return true
}

func setShufRange(value string, options *shufOptions, c *CommandContext) bool {
	matches := shufRangePattern.FindStringSubmatch(value)
	if matches == nil {
		fmt.Fprintf(c.Stderr, "shuf: invalid input range %q\n", value)
		return false
	}
	low, lowErr := strconv.Atoi(matches[1])
	high, highErr := strconv.Atoi(matches[2])
	if lowErr != nil || highErr != nil || low > high {
		fmt.Fprintf(c.Stderr, "shuf: invalid input range %q\n", value)
		return false
	}
	rangeSize := new(big.Int).Sub(big.NewInt(int64(high)), big.NewInt(int64(low)))
	rangeSize.Add(rangeSize, big.NewInt(1))
	if rangeSize.Cmp(big.NewInt(maxShufRangeItems)) > 0 {
		fmt.Fprintf(c.Stderr, "shuf: input range %q is too large (maximum %d values)\n", value, maxShufRangeItems)
		return false
	}
	options.low, options.high, options.rangeSet = low, high, true
	return true
}
