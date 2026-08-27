package gash

import (
	"context"
	"errors"
	"io"
	"sync"
)

const streamBufferSize = 32 << 10

// executionPipe is bounded so producers cannot outrun consumers. A small
// buffer avoids the goroutine rendezvous overhead of io.Pipe while preserving
// backpressure, and context cancellation wakes blocked readers and writers.
type executionPipe struct {
	ctx   context.Context
	scope *executionScope

	mu       sync.Mutex
	buffer   []byte
	readAt   int
	writeAt  int
	used     int
	closedR  bool
	closedW  bool
	closeErr error
	changed  chan struct{}
}

type (
	executionPipeReader struct{ pipe *executionPipe }
	executionPipeWriter struct{ pipe *executionPipe }
)

func newExecutionPipe(ctx context.Context, scope *executionScope) (io.ReadCloser, io.WriteCloser) {
	pipe := &executionPipe{ctx: ctx, scope: scope, buffer: make([]byte, streamBufferSize), changed: make(chan struct{})}
	context.AfterFunc(ctx, func() { pipe.signal() })
	return &executionPipeReader{pipe}, &executionPipeWriter{pipe}
}

func (p *executionPipe) signal() {
	p.mu.Lock()
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
}

func (r *executionPipeReader) Read(dst []byte) (int, error) {
	p := r.pipe
	for {
		p.mu.Lock()
		if p.used > 0 {
			n := min(len(dst), p.used, len(p.buffer)-p.readAt)
			copy(dst, p.buffer[p.readAt:p.readAt+n])
			p.readAt = (p.readAt + n) % len(p.buffer)
			p.used -= n
			close(p.changed)
			p.changed = make(chan struct{})
			p.mu.Unlock()
			return n, nil
		}
		if p.closedW {
			err := p.closeErr
			p.mu.Unlock()
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
		if err := p.ctx.Err(); err != nil {
			p.mu.Unlock()
			return 0, err
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-changed:
		case <-p.ctx.Done():
			return 0, p.ctx.Err()
		}
	}
}

func (r *executionPipeReader) Close() error {
	p := r.pipe
	p.mu.Lock()
	if !p.closedR {
		p.closedR = true
		close(p.changed)
		p.changed = make(chan struct{})
	}
	p.mu.Unlock()
	return nil
}

func (w *executionPipeWriter) Write(src []byte) (int, error) {
	p := w.pipe
	written := 0
	for len(src) > 0 {
		p.mu.Lock()
		if p.closedW {
			p.mu.Unlock()
			return written, io.ErrClosedPipe
		}
		if p.closedR {
			p.mu.Unlock()
			return written, io.ErrClosedPipe
		}
		if err := p.ctx.Err(); err != nil {
			p.mu.Unlock()
			return written, err
		}
		if p.used < len(p.buffer) {
			n := min(len(src), len(p.buffer)-p.used, len(p.buffer)-p.writeAt)
			n = p.scope.output.take(n)
			if n == 0 {
				p.mu.Unlock()
				if err := p.ctx.Err(); err != nil {
					return written, err
				}
				return written, io.ErrShortWrite
			}
			copy(p.buffer[p.writeAt:p.writeAt+n], src[:n])
			p.writeAt = (p.writeAt + n) % len(p.buffer)
			p.used += n
			written += n
			src = src[n:]
			close(p.changed)
			p.changed = make(chan struct{})
			p.mu.Unlock()
			continue
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-changed:
		case <-p.ctx.Done():
			return written, p.ctx.Err()
		}
	}
	return written, nil
}

func (w *executionPipeWriter) Close() error {
	p := w.pipe
	p.mu.Lock()
	if !p.closedW {
		p.closedW = true
		close(p.changed)
		p.changed = make(chan struct{})
	}
	p.mu.Unlock()
	return nil
}

func (w *executionPipeWriter) CloseWithError(err error) error {
	p := w.pipe
	p.mu.Lock()
	if !p.closedW {
		p.closedW = true
		if !errors.Is(err, io.EOF) {
			p.closeErr = err
		}
		close(p.changed)
		p.changed = make(chan struct{})
	}
	p.mu.Unlock()
	return nil
}
