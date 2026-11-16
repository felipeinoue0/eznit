package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseOrigins(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"blank entries", " , ,", nil},
		{"one", "https://ui.example.com", []string{"https://ui.example.com"}},
		{"trailing slash and spaces", " https://a.test/ , http://localhost:5173 ", []string{"https://a.test", "http://localhost:5173"}},
		{"any", "*", []string{"*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseOrigins(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseOrigins(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	// next records that it ran, so a preflight answered by the middleware can
	// be told apart from one that fell through to the router.
	tests := []struct {
		name        string
		origins     []string
		method      string
		origin      string
		preflight   bool
		wantAllow   string
		wantStatus  int
		wantNextRan bool
	}{
		{"off: no headers", nil, http.MethodGet, "https://ui.test", false, "", http.StatusOK, true},
		{"listed origin", []string{"https://ui.test"}, http.MethodGet, "https://ui.test", false, "https://ui.test", http.StatusOK, true},
		{"unlisted origin", []string{"https://ui.test"}, http.MethodGet, "https://evil.test", false, "", http.StatusOK, true},
		{"no origin header", []string{"https://ui.test"}, http.MethodGet, "", false, "", http.StatusOK, true},
		{"any origin", []string{"*"}, http.MethodGet, "https://else.test", false, "https://else.test", http.StatusOK, true},
		{"preflight answered here", []string{"https://ui.test"}, http.MethodOptions, "https://ui.test", true, "https://ui.test", http.StatusNoContent, false},
		{"preflight from unlisted origin falls through", []string{"https://ui.test"}, http.MethodOptions, "https://evil.test", true, "", http.StatusOK, true},
		{"plain OPTIONS is not a preflight", []string{"https://ui.test"}, http.MethodOptions, "https://ui.test", false, "https://ui.test", http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ran := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ran = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(tt.method, "/v1/templates", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.preflight {
				req.Header.Set("Access-Control-Request-Method", http.MethodPut)
				req.Header.Set("Access-Control-Request-Headers", "authorization")
			}
			rec := httptest.NewRecorder()

			CORS(tt.origins)(next).ServeHTTP(rec, req)

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.wantAllow {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.wantAllow)
			}
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ran != tt.wantNextRan {
				t.Errorf("next ran = %v, want %v", ran, tt.wantNextRan)
			}
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
				t.Errorf("Allow-Credentials = %q, want none: the session is a bearer token", got)
			}
			if tt.wantAllow != "" && rec.Header().Get("Access-Control-Expose-Headers") != "Content-Disposition" {
				t.Errorf("Content-Disposition is not exposed")
			}
			if tt.preflight && tt.wantAllow != "" && rec.Header().Get("Access-Control-Allow-Headers") == "" {
				t.Errorf("a preflight answer must list the allowed headers")
			}
			if len(tt.origins) > 0 && rec.Header().Values("Vary")[0] != "Origin" {
				t.Errorf("Vary = %v, want Origin first", rec.Header().Values("Vary"))
			}
		})
	}
}
