package fs

import iofs "io/fs"

type readOnly struct {
	filesystem iofs.FS
}

// ReadOnly returns a least-authority view of filesystem. Optional mutation
// capabilities implemented by the underlying filesystem are intentionally not
// exposed through the returned interface.
func ReadOnly(filesystem iofs.FS) iofs.FS {
	return readOnly{filesystem: filesystem}
}

func (r readOnly) Open(name string) (iofs.File, error) {
	return r.filesystem.Open(name)
}
