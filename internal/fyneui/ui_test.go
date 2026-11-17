//go:build fyne

package fyneui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/update"
)

// The address is resolved in one order, and the source is reported with it:
// the environment wins over a saved address so the documented GUI_API_URL keeps
// working after a login has stored one.
func TestAPISourceOrder(t *testing.T) {
	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("GUI_API_URL", "http://from-env.test:4000")

	flagged := apiFrom("http://from-flag.test:4000")
	if flagged.url != "http://from-flag.test:4000" || !strings.Contains(flagged.source, "-api") {
		t.Errorf("apiFrom(flag) = %+v, want the flag and its name", flagged)
	}

	fromEnv := apiFrom("")
	if fromEnv.url != "http://from-env.test:4000" || fromEnv.source != "GUI_API_URL" {
		t.Errorf("apiFrom(\"\") = %+v, want GUI_API_URL", fromEnv)
	}

	if err := config.Set("api_url", "http://saved.test:4000"); err != nil {
		t.Fatal(err)
	}

	// With both set, the environment still wins: that is the order that keeps
	// the documented GUI_API_URL working after a login has stored an address.
	both := apiFrom("")
	if both.url != "http://from-env.test:4000" || both.source != "GUI_API_URL" {
		t.Errorf("apiFrom(\"\") = %+v with both set, want the environment", both)
	}

	t.Setenv("GUI_API_URL", "")

	fromStore := apiFrom("")
	if fromStore.url != "http://saved.test:4000" || fromStore.source != "the saved address" {
		t.Errorf("apiFrom(\"\") = %+v without the environment, want the store", fromStore)
	}

	fromStore = apiFrom("")
	if fromStore.url != "http://saved.test:4000" {
		t.Errorf("apiFrom(\"\") = %+v without the environment, want the store", fromStore)
	}

	if err := config.Remove("api_url"); err != nil {
		t.Fatal(err)
	}

	fallback := apiFrom("")
	if fallback.url != apiclient.DefaultBaseURL || fallback.source != "the default" {
		t.Errorf("apiFrom(\"\") = %+v with nothing set, want the default", fallback)
	}
}

// A session belongs to the instance that issued it: a port change on one host
// keeps it, a different host does not.
func TestUseServerKeepsTheTokenOnOneHostOnly(t *testing.T) {
	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	u := &ui{client: apiclient.New("http://127.0.0.1:4000")}
	u.client.Token = "the-token"

	if err := config.Set("token", "the-token"); err != nil {
		t.Fatal(err)
	}

	signedOut, err := u.useServer(apiChoice{url: "http://127.0.0.1:5000", source: "the address you entered"})
	if err != nil {
		t.Fatal(err)
	}
	if signedOut {
		t.Error("a port change on the same host signed the session out")
	}
	if u.client.Token != "the-token" {
		t.Errorf("token = %q, want it kept", u.client.Token)
	}

	signedOut, err = u.useServer(apiChoice{url: "https://eznit.example.com", source: "the address you entered"})
	if err != nil {
		t.Fatal(err)
	}
	if !signedOut {
		t.Error("a different host kept the session")
	}
	if u.client.Token != "" {
		t.Errorf("token = %q, want it forgotten", u.client.Token)
	}

	if _, err := config.Value("token"); err == nil {
		t.Error("the token is still in the config file")
	}

	stored, err := config.Value("api_url")
	if err != nil || stored != "https://eznit.example.com" {
		t.Errorf("stored api_url = %q (%v), want the new address", stored, err)
	}
}

// "This machine" means the server the Local server tab would run, on the port that
// folder's configuration names.
func TestLocalAddressFollowsTheServerTabsPort(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4321\"\n"))

	if got := view.ui.localAddress(); got != "http://127.0.0.1:4321" {
		t.Errorf("localAddress = %q, want the Local server tab's port", got)
	}
}

// newTestUI is the window's tabs without the window: enough of it to sign in
// and out, and to look at what a tab says.
func newTestUI(t *testing.T) *ui {
	t.Helper()

	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	app := test.NewApp()
	t.Cleanup(app.Quit)

	u := &ui{
		app:     app,
		window:  app.NewWindow("test"),
		version: "test",
		clip:    test.NewClipboard(),
		address: apiChoice{url: "http://127.0.0.1:4321", source: "the default"},
		updater: &appUpdater{version: "test", updater: update.New(update.MemberGUI)},
	}
	u.client = apiclient.New(u.address.url)
	u.web = newWebView(u)
	u.account = newAccountView(u)
	u.settings = newSettingsView(u)

	// The window's own tabs, without the Local server one: these tests do not
	// start a server.
	u.buildTabs()
	u.window.SetContent(u.mainScreen)

	// Settle before the app quits. A request still in flight would otherwise
	// touch widgets after the driver is gone, and the -race suite would report
	// it against whichever test happened to run next (#333). Cleanups run last
	// in, first out, so this runs before the Quit registered above.
	t.Cleanup(u.settle)

	return u
}

// tabNamed finds a tab by its title, so a test does not depend on where the
// tab sits: adding the Shared tab moved About (#368).
func tabNamed(t *testing.T, u *ui, title string) *container.TabItem {
	t.Helper()

	for _, item := range u.tabs.Items {
		if item.Text == title {
			return item
		}
	}
	t.Fatalf("no %q tab", title)
	return nil
}

// A connect is not a login: connecting from another tab with no session leaves
// My Account asking for one, rather than showing a library it cannot fill
// (#336).
func TestConnectWithoutASessionShowsLogin(t *testing.T) {
	u := newTestUI(t)

	signedOut, err := u.connect(apiChoice{url: "http://127.0.0.1:4321", source: "the address you entered"})
	if err != nil {
		t.Fatal(err)
	}

	if !signedOut {
		t.Error("a connect without a session did not report a sign-out")
	}

	if findButton(u.loginBody.Objects[0], "Log in") == nil || u.hasWorkspaceTab() {
		t.Error("a connect without a session left Workspace up or no login card ready")
	}
}

// A client a request has captured is never written: signing out replaces the
// window's client, and the one already in a worker's hand keeps its token
// (#342). This is the contract that makes the request path race-free; the
// `-race` suite is what proves nothing writes a shared client.
func TestForgetSessionLeavesACapturedClientAlone(t *testing.T) {
	u := newTestUI(t)

	if err := config.Set("token", "the-token"); err != nil {
		t.Fatal(err)
	}

	u.client = apiclient.New("http://127.0.0.1:4321").WithToken("the-token")
	inFlight := u.client

	u.forgetSession(sessionEnded)

	if inFlight.Token != "the-token" {
		t.Errorf("the captured client's token = %q, want it untouched", inFlight.Token)
	}

	if u.client.Token != "" {
		t.Errorf("the window's client still carries %q", u.client.Token)
	}

	if _, err := config.Value("token"); err == nil {
		t.Error("the stored token is still there")
	}
}

// Log out is the bar's session button: it the login form replaces,
// so that is where signing out belongs. It forgets the token and the catalogue,
// and keeps the address and the host tokens.
func TestLogOutIsTheBarsButton(t *testing.T) {
	u := newTestUI(t)

	if err := config.Set("token", "the-token"); err != nil {
		t.Fatal(err)
	}
	if err := config.Set("api_url", "http://127.0.0.1:4321"); err != nil {
		t.Fatal(err)
	}

	u.client.Token = "the-token"
	u.account.templates = []templates.TemplateData{{Name: "a private starter"}}
	u.account.ready = true
	u.addWorkspaceTab()
	u.tabs.Select(u.workspaceTab)

	// Log out is the bar's session button, named by the session.
	u.updateSessionButton()
	if u.sessionButton.Text != "Log out" {
		t.Fatalf("session button = %q with a session, want Log out", u.sessionButton.Text)
	}

	test.Tap(u.sessionButton)
	u.settle()

	if _, err := config.Value("token"); err == nil {
		t.Error("the session token is still in the config file")
	}

	stored, err := config.Value("api_url")
	if err != nil || stored != "http://127.0.0.1:4321" {
		t.Errorf("api_url = %q (%v), want the address kept", stored, err)
	}

	if u.client.Token != "" {
		t.Errorf("client token = %q, want it cleared", u.client.Token)
	}

	if u.account.ready || u.account.templates != nil {
		t.Error("the account's templates are still loaded after signing out")
	}

	if u.hasWorkspaceTab() || u.tabs.Selected() != u.webTab {
		t.Error("Workspace is still in the bar after logging out")
	}

	if u.screenShown(u.loginScreen) {
		t.Error("a deliberate log-out put the login card in front of the tabs")
	}

	if findButton(u.aboutContent(), "Log out") != nil {
		t.Error("About offers Log out")
	}
}
