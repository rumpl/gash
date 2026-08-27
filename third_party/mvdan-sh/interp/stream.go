package interp

import (
	"context"
	"io"
)

// streamPipe is an in-memory, backpressured pipe. Closing both ends when the
// execution context ends makes blocked readers and writers observe cancellation.
func streamPipe(ctx context.Context) (io.ReadCloser, io.WriteCloser) {
	reader, writer := io.Pipe()
	context.AfterFunc(ctx, func() {
		err := ctx.Err()
		_ = reader.CloseWithError(err)
		_ = writer.CloseWithError(err)
	})
	return reader, writer
}

func (r *Runner) pipe(ctx context.Context) (io.ReadCloser, io.WriteCloser) {
	if r.pipeHandler != nil {
		return r.pipeHandler(ctx)
	}
	return streamPipe(ctx)
}
