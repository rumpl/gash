package utilities

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandhelp"
)

func UUIDGenCommand() command.Command {
	return command.Command{Name: "uuidgen", Run: commandUUIDGen}
}

func commandUUIDGen(_ context.Context, args []string, commandCtx *command.Context) int {
	endOptions := false
	for _, arg := range args {
		if endOptions {
			fmt.Fprintf(commandCtx.Stderr, "uuidgen: extra operand '%s'\n", arg)
			return 1
		}
		switch arg {
		case "-r", "--random":
			// Random version 4 UUIDs are the default and only supported mode.
		case "--":
			endOptions = true
		default:
			if strings.HasPrefix(arg, "-") {
				return commandhelp.UnknownOption(commandCtx, "uuidgen", arg)
			}
			fmt.Fprintf(commandCtx.Stderr, "uuidgen: extra operand '%s'\n", arg)
			return 1
		}
	}

	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		fmt.Fprintf(commandCtx.Stderr, "uuidgen: random UUID generation failed: %v\n", err)
		return 1
	}
	uuid[6] = uuid[6]&0x0f | 0x40
	uuid[8] = uuid[8]&0x3f | 0x80

	var encoded [36]byte
	hex.Encode(encoded[0:8], uuid[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], uuid[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], uuid[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], uuid[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], uuid[10:16])
	fmt.Fprintln(commandCtx.Stdout, string(encoded[:]))
	return 0
}
