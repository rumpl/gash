package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing/fstest"

	"github.com/rumpl/gash/pkg/gash"
	"github.com/rumpl/gash/pkg/network"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func main() {
	filesystem := fstest.MapFS{
		"config.txt": &fstest.MapFile{
			Data: []byte("mode=read-only\n"),
		},
	}

	var requests int
	policy := network.NewPolicy(network.Rule{
		Scheme:  "https",
		Host:    "api.example.test",
		Port:    "443",
		Path:    "/v1",
		Methods: []string{"GET"},
	})
	policy.MaxResponseBytes = 1024
	policy.Client = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("allowed response\n")),
				Request:    request,
			}, nil
		}),
	}

	shell, err := gash.New(gash.Options{
		FS:           filesystem,
		Cwd:          "/",
		LimitProfile: gash.HardenedProfile,
		Network:      &policy,
	})
	if err != nil {
		panic(err)
	}

	read := execute(shell, "read from the capability-scoped filesystem", `cat /config.txt`)
	require(read.ExitCode == 0, "expected the configured file to be readable")

	write := execute(shell, "attempt to modify the read-only filesystem", `printf 'changed\n' > /config.txt`)
	require(write.ExitCode != 0, "expected a write to the read-only filesystem to fail")

	unchanged := execute(shell, "verify the failed write changed nothing", `cat /config.txt`)
	require(unchanged.Stdout == "mode=read-only\n", "read-only file was unexpectedly modified")

	allowed := execute(shell, "request an explicitly allowed URL", `curl -s https://api.example.test/v1/status`)
	require(allowed.ExitCode == 0 && allowed.Stdout == "allowed response\n", "expected the allowed request to succeed")

	denied := execute(shell, "request an origin outside the network policy", `curl -sS https://evil.example.test/steal`)
	require(denied.ExitCode != 0, "expected the disallowed request to fail")
	require(requests == 1, "denied request reached the HTTP transport")

	hostCommand := execute(shell, "attempt to launch a host command", `uname -a`)
	require(hostCommand.ExitCode == 127, "expected unknown host command execution to be blocked")

	fmt.Println("All expected security boundaries were enforced.")
}

func execute(shell *gash.Bash, title, script string) gash.Result {
	fmt.Printf("\n--- %s ---\n", title)
	fmt.Printf("$ %s\n", script)
	result := shell.Exec(context.Background(), script, gash.ExecOptions{})
	fmt.Print(result.Stdout)
	fmt.Fprint(os.Stderr, result.Stderr)
	fmt.Printf("exit code: %d\n", result.ExitCode)
	return result
}

func require(condition bool, message string) {
	if !condition {
		panic(message)
	}
}
