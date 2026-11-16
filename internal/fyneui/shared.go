//go:build fyne

package fyneui

import (
	"fmt"
	"net/http"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// sharedView is the Shared tab (#368): every file anyone on the server made
// public. The Files tab lists only the signed-in account's own files, so
// without this a second account never saw a shared file in the window.
//
// Like the Templates tab it needs no session: a person without an account can
// still download what was shared. With one, each row also names its owner.
type sharedView struct {
	ui *ui

	items    []apiclient.SharedFile
	selected int
	loaded   bool

	table    *widget.Table
	refresh  *widget.Button
	download *widget.Button

	status *prose
	dot    *canvas.Circle
	built  fyne.CanvasObject
}

// sharedIntro is the line above the table: what is here, and that the
// account question is optional.
const sharedIntro = "Files anyone on this server made public. No account is needed to download them; to share one of yours, select it on the Files tab and press Share."

func newSharedView(u *ui) *sharedView {
	view := &sharedView{ui: u, selected: -1, status: newProse("")}

	view.table = widget.NewTable(
		func() (int, int) { return len(view.items) + 1, 3 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label, ok := cell.(*widget.Label)
			if !ok {
				return
			}
			if id.Row == 0 {
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.SetText([]string{"Name", "Version", "Shared by"}[id.Col])
				return
			}
			label.TextStyle = fyne.TextStyle{}

			index := id.Row - 1
			if index < 0 || index >= len(view.items) {
				label.SetText("")
				return
			}
			item := view.items[index]
			switch id.Col {
			case 0:
				label.SetText(sharedFileName(item.File))
			case 1:
				label.SetText(fmt.Sprintf("%d", item.Version))
			case 2:
				label.SetText(sharedBy(item))
			}
		},
	)
	view.table.SetColumnWidth(0, 360)
	view.table.SetColumnWidth(1, 80)
	view.table.SetColumnWidth(2, 260)

	view.table.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			view.table.UnselectAll()
			view.selected = -1
		} else {
			view.selected = id.Row - 1
		}
		view.updateActions()
	}

	view.refresh = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), view.reload)
	view.download = widget.NewButtonWithIcon("Download…", theme.DownloadIcon(), view.downloadSelected)
	view.download.Importance = widget.HighImportance
	view.updateActions()

	return view
}

// sharedFileName is the name a file was uploaded under. The extension is read
// without a leading dot: rows replaced before #358 stored ".txt".
func sharedFileName(file apiclient.File) string {
	ext := strings.TrimLeft(file.Ext, ".")
	if ext == "" {
		return file.Name
	}
	return file.Name + "." + ext
}

// sharedBy names the owner when the API sent the address — only to a caller
// with a session, so a visitor never gets the server's list of accounts.
func sharedBy(file apiclient.SharedFile) string {
	if file.OwnerEmail != "" {
		return file.OwnerEmail
	}
	return "someone on this server"
}

func (v *sharedView) content() fyne.CanvasObject {
	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	toolbar := container.NewHBox(layout.NewSpacer(), v.download, v.refresh)
	top := container.NewVBox(container.NewPadded(newProse(sharedIntro)), toolbar, widget.NewSeparator())

	v.built = container.NewBorder(top, statusBar(v.dot, v.status), nil, nil, v.table)
	return v.built
}

func (v *sharedView) setStatus(text string, kind statusKind) {
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

func (v *sharedView) selectedFile() (apiclient.SharedFile, bool) {
	if v.selected < 0 || v.selected >= len(v.items) {
		return apiclient.SharedFile{}, false
	}
	return v.items[v.selected], true
}

func (v *sharedView) updateActions() {
	if v.download == nil {
		return
	}
	if _, ok := v.selectedFile(); ok {
		v.download.Enable()
	} else {
		v.download.Disable()
	}
}

// reload lists the shared files. The client is read on the UI side, as the
// other tabs do: a connect replaces u.client while a request may be running.
func (v *sharedView) reload() {
	client := v.ui.client
	address := client.BaseURL

	v.setStatus("Loading the shared files from "+address+"…", statusPending)

	v.ui.background(func() {
		items, err := client.ListSharedFiles()

		fyne.Do(func() {
			v.selected = -1
			v.table.UnselectAll()

			if err != nil {
				v.items = nil
				v.table.Refresh()
				v.updateActions()

				// A token the API refuses is the same story as on the other
				// tabs: say so, and put the login card on the Files tab.
				if apiErrorStatus(err) == http.StatusUnauthorized {
					v.setStatus(sessionEnded, statusError)
					v.ui.forgetSession(sessionEnded)
					return
				}
				v.setStatus("Cannot load the shared files from "+address+".", statusError)
				return
			}

			v.items = items
			v.loaded = true
			v.table.Refresh()
			v.updateActions()

			if len(items) == 0 {
				v.setStatus("Nothing is shared on "+address+" yet. A file becomes shared when its owner presses Share on the Files tab.", statusOK)
				return
			}
			v.setStatus(fmt.Sprintf("%d shared file(s) on %s.", len(items), address), statusOK)
		})
	})
}

// ensureLoaded fetches the list the first time the tab is opened.
func (v *sharedView) ensureLoaded() {
	if !v.loaded {
		v.reload()
	}
}

// forget drops the list: a new server is a new list, and a sign-in or out
// changes whether the owners are named.
func (v *sharedView) forget() {
	v.loaded = false
	v.items = nil
	v.selected = -1
	if v.table != nil {
		v.table.UnselectAll()
		v.table.Refresh()
	}
	v.updateActions()
}

// downloadSelected saves the selected file where the person chooses, with its
// own name already filled in.
func (v *sharedView) downloadSelected() {
	file, ok := v.selectedFile()
	if !ok {
		v.setStatus("Select a file first.", statusWarn)
		return
	}
	name := sharedFileName(file.File)

	save := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			v.setStatus("The destination could not be chosen: "+err.Error(), statusError)
			return
		}
		if writer == nil {
			return
		}
		path := writer.URI().Path()
		writer.Close()

		v.setStatus("Downloading "+name+"…", statusPending)
		client := v.ui.client
		v.ui.background(func() {
			saved, err := client.DownloadFile(file.ID, path)
			fyne.Do(func() {
				if err != nil {
					v.setStatus("The download failed: "+messageOf(err), statusError)
					return
				}
				v.setStatus("Saved to "+saved+".", statusOK)
			})
		})
	}, v.ui.window)
	save.SetFileName(name)
	save.Show()
}
