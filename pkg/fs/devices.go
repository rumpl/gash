package fs

import (
	iofs "io/fs"
	"time"
)

type nullFileInfo struct{}

func (nullFileInfo) Name() string {
	return "null"
}

func (nullFileInfo) Size() int64 {
	return 0
}

func (nullFileInfo) Mode() iofs.FileMode {
	return iofs.ModeDevice | iofs.ModeCharDevice | 0o666
}

func (nullFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (nullFileInfo) IsDir() bool {
	return false
}

func (nullFileInfo) Sys() any {
	return nil
}
