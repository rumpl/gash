package commands

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rumpl/gash/internal/commandhelp"
)

func commandSleep(ctx context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		return 1
	}
	seconds, e := strconv.ParseFloat(args[0], 64)
	if e != nil {
		return report(c, "sleep", e)
	}
	select {
	case <-time.After(time.Duration(seconds * float64(time.Second))):
		return 0
	case <-ctx.Done():
		return 124
	}
}

func commandSeq(ctx context.Context, args []string, c *CommandContext) int {
	if len(args) == 0 {
		return 1
	}
	start, end := 1, 0
	if len(args) == 1 {
		end, _ = strconv.Atoi(args[0])
	} else {
		start, _ = strconv.Atoi(args[0])
		end, _ = strconv.Atoi(args[1])
	}
	for i := start; i <= end; i++ {
		select {
		case <-ctx.Done():
			return 124
		default:
		}
		fmt.Fprintln(c.Stdout, i)
	}
	return 0
}

func commandBase64(_ context.Context, args []string, c *CommandContext) int {
	decode := false
	wrap := 0
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-d" || arg == "--decode":
			decode = true
		case arg == "-w" && i+1 < len(args):
			i++
			wrap, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(arg, "-w") && len(arg) > 2:
			wrap, _ = strconv.Atoi(arg[2:])
		case strings.HasPrefix(arg, "--wrap="):
			wrap, _ = strconv.Atoi(strings.TrimPrefix(arg, "--wrap="))
		case strings.HasPrefix(arg, "-") && arg != "-":
			return commandhelp.UnknownOption(c, "base64", arg)
		default:
			files = append(files, arg)
		}
	}
	d, e := readInputs(files, c)
	if e != nil {
		return report(c, "base64", e)
	}
	if decode {
		out, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(d)))
		if e != nil {
			return report(c, "base64", e)
		}
		c.Stdout.Write(out)
	} else {
		encoded := base64.StdEncoding.EncodeToString(d)
		if wrap > 0 {
			for len(encoded) > wrap {
				fmt.Fprintln(c.Stdout, encoded[:wrap])
				encoded = encoded[wrap:]
			}
		}
		fmt.Fprintln(c.Stdout, encoded)
	}
	return 0
}

func commandCksum(_ context.Context, args []string, c *CommandContext) int {
	operands := make([]string, 0, len(args))
	optionsDone := false
	for _, arg := range args {
		if !optionsDone && arg == "--" {
			optionsDone = true
			continue
		}
		if !optionsDone && strings.HasPrefix(arg, "-") && arg != "-" {
			return commandhelp.UnknownOption(c, "cksum", arg)
		}
		operands = append(operands, arg)
	}

	if len(operands) == 0 {
		data, err := readInputs(nil, c)
		if err != nil {
			return report(c, "cksum", err)
		}
		fmt.Fprintf(c.Stdout, "%d %d\n", posixChecksum(data), len(data))
		return 0
	}

	code := 0
	for _, operand := range operands {
		data, err := readInputs([]string{operand}, c)
		if err != nil {
			report(c, "cksum: "+operand, err)
			code = 1
			continue
		}
		fmt.Fprintf(c.Stdout, "%d %d %s\n", posixChecksum(data), len(data), operand)
	}
	return code
}

func posixChecksum(data []byte) uint32 {
	const polynomial uint32 = 0x04c11db7

	crc := uint32(0)
	update := func(value byte) {
		crc ^= uint32(value) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ polynomial
			} else {
				crc <<= 1
			}
		}
	}
	for _, value := range data {
		update(value)
	}
	for length := uint64(len(data)); length != 0; length >>= 8 {
		update(byte(length))
	}
	return ^crc
}

func checksum(kind string) CommandFunc {
	return func(_ context.Context, args []string, c *CommandContext) int {
		commandName := kind + "sum"
		files := make([]string, 0, len(args))
		parseOptions := true
		for _, arg := range args {
			if parseOptions && arg == "--" {
				parseOptions = false
				continue
			}
			if parseOptions && strings.HasPrefix(arg, "-") && arg != "-" {
				return commandhelp.UnknownOption(c, commandName, arg)
			}
			files = append(files, arg)
		}
		if len(files) == 0 {
			d, e := readInputs(nil, c)
			if e != nil {
				return report(c, commandName, e)
			}
			fmt.Fprintf(c.Stdout, "%s  -\n", checksumString(kind, d))
			return 0
		}

		code := 0
		for _, name := range files {
			d, e := readInputs([]string{name}, c)
			if e != nil {
				code = report(c, commandName+": "+name, e)
				continue
			}
			fmt.Fprintf(c.Stdout, "%s  %s\n", checksumString(kind, d), name)
		}
		return code
	}
}

func checksumString(kind string, d []byte) string {
	switch kind {
	case "md5":
		return fmt.Sprintf("%x", md5.Sum(d))
	case "sha1":
		return fmt.Sprintf("%x", sha1.Sum(d))
	case "sha512":
		return fmt.Sprintf("%x", sha512.Sum512(d))
	default:
		return fmt.Sprintf("%x", sha256.Sum256(d))
	}
}
