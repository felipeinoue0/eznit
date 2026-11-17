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
	// fill themselves from the Local server tab's folder; the choice is remembered.
	local bool

	client   *apiclient.Client
	web      *webView
	account  *accountView
	settings *settingsView
	server   *serverView
	updater  *appUpdater

	tabs *container.AppTabs

	// webTab and settingsTab are kept by identity, not position.
	webTab      *container.TabItem
	settingsTab *container.TabItem

	// The window shows one of two screens (#376): the tabs, with the account
	// button at the far right of their bar, or the account — the login card,
	// the account's list, or the unreachable panel in accountBody — with Close.
	mainScreen     fyne.CanvasObject
	accountScreen  fyne.CanvasObject
	accountBody    *fyne.Container
	accountButton  *widget.Button
	accountSignOut *widget.Button

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

	// The default mode is this machine, and its address is the Local server tab's
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

	// The tabs exist before a session does. Web needs none, and the Local
	// server tab has to be usable on a machine whose server has never run —
	// running it and pointing it at a database is how a first install gets
	// something to log in to.
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

	// Web is the first tab, so it loads now rather than on a tab change.
	u.web.ensureLoaded()

	go u.server.pollHealth()

	window.ShowAndRun()

	return nil
}

// buildTabs lays out the window (#376): on the left the tabs a person works
// in — Web, Local server, Settings, About — and on the far right of their bar
// the account button, which opens the account screen.
func (u *ui) buildTabs() {
	u.webTab = container.NewTabItemWithIcon("Web", theme.GridIcon(), u.web.content())
	u.settingsTab = container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), u.settings.content())

	items := []*container.TabItem{u.webTab}
	if u.server != nil {
		items = append(items, container.NewTabItemWithIcon("Local server", theme.ComputerIcon(), u.server.content()))
	}
	items = append(items, u.settingsTab, container.NewTabItemWithIcon("About", theme.InfoIcon(), u.aboutContent()))

	u.tabs = container.NewAppTabs(items...)

	// The button sits over the empty right end of the tab bar: the bar is
	// the tabs' own, and a button in it reads as part of it.
	u.accountButton = widget.NewButtonWithIcon("Sign in", theme.AccountIcon(), u.showAccount)
	u.accountButton.Importance = widget.LowImportance
	corner := container.NewVBox(container.NewHBox(layout.NewSpacer(), u.accountButton))
	u.mainScreen = container.NewStack(u.tabs, container.New(layout.NewCustomPaddedLayout(0, 0, 0, pageInset), corner))

	u.accountSignOut = widget.NewButtonWithIcon("Sign out", theme.LogoutIcon(), u.signOut)
	u.accountSignOut.Importance = widget.LowImportance
	closeAccount := widget.NewButtonWithIcon("Close", theme.CancelIcon(), u.hideAccount)

	title := widget.NewLabelWithStyle("My Account", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	header := container.NewVBox(
		container.New(layout.NewCustomPaddedLayout(theme.Padding(), 0, pageInset, pageInset),
			container.NewBorder(nil, nil, widget.NewIcon(theme.AccountIcon()), container.NewHBox(u.accountSignOut, closeAccount), title)),
		widget.NewSeparator(),
	)
	u.accountBody = container.NewStack(u.account.content())
	u.accountScreen = container.NewBorder(header, nil, nil, nil, u.accountBody)
	u.updateAccountButton()

	// The Web list is per server and per session; it reloads when it is shown
	// after either changed.
	u.tabs.OnSelected = func(item *container.TabItem) {
		if item == u.webTab {
			u.web.ensureLoaded()
		}
	}
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

// localAddress is the API that a server started from the Local server tab's folder
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

	// A connect is not a login. Connecting from the Web tab with no
	// session used to put the empty library in front of a window that had
	// nothing to fill it with, and the tab only said so after a request failed
	// (#336). The message names the address, because the card's own field is
	// where a wrong one gets fixed.
	if signedOut || u.client.Token == "" {
		u.showLogin("Sign in to " + choice.url + ".")

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

// showAccount puts the account screen in the window.
func (u *ui) showAccount() {
	if u.accountScreen != nil {
		u.window.SetContent(u.accountScreen)
	}
}

// hideAccount goes back to the tabs.
func (u *ui) hideAccount() {
	if u.mainScreen != nil {
		u.window.SetContent(u.mainScreen)
	}
}

// openSettings leaves the account screen for the Settings tab, where the
// server is chosen.
func (u *ui) openSettings() {
	u.hideAccount()

	if u.tabs != nil && u.settingsTab != nil {
		u.tabs.Select(u.settingsTab)
	}
}

// accountShown reports whether the account screen is the one on show.
func (u *ui) accountShown() bool {
	return u.accountScreen != nil && u.window.Content() == u.accountScreen
}

// updateAccountButton names the account button by the session: Sign in
// without one, My Account with one. Sign out is only offered with one.
func (u *ui) updateAccountButton() {
	if u.accountButton == nil {
		return
	}

	if u.client.Token == "" {
		u.accountButton.SetText("Sign in")
		u.accountSignOut.Hide()
		return
	}

	u.accountButton.SetText("My Account")
	u.accountSignOut.Show()
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

// showMain swaps My Account to the signed-in account.
func (u *ui) showMain() {
	// A session names the owners on the Web tab: drop the list, and it loads
	// again when the tab is next shown. It is not reloaded from here — the
	// account's own listing is already running, and two workers touching
	// widgets at once is what #333 removed.
	if u.web != nil {
		u.web.forget()
	}

	u.setAccountTab(u.account.content())
	u.updateAccountButton()
	u.account.reload()
}

// showLogin puts the sign-in form on My Account. The window keeps its other
// tabs: a signed-out user can still run a local server from here.
func (u *ui) showLogin(message string) {
	u.showLoginWith(message, statusIdle)
}

// showLoginWith is showLogin with the colour the message earns: a session that
// could not be forgotten is an error, and the card should not say it in the
// same grey as "you are signed out".
func (u *ui) showLoginWith(message string, kind statusKind) {
	u.setAccountTab(newLoginView(u, message, kind).content())
	u.updateAccountButton()
}

// showUnreachable puts My Account's "cannot reach" panel in place. The
// session is kept: a server that is not there has said nothing about the token.
func (u *ui) showUnreachable(address string, cause error) {
	content := u.account.unreachableContent(address, cause)
	u.setAccountTab(content)

	// The panel's card is measured before it has a width, and a sentence that
	// wraps is taller than one line: lay it out again now that it has one.
	relayout(content)
}

// setAccountTab replaces what the account screen shows: the login card, the
// account, or the unreachable panel. The tabs stay as they are, so the Local
// server tab keeps its log and its status across a sign-in or a sign-out.
func (u *ui) setAccountTab(content fyne.CanvasObject) {
	u.accountBody.Objects = []fyne.CanvasObject{content}
	u.accountBody.Refresh()
}

// sessionEnded is what a refused token says. The API answered — with "not a
// session I issued" — so the window stops pretending and asks again.
const sessionEnded = "Your session ended. Sign in again."

// signOut forgets the session token and only the token: the API address and
// the git-host tokens stay. The catalogue is per account, so it is dropped too:
// a signed-out window should not keep showing a private template.
func (u *ui) signOut() {
	u.forgetSession("You are signed out.")
}

// forgetSession is signOut for a caller with its own sentence: the API refused
// the token, and the reason belongs on the card rather than in a log.
func (u *ui) forgetSession(message string) {
	if err := config.Remove("token"); err != nil {
		u.showLoginWith("The session could not be forgotten: "+err.Error(), statusError)
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

	// The login card says why, so it is put on show: a session that ended
	// while the tabs were up would otherwise end silently behind them.
	u.showLogin(message)
	u.showAccount()
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
