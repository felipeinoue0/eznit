//go:build fyne

package fyneui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// sharedServer answers /v1/files/shared like the API: the owner's address only
// for a request with a session.
func sharedServer(t *testing.T, status int) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/shared" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"Invalid token"}`))
			return
		}
		email := ""
		if r.Header.Get("Authorization") != "" {
			email = "ana@example.com"
		}
		_, _ = w.Write([]byte(`{"data":{"data":[{"id":"a1","name":"gitconfig","ext":"txt","version":2,"is_public":true,"owner":"u1","owner_email":"` + email + `"}],"total":1}}`))
	}))
	t.Cleanup(server.Close)

	return server
}

func cellText(v *sharedView, row, col int) string {
	label := widget.NewLabel("")
	v.table.UpdateCell(widget.TableCellID{Row: row, Col: col}, label)
	return label.Text
}

// A person without an account sees what was shared and can download it; the
// owner is not named to a visitor.
func TestSharedTabWorksWithoutAnAccount(t *testing.T) {
	server := sharedServer(t, http.StatusOK)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL)

	u.shared.reload()
	u.settle()

	if len(u.shared.items) != 1 {
		t.Fatalf("rows = %d, want 1 (status %q)", len(u.shared.items), u.shared.status.Text)
	}
	if got := cellText(u.shared, 1, 0); got != "gitconfig.txt" {
		t.Errorf("name cell = %q, want gitconfig.txt", got)
	}
	if got := cellText(u.shared, 1, 2); got != "someone on this server" {
		t.Errorf("owner cell = %q, want the visitor wording", got)
	}

	if !u.shared.download.Disabled() {
		t.Error("Download is enabled with nothing selected")
	}
	u.shared.table.Select(widget.TableCellID{Row: 1, Col: 0})
	if u.shared.download.Disabled() {
		t.Error("Download stays disabled after selecting a file")
	}
}

// With a session the API names the owner, and the tab shows it.
func TestSharedTabNamesTheOwnerWhenSignedIn(t *testing.T) {
	server := sharedServer(t, http.StatusOK)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL).WithToken("the-token")

	u.shared.reload()
	u.settle()

	if got := cellText(u.shared, 1, 2); got != "ana@example.com" {
		t.Errorf("owner cell = %q, want ana@example.com", got)
	}
	if !strings.Contains(u.shared.status.Text, "1 shared file") {
		t.Errorf("status = %q", u.shared.status.Text)
	}
}

// An empty server says how a file gets there, rather than an empty table.
func TestSharedTabEmptySaysHowToShare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":[],"total":0}}`))
	}))
	t.Cleanup(server.Close)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL)

	u.shared.reload()
	u.settle()

	if !strings.Contains(u.shared.status.Text, "presses Share on the Files tab") {
		t.Errorf("status = %q, want it to say how a file becomes shared", u.shared.status.Text)
	}
}

// A refused token ends the session the way the other tabs do.
func TestSharedTabRefusedTokenEndsTheSession(t *testing.T) {
	server := sharedServer(t, http.StatusUnauthorized)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL).WithToken("dead-token")

	u.shared.reload()
	u.settle()

	if findButton(u.tabs.Items[0].Content, "Log in") == nil {
		t.Fatal("a refused token did not bring the login card back")
	}
	if !strings.Contains(u.shared.status.Text, "session ended") {
		t.Errorf("status = %q", u.shared.status.Text)
	}
}
