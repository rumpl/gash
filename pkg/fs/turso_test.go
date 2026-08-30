package fs

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// fakeTurso implements enough of the Hrana v2 pipeline protocol to exercise
// the client end to end against a real SQLite database.
type fakeTurso struct {
	t        *testing.T
	db       *sql.DB
	token    string
	requests int
}

func newFakeTurso(t *testing.T, token string) *httptest.Server {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	fake := &fakeTurso{t: t, db: db, token: token}
	server := httptest.NewServer(fake)
	t.Cleanup(func() {
		server.Close()
		_ = db.Close()
	})
	return server
}

func (f *fakeTurso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests++
	if r.URL.Path != "/v2/pipeline" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if f.token != "" && r.Header.Get("Authorization") != "Bearer "+f.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var request tursoPipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response := tursoPipelineResponse{}
	for _, streamRequest := range request.Requests {
		response.Results = append(response.Results, f.run(streamRequest))
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		f.t.Error(err)
	}
}

func (f *fakeTurso) run(request tursoStreamRequest) tursoStreamResult {
	if request.Type != "execute" {
		return tursoStreamResult{Type: "ok"}
	}
	args := make([]any, 0, len(request.Stmt.Args))
	for _, arg := range request.Stmt.Args {
		value, err := arg.decode()
		if err != nil {
			return tursoStreamResult{Type: "error", Error: &tursoError{Message: err.Error(), Code: "ARGS"}}
		}
		args = append(args, value)
	}
	rows, err := f.db.Query(request.Stmt.SQL, args...)
	if err != nil {
		return tursoStreamResult{Type: "error", Error: &tursoError{Message: err.Error(), Code: "SQL"}}
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return tursoStreamResult{Type: "error", Error: &tursoError{Message: err.Error(), Code: "SQL"}}
	}
	result := &tursoExecuteResult{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return tursoStreamResult{Type: "error", Error: &tursoError{Message: err.Error(), Code: "SCAN"}}
		}
		row := make([]tursoValue, 0, len(values))
		for _, value := range values {
			row = append(row, encodeFakeValue(value))
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return tursoStreamResult{Type: "error", Error: &tursoError{Message: err.Error(), Code: "SQL"}}
	}
	return tursoStreamResult{
		Type: "ok",
		Response: &struct {
			Type   string              `json:"type"`
			Result *tursoExecuteResult `json:"result"`
		}{Type: "execute", Result: result},
	}
}

func encodeFakeValue(value any) tursoValue {
	switch v := value.(type) {
	case nil:
		return tursoValue{Type: "null"}
	case int64:
		return tursoValue{Type: "integer", Value: json.RawMessage(strconv.Quote(strconv.FormatInt(v, 10)))}
	case float64:
		return tursoValue{Type: "float", Value: json.RawMessage(strconv.FormatFloat(v, 'g', -1, 64))}
	case string:
		encoded, _ := json.Marshal(v)
		return tursoValue{Type: "text", Value: encoded}
	case []byte:
		// Padded base64 is deliberately used here: the client must accept
		// both the padded and unpadded forms seen in the wild.
		return tursoValue{Type: "blob", Base64: base64.StdEncoding.EncodeToString(v)}
	default:
		encoded, _ := json.Marshal(v)
		return tursoValue{Type: "text", Value: encoded}
	}
}

func TestTursoDatabaseSuite(t *testing.T) {
	runDatabaseSuite(t, func(t *testing.T) *Database {
		t.Helper()
		server := newFakeTurso(t, "test-token")
		filesystem, err := NewTurso(TursoOptions{URL: server.URL, AuthToken: "test-token"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = filesystem.Close() })
		return filesystem
	})
}

func TestTursoRejectsBadCredentials(t *testing.T) {
	server := newFakeTurso(t, "test-token")
	_, err := NewTurso(TursoOptions{URL: server.URL, AuthToken: "wrong"})
	if err == nil {
		t.Fatal("expected an authentication failure")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("err=%v", err)
	}
}

func TestTursoReportsStatementErrors(t *testing.T) {
	server := newFakeTurso(t, "")
	filesystem, err := NewTurso(TursoOptions{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer filesystem.Close()
	client := &tursoClient{endpoint: server.URL + "/v2/pipeline", client: server.Client()}
	if _, err := client.Query(t.Context(), "SELECT * FROM missing_table"); err == nil {
		t.Fatal("expected a statement error")
	} else if !strings.Contains(err.Error(), "no such table") {
		t.Fatalf("err=%v", err)
	}
}

func TestTursoEndpointNormalization(t *testing.T) {
	cases := map[string]string{
		"libsql://db-org.turso.io":             "https://db-org.turso.io/v2/pipeline",
		"wss://db-org.turso.io":                "https://db-org.turso.io/v2/pipeline",
		"ws://127.0.0.1:8080":                  "http://127.0.0.1:8080/v2/pipeline",
		"http://127.0.0.1:8080/":               "http://127.0.0.1:8080/v2/pipeline",
		"https://db-org.turso.io/v2/pipeline":  "https://db-org.turso.io/v2/pipeline",
		"db-org.turso.io":                      "https://db-org.turso.io/v2/pipeline",
		"https://db-org.turso.io/?authToken=x": "https://db-org.turso.io/v2/pipeline",
	}
	for input, want := range cases {
		got, err := TursoEndpoint(input)
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		if got != want {
			t.Fatalf("%s: got %q want %q", input, got, want)
		}
	}
	for _, input := range []string{"", "ftp://example.com", "https://"} {
		if _, err := TursoEndpoint(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestTursoValueRoundTrip(t *testing.T) {
	values := []any{nil, int64(-42), 1.5, "text", []byte{0x00, 0xff}}
	for _, value := range values {
		encoded, err := encodeTursoValue(value)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := encoded.decode()
		if err != nil {
			t.Fatal(err)
		}
		if asString(decoded) != asString(value) {
			t.Fatalf("got %#v want %#v", decoded, value)
		}
	}
	if _, err := encodeTursoValue(struct{}{}); err == nil {
		t.Fatal("expected an unsupported argument type to be rejected")
	}
}
