package files

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func commandStat(_ context.Context, args []string, c *CommandContext) int {
	format := ""
	var files []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" && i+1 < len(args) {
			format = args[i+1]
			i++
		} else {
			files = append(files, args[i])
		}
	}
	if len(files) == 0 {
		return report(c, "stat", fmt.Errorf("missing operand"))
	}
	code := 0
	for _, name := range files {
		info, err := gfs.Stat(c.FS, abs(c, name))
		if err != nil {
			fmt.Fprintf(c.Stderr, "stat: cannot stat '%s': No such file or directory\n", name)
			code = 1
			continue
		}
		if format != "" {
			kind := "regular file"
			if info.IsDir() {
				kind = "directory"
			}
			modeString := info.Mode().String()
			r := strings.NewReplacer("%n", name, "%N", "'"+name+"'", "%s", strconv.FormatInt(info.Size(), 10), "%F", kind, "%a", fmt.Sprintf("%o", info.Mode().Perm()), "%A", modeString, "%u", "1000", "%U", "user", "%g", "1000", "%G", "group")
			fmt.Fprintln(c.Stdout, r.Replace(format))
		} else {
			fmt.Fprintf(c.Stdout, "  File: %s\n  Size: %d\t\tBlocks: %d\nAccess: (%04o/%s)\nModify: %s\n", name, info.Size(), (info.Size()+511)/512, info.Mode().Perm(), info.Mode().String(), info.ModTime().UTC().Format("2006-01-02T15:04:05.000Z"))
		}
	}
	return code
}

var textExtensions = map[string][2]string{".go": {"Go source", "text/x-go"}, ".js": {"JavaScript source", "text/javascript"}, ".ts": {"TypeScript source", "text/typescript"}, ".py": {"Python script", "text/x-python"}, ".sh": {"Bourne-Again shell script", "text/x-shellscript"}, ".json": {"JSON data", "application/json"}, ".yaml": {"YAML data", "text/yaml"}, ".yml": {"YAML data", "text/yaml"}, ".xml": {"XML document", "application/xml"}, ".csv": {"CSV text", "text/csv"}, ".html": {"HTML document", "text/html"}, ".css": {"CSS stylesheet", "text/css"}, ".md": {"Markdown document", "text/markdown"}, ".txt": {"ASCII text", "text/plain"}}
