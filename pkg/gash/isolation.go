package gash

import (
	"fmt"
	"strconv"

	"mvdan.cc/sh/v3/syntax"
)

const (
	virtualPID  = "2000"
	virtualPPID = "1999"
)

var internalEnv = map[string]string{
	"PATH":      "/usr/bin:/bin",
	"OSTYPE":    "linux-gnu",
	"MACHTYPE":  "x86_64-pc-linux-gnu",
	"HOSTTYPE":  "x86_64",
	"HOSTNAME":  "localhost",
	"USER":      "user",
	"UID":       "1000",
	"EUID":      "1000",
	"GID":       "1000",
	"PPID":      virtualPPID,
	"GASH_PID":  virtualPID,
	"GASH_PPID": virtualPPID,
}

func executionEnv(base map[string]string) map[string]string {
	env := cloneMap(base)
	if env == nil {
		env = map[string]string{}
	}
	enforceInternalEnv(env)
	return env
}

func enforceInternalEnv(env map[string]string) {
	for k, v := range internalEnv {
		env[k] = v
	}
}

func enforcePublicInternalEnv(env map[string]string) {
	for k, v := range internalEnv {
		if !isHiddenInternalEnv(k) {
			env[k] = v
		}
	}
}

func isHiddenInternalEnv(name string) bool {
	return name == "GASH_PID" || name == "GASH_PPID"
}

func virtualizeHostParameters(program syntax.Node) {
	syntax.Walk(program, func(node syntax.Node) bool {
		param, ok := node.(*syntax.ParamExp)
		if !ok || param.Param == nil {
			return true
		}
		switch param.Param.Value {
		case "$":
			param.Param.Value = "GASH_PID"
		case "PPID":
			param.Param.Value = "GASH_PPID"
		case "!":
			param.Param.Value = lastBackgroundVariable
		}
		return true
	})
}

func rejectHostBackedSyntax(program syntax.Node) error {
	var err error
	syntax.Walk(program, func(node syntax.Node) bool {
		if err != nil || node == nil {
			return false
		}
		if _, ok := node.(*syntax.CoprocClause); ok {
			err = fmt.Errorf("coproc is not supported in isolated execution")
			return false
		}
		if parameter, ok := node.(*syntax.ParamExp); ok && parameter.Param != nil {
			switch parameter.Param.Value {
			case "PIPESTATUS":
				err = fmt.Errorf("PIPESTATUS is not supported by the isolated interpreter")
				return false
			}
		}
		if call, ok := node.(*syntax.CallExpr); ok && unsupportedPrintfAssignment(call) {
			err = fmt.Errorf("printf -v is not supported by the isolated interpreter")
			return false
		}
		if _, ok := node.(*syntax.ProcSubst); ok {
			err = fmt.Errorf("process substitution is not supported in isolated execution")
			return false
		}
		if redirect, ok := node.(*syntax.Redirect); ok && redirect.N != nil {
			descriptor, parseErr := strconv.Atoi(redirect.N.Value)
			if parseErr != nil || descriptor > 2 {
				err = fmt.Errorf("file descriptor %q is not supported; only 0, 1, and 2 are available", redirect.N.Value)
				return false
			}
		}
		return true
	})
	return err
}

func unsupportedPrintfAssignment(call *syntax.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	name := call.Args[0].Lit()
	argumentIndex := 1
	if (name == "builtin" || name == "command") && len(call.Args) > 2 && call.Args[1].Lit() == "printf" {
		argumentIndex = 2
	} else if name != "printf" {
		return false
	}
	return len(call.Args) > argumentIndex && call.Args[argumentIndex].Lit() == "-v"
}
