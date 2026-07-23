package gash

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"mvdan.cc/sh/v3/syntax"
)

type arithmeticErrorWriter struct {
	target io.Writer
	cancel context.CancelFunc
	seen   atomic.Bool
	mu     sync.Mutex
	line   bytes.Buffer
}

func (w *arithmeticErrorWriter) Write(data []byte) (int, error) {
	written, err := w.target.Write(data)
	w.mu.Lock()
	for _, character := range data {
		if character == '\n' {
			if isFatalArithmeticDiagnostic(strings.TrimSpace(w.line.String())) {
				w.seen.Store(true)
				w.cancel()
			}
			w.line.Reset()
			continue
		}
		w.line.WriteByte(character)
	}
	w.mu.Unlock()
	return written, err
}

func isFatalArithmeticDiagnostic(message string) bool {
	return message == "division by zero" ||
		strings.HasPrefix(message, "invalid number") ||
		strings.HasPrefix(message, "exponent must be positive")
}

func containsArithmeticExpansion(program syntax.Node) bool {
	found := false
	syntax.Walk(program, func(node syntax.Node) bool {
		if _, ok := node.(*syntax.ArithmExp); ok {
			found = true
			return false
		}
		return !found
	})
	return found
}
