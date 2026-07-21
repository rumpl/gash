package curl_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rumpl/gash/pkg/gash"
	"github.com/rumpl/gash/pkg/network"
)

func TestCurlUnavailableByDefaultAndOptInFetch(t *testing.T) {
	shell, err := gash.New(gash.Options{})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `curl https://example.com`, gash.ExecOptions{})
	if result.ExitCode != 127 || !strings.Contains(result.Stderr, "command not found") {
		t.Fatalf("curl should be unavailable by default: %#v", result)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ok" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		fmt.Fprint(w, "hello")
	}))
	defer server.Close()
	policy := network.NewPolicy(network.AllowOrigin(server.URL + "/ok"))
	policy.AllowPrivateIPs = true
	shell, err = gash.New(gash.Options{Network: &policy})
	if err != nil {
		t.Fatal(err)
	}
	result = shell.Exec(context.Background(), `curl `+server.URL+`/ok`, gash.ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "hello" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestCurlPolicyRevalidatesRedirects(t *testing.T) {
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "blocked") }))
	defer blocked.Close()
	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blocked.URL+"/secret", http.StatusFound)
	}))
	defer start.Close()
	policy := network.NewPolicy(network.AllowOrigin(start.URL))
	policy.AllowPrivateIPs = true
	shell, err := gash.New(gash.Options{Network: &policy})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `curl -L `+start.URL, gash.ExecOptions{})
	if result.ExitCode == 0 || !strings.Contains(result.Stderr, "not allowed by policy") {
		t.Fatalf("redirect was not denied: %#v", result)
	}
}

func TestCurlDataAuthHeadersWriteOutAndVirtualFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		user, pass, ok := r.BasicAuth()
		if !ok || user != "u" || pass != "p" {
			t.Fatalf("missing auth")
		}
		if r.Header.Get("X-Test") != "yes" {
			t.Fatalf("missing header")
		}
		fmt.Fprintf(w, "%s:%s", r.Method, string(body))
	}))
	defer server.Close()
	policy := network.NewPolicy(network.AllowOrigin(server.URL))
	policy.AllowPrivateIPs = true
	shell, err := gash.New(gash.Options{Network: &policy, Files: map[string]string{"/payload.txt": "a=1"}})
	if err != nil {
		t.Fatal(err)
	}
	result := shell.Exec(context.Background(), `curl -u u:p -H 'X-Test: yes' -d @/payload.txt -o out.txt -w '%{http_code}
' `+server.URL, gash.ExecOptions{})
	if result.ExitCode != 0 || result.Stdout != "200\n" {
		t.Fatalf("unexpected result: %#v", result)
	}
	check := shell.Exec(context.Background(), `cat out.txt`, gash.ExecOptions{})
	if check.Stdout != "POST:a=1" {
		t.Fatalf("output file=%#v", check)
	}
}
