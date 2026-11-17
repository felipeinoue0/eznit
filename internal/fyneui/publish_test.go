//go:build fyne

package fyneui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
	"github.com/fiwon123/eznit/internal/templatefolder"
)

func TestBuildPublishRequest(t *testing.T) {
	files := []templateapply.File{
		{Path: "AGENTS.md", Kind: templates.FileKindText, Content: "# {{PROJECT_NAME}} by {{AUTHOR}}"},
		{Path: "logo.png", Kind: templates.FileKindBinary, ContentBase64: "AAE=", IsExecutable: false},
		{Path: "setup.sh", Kind: templates.FileKindText, Content: "#!/bin/sh", IsExecutable: true},
	}

	t.Run("refusals", func(t *testing.T) {
		tests := []struct {
			name  string
			form  publishForm
			files []templateapply.File
			want  string
		}{
			{"no name", publishForm{name: "  "}, files, "needs a name"},
			{"unknown kind", publishForm{name: "x", kind: "emacs"}, files, "not a kind"},
			{"no files", publishForm{name: "x"}, nil, "no file"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := buildPublishRequest(tt.form, tt.files)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Errorf("err = %v, want it to say %q", err, tt.want)
				}
			})
		}
	})

	t.Run("the CLI's request", func(t *testing.T) {
		request, err := buildPublishRequest(publishForm{
			name: " Go starter ", description: " a service ", public: true,
			defaults: map[string]string{"AUTHOR": " Ana "},
		}, files)
		if err != nil {
			t.Fatalf("buildPublishRequest: %v", err)
		}
		if request.Name != "Go starter" || request.Description != "a service" || !request.IsPublic {
			t.Errorf("details = %q %q %v", request.Name, request.Description, request.IsPublic)
		}
		if request.Kind != templates.KindGeneric {
			t.Errorf("kind = %q, want the generic default", request.Kind)
		}
		if len(request.Files) != 3 || request.Files[1].ContentBase64 != "AAE=" || request.Files[1].Content != "" || !request.Files[2].IsExecutable {
			t.Errorf("files = %+v", request.Files)
		}

		got := map[string]templates.PublishVariable{}
		for _, variable := range request.Variables {
			got[variable.Key] = variable
		}
		// A default makes the question optional; none makes it required —
		// the rule `eznit template publish` applies.
		if v := got["AUTHOR"]; v.DefaultValue != "Ana" || v.Required {
			t.Errorf("AUTHOR = %+v, want default Ana, optional", v)
		}
		if v := got["PROJECT_NAME"]; v.DefaultValue != "" || !v.Required {
			t.Errorf("PROJECT_NAME = %+v, want required", v)
		}
		if len(got) != 2 {
			t.Errorf("variables = %+v, want two", request.Variables)
		}
	})
}

// Without a session the button explains itself and opens nothing: browsing and
// applying stay available to a person with no account.
func TestPublishSignedOutSaysWhere(t *testing.T) {
	u := newTestUI(t)

	u.account.publishFolder()

	if u.account.status.Text != publishSignedOut {
		t.Errorf("status = %q, want %q", u.account.status.Text, publishSignedOut)
	}
	if overlay := u.window.Canvas().Overlays().Top(); overlay != nil {
		t.Errorf("a dialog opened without a session: %T", overlay)
	}
}

// The whole form, end to end against a fake API: the folder is read by the
// shared walk, Publish sends the CLI's request with the session, and My Account
// confirms and reloads its list.
func TestPublishFormSendsTheTemplate(t *testing.T) {
	var (
		mu       sync.Mutex
		received templates.PublishRequest
		auth     string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/templates":
			mu.Lock()
			auth = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&received)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"6f1c1f18-5c2e-4b1e-9d0e-6a1f3b3c0a11","name":"demo","version":1,"is_public":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files":
			_, _ = w.Write([]byte(`{"data":{"data":[],"total":0}}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/templates"):
			_, _ = w.Write([]byte(`{"data":{"data":[{"id":"6f1c1f18-5c2e-4b1e-9d0e-6a1f3b3c0a11","name":"demo","kind":"generic","version":1,"file_count":2}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL).WithToken("the-token")

	root := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"AGENTS.md":            "# {{PROJECT_NAME}}\n",
		"Makefile":             "all:\n",
		"node_modules/skip.js": "ignored",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, skipped, err := templatefolder.Collect(root)
	if err != nil {
		t.Fatal(err)
	}

	form := u.account.showPublishForm(root, files, skipped)
	if u.window.Canvas().Overlays().Top() == nil {
		t.Fatal("the publish form did not open")
	}
	if form.name.Text != "demo" {
		t.Errorf("name = %q, want the folder's name as the default", form.name.Text)
	}
	if _, ok := form.defaults["PROJECT_NAME"]; !ok || len(form.defaults) != 1 {
		t.Errorf("placeholder fields = %v, want PROJECT_NAME only", form.defaults)
	}
	// Private is the default; the dropdown is the choice (#376).
	if form.visibility.Selected != "Private" {
		t.Errorf("visibility = %q, want Private by default", form.visibility.Selected)
	}
	if !form.description.MultiLine || form.description.Wrapping != fyne.TextWrapWord {
		t.Error("the description is not a wrapped multi-line field")
	}
	form.visibility.SetSelected("Public")
	publish := form.send

	test.Tap(publish)
	u.settle()

	mu.Lock()
	defer mu.Unlock()
	if auth != "Bearer the-token" {
		t.Errorf("Authorization = %q, want the session", auth)
	}
	// The name defaults to the folder's; node_modules is left out by the walk.
	if received.Name != "demo" || !received.IsPublic || len(received.Files) != 2 {
		t.Errorf("request = name %q public %v files %d", received.Name, received.IsPublic, len(received.Files))
	}
	if len(received.Variables) != 1 || received.Variables[0].Key != "PROJECT_NAME" || !received.Variables[0].Required {
		t.Errorf("variables = %+v, want PROJECT_NAME, required", received.Variables)
	}
	if !strings.Contains(u.account.status.Text, "Published demo") {
		t.Errorf("status = %q, want the publish confirmation (not the template count)", u.account.status.Text)
	}
	if len(u.account.templates) != 1 {
		t.Errorf("the account was not reloaded: %d template(s)", len(u.account.templates))
	}
}
