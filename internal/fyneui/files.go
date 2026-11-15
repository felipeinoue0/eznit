//go:build fyne

package fyneui

import (
	"fmt"
	"net/http"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// filesView is the caller's library: list, upload, download, delete and share,
// on one server — the one the header names.
type filesView struct {
	ui       *ui
	table    *widget.Table
	files    []apiclient.File
	selected int
	status   *prose

	// built is the tab's outer container, so a status or a selection that
	// changes the height of a wrapped line can lay the tab out again.
	built fyne.CanvasObject

	// ready is set by the first successful listing. Actions that need the
	// server stay off until then, so a stale or wrong address cannot look like
	// an empty library.
	ready bool

	// bar is the API address: the same component the Templates tab has.
	bar *addressBar

	dot *canvas.Circle

	upload   *widget.Button
	download *widget.Button
	remove   *widget.Button
	share    *widget.Button

	// The table's cells are selectable, so the text of a row can be copied.
	// A selectable cell would otherwise take the tap that selects the row, so
	// each cell carries a transparent tap layer above its label; the detail
	// line under the table still describes the selection in one sentence.
	detail      *prose
	copyName    *widget.Button
	copyDetails *widget.Button
}

func newFilesView(u *ui) *filesView {
	view := &filesView{
		ui:       u,
		selected: -1,
		status:   newProse(""),
	}

	view.table = widget.NewTable(
		func() (int, int) { return len(view.files) + 1, 3 },
		func() fyne.CanvasObject {
			// A selectable label for the value, and a transparent layer that
			// receives the tap: the label's selection would otherwise swallow
			// the tap that selects the row. A drag still reaches the label.
			label := widget.NewLabel("")
			label.Selectable = true

			return container.NewStack(label, newTapTarget(nil))
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			stack, ok := cell.(*fyne.Container)
			if !ok || len(stack.Objects) != 2 {
				return
			}

			label, ok := stack.Objects[0].(*widget.Label)
			tap, tappable := stack.Objects[1].(*tapTarget)
			if !ok || !tappable {
				return
			}

			// The layer is what a click lands on, so it is what selects the
			// cell — including the header row, which OnSelected unselects.
			tap.onTap = func() { view.table.Select(id) }

			if id.Row == 0 {
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.SetText([]string{"Name", "Version", "Visibility"}[id.Col])
				alignFileColumn(label, id.Col)

				return
			}

			label.TextStyle = fyne.TextStyle{}
			alignFileColumn(label, id.Col)

			index := id.Row - 1
			if index < 0 || index >= len(view.files) {
				label.SetText("")
				return
			}

			file := view.files[index]
			switch id.Col {
			case 0:
				label.SetText(file.Name)
			case 1:
				label.SetText(fmt.Sprintf("%d", file.Version))
			case 2:
				if file.IsPublic {
					label.SetText("shared")
				} else {
					label.SetText("private")
				}
			}
		},
	)

	view.table.SetColumnWidth(0, 380)
	view.table.SetColumnWidth(1, 90)
	view.table.SetColumnWidth(2, 110)

	view.table.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			view.table.UnselectAll()
			view.selected = -1
			view.updateActions()
			return
		}

		view.selected = id.Row - 1
		view.updateActions()
	}

	view.upload = widget.NewButtonWithIcon("Upload", theme.ContentAddIcon(), view.uploadFile)
	view.upload.Importance = widget.HighImportance
	view.download = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), view.downloadSelected)
	view.remove = widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), view.deleteSelected)
	view.remove.Importance = widget.DangerImportance
	view.share = widget.NewButton("Share", view.toggleShare)

	view.updateActions()

	return view
}

// alignFileColumn keeps a numeric column at its right edge: a version is a
// number, and a column of numbers reads as one when the digits line up.
func alignFileColumn(label *widget.Label, column int) {
	label.Alignment = fyne.TextAlignLeading

	if column == 1 {
		label.Alignment = fyne.TextAlignTrailing
	}
}

// header is the address bar. It is rebuilt with the content, so a change shows
// up without a restart. The session belongs here, so this bar carries Sign out
// and the Templates one does not.
func (v *filesView) header() fyne.CanvasObject {
	v.bar = newAddressBar(v.ui, v.setStatus, true)

	return container.NewVBox(v.bar.content(), widget.NewSeparator())
}

func (v *filesView) content() fyne.CanvasObject {
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), v.refresh)

	toolbar := container.NewHBox(refresh, v.upload, widget.NewSeparator(), v.download, v.remove, v.share)
	top := container.NewVBox(toolbar, widget.NewSeparator())

	v.detail = newProse("")
	v.copyName = widget.NewButton("Copy name", v.copySelectedName)
	v.copyDetails = widget.NewButton("Copy details", v.copySelectedDetails)

	detailRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(v.copyName, v.copyDetails), v.detail)

	// The dot lives in the status bar, beside the message it colours, so it is
	// created before the bar that holds it.
	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	footer := container.NewVBox(container.NewPadded(detailRow), statusBar(v.dot, v.status))
	body := container.NewBorder(top, footer, nil, nil, v.table)

	// A rebuilt view has not listed anything yet.
	v.ready = false
	v.updateActions()

	v.built = container.NewBorder(v.header(), nil, nil, nil, body)

	return v.built
}

// setStatus writes the one line under the table, colours the dot with it, and
// colours the words: a red dot and grey text is half an error message.
func (v *filesView) setStatus(text string, kind statusKind) {
	if v.status == nil {
		return
	}

	v.status.Importance = statusImportance(kind)
	v.status.SetText(text)

	if v.dot != nil {
		v.dot.FillColor = statusColour(v.ui.app.Settings().ThemeVariant(), kind)
		v.dot.Refresh()
	}

	// A longer message is a taller bar, and the row it sits in was measured
	// before the text arrived.
	relayout(v.built)
}

func (v *filesView) selectedFile() (apiclient.File, bool) {
	if v.selected < 0 || v.selected >= len(v.files) {
		return apiclient.File{}, false
	}

	return v.files[v.selected], true
}

func (v *filesView) updateActions() {
	if v.upload != nil {
		if v.ready {
			v.upload.Enable()
		} else {
			v.upload.Disable()
		}
	}

	file, ok := v.selectedFile()

	if v.detail != nil {
		if !ok {
			v.detail.SetText("Select a file to see its details here.")
		} else {
			v.detail.SetText(fileDescription(file))
		}
	}

	if v.copyName != nil {
		for _, button := range []*widget.Button{v.copyName, v.copyDetails} {
			if ok {
				button.Enable()
			} else {
				button.Disable()
			}
		}
	}

	if !ok || !v.ready {
		v.download.Disable()
		v.remove.Disable()
		v.share.Disable()
		v.share.SetText("Share")
		relayout(v.built)

		return
	}

	v.download.Enable()
	v.remove.Enable()
	v.share.Enable()

	if file.IsPublic {
		v.share.SetText("Unshare")
	} else {
		v.share.SetText("Share")
	}

	// The description can wrap to a second line, which is taller than the row
	// the footer was measured for.
	relayout(v.built)
}

// fileDescription is the line under the table: what the row is, in words a
// person can read and copy.
func fileDescription(file apiclient.File) string {
	visibility := "private"
	if file.IsPublic {
		visibility = "shared"
	}

	return fmt.Sprintf("%s · version %d · %s · id %s", file.Name, file.Version, visibility, file.ID)
}

// fileRecord is the same thing as one tab-separated line, for a spreadsheet or
// a script.
func fileRecord(file apiclient.File) string {
	visibility := "private"
	if file.IsPublic {
		visibility = "shared"
	}

	return fmt.Sprintf("%s\t%d\t%s\t%s", file.Name, file.Version, visibility, file.ID)
}

func (v *filesView) copySelectedName() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	if !v.ui.copyToClipboard(file.Name) {
		v.setStatus("This window has no clipboard available — the name could not be copied.", statusError)
		return
	}

	v.setStatus("Copied the name of "+file.Name+".", statusOK)
}

func (v *filesView) copySelectedDetails() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	if !v.ui.copyToClipboard(fileRecord(file)) {
		v.setStatus("This window has no clipboard available — the details could not be copied.", statusError)
		return
	}

	v.setStatus("Copied the details of "+file.Name+".", statusOK)
}

// The request runs off the UI goroutine, but the window counts it, so a
// test can wait for the answer instead of polling for it.
//
// The client is captured before the request: a connect can replace
// u.client while a listing is in flight, and an answer that arrives for the
// previous instance must not be applied to the new one — a 401 from the old
// server would otherwise end a session the new one still honours.
func (v *filesView) refresh() {
	client := v.ui.client
	address := client.BaseURL

	v.setStatus("Loading your files from "+address+"…", statusPending)

	v.ui.background(func() {
		files, err := client.ListFiles()

		fyne.Do(func() {
			if v.ui.client != client {
				return
			}

			if err != nil {
				v.failedToLoad(address, err)
				return
			}

			v.ready = true
			v.files = files
			v.selected = -1
			v.table.UnselectAll()
			v.table.Refresh()
			v.updateActions()

			if len(files) == 0 {
				v.setStatus("No files yet. Upload one with the button above.", statusOK)
				return
			}

			v.setStatus(fmt.Sprintf("%d file(s) on %s.", len(files), address), statusOK)
		})
	})
}

// failedToLoad routes a listing that did not arrive. The two failures are not
// the same answer: a refused token means the API judged the session and said
// no, so the window forgets it and asks again; an unreachable server has said
// nothing about the token, so the session stays and the panel says so (#335).
//
// What neither does is leave the library on screen: a table with no rows and a
// grey line under it reads as "signed in, nothing here", which is the opposite
// of the truth.
func (v *filesView) failedToLoad(address string, err error) {
	if apiErrorStatus(err) == http.StatusUnauthorized {
		v.ui.forgetSession(sessionEnded)

		return
	}

	v.ui.showUnreachable(address, err)
}

// unreachableContent is the tab when the server did not answer: the same
// address bar as the library — so another address can be typed and connected —
// the panel that explains the state, and a status line of its own.
//
// The status line is the panel's, not the library's: the library's line sits in
// a container the tab no longer shows, so a Test or a refused address would
// answer into a label nobody can see.
func (v *filesView) unreachableContent(address string, cause error) fyne.CanvasObject {
	panel := newUnreachableView(v.ui, address, cause)

	v.bar = newAddressBar(v.ui, panel.setStatus, true)
	header := container.NewVBox(v.bar.content(), widget.NewSeparator())

	return container.NewBorder(header, statusBar(panel.dot, panel.status), nil, nil, panel.content())
}

// forget drops the rows. They belong to the session that just ended, and the
// next one may be a different account: a stale table would show the last
// account's files until the listing returns.
func (v *filesView) forget() {
	v.files = nil
	v.selected = -1
	v.ready = false

	if v.table == nil {
		return
	}

	v.table.UnselectAll()
	v.table.Refresh()
	v.updateActions()
}

func (v *filesView) uploadFile() {
	if !v.ready {
		v.setStatus("There is no server to upload to yet — check the address above.", statusError)
		return
	}

	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			v.setStatus("The file could not be chosen: "+err.Error(), statusError)
			return
		}
		if reader == nil {
			return
		}

		path := reader.URI().Path()
		reader.Close()

		v.setStatus("Uploading "+path+"…", statusPending)

		client := v.ui.client

		v.ui.background(func() {
			err := client.UploadFile(path)

			fyne.Do(func() {
				if err != nil {
					v.setStatus("The upload failed: "+messageOf(err), statusError)
					return
				}

				v.refresh()
			})
		})
	}, v.ui.window)
}

func (v *filesView) downloadSelected() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			v.setStatus("The destination could not be chosen: "+err.Error(), statusError)
			return
		}
		if writer == nil {
			return
		}

		path := writer.URI().Path()
		writer.Close()

		v.setStatus("Downloading "+file.Name+"…", statusPending)

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
}

func (v *filesView) deleteSelected() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	dialog.ShowConfirm("Delete "+file.Name,
		"This removes the file from the server. It cannot be undone.",
		func(confirmed bool) {
			if !confirmed {
				return
			}

			v.setStatus("Deleting "+file.Name+"…", statusPending)

			client := v.ui.client

			v.ui.background(func() {
				err := client.DeleteFile(file.ID)

				fyne.Do(func() {
					if err != nil {
						v.setStatus("The delete failed: "+messageOf(err), statusError)
						return
					}

					v.refresh()
				})
			})
		}, v.ui.window)
}

func (v *filesView) toggleShare() {
	file, ok := v.selectedFile()
	if !ok {
		return
	}

	apply := func(public bool) {
		v.setStatus("Updating "+file.Name+"…", statusPending)

		client := v.ui.client

		v.ui.background(func() {
			err := client.SetFilePublic(file.ID, public)

			fyne.Do(func() {
				if err != nil {
					v.setStatus("The change failed: "+messageOf(err), statusError)
					return
				}

				v.refresh()
			})
		})
	}

	if file.IsPublic {
		apply(false)
		return
	}

	dialog.ShowConfirm("Share "+file.Name,
		"Anyone on this instance will be able to download it.\n\nThis shares one file. It does not publish a template: a template is a whole folder, published with `eznit template publish`.",
		func(confirmed bool) {
			if confirmed {
				apply(true)
			}
		}, v.ui.window)
}
