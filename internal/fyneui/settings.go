//go:build fyne

package fyneui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// settingsView is Settings (#379): the window's configuration, one section at a
// time, listed down the left the way a settings page usually is —
//
//   - Server: which eznit server the window talks to;
//   - Local server: the server this machine can run — its folder, Start and
//     Stop, its output and its configuration;
//   - Reset: erase the local server's data and start from zero.
//
// Server and Reset each end in the page's status line — the same words in both
// — and Local server keeps its own panel's: three lines stacked under the
// panel would leave it no room at the smallest window.
type settingsView struct {
	ui *ui

	bar *addressBar

	sections      *container.AppTabs
	serverSection *container.TabItem
	localSection  *container.TabItem
	resetSection  *container.TabItem

	resetButton *widget.Button
	resetFolder *widget.Label

	// The status line, shown at the bottom of Server and of Reset.
	statuses []*prose
	dots     []*canvas.Circle
	built    fyne.CanvasObject
}

// The words at the top of each section: what it is for, before what it holds.
const (
	settingsIntro = "The eznit server this window talks to: Explore lists what is public on it, and Log in signs in to it. Use this machine for the server in Local server, or type the address of another one — Test asks it without saving, Connect switches to it."

	localIntro = "The server this machine runs from a folder: Start brings up its database and the API."

	resetIntro = "Erase everything the local server holds and start from zero: every account, every file and every template. It cannot be undone.\n\nKept: the folder's configuration (.env) and its database password, so Start brings the server back, empty. Then create a new account — the first one is the server's admin.\n\nOnly this machine's local server is reset: a server elsewhere is never touched, and the window logs out if its session was on the local one."
)

func newSettingsView(u *ui) *settingsView {
	return &settingsView{ui: u}
}

// statusLine is one copy of the page's status line, kept in step with the
// others by setStatus.
func (v *settingsView) statusLine() fyne.CanvasObject {
	message := newProse("")
	dot := canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	v.statuses = append(v.statuses, message)
	v.dots = append(v.dots, dot)

	return statusBar(dot, message)
}

func (v *settingsView) content() fyne.CanvasObject {
	v.statuses, v.dots = nil, nil
	v.bar = newAddressBar(v.ui, v.setStatus, false)

	server := container.NewVBox(heading("Server"), newProse(settingsIntro), v.bar.content())
	v.serverSection = container.NewTabItemWithIcon("Server", theme.ComputerIcon(),
		inset(container.NewBorder(server, v.statusLine(), nil, nil, nil)))

	items := []*container.TabItem{v.serverSection}

	// The local server and its reset exist only where a server view does: a
	// window built without one (the tests' window) has the Server section alone.
	if v.ui.server != nil {
		// The tab names the section, so one line says what it is; the panel
		// under it is the tallest thing in the window and keeps the room.
		local := container.NewBorder(
			container.New(paddedTop(), newProse(localIntro)),
			nil, nil, nil, v.ui.server.content())
		v.localSection = container.NewTabItemWithIcon("Local server", theme.StorageIcon(), local)

		v.resetSection = container.NewTabItemWithIcon("Reset", theme.DeleteIcon(),
			inset(container.NewBorder(v.resetContent(), v.statusLine(), nil, nil, nil)))

		items = append(items, v.localSection, v.resetSection)
	}

	v.sections = container.NewAppTabs(items...)
	v.sections.SetTabLocation(container.TabLocationLeading)

	v.built = v.sections

	return v.built
}

// paddedTop gives a section's heading the page margin on three sides; the
// local server's own panel keeps its padding below it.
func paddedTop() fyne.Layout {
	return layout.NewCustomPaddedLayout(pageInset, 0, pageInset, pageInset)
}

// resetContent is the Reset section: what is erased, what is kept, which folder,
// and the one red button — apart, at the bottom of what it explains.
func (v *settingsView) resetContent() fyne.CanvasObject {
	v.resetFolder = oneLine(v.ui.server.folderPath())

	v.resetButton = widget.NewButtonWithIcon("Reset local server…", theme.DeleteIcon(), v.ui.server.askReset)
	v.resetButton.Importance = widget.DangerImportance

	folder := container.NewBorder(nil, nil, widget.NewLabel("Folder"), nil, v.resetFolder)

	return container.NewVBox(
		heading("Reset the local server"),
		newProse(resetIntro),
		widget.NewSeparator(),
		folder,
		container.NewHBox(v.resetButton),
	)
}

// syncFolder shows the local server's current folder in the Reset section.
func (v *settingsView) syncFolder() {
	if v.resetFolder != nil && v.ui.server != nil {
		v.resetFolder.SetText(v.ui.server.folderPath())
	}
}

// showSection selects one section.
func (v *settingsView) showSection(section *container.TabItem) {
	if v.sections != nil && section != nil {
		v.sections.Select(section)
	}
}

func (v *settingsView) setStatus(text string, kind statusKind) {
	for _, message := range v.statuses {
		message.Importance = statusImportance(kind)
		message.SetText(text)
	}

	for _, dot := range v.dots {
		dot.FillColor = statusColour(v.ui.app.Settings().ThemeVariant(), kind)
		dot.Refresh()
	}

	relayout(v.built)
}

// status is the page's status line, for a caller that reads it.
func (v *settingsView) status() string {
	if len(v.statuses) == 0 {
		return ""
	}

	return v.statuses[0].Text
}
