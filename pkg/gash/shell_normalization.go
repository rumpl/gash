package gash

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

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

func serializeBackgroundStatements(program syntax.Node) {
	// mvdan v3.10 backgrounds a runner whose environment overlays the parent
	// environment while the parent continues mutating it, which is racy. Until
	// isolated job environments are available, execute virtual jobs serially.
	syntax.Walk(program, func(node syntax.Node) bool {
		statement, ok := node.(*syntax.Stmt)
		if ok {
			statement.Background = false
		}
		return true
	})
}

func rewriteWaitBuiltin(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		name := call.Args[0].Lit()
		if (name == "command" || name == "builtin") && call.Args[1].Lit() == "wait" {
			call.Args = call.Args[1:2]
			return true
		}
		if name != "wait" {
			return true
		}
		// mvdan v3.10 panics for argument-bearing wait calls. It does not expose
		// individual virtual job IDs to handlers, so safely wait for all jobs.
		call.Args = call.Args[:1]
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
	// lists directly rather than calling Runner.Run, so their EXIT callback is
	// otherwise skipped. An implicit final exit preserves the previous status
	// through Runner.lastExit and invokes a trap installed inside that scope.
	syntax.Walk(program, func(node syntax.Node) bool {
		switch scope := node.(type) {
		case *syntax.Subshell:
			scope.Stmts = append(scope.Stmts, literalStatement("exit"))
		case *syntax.CmdSubst:
			scope.Stmts = append(scope.Stmts, literalStatement("exit"))
		}
		return true
	})
}

func literalStatement(name string) *syntax.Stmt {
	return &syntax.Stmt{Cmd: &syntax.CallExpr{Args: []*syntax.Word{{Parts: []syntax.WordPart{&syntax.Lit{Value: name}}}}}}
}
