package gash

import (
	"context"

	gfs "github.com/rumpl/gash/pkg/fs"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

const internalReadablePredicateCommand = "__gash_readable_predicate"

func rewriteReadableTestClauses(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		statement, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}
		clause, ok := statement.Cmd.(*syntax.TestClause)
		if !ok {
			return true
		}
		predicate, ok := clause.X.(*syntax.UnaryTest)
		if !ok || predicate.Op != syntax.TsRead {
			return true
		}
		operand, ok := predicate.X.(*syntax.Word)
		if !ok {
			return true
		}
		statement.Cmd = &syntax.CallExpr{Args: []*syntax.Word{
			{Parts: []syntax.WordPart{&syntax.Lit{Value: internalReadablePredicateCommand}}},
			operand,
		}}
		return true
	})
}

func (b *Bash) virtualReadablePredicate(ctx context.Context, argv []string) ([]string, bool) {
	if len(argv) < 3 || argv[0] != "test" && argv[0] != "[" {
		return argv, false
	}
	arguments := argv[1:]
	if argv[0] == "[" {
		if arguments[len(arguments)-1] != "]" {
			return argv, false
		}
		arguments = arguments[:len(arguments)-1]
	}
	negated := false
	if len(arguments) == 3 && arguments[0] == "!" {
		negated = true
		arguments = arguments[1:]
	}
	if len(arguments) != 2 || arguments[0] != "-r" {
		return argv, false
	}
	readable := b.pathIsReadable(interp.HandlerCtx(ctx).Dir, arguments[1])
	if readable != negated {
		return []string{"true"}, true
	}
	return []string{"false"}, true
}

func (b *Bash) pathIsReadable(cwd, name string) bool {
	file, err := b.FS.Open(gfs.Name(resolve(cwd, name)))
	if err != nil {
		return false
	}
	return file.Close() == nil
}
