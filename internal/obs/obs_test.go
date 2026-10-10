package obs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestID(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		wantSame bool
	}{
		{name: "generated when missing", incoming: ""},
		{name: "safe incoming id reused", incoming: "abc-123_x.y", wantSame: true},
		{name: "unsafe incoming id replaced", incoming: "bad id\nwith newline"},
		{name: "too long incoming id replaced", incoming: strings.Repeat("a", 65)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			var seen, inside string
			h := RequestID(NewLogger(&buf), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				inside = RequestIDFrom(r.Context())
				Logger(r.Context()).Info("inside")
				w.WriteHeader(http.StatusTeapot)
			}))
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tt.incoming != "" {
				req.Header.Set("X-Request-Id", tt.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			seen = rec.Header().Get("X-Request-Id")
			if !validID.MatchString(seen) {
				t.Fatalf("response id %q is not valid", seen)
			}
			if inside != seen {
				t.Fatalf("RequestIDFrom = %q, want %q", inside, seen)
			}
			if tt.wantSame != (seen == tt.incoming) {
				t.Fatalf("id = %q, incoming = %q, wantSame = %v", seen, tt.incoming, tt.wantSame)
			}

			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			if len(lines) != 2 {
				t.Fatalf("want 2 log lines, got %d: %s", len(lines), buf.String())
			}
			for _, line := range lines {
				var m map[string]any
				if err := json.Unmarshal([]byte(line), &m); err != nil {
					t.Fatalf("log line is not JSON: %s", line)
				}
				if m["request_id"] != seen {
					t.Fatalf("log line missing request id: %s", line)
				}
			}
			var summary map[string]any
			_ = json.Unmarshal([]byte(lines[1]), &summary)
			if summary["status"] != float64(http.StatusTeapot) || summary["path"] != "/x" {
				t.Fatalf("bad summary line: %s", lines[1])
			}
		})
	}
}

func TestHealthChecksAreNotLoggedAtInfo(t *testing.T) {
	var buf bytes.Buffer
	h := RequestID(NewLogger(&buf), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if buf.Len() != 0 {
		t.Fatalf("health check was logged: %s", buf.String())
	}
}
