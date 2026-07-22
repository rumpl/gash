package curl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/rumpl/gash/internal/command"
	"github.com/rumpl/gash/internal/commandhelp"
	gfs "github.com/rumpl/gash/pkg/fs"
	"github.com/rumpl/gash/pkg/network"
)

type CommandContext = command.Context

func Commands(policy network.Policy) []command.Command {
	return []command.Command{{Name: "curl", Run: Command(policy)}}
}

func Command(policy network.Policy) command.Func {
	policy = policy.Normalized()
	return func(ctx context.Context, args []string, c *CommandContext) int {
		if commandhelp.Requested(args) {
			return commandhelp.Show(c, helpInfo)
		}
		opts, err := parse(args)
		if err != nil {
			fmt.Fprintf(c.Stderr, "curl: %v\n", err)
			return 2
		}
		if opts.url == "" {
			fmt.Fprintln(c.Stderr, "curl: no URL specified")
			return 2
		}
		return run(ctx, opts, policy, c)
	}
}

type options struct {
	url            string
	method         string
	headers        http.Header
	dataParts      []string
	formParts      []string
	user           string
	includeHeaders bool
	headOnly       bool
	fail           bool
	location       bool
	verbose        bool
	silent         bool
	showError      bool
	writeOut       string
	output         string
	uploadFile     string
	maxTime        time.Duration
	connectTime    time.Duration
	cookie         string
	cookieJar      string
}

func parse(args []string) (options, error) {
	opts := options{headers: http.Header{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				opts.url = args[i+1]
			}
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			opts.url = arg
			continue
		}
		name, value, hasValue := splitOpt(arg)
		take := func() (string, error) {
			if hasValue {
				return value, nil
			}
			i++
			if i >= len(args) {
				return "", fmt.Errorf("option %s requires an argument", name)
			}
			return args[i], nil
		}
		switch name {
		case "-X", "--request":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.method = strings.ToUpper(v)
		case "-H", "--header":
			v, err := take()
			if err != nil {
				return opts, err
			}
			addHeader(opts.headers, v)
		case "-A", "--user-agent":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.headers.Set("User-Agent", v)
		case "-u", "--user":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.user = v
		case "-d", "--data", "--data-raw", "--data-binary", "--data-ascii", "--data-urlencode":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.dataParts = append(opts.dataParts, v)
		case "-F", "--form", "--form-string":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.formParts = append(opts.formParts, v)
		case "-I", "--head":
			opts.headOnly = true
		case "-i", "--include":
			opts.includeHeaders = true
		case "-f", "--fail":
			opts.fail = true
		case "-L", "--location":
			opts.location = true
		case "-v", "--verbose":
			opts.verbose = true
		case "-s", "--silent":
			opts.silent = true
		case "-S", "--show-error":
			opts.showError = true
		case "-w", "--write-out":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.writeOut = v
		case "-o", "--output":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.output = v
		case "-T", "--upload-file":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.uploadFile = v
		case "--max-time", "-m":
			v, err := take()
			if err != nil {
				return opts, err
			}
			d, err := parseSeconds(v)
			if err != nil {
				return opts, err
			}
			opts.maxTime = d
		case "--connect-timeout":
			v, err := take()
			if err != nil {
				return opts, err
			}
			d, err := parseSeconds(v)
			if err != nil {
				return opts, err
			}
			opts.connectTime = d
		case "-b", "--cookie":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.cookie = v
		case "-c", "--cookie-jar":
			v, err := take()
			if err != nil {
				return opts, err
			}
			opts.cookieJar = v
		default:
			if strings.HasPrefix(name, "--") {
				return opts, fmt.Errorf("unrecognized option %q", name)
			}
			for _, r := range strings.TrimPrefix(name, "-") {
				switch r {
				case 'i':
					opts.includeHeaders = true
				case 'I':
					opts.headOnly = true
				case 'f':
					opts.fail = true
				case 'L':
					opts.location = true
				case 'v':
					opts.verbose = true
				case 's':
					opts.silent = true
				case 'S':
					opts.showError = true
				default:
					return opts, fmt.Errorf("invalid option -- %q", string(r))
				}
			}
		}
	}
	return opts, nil
}

func splitOpt(arg string) (name, value string, hasValue bool) {
	if strings.HasPrefix(arg, "--") {
		if p := strings.IndexByte(arg, '='); p >= 0 {
			return arg[:p], arg[p+1:], true
		}
		return arg, "", false
	}
	if len(arg) > 2 && strings.ContainsAny(arg[:2], "XHAudFoTwmbc") {
		return arg[:2], arg[2:], true
	}
	return arg, "", false
}

func run(parent context.Context, opts options, policy network.Policy, c *CommandContext) int {
	ctx := parent
	cancel := func() {}
	if opts.maxTime > 0 {
		ctx, cancel = context.WithTimeout(parent, opts.maxTime)
	} else if policy.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, policy.Timeout)
	}
	defer cancel()

	req, bodyBytes, err := request(ctx, opts, policy, c)
	if err != nil {
		return curlError(c, opts, err)
	}
	if err := policy.Check(req); err != nil {
		return curlError(c, opts, err)
	}
	p := policy
	if !opts.location {
		p.MaxRedirects = -1
	}
	client := p.HTTPClient()
	if !opts.location {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	if opts.verbose {
		writeVerboseRequest(c, req, len(bodyBytes))
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return curlError(c, opts, errors.New("operation timed out"))
		}
		return curlError(c, opts, err)
	}
	defer resp.Body.Close()
	if opts.verbose {
		writeVerboseResponse(c, resp)
	}
	if opts.fail && resp.StatusCode >= 400 {
		return curlError(c, opts, fmt.Errorf("the requested URL returned error: %d", resp.StatusCode))
	}
	var out bytes.Buffer
	if opts.includeHeaders || opts.headOnly {
		writeStatusAndHeaders(&out, resp)
	}
	if !opts.headOnly {
		limited := io.LimitReader(resp.Body, policy.MaxResponseBytes+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			return curlError(c, opts, err)
		}
		if int64(len(data)) > policy.MaxResponseBytes {
			return curlError(c, opts, errors.New("response size limit exceeded"))
		}
		out.Write(data)
	}
	if opts.output != "" {
		if err := writeVirtualFile(c, opts.output, out.Bytes()); err != nil {
			return curlError(c, opts, err)
		}
	} else {
		_, _ = c.Stdout.Write(out.Bytes())
	}
	if opts.writeOut != "" {
		fmt.Fprint(c.Stdout, formatWriteOut(opts.writeOut, resp))
	}
	return 0
}

func request(ctx context.Context, opts options, policy network.Policy, c *CommandContext) (*http.Request, []byte, error) {
	method := opts.method
	if method == "" {
		method = "GET"
	}
	var body []byte
	contentType := ""
	if opts.uploadFile != "" {
		data, err := readVirtualOrStdin(c, opts.uploadFile)
		if err != nil {
			return nil, nil, err
		}
		body = data
		if method == "GET" {
			method = "PUT"
		}
	} else if len(opts.formParts) > 0 {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for _, part := range opts.formParts {
			if err := addFormPart(c, mw, part); err != nil {
				return nil, nil, err
			}
		}
		mw.Close()
		body = buf.Bytes()
		contentType = mw.FormDataContentType()
		if method == "GET" {
			method = "POST"
		}
	} else if len(opts.dataParts) > 0 {
		var parts []string
		for _, part := range opts.dataParts {
			if strings.HasPrefix(part, "@") {
				data, err := readVirtualOrStdin(c, strings.TrimPrefix(part, "@"))
				if err != nil {
					return nil, nil, err
				}
				parts = append(parts, string(data))
			} else {
				parts = append(parts, part)
			}
		}
		body = []byte(strings.Join(parts, "&"))
		contentType = "application/x-www-form-urlencoded"
		if method == "GET" {
			method = "POST"
		}
	}
	if int64(len(body)) > policy.MaxRequestBytes {
		return nil, nil, errors.New("request size limit exceeded")
	}
	req, err := http.NewRequestWithContext(ctx, method, opts.url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	for k, values := range opts.headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	if opts.user != "" {
		user, pass, _ := strings.Cut(opts.user, ":")
		req.SetBasicAuth(user, pass)
	}
	if opts.cookie != "" {
		if strings.Contains(opts.cookie, "=") {
			req.Header.Set("Cookie", opts.cookie)
		} else {
			data, err := gfs.ReadFile(c.FS, resolve(*c.Cwd, opts.cookie))
			if err != nil {
				return nil, nil, err
			}
			req.Header.Set("Cookie", strings.TrimSpace(string(data)))
		}
	}
	return req, body, nil
}

func addHeader(h http.Header, line string) {
	name, value, ok := strings.Cut(line, ":")
	if !ok || strings.TrimSpace(name) == "" {
		return
	}
	h.Set(strings.TrimSpace(name), strings.TrimSpace(value))
}

func addFormPart(c *CommandContext, mw *multipart.Writer, part string) error {
	name, value, _ := strings.Cut(part, "=")
	if strings.HasPrefix(value, "@") {
		filename := strings.TrimPrefix(value, "@")
		data, err := readVirtualOrStdin(c, filename)
		if err != nil {
			return err
		}
		fw, err := mw.CreateFormFile(name, path.Base(filename))
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	}
	return mw.WriteField(name, value)
}

func readVirtualOrStdin(c *CommandContext, name string) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(c.Stdin)
	}
	return gfs.ReadFile(c.FS, resolve(*c.Cwd, name))
}

func writeVirtualFile(c *CommandContext, name string, data []byte) error {
	abs := resolve(*c.Cwd, name)
	if err := gfs.MkdirAll(c.FS, path.Dir(abs), c.CreationMode(0o777)); err != nil {
		return err
	}
	return gfs.WriteFile(c.FS, abs, data, c.CreationMode(0o666))
}

func writeStatusAndHeaders(w io.Writer, resp *http.Response) {
	fmt.Fprintf(w, "HTTP/%d.%d %s\r\n", resp.ProtoMajor, resp.ProtoMinor, resp.Status)
	resp.Header.Write(w)
	fmt.Fprint(w, "\r\n")
}

func writeVerboseRequest(c *CommandContext, req *http.Request, bodyLen int) {
	fmt.Fprintf(c.Stderr, "*   Trying %s...\n", req.URL.Host)
	fmt.Fprintf(c.Stderr, "> %s %s HTTP/1.1\n> Host: %s\n", req.Method, req.URL.RequestURI(), req.URL.Host)
	for k, values := range req.Header {
		for _, v := range values {
			fmt.Fprintf(c.Stderr, "> %s: %s\n", k, redact(k, v))
		}
	}
	if bodyLen > 0 {
		fmt.Fprintf(c.Stderr, "> Content-Length: %d\n", bodyLen)
	}
}

func writeVerboseResponse(c *CommandContext, resp *http.Response) {
	fmt.Fprintf(c.Stderr, "< HTTP/%d.%d %s\n", resp.ProtoMajor, resp.ProtoMinor, resp.Status)
	for k, values := range resp.Header {
		for _, v := range values {
			fmt.Fprintf(c.Stderr, "< %s: %s\n", k, v)
		}
	}
}

func redact(k, v string) string {
	if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Cookie") {
		return "<redacted>"
	}
	return v
}

func formatWriteOut(format string, resp *http.Response) string {
	repl := map[string]string{
		"%{http_code}":     strconv.Itoa(resp.StatusCode),
		"%{response_code}": strconv.Itoa(resp.StatusCode),
		"%{url_effective}": resp.Request.URL.String(),
		"%{method}":        resp.Request.Method,
		"%{content_type}":  resp.Header.Get("Content-Type"),
	}
	out := format
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	out = strings.ReplaceAll(out, "\\n", "\n")
	out = strings.ReplaceAll(out, "\\t", "\t")
	return out
}

func parseSeconds(s string) (time.Duration, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return time.Duration(f * float64(time.Second)), nil
}

func curlError(c *CommandContext, opts options, err error) int {
	if !opts.silent || opts.showError {
		fmt.Fprintf(c.Stderr, "curl: %v\n", err)
	}
	return 1
}

func resolve(base, name string) string {
	if strings.HasPrefix(name, "/") {
		return path.Clean(name)
	}
	return path.Clean(path.Join(base, name))
}
