//go:build fyne

package fyneui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// settingsView is the Settings tab (#376): the one place the server is chosen.
// The address bar used to sit on top of every list; a choice made once and
// read everywhere belongs on a page of its own, like the web page's Settings.
type settingsView struct {
	ui *ui

	bar *addressBar

	status *prose
	dot    *canvas.Circle
	built  fyne.CanvasObject
}

// settingsIntro says what the address governs, so a person knows that
// changing it changes every tab.
const settingsIntro = "The eznit server this window talks to: the Web tab lists what is public on it, and My Account signs in to it. Use this machine for the server the Local server tab runs, or type the address of another one — Test asks it without saving, Connect switches to it."

func newSettingsView(u *ui) *settingsView {
	return &settingsView{ui: u, status: newProse("")}
}

func (v *settingsView) content() fyne.CanvasObject {
	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))
	v.bar = newAddressBar(v.ui, v.setStatus, false)

	server := container.NewVBox(heading("Server"), newProse(settingsIntro), v.bar.content())

	v.built = inset(container.NewBorder(server, statusBar(v.dot, v.status), nil, nil, widget.NewLabel("")))

	return v.built
}

func (v *settingsView) setStatus(text string, kind statusKind) {
	if v.status == nil {
		return
	}

	v.status.Importance = statusImportance(kind)
	v.status.SetText(text)

	if v.dot != nil {
		v.dot.FillColor = statusColour(v.ui.app.Settings().ThemeVariant(), kind)
		v.dot.Refresh()
	}

	relayout(v.built)
}
