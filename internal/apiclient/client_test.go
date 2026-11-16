package apiclient

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := New(server.URL)
	client.Token = "the-token"

	return client
}

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", DefaultBaseURL},
		{"http://localhost:4000", "http://localhost:4000"},
		{"http://localhost:4000/", "http://localhost:4000"},
		{"  http://example.test/api/  ", "http://example.test/api"},
	}

	for _, tt := range tests {
		if got := NormalizeBaseURL(tt.in); got != tt.want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// An address a person typed is checked before it is saved, and the refusal
// says what to fix.
func TestValidateBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{"local", "http://127.0.0.1:4000", "http://127.0.0.1:4000", ""},
		{"https with path", "https://eznit.example.com/api/", "https://eznit.example.com/api", ""},
		{"surrounding space", "  http://example.test  ", "http://example.test", ""},
		{"no scheme", "eznit.example.com", "", "http:// or https://"},
		{"wrong scheme", "ftp://example.test", "", "http:// or https://"},
		{"no host", "http://", "", "needs a host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateBaseURL(tt.in)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ValidateBaseURL(%q) = %q, want an error", tt.in, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to mention %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ValidateBaseURL(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ValidateBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// A token belongs to the instance that issued it, so a port change on one host
// is the same instance and a different host is not.
func TestSameHost(t *testing.T) {
	tests := []struct {
		left, right string
		want        bool
	}{
		{"http://127.0.0.1:4000", "http://127.0.0.1:5000", true},
		{"http://localhost:4000", "https://localhost:8443/api", true},
		{"http://127.0.0.1:4000", "http://localhost:4000", false},
		{"http://127.0.0.1:4000", "https://eznit.example.com", false},
		{"", "", true},
	}

	for _, tt := range tests {
		if got := SameHost(tt.left, tt.right); got != tt.want {
			t.Errorf("SameHost(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
		}
	}
}

// Health is what the Files tab's Test button calls: it must not need a
// session, and it must report a server that answers wrongly.
func TestHealth(t *testing.T) {
	t.Run("answering", func(t *testing.T) {
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/healthcheck" {
				t.Errorf("path = %q, want the health check", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("Authorization = %q, want none: the health check needs no session", got)
			}

			w.WriteHeader(http.StatusOK)
		})

		if err := client.Health(); err != nil {
			t.Errorf("Health() = %v, want nil", err)
		}
	})

	t.Run("answering with an error", func(t *testing.T) {
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})

		err := client.Health()
		if err == nil {
			t.Fatal("Health() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "503") {
			t.Errorf("Health() = %q, want the status", err)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		address := server.URL
		server.Close()

		err := New(address).Health()
		if err == nil {
			t.Fatal("Health() against a closed server = nil, want an error")
		}

		// The window tells "the server is not there" from "the server said no"
		// by this sentinel, so a refusal and a transport failure must not be
		// the same error (#335).
		if !errors.Is(err, ErrUnreachable) {
			t.Errorf("Health() = %v, want ErrUnreachable in the chain", err)
		}
	})
}

// A client is built and then read: the window replaces it to change the
// session, so a request that captured one keeps the token it was given (#342).
func TestWithTokenLeavesTheOriginalAlone(t *testing.T) {
	client := New("http://127.0.0.1:4000")
	client.Token = "first"

	signedOut := client.WithToken("")

	if client.Token != "first" {
		t.Errorf("the original token = %q, want it untouched", client.Token)
	}

	if signedOut.Token != "" {
		t.Errorf("WithToken(\"\") token = %q, want none", signedOut.Token)
	}

	if signedOut.BaseURL != client.BaseURL {
		t.Errorf("BaseURL = %q, want the same instance %q", signedOut.BaseURL, client.BaseURL)
	}

	if signedOut.HTTP != client.HTTP {
		t.Error("WithToken() replaced the transport; the copy shares it")
	}
}

func TestLogin(t *testing.T) {
	var got map[string]string

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/login" || r.Method != http.MethodPost {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"status_code":200,"data":{"token":"abc"}}`))
	})

	token, err := client.Login("alice@example.com", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token != "abc" {
		t.Errorf("token = %q, want abc", token)
	}
	if got["email"] != "alice@example.com" || got["password"] != "secret" {
		t.Errorf("body = %v, want the credentials", got)
	}
}

func TestLoginRefused(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
	})

	_, err := client.Login("alice@example.com", "wrong")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an APIError", err)
	}
	if apiErr.Status != http.StatusUnauthorized || apiErr.Message != "invalid credentials" {
		t.Errorf("error = %+v, want 401 + the host message", apiErr)
	}
}

func TestSignupSendsTheConfirmation(t *testing.T) {
	var got map[string]string

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status_code":201}`))
	})

	if err := client.Signup("a@b.c", "pw", "pw"); err != nil {
		t.Fatalf("Signup() error = %v", err)
	}
	if got["confirm_password"] != "pw" {
		t.Errorf("confirm_password = %q, want pw", got["confirm_password"])
	}
}

func TestListFiles(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer the-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"data":[{"id":"1","name":"a.txt","ext":"txt","version":2,"is_public":true}],"total":1}}`))
	})

	files, err := client.ListFiles()
	if err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}
	if len(files) != 1 || files[0].Name != "a.txt" || files[0].Version != 2 || !files[0].IsPublic {
		t.Errorf("files = %+v, want the one file", files)
	}
}

func TestListFilesEmptyIsNotNil(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"data":null,"total":0}}`))
	})

	files, err := client.ListFiles()
	if err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}
	if files == nil {
		t.Error("files = nil, want an empty slice")
	}
}

func TestUploadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var name, content string

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files" || r.Method != http.MethodPost {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		raw, _ := io.ReadAll(file)
		name, content = header.Filename, string(raw)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status_code":201}`))
	})

	if err := client.UploadFile(path); err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if name != "notes.txt" || content != "hello" {
		t.Errorf("uploaded %q with %q, want notes.txt with hello", name, content)
	}
}

func TestDownloadFile(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/abc/content" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="server-name.txt"`)
		_, _ = w.Write([]byte("bytes"))
	})

	dest := filepath.Join(t.TempDir(), "out.txt")
	if _, err := client.DownloadFile("abc", dest); err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}

	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(raw) != "bytes" {
		t.Errorf("downloaded %q, want bytes", raw)
	}
}

func TestDownloadFileRefused(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})

	if _, err := client.DownloadFile("abc", filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("DownloadFile() error = nil, want the refusal")
	}
}

func TestDeleteAndShare(t *testing.T) {
	var share map[string]bool

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/files/abc":
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/abc/share":
			_ = json.NewDecoder(r.Body).Decode(&share)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status_code":200}`))
	})

	if err := client.DeleteFile("abc"); err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
	if err := client.SetFilePublic("abc", true); err != nil {
		t.Fatalf("SetFilePublic() error = %v", err)
	}
	if !share["is_public"] {
		t.Errorf("share body = %v, want is_public true", share)
	}
}

func TestErrorsUseTheEnvelopeMessage(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"already there"}`))
	})

	err := client.DeleteFile("abc")

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "already there" {
		t.Errorf("error = %v, want an APIError with the message", err)
	}
	if !strings.Contains(err.Error(), "already there") {
		t.Errorf("Error() = %q", err.Error())
	}
}

// The shared list needs no session and decodes the owner fields; an empty
// server is an empty list, not an error.
func TestListSharedFiles(t *testing.T) {
	t.Run("rows", func(t *testing.T) {
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/files/shared" {
				t.Errorf("path = %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"data":[{"id":"a1","name":"gitconfig","ext":"txt","version":2,"is_public":true,"owner":"u1","owner_email":"ana@example.com"}],"total":1}}`))
		})
		files, err := client.ListSharedFiles()
		if err != nil {
			t.Fatalf("ListSharedFiles: %v", err)
		}
		if len(files) != 1 || files[0].Name != "gitconfig" || files[0].Ext != "txt" || files[0].OwnerEmail != "ana@example.com" || files[0].Version != 2 {
			t.Errorf("files = %+v", files)
		}
	})
	t.Run("empty", func(t *testing.T) {
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"data":null,"total":0}}`))
		})
		files, err := client.ListSharedFiles()
		if err != nil || files == nil || len(files) != 0 {
			t.Errorf("files = %v, err = %v; want an empty list", files, err)
		}
	})
}
