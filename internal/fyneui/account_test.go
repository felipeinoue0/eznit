//go:build fyne

package fyneui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/domain/templates"
)

// newTestAccountView builds My Account for an address. Nothing is contacted:
// these tests cover what the tab says and what it allows.
func newTestAccountView(t *testing.T, address apiChoice) *accountView {
	t.Helper()

	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	app := test.NewApp()
	t.Cleanup(app.Quit)

	u := &ui{
		app:     app,
		window:  app.NewWindow("test"),
		address: address,
		client:  apiclient.New(address.url),
	}

	u.account = newAccountView(u)
	u.window.SetContent(u.account.content())

	// A bar's Test and a listing run on the window's tally: wait for them
	// before the driver goes away (#333).
	t.Cleanup(u.settle)

	return u.account
}

// The tab says which server it talks to, and why that address: an address that
// came from an environment variable must not look like one a person saved.
func TestAccountHeaderNamesTheServerAndItsSource(t *testing.T) {
	view := newTestAccountView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the local server"})

	if got := view.bar.entry.Text; got != "http://127.0.0.1:4321" {
		t.Errorf("address = %q, want the resolved URL", got)
	}
	if view.bar.entry.Disabled() {
		t.Error("the address field is disabled, so it cannot be typed")
	}
	if !view.bar.source.Selectable {
		t.Error("the source is not selectable")
	}
	if got := view.bar.source.Text; !strings.Contains(got, "the local server") {
		t.Errorf("source = %q, want where the address came from", got)
	}
}

// Nothing that adds to the account may work before the server has answered
// once: a wrong address must not look like an empty account.
func TestAccountActionsWaitForTheServer(t *testing.T) {
	view := newTestAccountView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	view.ready = false
	view.updateActions()

	if !view.upload.Disabled() || !view.publish.Disabled() {
		t.Error("an action is enabled before the server has answered")
	}

	view.ready = true
	view.updateActions()

	if view.upload.Disabled() || view.publish.Disabled() {
		t.Error("an action is still disabled after a successful listing")
	}
}

// A refused session is not a missing server: the API answered, so the tab drops
// the token and asks again instead of leaving an empty library on screen (#335).
func TestRefusedSessionReturnsToLogin(t *testing.T) {
	u := newTestUI(t)

	if err := config.Set("token", "dead-token"); err != nil {
		t.Fatal(err)
	}
	u.client.Token = "dead-token"

	library := u.accountTab.Content

	u.account.failedToLoad("https://eznit.example.com", &apiclient.APIError{Status: 401, Message: "Invalid token"})

	if _, err := config.Value("token"); err == nil {
		t.Error("the refused token is still in the config file")
	}
	if u.client.Token != "" {
		t.Errorf("client token = %q, want it cleared", u.client.Token)
	}
	if findButton(u.accountTab.Content, "Log in") == nil {
		t.Error("My Account does not offer to sign in again")
	}
	if u.accountTab.Content == library {
		t.Error("the library is still on screen after the session ended")
	}
}

// The rule is not applied by hand: a real listing answered with 401 must end
// the session too, which is the wiring from the client's error to the tab.
func TestRefusedListingGoesToLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status_code":401,"error":"Invalid token"}`))
	}))
	t.Cleanup(server.Close)

	u := newTestUI(t)

	if err := config.Set("token", "dead-token"); err != nil {
		t.Fatal(err)
	}

	u.client = apiclient.New(server.URL)
	u.client.Token = "dead-token"

	u.account.reload()

	// Wait for the answer instead of polling the tree: the poll reads widgets
	// the worker writes (the test driver runs fyne.Do inline), which was one of
	// the races #333 recorded.
	u.settle()

	if findButton(u.accountTab.Content, "Log in") == nil {
		t.Fatal("a refused listing did not bring the login card back")
	}

	if u.client.Token != "" {
		t.Errorf("client token = %q, want it cleared", u.client.Token)
	}
}

// A server that does not answer has said nothing about the token, so the
// session stays and the tab shows the panel with both ways out (#335).
func TestUnreachableServerKeepsTheSession(t *testing.T) {
	u := newTestUI(t)

	if err := config.Set("token", "live-token"); err != nil {
		t.Fatal(err)
	}
	u.client.Token = "live-token"

	u.account.failedToLoad("http://127.0.0.1:4321",
		fmt.Errorf("%w: dial tcp 127.0.0.1:4321: connect: connection refused", apiclient.ErrUnreachable))

	if stored, err := config.Value("token"); err != nil || stored != "live-token" {
		t.Errorf("token = %q (%v), want it kept", stored, err)
	}
	if u.client.Token != "live-token" {
		t.Errorf("client token = %q, want it kept", u.client.Token)
	}

	content := u.accountTab.Content
	if findButton(content, "Try again") == nil {
		t.Error("the not-connected panel has no Try again")
	}

	// The bar the panel shows is the one a later sync writes to, and it is in
	// the tree: a detached bar would swallow every address typed into it.
	if !holds(content, u.account.bar.entry) {
		t.Error("the panel's address bar is not the one on screen")
	}

	u.local = false
	u.address = apiChoice{url: "http://127.0.0.1:9999", source: "the address you entered"}
	u.syncAddressBars()

	if got := u.account.bar.entry.Text; got != "http://127.0.0.1:9999" {
		t.Errorf("the tab's address bar shows %q, want the current address", got)
	}
}

// A row is one thing to pick (#373): it is a whole row, not a grid of cells,
// it carries its own Download, and tapping it opens the file's page — where the
// facts are selectable and both copies work. Back returns to the list.
func TestAccountRowOpensAPageWithBack(t *testing.T) {
	view := newTestAccountView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})
	view.ui.clip = test.NewClipboard()

	view.files = []apiclient.File{{ID: "9f1a", Name: "notes", Ext: "txt", Version: 3}}
	view.ready = true
	view.fill()

	row, ok := view.list.list.CreateItem().(*entryRow)
	if !ok {
		t.Fatal("a list item is not an entry row")
	}
	view.list.list.UpdateItem(0, row)

	if row.name.Text != "notes.txt" || !strings.Contains(row.info.Text, "version 3") || !strings.Contains(row.info.Text, "private") {
		t.Errorf("row = %q / %q, want the name and one line of facts", row.name.Text, row.info.Text)
	}
	if row.download.Text != "Download" || row.download.OnTapped == nil {
		t.Error("the row has no Download of its own")
	}

	view.list.list.Select(0)

	if view.pages.atHome() {
		t.Fatal("tapping the row did not open its page")
	}

	page := view.pages.stack.Objects[0]
	for _, caption := range []string{"Back", "Download…", "Share", "Delete", "Copy name", "Copy details"} {
		if findButton(page, caption) == nil {
			t.Errorf("the file's page has no %q", caption)
		}
	}

	test.Tap(findButton(page, "Copy name"))
	if got := view.ui.clip.Content(); got != "notes.txt" {
		t.Errorf("clipboard = %q, want just the name", got)
	}

	test.Tap(findButton(page, "Copy details"))
	if got := view.ui.clip.Content(); got != "notes.txt\t3\tprivate\t9f1a" {
		t.Errorf("clipboard = %q, want a tab-separated record", got)
	}
	if !strings.Contains(view.status.Text, "Copied") {
		t.Errorf("status = %q, want confirmation of the copy", view.status.Text)
	}

	test.Tap(findButton(page, "Back"))

	if !view.pages.atHome() {
		t.Error("Back did not return to the list")
	}
}

// The filter is Templates / Files / All, and All lists templates first.
func TestAccountFilterChoosesWhatIsListed(t *testing.T) {
	view := newTestAccountView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	view.files = []apiclient.File{{ID: "1", Name: "notes", Ext: "txt", Version: 1}}
	view.templates = []templates.TemplateData{{Name: "go-service", Kind: "go", Version: 2, FileCount: 4}}

	view.fill()

	tests := []struct {
		show string
		want []string
	}{
		{showAll, []string{"go-service", "notes.txt"}},
		{showTemplates, []string{"go-service"}},
		{showFiles, []string{"notes.txt"}},
	}

	for _, tt := range tests {
		view.show.SetSelected(tt.show)

		var got []string
		for _, item := range view.list.items {
			got = append(got, item.name)
		}

		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%s lists %v, want %v", tt.show, got, tt.want)
		}
	}
}

// A real listing fills the list from both routes.
func TestAccountListsFilesAndTemplates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/files"):
			_, _ = w.Write([]byte(`{"data":{"data":[{"id":"1","name":"notes","ext":"txt","version":1,"is_public":true}]}}`))
		case strings.HasPrefix(r.URL.Path, "/v1/templates"):
			_, _ = w.Write([]byte(`{"data":{"data":[{"id":"00000000-0000-0000-0000-000000000001","name":"go-service","kind":"go","version":1,"file_count":2}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	u := newTestUI(t)
	u.client = apiclient.New(server.URL).WithToken("the-token")

	u.account.reload()
	u.settle()

	if len(u.account.list.items) != 2 {
		t.Fatalf("listed %d rows, want a template and a file (status %q)", len(u.account.list.items), u.account.status.Text)
	}
	if u.account.list.items[0].kind != entryTemplate || u.account.list.items[1].kind != entryFile {
		t.Error("the template is not listed before the file")
	}
	if !strings.Contains(u.account.list.items[1].info, "shared") {
		t.Errorf("file info = %q, want its visibility", u.account.list.items[1].info)
	}
}

// The address in the header is the thing a person changes, so it is a field
// with a Connect action: typing a valid address switches the tab and is
// remembered, and the session stays on the same host.
func TestAccountAddressIsTypedAndConnected(t *testing.T) {
	u := newTestUI(t)

	u.client.Token = "the-token"
	if err := config.Set("token", "the-token"); err != nil {
		t.Fatal(err)
	}

	u.account.bar.entry.SetText("http://127.0.0.1:5000")
	u.account.bar.connectTyped()

	// The connect lands on the library, which starts a listing: let it finish
	// before reading the window, or the worker's widget work overlaps this
	// goroutine (#333).
	u.settle()

	stored, err := config.Value("api_url")
	if err != nil || stored != "http://127.0.0.1:5000" {
		t.Errorf("api_url = %q (%v), want the typed address", stored, err)
	}

	if u.client.Token != "the-token" {
		t.Errorf("token = %q, want it kept on the same host", u.client.Token)
	}

	if u.address.url != "http://127.0.0.1:5000" {
		t.Errorf("address = %q, want the typed one", u.address.url)
	}
}

// An address the client cannot use is refused where it was typed: nothing is
// saved and the status says what is wrong.
func TestAccountRefusesABadTypedAddress(t *testing.T) {
	u := newTestUI(t)

	u.account.bar.entry.SetText("not a url")
	u.account.bar.connectTyped()

	if _, err := config.Value("api_url"); err == nil {
		t.Error("a bad address was saved")
	}

	if !strings.Contains(u.account.status.Text, "http://") {
		t.Errorf("status = %q, want the validation message", u.account.status.Text)
	}
}

// The checkbox is the choice: checked, the bar shows this machine's address and
// keeps the field shut; unchecking opens it. Neither saves an address — Connect
// is what switches.
func TestAccountLocalCheckboxFillsTheField(t *testing.T) {
	u := newTestUI(t)

	u.local = true
	u.account.bar.sync()

	if got := u.account.bar.entry.Text; got != u.localAddress() {
		t.Errorf("address = %q, want the Local server tab's address %q", got, u.localAddress())
	}

	if !u.account.bar.entry.Disabled() {
		t.Error("the field is open while the bar is set to this machine")
	}

	if _, err := config.Value("api_url"); err == nil {
		t.Error("setting the choice saved an address")
	}

	u.account.bar.local.SetChecked(false)

	if u.account.bar.entry.Disabled() {
		t.Error("unchecking did not open the field")
	}

	stored, err := config.Value(localModeKey)
	if err != nil || stored != "false" {
		t.Errorf("%s = %q (%v), want the manual mode remembered", localModeKey, stored, err)
	}
}
