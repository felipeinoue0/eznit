//go:build fyne

// Package fyneui is the eznit desktop app: a native window over the same /v1
// API the CLI and the web UI use.
//
// It is a client, not a server. The server's browser UI is unchanged; this is
// the tool a developer installs beside the CLI, reading the same
// ~/.eznit/config.json so one login serves both.
package fyneui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/serverctl"
)

// projectURL is where About sends a reader.
const projectURL = "https://github.com/fiwon123/eznit"

type ui struct {
	app     fyne.App
	window  fyne.Window
	version string

	// clip is the system clipboard. It is a field rather than a call so a test
	// can supply one: the software driver's clipboard is nil.
	clip fyne.Clipboard

	// address is the instance the window talks to, and where that address
	// came from — shown in each tab's header rather than picked silently.
	address apiChoice

	// local is the "Use this machine" choice. While it is on, the address bars
	// fill themselves from the local server's folder; the choice is remembered.
	local bool

	client   *apiclient.Client
	web      *webView
	account  *accountView
	settings *settingsView
	server   *serverView
	updater  *appUpdater

	tabs *container.AppTabs

	// The two tabs, kept by identity: Workspace is in the bar only while a
	// session is (#379).
	webTab       *container.TabItem
	workspaceTab *container.TabItem

	// The window shows the tabs (mainScreen) or one full-window screen with
	// Close: the login card, Settings or About. The bar's right end carries the
	// buttons that open them, and Log in turns into Log out with a session.
	mainScreen     fyne.CanvasObject
	loginScreen    fyne.CanvasObject
	settingsScreen fyne.CanvasObject
	aboutScreen    fyne.CanvasObject
	loginBody      *fyne.Container
	workspaceBody  *fyne.Container
	sessionButton  *widget.Button

	// quietTabs is set while the window changes the tab bar itself, so the
	// selection that follows does not start a load mid-change (#333).
	quietTabs bool

	// work counts the background requests the tabs have running, so a test can
	// wait for the window to be idle instead of polling it while it is busy
	// (see background and settle in work.go).
	work sync.WaitGroup
}

// apiChoice is one server address and the reason it is the current one.
type apiChoice struct {
	url    string
	source string
}

// Run opens the window and blocks until it closes.
func Run(version string, apiURL string, serverDir string) error {
	a := app.NewWithID("com.fiwon123.eznit")
	a.Settings().SetTheme(newBrandTheme())
	a.SetIcon(logoResource())

	window := a.NewWindow("eznit")
	window.Resize(fyne.NewSize(980, 640))

	u := &ui{
		app:     a,
		window:  window,
		version: version,
		clip:    a.Clipboard(),
		local:   localMode(apiURL),
		address: apiFrom(apiURL),
		updater: newAppUpdater(version),
	}

	// The default mode is this machine, and its address is the local server's
	// folder. That folder is not known until the view exists, so the default
	// stand-in is replaced below.
	if u.local {
		u.address = apiChoice{url: apiclient.NormalizeBaseURL(""), source: localSource}
	}

	u.client = apiclient.New(u.address.url)
	u.web = newWebView(u)
	u.account = newAccountView(u)
	u.settings = newSettingsView(u)
	u.server = newServerView(u, serverFrom(serverDir))

	if u.local {
		u.address = apiChoice{url: u.localAddress(), source: localSource}
		u.client = apiclient.New(u.address.url)
	}

	// The tabs exist before a session does. Explore needs none, and the local
	// server in Settings has to be usable on a machine whose server has never
	// run — running it and pointing it at a database is how a first install
	// gets something to log in to.
	u.buildTabs()

	window.SetContent(u.mainScreen)

	// A server this window started should not outlive it; ask before stopping
	// it, so closing the window is never a surprise shutdown.
	window.SetCloseIntercept(func() {
		if !u.server.running() {
			window.Close()
			return
		}

		dialog.ShowConfirm("Stop the local server?",
			"The server this window started is still running. Stop it and close the window?",
			func(confirmed bool) {
				if !confirmed {
					return
				}

				u.background(func() {
					_ = u.server.server().Stop(stopTimeout)
					fyne.Do(window.Close)
				})
			}, window)
	})

	if token, err := config.Value("token"); err == nil {
		u.client = u.client.WithToken(token)
	}

	if u.client.Token == "" {
		u.showLogin("")
	} else {
		u.showMain()
	}

	// Explore is the first tab, so it loads now rather than on a tab change.
	u.web.ensureLoaded()

	go u.server.pollHealth()

	window.ShowAndRun()

	return nil
}

// buildTabs lays out the window (#379): on the left the tabs a person works
// in — Explore, and Workspace once logged in — and on the right of their bar
// Log in (Log out with a session), Settings and About.
func (u *ui) buildTabs() {
	u.webTab = container.NewTabItemWithIcon("Explore", theme.SearchIcon(), u.web.content())

	u.workspaceBody = container.NewStack(u.account.content())
	u.workspaceTab = container.NewTabItemWithIcon("Workspace", theme.FolderIcon(), u.workspaceBody)

	u.tabs = container.NewAppTabs(u.webTab)

	u.sessionButton = widget.NewButtonWithIcon("Log in", theme.LoginIcon(), u.sessionAction)
	u.sessionButton.Importance = widget.LowImportance
	settings := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), u.openSettings)
	settings.Importance = widget.LowImportance
	about := widget.NewButtonWithIcon("About", theme.InfoIcon(), func() { u.showScreen(u.aboutScreen) })
	about.Importance = widget.LowImportance

	// The buttons sit over the empty right end of the tab bar: the bar is the
	// tabs' own, and buttons in it read as part of it.
	corner := container.NewVBox(container.NewHBox(layout.NewSpacer(), u.sessionButton, settings, about))
	u.mainScreen = container.NewStack(u.tabs, container.New(layout.NewCustomPaddedLayout(0, 0, 0, pageInset), corner))

	u.loginBody = container.NewStack(newLoginView(u, "", statusIdle).content())
	u.loginScreen = u.screen("Log in", theme.LoginIcon(), u.loginBody)
	u.settingsScreen = u.screen("Settings", theme.SettingsIcon(), u.settings.content())
	u.aboutScreen = u.screen("About", theme.InfoIcon(), inset(u.aboutContent()))

	u.updateSessionButton()

	// Explore is per server and per session; it reloads when it is shown after
	// either changed.
	u.tabs.OnSelected = func(item *container.TabItem) {
		if item == u.webTab && !u.quietTabs {
			u.web.ensureLoaded()
		}
	}
}

// screen is a full-window page: its title on the left of a header, Close on
// the right, and its body under them.
func (u *ui) screen(title string, icon fyne.Resource, body fyne.CanvasObject) fyne.CanvasObject {
	closeButton := widget.NewButtonWithIcon("Close", theme.CancelIcon(), u.showTabs)

	name := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	header := container.NewVBox(
		container.New(layout.NewCustomPaddedLayout(theme.Padding(), 0, pageInset, pageInset),
			container.NewBorder(nil, nil, widget.NewIcon(icon), closeButton, name)),
		widget.NewSeparator(),
	)

	return container.NewBorder(header, nil, nil, nil, body)
}

// serverFrom decides the server folder: the flag, then the stored setting,
// then the folder this binary was installed into.
func serverFrom(flagValue string) string {
	if value := strings.TrimSpace(flagValue); value != "" {
		return value
	}

	if stored, err := config.Value("server_dir"); err == nil && stored != "" {
		return stored
	}

	return defaultServerDir()
}

// defaultServerDir is the folder this window was installed into — the bundle a
// released window sits in. It is what "reset to default" returns to, and it is
// never written to the config file: a default that was stored on the first
// launch is indistinguishable from a choice.
func defaultServerDir() string {
	if executable, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(executable); err == nil {
			executable = resolved
		}

		return filepath.Dir(executable)
	}

	return "."
}

// apiFrom decides the API base: the flag, then GUI_API_URL, then the saved
// address, then the local stack. The environment beats the saved address so
// the documented GUI_API_URL keeps working after a login has stored one; the
// tabs' headers name whichever source won, so it is never silent.
func apiFrom(flagValue string) apiChoice {
	if value := strings.TrimSpace(flagValue); value != "" {
		return apiChoice{url: apiclient.NormalizeBaseURL(value), source: "the -api flag"}
	}

	if value := strings.TrimSpace(os.Getenv("GUI_API_URL")); value != "" {
		return apiChoice{url: apiclient.NormalizeBaseURL(value), source: "GUI_API_URL"}
	}

	if stored, err := config.Value("api_url"); err == nil && strings.TrimSpace(stored) != "" {
		return apiChoice{url: apiclient.NormalizeBaseURL(stored), source: "the saved address"}
	}

	return apiChoice{url: apiclient.NormalizeBaseURL(""), source: "the default"}
}

// localAddress is the API that a server started from the local server's folder
// answers on. The port comes from that folder's configuration, so the two tabs
// cannot disagree about where "this machine" is.
func (u *ui) localAddress() string {
	if u.server == nil {
		return apiclient.NormalizeBaseURL("")
	}

	return serverctl.LocalAddress(u.server.folderPath())
}

// setLocal remembers whether the window talks to this machine. It does not
// switch the client: Connect does that, so a wrong folder cannot take a session
// by surprise.
func (u *ui) setLocal(local bool) error {
	u.local = local

	return config.Set(localModeKey, strconv.FormatBool(local))
}

// connect switches every tab to a validated address and applies the session
// rule: a different host loses the token and brings the login form back.
func (u *ui) connect(choice apiChoice) (bool, error) {
	signedOut, err := u.useServer(choice)
	if err != nil {
		return false, err
	}

	u.syncAddressBars()

	// A connect is not a login. Connecting from Settings with no
	// session used to put the empty library in front of a window that had
	// nothing to fill it with, and the tab only said so after a request failed
	// (#336). The message names the address, because the card's own field is
	// where a wrong one gets fixed.
	if signedOut || u.client.Token == "" {
		u.showLogin("Log in to " + choice.url + ".")

		return true, nil
	}

	u.showMain()

	return false, nil
}

// syncAddressBars keeps every bar on the current mode and address. A tab
// rebuilds its bar with its content, so this reads the two views' current ones.
func (u *ui) syncAddressBars() {
	if u.settings != nil && u.settings.bar != nil {
		u.settings.bar.sync()
	}
}

// currentAddress is the server the window means right now: this machine's when
// the local choice is on, the chosen address otherwise.
func (u *ui) currentAddress() string {
	if u.local {
		return u.localAddress()
	}

	return u.address.url
}

// showScreen puts a full-window screen in the window.
func (u *ui) showScreen(screen fyne.CanvasObject) {
	if screen != nil {
		u.window.SetContent(screen)
	}
}

// showTabs goes back to the tabs.
func (u *ui) showTabs() {
	if u.mainScreen != nil {
		u.window.SetContent(u.mainScreen)
	}
}

// screenShown reports whether screen is the one on show.
func (u *ui) screenShown(screen fyne.CanvasObject) bool {
	return screen != nil && u.window.Content() == screen
}

// openSettings shows Settings at its Server section, where the address is
// chosen.
func (u *ui) openSettings() {
	if u.settings != nil {
		u.settings.showSection(u.settings.serverSection)
	}

	u.showScreen(u.settingsScreen)
}

// sessionAction is the bar's Log in / Log out button.
func (u *ui) sessionAction() {
	if u.client.Token == "" {
		u.showScreen(u.loginScreen)
		return
	}

	u.signOut()
}

// updateSessionButton names the button by the session.
func (u *ui) updateSessionButton() {
	if u.sessionButton == nil {
		return
	}

	if u.client.Token == "" {
		u.sessionButton.SetText("Log in")
		u.sessionButton.SetIcon(theme.LoginIcon())
		return
	}

	u.sessionButton.SetText("Log out")
	u.sessionButton.SetIcon(theme.LogoutIcon())
}

// hasWorkspaceTab reports whether Workspace is in the bar.
func (u *ui) hasWorkspaceTab() bool {
	if u.tabs == nil {
		return false
	}

	for _, item := range u.tabs.Items {
		if item == u.workspaceTab {
			return true
		}
	}

	return false
}

// addWorkspaceTab puts Workspace in the bar after Explore.
func (u *ui) addWorkspaceTab() {
	if u.tabs != nil && !u.hasWorkspaceTab() {
		u.tabs.Append(u.workspaceTab)
	}
}

// removeWorkspaceTab takes Workspace out of the bar, landing on Explore when
// it was the tab on show.
func (u *ui) removeWorkspaceTab() {
	if !u.hasWorkspaceTab() {
		return
	}

	u.quietTabs = true
	defer func() { u.quietTabs = false }()

	if u.tabs.Selected() == u.workspaceTab {
		u.tabs.Select(u.webTab)
	}

	u.tabs.Remove(u.workspaceTab)
}

// reloadExploreLast loads Explore again when it is the tab on show. Callers
// make it their last step: the load runs in the background, and nothing may
// touch a widget while its answer can (#333).
func (u *ui) reloadExploreLast() {
	if u.tabs != nil && u.web != nil && u.tabs.Selected() == u.webTab {
		u.web.reload()
	}
}

// setAPI points the client at an instance and keeps the stored session: the
// caller decides whether that token still belongs there.
func (u *ui) setAPI(address apiChoice) {
	u.address = apiChoice{
		url:    apiclient.NormalizeBaseURL(address.url),
		source: address.source,
	}
	u.client = apiclient.New(u.address.url)

	if token, err := config.Value("token"); err == nil {
		u.client = u.client.WithToken(token)
	}

	// The lists are per instance: a new address is a new list. The Web bar's
	// own connect reloads it; a connect from elsewhere leaves it to the next
	// visit.
	if u.web != nil {
		u.web.forget()
	}
	if u.account != nil {
		u.account.forget()
	}
}

// useServer points the window at another instance and remembers the choice.
// A different host is a different instance, and the token was issued by the old
// one, so it is forgotten; it reports whether the session had to go.
func (u *ui) useServer(address apiChoice) (bool, error) {
	sameHost := apiclient.SameHost(u.client.BaseURL, address.url)

	if err := config.Set("api_url", address.url); err != nil {
		return false, fmt.Errorf("the address could not be saved: %w", err)
	}

	u.setAPI(address)

	if sameHost {
		return false, nil
	}

	if err := config.Remove("token"); err != nil {
		return false, fmt.Errorf("the old session could not be forgotten: %w", err)
	}

	// A new client rather than a written one: a request that captured the old
	// client still carries the token it was given, and this one is new (#342).
	u.client = u.client.WithToken("")

	return true, nil
}

// showMain puts the signed-in account in Workspace and the tab in the bar.
func (u *ui) showMain() {
	// A session names the owners on Explore: drop the list, and it loads
	// again when the tab is next shown. It is not reloaded from here — the
	// account's own listing is already running, and two workers touching
	// widgets at once is what #333 removed.
	if u.web != nil {
		u.web.forget()
	}

	u.setWorkspace(u.account.content())
	u.addWorkspaceTab()
	u.updateSessionButton()
	u.account.reload()
}

// openWorkspace is a fresh login's landing: the tabs, on Workspace. It is
// showMain with the tab selected before the listing starts, so nothing here
// touches a widget while the listing's answer can (#333).
func (u *ui) openWorkspace() {
	u.showTabs()

	if u.web != nil {
		u.web.forget()
	}

	u.setWorkspace(u.account.content())
	u.addWorkspaceTab()
	u.updateSessionButton()

	if u.tabs != nil {
		u.tabs.Select(u.workspaceTab)
	}

	u.account.reload()
}

// showLogin prepares the login card. It does not put it on show: Log in does,
// or a session that ended does.
func (u *ui) showLogin(message string) {
	u.showLoginWith(message, statusIdle)
}

// showLoginWith is showLogin with the colour the message earns: a session that
// could not be forgotten is an error, and the card should not say it in the
// same grey as "you are signed out".
func (u *ui) showLoginWith(message string, kind statusKind) {
	u.setLoginCard(newLoginView(u, message, kind).content())
	u.updateSessionButton()

	if u.client.Token == "" {
		u.removeWorkspaceTab()
	}
}

// showUnreachable puts Workspace's "cannot reach" panel in place. The session
// is kept: a server that is not there has said nothing about the token.
func (u *ui) showUnreachable(address string, cause error) {
	content := u.account.unreachableContent(address, cause)
	u.setWorkspace(content)

	// The panel's card is measured before it has a width, and a sentence that
	// wraps is taller than one line: lay it out again now that it has one.
	relayout(content)
}

// setLoginCard replaces the login screen's card.
func (u *ui) setLoginCard(content fyne.CanvasObject) {
	if u.loginBody == nil {
		return
	}

	u.loginBody.Objects = []fyne.CanvasObject{content}
	u.loginBody.Refresh()
}

// setWorkspace replaces what Workspace shows: the account, or the unreachable
// panel.
func (u *ui) setWorkspace(content fyne.CanvasObject) {
	if u.workspaceBody == nil {
		return
	}

	u.workspaceBody.Objects = []fyne.CanvasObject{content}
	u.workspaceBody.Refresh()
}

// sessionEnded is what a refused token says. The API answered — with "not a
// session I issued" — so the window stops pretending and asks again.
const sessionEnded = "Your session ended. Log in again."

// signOut forgets the session token and only the token: the API address and
// the git-host tokens stay. The window stays on its tabs, without Workspace.
func (u *ui) signOut() {
	u.endSession("You are logged out.", false)

	// Explore named owners only with a session; the window lands on it.
	u.reloadExploreLast()
}

// forgetSession ends a session the API refused, and puts the login card on
// show with the reason: a session that ended behind the tabs would otherwise
// end silently.
func (u *ui) forgetSession(message string) {
	u.endSession(message, true)
	u.reloadExploreLast()
}

// endSession forgets the token, drops what belonged to the session, and leaves
// the login card ready with message. show puts it on show.
func (u *ui) endSession(message string, show bool) {
	if err := config.Remove("token"); err != nil {
		u.showLoginWith("The session could not be forgotten: "+err.Error(), statusError)
		u.showScreen(u.loginScreen)
		return
	}

	// A new client, not a cleared field: a request that captured the old one
	// keeps the token it was sent with, and the window's next request has none
	// (#342).
	u.client = u.client.WithToken("")

	if u.account != nil {
		u.account.forget()
	}

	if u.web != nil {
		u.web.forget()
	}

	u.showLogin(message)

	if show {
		u.showScreen(u.loginScreen)
	}
}

// copyToClipboard puts text where a person can paste it. It reports whether a
// clipboard existed: the software driver used by the tests has none, and a
// silent no-op would read as a successful copy.
func (u *ui) copyToClipboard(text string) bool {
	clipboard := u.clip

	if clipboard == nil && u.app != nil {
		clipboard = u.app.Clipboard()
	}

	if clipboard == nil {
		return false
	}

	clipboard.SetContent(text)

	return true
}
