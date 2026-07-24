//go:build js && wasm

package main

import (
	"context"
	"sync"
	"syscall/js"

	"github.com/rumpl/gash/pkg/gash"
)

var (
	shell     *gash.Bash
	execFunc  js.Func
	resetFunc js.Func
	shellMu   sync.RWMutex
)

func main() {
	if err := resetShell(); err != nil {
		panic(err)
	}

	execFunc = js.FuncOf(exec)
	resetFunc = js.FuncOf(reset)
	api := js.Global().Get("Object").New()
	api.Set("exec", execFunc)
	api.Set("reset", resetFunc)
	js.Global().Set("gash", api)

	select {}
}

func exec(_ js.Value, args []js.Value) any {
	promise := js.Global().Get("Promise")
	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, callbacks []js.Value) any {
		defer executor.Release()
		resolve, reject := callbacks[0], callbacks[1]
		go func() {
			if len(args) == 0 || args[0].Type() != js.TypeString {
				reject.Invoke("gash.exec requires a script string")
				return
			}
			options := gash.ExecOptions{}
			if len(args) > 1 && args[1].Type() == js.TypeObject {
				options.Stdin = stringProperty(args[1], "stdin")
				options.Cwd = stringProperty(args[1], "cwd")
				options.ScriptName = stringProperty(args[1], "scriptName")
				options.Args = stringArrayProperty(args[1], "args")
				options.Env = stringMapProperty(args[1], "env")
				options.ReplaceEnv = boolProperty(args[1], "replaceEnv")
			}
			shellMu.RLock()
			currentShell := shell
			shellMu.RUnlock()
			result := currentShell.Exec(context.Background(), args[0].String(), options)
			resolve.Invoke(resultValue(result))
		}()
		return nil
	})
	return promise.New(executor)
}

func reset(_ js.Value, _ []js.Value) any {
	if err := resetShell(); err != nil {
		return err.Error()
	}
	return nil
}

func resetShell() error {
	newShell, err := gash.New(gash.Options{})
	if err != nil {
		return err
	}
	shellMu.Lock()
	shell = newShell
	shellMu.Unlock()
	return nil
}

func resultValue(result gash.Result) js.Value {
	value := js.Global().Get("Object").New()
	value.Set("stdout", result.Stdout)
	value.Set("stderr", result.Stderr)
	value.Set("exitCode", result.ExitCode)
	env := js.Global().Get("Object").New()
	for key, item := range result.Env {
		env.Set(key, item)
	}
	value.Set("env", env)
	return value
}

func stringProperty(value js.Value, name string) string {
	property := value.Get(name)
	if property.Type() == js.TypeString {
		return property.String()
	}
	return ""
}

func boolProperty(value js.Value, name string) bool {
	property := value.Get(name)
	return property.Type() == js.TypeBoolean && property.Bool()
}

func stringArrayProperty(value js.Value, name string) []string {
	property := value.Get(name)
	if !js.Global().Get("Array").Call("isArray", property).Bool() {
		return nil
	}
	items := make([]string, 0, property.Length())
	for index := 0; index < property.Length(); index++ {
		item := property.Index(index)
		if item.Type() == js.TypeString {
			items = append(items, item.String())
		}
	}
	return items
}

func stringMapProperty(value js.Value, name string) map[string]string {
	property := value.Get(name)
	if property.Type() != js.TypeObject || property.IsNull() {
		return nil
	}
	keys := js.Global().Get("Object").Call("keys", property)
	items := make(map[string]string, keys.Length())
	for index := 0; index < keys.Length(); index++ {
		key := keys.Index(index).String()
		item := property.Get(key)
		if item.Type() == js.TypeString {
			items[key] = item.String()
		}
	}
	return items
}
