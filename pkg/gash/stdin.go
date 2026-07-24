//go:build !js || !wasm

package gash

import (
	"io"
	"strings"
)

func interpreterStdin(stdin string) io.Reader {
	return strings.NewReader(stdin)
}
