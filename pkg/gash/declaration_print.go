package gash

import (
	"fmt"
	"sort"
	"strconv"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
)

type declarationRecord struct {
	name     string
	value    string
	exported bool
	readonly bool
}

func runDeclarationPrint(args []string, handler interp.HandlerContext) int {
	if len(args) != 1 {
		fmt.Fprintln(handler.Stderr, "declare: invalid print request")
		return 2
	}
	variant := args[0]
	var records []declarationRecord
	handler.Env.Each(func(name string, variable expand.Variable) bool {
		if !variable.IsSet() || isHiddenInternalEnv(name) {
			return true
		}
		if (variant == "export" && !variable.Exported) || (variant == "readonly" && !variable.ReadOnly) {
			return true
		}
		records = append(records, declarationRecord{
			name:     name,
			value:    variable.String(),
			exported: variable.Exported,
			readonly: variable.ReadOnly,
		})
		return true
	})
	sort.Slice(records, func(left, right int) bool {
		return records[left].name < records[right].name
	})
	for _, record := range records {
		attributes := ""
		if variant == "export" || record.exported {
			attributes += "x"
		}
		if variant == "readonly" || record.readonly {
			attributes += "r"
		}
		if attributes == "" {
			attributes = "-"
		}
		fmt.Fprintf(handler.Stdout, "declare -%s %s=%s\n", attributes, record.name, strconv.Quote(record.value))
	}
	return 0
}
