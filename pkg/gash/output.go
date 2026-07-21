package gash

import (
	"bytes"
	"sync"
)

type boundedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
	budget *outputBudget
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	allowed := b.budget.take(len(p))
	if allowed > 0 {
		_, _ = b.Buffer.Write(p[:allowed])
	}
	return len(p), nil
}

func (b *boundedBuffer) WriteString(s string) (int, error) {
	return b.Write([]byte(s))
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}
