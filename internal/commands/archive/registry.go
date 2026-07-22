package archive

import "github.com/rumpl/gash/internal/command"

func Commands() []command.Command {
	return []command.Command{
		{Name: "gzip", Run: commandGzip},
		{Name: "gunzip", Run: commandGunzip},
		{Name: "zcat", Run: commandZcat},
		{Name: "tar", Run: commandTar},
	}
}
