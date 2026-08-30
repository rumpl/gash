package fs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TursoOptions struct {
	// URL is the database URL. libsql://, wss://, ws://, https:// and http://
	// are accepted and normalized to the Hrana over HTTP endpoint.
	URL string
	// AuthToken is the database token sent as a bearer token. Local libsql
	// servers that require no authentication accept an empty token.
	AuthToken string
	// HTTPClient overrides the default client, which applies Timeout.
	HTTPClient *http.Client
	// Table is the prefix of the tables owned by the filesystem.
	Table string
	// Limit bounds the total number of file data bytes retained.
	Limit int64
	// Timeout bounds a single database statement.
	Timeout time.Duration
	// Mounts are copied into the database before the filesystem is returned.
	Mounts []CopyMount
}

// NewTurso returns a filesystem persisted in a Turso (libSQL) database. It
// speaks the Hrana v2 pipeline protocol over HTTP directly, so it adds no
// driver dependency and works anywhere net/http does, including js/wasm.
func NewTurso(options TursoOptions) (*Database, error) {
	endpoint, err := TursoEndpoint(options.URL)
	if err != nil {
		return nil, err
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultQueryTimeout
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return NewDatabase(DatabaseOptions{
		DB:      &tursoClient{endpoint: endpoint, token: options.AuthToken, client: client},
		Table:   options.Table,
		Limit:   options.Limit,
		Timeout: timeout,
		Mounts:  options.Mounts,
	})
}

// TursoEndpoint converts a Turso database URL into its HTTP pipeline endpoint.
func TursoEndpoint(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("turso filesystem requires a database URL")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "libsql", "wss", "https":
		parsed.Scheme = "https"
	case "ws", "http":
		parsed.Scheme = "http"
	default:
		return "", fmt.Errorf("unsupported database scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("database URL %q has no host", raw)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/v2/pipeline") {
		parsed.Path += "/v2/pipeline"
	}
	return parsed.String(), nil
}

// tursoClient implements DB over the Hrana v2 pipeline endpoint. Each call
// opens a fresh implicit stream and closes it in the same request, which keeps
// the client stateless and avoids leaking server-side streams.
type tursoClient struct {
	endpoint string
	token    string
	client   *http.Client
}

func (t *tursoClient) Exec(ctx context.Context, query string, args ...any) error {
	_, err := t.pipeline(ctx, query, args)
	return err
}

func (t *tursoClient) Query(ctx context.Context, query string, args ...any) ([][]any, error) {
	return t.pipeline(ctx, query, args)
}

func (t *tursoClient) Close() error {
	return nil
}

type tursoPipelineRequest struct {
	Requests []tursoStreamRequest `json:"requests"`
}

type tursoStreamRequest struct {
	Type string          `json:"type"`
	Stmt *tursoStatement `json:"stmt,omitempty"`
}

type tursoStatement struct {
	SQL      string       `json:"sql"`
	Args     []tursoValue `json:"args,omitempty"`
	WantRows bool         `json:"want_rows"`
}

type tursoValue struct {
	Type   string          `json:"type"`
	Value  json.RawMessage `json:"value,omitempty"`
	Base64 string          `json:"base64,omitempty"`
}

type tursoPipelineResponse struct {
	Results []tursoStreamResult `json:"results"`
}

type tursoStreamResult struct {
	Type     string `json:"type"`
	Response *struct {
		Type   string              `json:"type"`
		Result *tursoExecuteResult `json:"result"`
	} `json:"response"`
	Error *tursoError `json:"error"`
}

type tursoExecuteResult struct {
	Rows [][]tursoValue `json:"rows"`
}

type tursoError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (e *tursoError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func (t *tursoClient) pipeline(ctx context.Context, query string, args []any) ([][]any, error) {
	encoded, err := encodeTursoArgs(args)
	if err != nil {
		return nil, err
	}
	payload := tursoPipelineRequest{Requests: []tursoStreamRequest{
		{Type: "execute", Stmt: &tursoStatement{SQL: query, Args: encoded, WantRows: true}},
		{Type: "close"},
	}}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if t.token != "" {
		request.Header.Set("Authorization", "Bearer "+t.token)
	}
	response, err := t.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxTursoResponseBytes))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("turso: %s: %s", response.Status, summarize(data))
	}
	var decoded tursoPipelineResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("turso: invalid response: %w", err)
	}
	return decodeTursoResults(decoded)
}

const maxTursoResponseBytes = 512 << 20

func decodeTursoResults(response tursoPipelineResponse) ([][]any, error) {
	if len(response.Results) == 0 {
		return nil, errors.New("turso: empty response")
	}
	result := response.Results[0]
	if result.Error != nil {
		return nil, fmt.Errorf("turso: %w", result.Error)
	}
	if result.Type != "ok" || result.Response == nil || result.Response.Result == nil {
		return nil, errors.New("turso: statement was not executed")
	}
	out := make([][]any, 0, len(result.Response.Result.Rows))
	for _, row := range result.Response.Result.Rows {
		values := make([]any, 0, len(row))
		for _, cell := range row {
			value, err := cell.decode()
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		out = append(out, values)
	}
	return out, nil
}

func summarize(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) > 200 {
		return text[:200] + "..."
	}
	return text
}

func encodeTursoArgs(args []any) ([]tursoValue, error) {
	out := make([]tursoValue, 0, len(args))
	for _, arg := range args {
		value, err := encodeTursoValue(arg)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func encodeTursoValue(arg any) (tursoValue, error) {
	switch v := arg.(type) {
	case nil:
		return tursoValue{Type: "null"}, nil
	case bool:
		return tursoIntegerValue(boolToInt(v)), nil
	case int:
		return tursoIntegerValue(int64(v)), nil
	case int64:
		return tursoIntegerValue(v), nil
	case float64:
		encoded, err := json.Marshal(v)
		if err != nil {
			return tursoValue{}, err
		}
		return tursoValue{Type: "float", Value: encoded}, nil
	case string:
		encoded, err := json.Marshal(v)
		if err != nil {
			return tursoValue{}, err
		}
		return tursoValue{Type: "text", Value: encoded}, nil
	case []byte:
		return tursoValue{Type: "blob", Base64: base64.RawStdEncoding.EncodeToString(v)}, nil
	default:
		return tursoValue{}, fmt.Errorf("turso: unsupported argument type %T", arg)
	}
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

// tursoIntegerValue encodes integers as strings, which is how the protocol
// keeps 64-bit values exact through JSON.
func tursoIntegerValue(value int64) tursoValue {
	return tursoValue{Type: "integer", Value: json.RawMessage(strconv.Quote(strconv.FormatInt(value, 10)))}
}

func (v tursoValue) decode() (any, error) {
	switch v.Type {
	case "null", "":
		return nil, nil
	case "integer":
		var text string
		if err := json.Unmarshal(v.Value, &text); err != nil {
			var number int64
			if err := json.Unmarshal(v.Value, &number); err != nil {
				return nil, fmt.Errorf("turso: invalid integer: %w", err)
			}
			return number, nil
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("turso: invalid integer: %w", err)
		}
		return parsed, nil
	case "float":
		var number float64
		if err := json.Unmarshal(v.Value, &number); err != nil {
			return nil, fmt.Errorf("turso: invalid float: %w", err)
		}
		return number, nil
	case "text":
		var text string
		if err := json.Unmarshal(v.Value, &text); err != nil {
			return nil, fmt.Errorf("turso: invalid text: %w", err)
		}
		return text, nil
	case "blob":
		return decodeTursoBlob(v.Base64)
	default:
		return nil, fmt.Errorf("turso: unsupported value type %q", v.Type)
	}
}

func decodeTursoBlob(encoded string) ([]byte, error) {
	if decoded, err := base64.RawStdEncoding.DecodeString(encoded); err == nil {
		return decoded, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("turso: invalid blob: %w", err)
	}
	return decoded, nil
}

var _ DB = (*tursoClient)(nil)
