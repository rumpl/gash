package gash

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	internalDeclarationPrintCommand = "__gash_declaration_print"
	internalUmaskPushCommand        = "__gash_umask_push"
	internalUmaskPopCommand         = "__gash_umask_pop"
)

func rewriteDeclarationPrinting(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		statement, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}
		declaration, ok := statement.Cmd.(*syntax.DeclClause)
		if !ok || declaration.Variant == nil {
			return true
		}
		variant := declaration.Variant.Value
		if variant != "export" && variant != "declare" && variant != "readonly" {
			return true
		}
		printRequested := false
		if len(declaration.Args) == 1 {
			argument := declaration.Args[0]
			printRequested = argument.Name != nil && argument.Name.Value == "-p" || argument.Value != nil && argument.Value.Lit() == "-p"
		}
		if !printRequested {
			return true
		}
		statement.Cmd = &syntax.CallExpr{Args: []*syntax.Word{
			{Parts: []syntax.WordPart{&syntax.Lit{Value: internalDeclarationPrintCommand}}},
			{Parts: []syntax.WordPart{&syntax.Lit{Value: variant}}},
		}}
		return true
	})
}

func normalizeClobberRedirects(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		redirect, ok := node.(*syntax.Redirect)
		if !ok || redirect.Op != syntax.ClbOut || redirect.Word == nil {
			return true
		}
		redirect.Op = syntax.RdrOut
		redirect.Word.Parts = append(
			[]syntax.WordPart{&syntax.Lit{Value: forceClobberPrefix}},
			redirect.Word.Parts...,
		)
		return true
	})
}

func rewriteUmaskBuiltin(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 || call.Args[0].Lit() != "umask" {
			return true
		}
		call.Args[0].Parts = []syntax.WordPart{&syntax.Lit{Value: "/bin/umask"}}
		return true
	})
}

func rewritePrintfBuiltin(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 || call.Args[0].Lit() != "printf" {
			return true
		}
		if len(call.Args) > 1 {
			option, static := staticWordText(call.Args[1])
			if static && option == "-v" {
				return true
			}
		}
		call.Args[0].Parts = []syntax.WordPart{&syntax.Lit{Value: "/bin/printf"}}
		return true
	})
}

func staticWordText(word *syntax.Word) (string, bool) {
	var text strings.Builder
	var appendParts func([]syntax.WordPart) bool
	appendParts = func(parts []syntax.WordPart) bool {
		for _, part := range parts {
			switch part := part.(type) {
			case *syntax.Lit:
				text.WriteString(part.Value)
			case *syntax.SglQuoted:
				if part.Dollar {
					return false
				}
				text.WriteString(part.Value)
			case *syntax.DblQuoted:
				if !appendParts(part.Parts) {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	if word == nil || !appendParts(word.Parts) {
		return "", false
	}
	return text.String(), true
}

func normalizeSubshellSemantics(program syntax.Node) {
	// mvdan intentionally runs the rightmost pipeline component in the parent
	// runner. Bash only does that with the non-default lastpipe option, so wrap
	// each right-hand component in an explicit isolated runner.
	syntax.Walk(program, func(node syntax.Node) bool {
		binary, ok := node.(*syntax.BinaryCmd)
		if !ok || binary.Op != syntax.Pipe && binary.Op != syntax.PipeAll {
			return true
		}
		original := binary.Y
		binary.Y = &syntax.Stmt{Cmd: &syntax.Subshell{Stmts: []*syntax.Stmt{original}}}
		return true
	})

	// mvdan's explicit subshell and command-substitution paths execute statement
	// lists directly rather than calling Runner.Run. Bracket process-local umask
	// mutations and add an implicit final exit so EXIT callbacks still run. Do
	// not touch umask state for ordinary pipeline subshells: those run concurrently.
	syntax.Walk(program, func(node syntax.Node) bool {
		var statements *[]*syntax.Stmt
		switch scope := node.(type) {
		case *syntax.Subshell:
			statements = &scope.Stmts
		case *syntax.CmdSubst:
			statements = &scope.Stmts
		}
		if statements == nil {
			return true
		}
		if statementsMutateUmask(*statements) {
			*statements = append([]*syntax.Stmt{literalStatement(internalUmaskPushCommand)}, *statements...)
			*statements = append(*statements, umaskScopeRestoreStatements()...)
		} else {
			*statements = append(*statements, literalStatement("exit"))
		}
		return true
	})
}

func statementsMutateUmask(statements []*syntax.Stmt) bool {
	for _, statement := range statements {
		if nodeMutatesUmask(statement) {
			return true
		}
	}
	return false
}

func nodeMutatesUmask(root syntax.Node) bool {
	mutates := false
	syntax.Walk(root, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) < 2 || call.Args[0].Lit() != "/bin/umask" && call.Args[0].Lit() != "umask" {
			return true
		}
		for _, argument := range call.Args[1:] {
			value := argument.Lit()
			if value != "-S" && value != "-p" && value != "--" {
				mutates = true
				return false
			}
		}
		return true
	})
	return mutates
}

func umaskScopeRestoreStatements() []*syntax.Stmt {
	parsed, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(
		strings.NewReader("__gash_umask_status=$?; "+internalUmaskPopCommand+"; exit \"$__gash_umask_status\""),
		"umask-scope",
	)
	if err != nil {
		panic(err) // static internal syntax
	}
	return parsed.Stmts
}

func literalStatement(name string) *syntax.Stmt {
	return &syntax.Stmt{Cmd: &syntax.CallExpr{Args: []*syntax.Word{{Parts: []syntax.WordPart{&syntax.Lit{Value: name}}}}}}
}
