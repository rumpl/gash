//go:build js && wasm

package gash

import (
	"io"
	"strings"
)

func interpreterStdin(stdin string) io.Reader {
	// mvdan's interpreter converts non-nil stdin to an os.Pipe, which Go's
	// js/wasm port does not implement. Most browser executions have no stdin;
	// preserving nil here lets those scripts run normally.
	if stdin == "" {
		return nil
	}
	return strings.NewReader(stdin)
}
