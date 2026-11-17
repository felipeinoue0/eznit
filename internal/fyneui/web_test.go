//go:build fyne

package fyneui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// webServer answers the two routes the Web tab reads, like the API: a public
// and a private template, and one shared file whose owner is named only to a
// request with a session.
func webServer(t *testing.T, status int) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"Invalid token"}`))
			return
		}

		switch r.URL.Path {
		case "/v1/files/shared":
			email := ""
			if r.Header.Get("Authorization") != "" {
				email = "ana@example.com"
			}
			_, _ = w.Write([]byte(`{"data":{"data":[{"id":"a1","name":"gitconfig","ext":"txt","version":2,"is_public":true,"owner":"u1","owner_email":"` + email + `"}],"total":1}}`))
		case "/v1/templates":
			_, _ = w.Write([]byte(`{"data":{"data":[` +
				`{"id":"00000000-0000-0000-0000-000000000001","name":"go-service","kind":"go","version":1,"file_count":3,"is_public":true},` +
				`{"id":"00000000-0000-0000-0000-000000000002","name":"my-draft","kind":"generic","version":1,"file_count":1,"is_public":false}` +
				`],"total":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func rowNames(items []entry) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.name)
	}
	return names
}

// A person without an account sees what is public — templates and files in
// one list — and every row has its own Download; the owner is not named to a
// visitor, and a private template of the caller's is not on the Web tab.
func TestWebTabWorksWithoutAnAccount(t *testing.T) {
	server := webServer(t, http.StatusOK)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL)

	u.web.reload()
	u.settle()

	if got := strings.Join(rowNames(u.web.list.items), ","); got != "go-service,gitconfig.txt" {
		t.Fatalf("rows = %q, want the public template then the shared file (status %q)", got, u.web.status.Text)
	}
	if got := u.web.list.items[1].info; !strings.Contains(got, "someone on this server") {
		t.Errorf("file info = %q, want the visitor wording", got)
	}

	row := u.web.list.list.CreateItem().(*entryRow)
	u.web.list.list.UpdateItem(1, row)
	if row.download.OnTapped == nil {
		t.Error("the row has no Download of its own")
	}

	u.web.show.SetSelected(showFiles)
	if got := strings.Join(rowNames(u.web.list.items), ","); got != "gitconfig.txt" {
		t.Errorf("Files shows %q, want only the file", got)
	}

	u.web.show.SetSelected(showTemplates)
	if got := strings.Join(rowNames(u.web.list.items), ","); got != "go-service" {
		t.Errorf("Templates shows %q, want only the template", got)
	}
}

// Opening a shared file shows its facts and a Download, under Back.
func TestWebFileOpensAPage(t *testing.T) {
	server := webServer(t, http.StatusOK)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL).WithToken("the-token")

	u.web.reload()
	u.settle()

	u.web.show.SetSelected(showFiles)
	u.web.list.list.Select(0)
	u.settle()

	if u.web.pages.atHome() {
		t.Fatal("tapping the row did not open its page")
	}

	page := u.web.pages.stack.Objects[0]
	if findButton(page, "Download…") == nil || findButton(page, "Back") == nil {
		t.Error("the file's page needs Download… and Back")
	}
	if text := panelText(page); !strings.Contains(text, "ana@example.com") || !strings.Contains(text, "gitconfig.txt") {
		t.Errorf("page text = %q, want the name and the owner", text)
	}

	test := findButton(page, "Back")
	test.OnTapped()

	if !u.web.pages.atHome() {
		t.Error("Back did not return to the list")
	}
}

// With a session the API names the owner, and the row shows it.
func TestWebTabNamesTheOwnerWhenSignedIn(t *testing.T) {
	server := webServer(t, http.StatusOK)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL).WithToken("the-token")

	u.web.reload()
	u.settle()

	if got := u.web.list.items[1].info; !strings.Contains(got, "ana@example.com") {
		t.Errorf("file info = %q, want ana@example.com", got)
	}
	if !strings.Contains(u.web.status.Text, "1 template(s) and 1 file(s)") {
		t.Errorf("status = %q", u.web.status.Text)
	}
}

// An empty server says how something gets there, rather than an empty list.
func TestWebTabEmptySaysHowToShare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":[],"total":0}}`))
	}))
	t.Cleanup(server.Close)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL)

	u.web.reload()
	u.settle()

	if !strings.Contains(u.web.status.Text, "from Workspace") {
		t.Errorf("status = %q, want it to say how something becomes public", u.web.status.Text)
	}
}

// A refused token ends the session the way My Account does.
func TestWebTabRefusedTokenEndsTheSession(t *testing.T) {
	server := webServer(t, http.StatusUnauthorized)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL).WithToken("dead-token")

	u.web.reload()
	u.settle()

	if findButton(u.loginBody.Objects[0], "Log in") == nil {
		t.Fatal("a refused token did not bring the login card back")
	}
	if u.client.Token != "" {
		t.Errorf("client token = %q, want it cleared", u.client.Token)
	}

	// The list loads again without the token; this server refuses that too,
	// and the tab says so instead of ending a session that is already gone —
	// which, before the guard, reloaded the list in a loop.
	if !strings.Contains(u.web.status.Text, "without an account") {
		t.Errorf("status = %q, want the tokenless refusal", u.web.status.Text)
	}
}

// Explore is the first tab; Workspace joins it only with a session. The right
// end of the bar is Log in (Log out with a session), Settings and About, each
// opening a full-window screen with Close (#379).
func TestTabOrder(t *testing.T) {
	u := newTestUI(t)

	titles := func() string {
		var names []string
		for _, item := range u.tabs.Items {
			names = append(names, item.Text)
		}
		return strings.Join(names, ",")
	}

	if got := titles(); got != "Explore" {
		t.Errorf("tabs without a session = %q, want Explore alone", got)
	}
	if u.tabs.Selected() != u.webTab {
		t.Error("the window does not open on Explore")
	}
	if u.sessionButton.Text != "Log in" {
		t.Errorf("session button = %q without a session, want Log in", u.sessionButton.Text)
	}

	test.Tap(u.sessionButton)
	if !u.screenShown(u.loginScreen) {
		t.Fatal("Log in did not open the login screen")
	}
	test.Tap(findButton(u.loginScreen, "Close"))
	if u.screenShown(u.loginScreen) {
		t.Error("Close did not return to the tabs")
	}

	u.client = u.client.WithToken("the-token")
	u.openWorkspace()
	u.settle()

	if got := titles(); got != "Explore,Workspace" {
		t.Errorf("tabs with a session = %q, want Explore,Workspace", got)
	}
	if u.tabs.Selected() != u.workspaceTab {
		t.Error("a login does not land on Workspace")
	}
	if u.sessionButton.Text != "Log out" {
		t.Errorf("session button = %q with a session, want Log out", u.sessionButton.Text)
	}

	for _, screen := range []struct {
		caption string
		want    fyne.CanvasObject
	}{
		{"Settings", u.settingsScreen},
		{"About", u.aboutScreen},
	} {
		button := findButton(u.mainScreen, screen.caption)
		if button == nil {
			t.Fatalf("the bar has no %s", screen.caption)
		}
		test.Tap(button)
		if !u.screenShown(screen.want) {
			t.Errorf("%s did not open its screen", screen.caption)
		}
		u.showTabs()
	}

	test.Tap(u.sessionButton)
	if got := titles(); got != "Explore" || u.sessionButton.Text != "Log in" {
		t.Errorf("after Log out: tabs %q, button %q", got, u.sessionButton.Text)
	}
	u.settle()
}
