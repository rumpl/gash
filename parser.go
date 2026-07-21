package gash

import (
	"fmt"
	"strings"
	"unicode"
)

type simpleCommand struct {
	Args          []string
	Assign        map[string]string
	Input, Output string
	Append        bool
}
type commandChain struct {
	Gate     string
	Pipeline []simpleCommand
}

func parse(script string, env map[string]string) ([]commandChain, error) {
	tokens, err := lex(script, env)
	if err != nil {
		return nil, err
	}
	var out []commandChain
	chain := commandChain{}
	cmd := simpleCommand{Assign: map[string]string{}}
	gate := ""
	flushCmd := func() error {
		if len(cmd.Args) == 0 && len(cmd.Assign) == 0 {
			return fmt.Errorf("unexpected operator")
		}
		chain.Pipeline = append(chain.Pipeline, cmd)
		cmd = simpleCommand{Assign: map[string]string{}}
		return nil
	}
	flushChain := func() error {
		if len(cmd.Args) > 0 || len(cmd.Assign) > 0 {
			if err := flushCmd(); err != nil {
				return err
			}
		}
		if len(chain.Pipeline) == 0 {
			return nil
		}
		chain.Gate = gate
		out = append(out, chain)
		chain = commandChain{}
		return nil
	}
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch t {
		case "|":
			if err := flushCmd(); err != nil {
				return nil, err
			}
		case ";", "\n", "&&", "||":
			if err := flushChain(); err != nil {
				return nil, err
			}
			gate = t
			if t == ";" || t == "\n" {
				gate = ""
			}
		case ">", ">>", "<":
			if i+1 >= len(tokens) {
				return nil, fmt.Errorf("missing redirection target")
			}
			i++
			if t == "<" {
				cmd.Input = tokens[i]
			} else {
				cmd.Output = tokens[i]
				cmd.Append = t == ">>"
			}
		default:
			if len(cmd.Args) == 0 && strings.Contains(t, "=") {
				p := strings.SplitN(t, "=", 2)
				if validName(p[0]) {
					cmd.Assign[p[0]] = p[1]
					continue
				}
			}
			cmd.Args = append(cmd.Args, t)
		}
	}
	if len(cmd.Args) > 0 || len(cmd.Assign) > 0 || len(chain.Pipeline) > 0 {
		if err := flushChain(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func validName(s string) bool {
	if s == "" || (!unicode.IsLetter(rune(s[0])) && s[0] != '_') {
		return false
	}
	for _, r := range s[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func lex(s string, env map[string]string) ([]string, error) {
	var tokens []string
	var word strings.Builder
	quote := rune(0)
	escaped := false
	flush := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		ch := rune(s[i])
		if escaped {
			word.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
				continue
			}
			if ch == '$' && quote == '"' {
				value, n := expandVariable(s[i:], env)
				word.WriteString(value)
				i += n - 1
				continue
			}
			word.WriteRune(ch)
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '#':
			if word.Len() == 0 {
				for i < len(s) && s[i] != '\n' {
					i++
				}
				i--
				continue
			}
			word.WriteRune(ch)
		case ' ', '\t', '\r':
			flush()
		case '\n':
			flush()
			tokens = append(tokens, "\n")
		case '$':
			value, n := expandVariable(s[i:], env)
			word.WriteString(value)
			i += n - 1
		case ';':
			flush()
			tokens = append(tokens, ";")
		case '|':
			flush()
			if i+1 < len(s) && s[i+1] == '|' {
				tokens = append(tokens, "||")
				i++
			} else {
				tokens = append(tokens, "|")
			}
		case '&':
			flush()
			if i+1 < len(s) && s[i+1] == '&' {
				tokens = append(tokens, "&&")
				i++
			} else {
				return nil, fmt.Errorf("background jobs are not supported")
			}
		case '>':
			flush()
			if i+1 < len(s) && s[i+1] == '>' {
				tokens = append(tokens, ">>")
				i++
			} else {
				tokens = append(tokens, ">")
			}
		case '<':
			flush()
			tokens = append(tokens, "<")
		default:
			word.WriteRune(ch)
		}
	}
	if escaped {
		return nil, fmt.Errorf("trailing backslash")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	flush()
	return tokens, nil
}
func expandVariable(s string, env map[string]string) (string, int) {
	if len(s) < 2 {
		return "$", 1
	}
	if s[1] == '{' {
		end := strings.IndexByte(s, '}')
		if end < 0 {
			return "$", 1
		}
		expr := s[2:end]
		if p := strings.Index(expr, ":-"); p >= 0 {
			if v := env[expr[:p]]; v != "" {
				return v, end + 1
			}
			return expr[p+2:], end + 1
		}
		return env[expr], end + 1
	}
	if s[1] == '?' {
		return env["?"], 2
	}
	i := 1
	for i < len(s) && (s[i] == '_' || unicode.IsLetter(rune(s[i])) || unicode.IsDigit(rune(s[i]))) {
		i++
	}
	if i == 1 {
		return "$", 1
	}
	return env[s[1:i]], i
}
