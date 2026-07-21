package commands

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
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
	decode := len(args) > 0 && (args[0] == "-d" || args[0] == "--decode")
	if decode {
		args = args[1:]
	}
	d, e := readInputs(args, c)
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
		fmt.Fprintln(c.Stdout, base64.StdEncoding.EncodeToString(d))
	}
	return 0
}

func checksum(kind string) CommandFunc {
	return func(_ context.Context, args []string, c *CommandContext) int {
		d, e := readInputs(args, c)
		if e != nil {
			return report(c, kind+"sum", e)
		}
		var sum string
		switch kind {
		case "md5":
			sum = fmt.Sprintf("%x", md5.Sum(d))
		case "sha1":
			sum = fmt.Sprintf("%x", sha1.Sum(d))
		default:
			sum = fmt.Sprintf("%x", sha256.Sum256(d))
		}
		name := "-"
		if len(args) > 0 {
			name = args[0]
		}
		fmt.Fprintf(c.Stdout, "%s  %s\n", sum, name)
		return 0
	}
}
