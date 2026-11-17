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
	"github.com/fiwon123/eznit/internal/domain/templates"
)

// accountView is My Account (#373): the signed-in account's own files and
// templates in one list, the same rows as the Web tab, and the two ways to add
// one — upload a file, publish a template folder. Opening a row gives it a page
// of its own, with Back.
//
// Signed out, the tab holds the login card instead; see ui.showLogin.
type accountView struct {
	ui *ui

	files     []apiclient.File
	templates []templates.TemplateData

	// ready is set by the first successful listing. Actions that need the
	// server stay off until then, so a stale or wrong address cannot look like
	// an empty account.
	ready bool

	// afterLoad is said instead of the count when the next load succeeds: a
	// publish reloads the list, and its confirmation must not be replaced.
	afterLoad string

	show    *widget.Select
	list    *entryList
	pages   *pager
	upload  *widget.Button
	publish *widget.Button
	refresh *widget.Button

	status *prose
	dot    *canvas.Circle
	built  fyne.CanvasObject
}

func newAccountView(u *ui) *accountView {
	view := &accountView{ui: u, status: newProse("")}

	view.list = newEntryList(view.open, view.download)
	view.show = newShowFilter(func(string) { view.fill() })

	view.upload = widget.NewButtonWithIcon("Upload file…", theme.ContentAddIcon(), view.uploadFile)
	view.upload.Importance = widget.HighImportance
	view.publish = widget.NewButtonWithIcon("Publish template…", theme.UploadIcon(), view.publishFolder)
	view.refresh = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), view.reload)

	view.updateActions()

	return view
}

// content is the signed-in account (#376): what it holds and how to add to
// it on top, the list, and the status line — inside the page margins. The
// server is chosen on the Settings tab.
func (v *accountView) content() fyne.CanvasObject {
	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	title := container.NewBorder(nil, nil, nil, v.refresh, heading("Your files and templates"))
	filters := container.NewBorder(nil, nil,
		container.NewHBox(widget.NewLabel("Show"), v.show),
		container.NewHBox(v.upload, v.publish))
	top := container.NewVBox(title, filters, widget.NewSeparator())
	v.pages = newPager(container.NewBorder(top, nil, nil, nil, v.list.list))

	// A rebuilt view has not listed anything yet.
	v.ready = false
	v.updateActions()

	v.built = inset(container.NewBorder(nil, statusBar(v.dot, v.status), nil, nil, v.pages.stack))

	return v.built
}

// setStatus writes the one line at the bottom of the tab and colours its dot.
func (v *accountView) setStatus(text string, kind statusKind) {
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

func (v *accountView) updateActions() {
	for _, button := range []*widget.Button{v.upload, v.publish} {
		if v.ready {
			button.Enable()
		} else {
			button.Disable()
		}
	}
}

func (v *accountView) fill() {
	show := showAll
	if v.show != nil {
		show = v.show.Selected
	}

	files := make([]entry, 0, len(v.files))
	for _, file := range v.files {
		files = append(files, ownFileEntry(file))
	}

	v.list.set(filterEntries(show, v.templates, files, func(item templates.TemplateData) entry {
		return templateEntry(item, true)
	}))
}

// reload lists the account's files and templates.
//
// The client is captured before the request: a connect can replace u.client
// while a listing is in flight, and an answer that arrives for the previous
// instance must not be applied to the new one — a 401 from the old server
// would otherwise end a session the new one still honours.
func (v *accountView) reload() {
	client := v.ui.client
	address := client.BaseURL

	v.setStatus("Loading your account from "+address+"…", statusPending)

	v.ui.background(func() {
		files, err := client.ListFiles()

		var (
			items       []templates.TemplateData
			templateErr error
		)
		if err == nil {
			items, templateErr = client.ListTemplates(true, "")
		}

		fyne.Do(func() {
			if v.ui.client != client {
				return
			}

			if err != nil {
				v.failedToLoad(address, err)
				return
			}
			if apiErrorStatus(templateErr) == http.StatusUnauthorized {
				v.failedToLoad(address, templateErr)
				return
			}

			v.ready = true
			v.files = files
			v.templates = items
			v.fill()
			v.updateActions()

			if templateErr != nil {
				v.setStatus("Your templates could not be loaded: "+messageOf(templateErr), statusError)
				return
			}

			if note := v.afterLoad; note != "" {
				v.afterLoad = ""
				v.setStatus(note, statusOK)
				return
			}

			if len(files)+len(items) == 0 {
				v.setStatus("Nothing here yet. Upload a file or publish a template folder with the buttons above.", statusOK)
				return
			}

			v.setStatus(fmt.Sprintf("%d template(s) and %d file(s) on %s.", len(items), len(files), address), statusOK)
		})
	})
}

// failedToLoad routes a listing that did not arrive. The two failures are not
// the same answer: a refused token means the API judged the session and said
// no, so the window forgets it and asks again; an unreachable server has said
// nothing about the token, so the session stays and the panel says so (#335).
//
// What neither does is leave the account on screen: a list with no rows reads
// as "signed in, nothing here", which is the opposite of the truth.
func (v *accountView) failedToLoad(address string, err error) {
	if apiErrorStatus(err) == http.StatusUnauthorized {
		v.ui.forgetSession(sessionEnded)

		return
	}

	v.ui.showUnreachable(address, err)
}

// unreachableContent is the account when the server did not answer: the panel
// that explains the state, and a status line of its own.
func (v *accountView) unreachableContent(address string, cause error) fyne.CanvasObject {
	panel := newUnreachableView(v.ui, address, cause)

	return inset(container.NewBorder(nil, statusBar(panel.dot, panel.status), nil, nil, panel.content()))
}

// forget drops the rows. They belong to the session that just ended, and the
// next one may be a different account.
func (v *accountView) forget() {
	v.files = nil
	v.templates = nil
	v.ready = false
	v.list.set(nil)
	v.updateActions()

	if v.pages != nil {
		v.pages.back()
	}
}

// open shows one row's page.
func (v *accountView) open(item entry) {
	if item.kind == entryTemplate {
		openTemplate(v.ui, v.pages, item.template, v.setStatus, true, func(note string) {
			v.afterLoad = note
			v.reload()
		})
		return
	}

	v.openFile(item.file)
}

// openFile is a file's page: its details with the Visibility dropdown and the
// two copies, a preview, Download at the top and Delete apart at the bottom.
func (v *accountView) openFile(opened apiclient.File) {
	// The page's buttons read the file through one pointer, so a visibility
	// change made on the page is what Copy details copies afterwards.
	file := &opened
	name := fileName(opened)

	copyName := widget.NewButton("Copy name", func() {
		if !v.ui.copyToClipboard(name) {
			v.setStatus("This window has no clipboard available — the name could not be copied.", statusError)
			return
		}
		v.setStatus("Copied the name of "+name+".", statusOK)
	})
	copyDetails := widget.NewButton("Copy details", func() {
		if !v.ui.copyToClipboard(fileRecord(*file)) {
			v.setStatus("This window has no clipboard available — the details could not be copied.", statusError)
			return
		}
		v.setStatus("Copied the details of "+name+".", statusOK)
	})

	visibility := newVisibilitySelect(file.IsPublic, func(public bool, revert func()) {
		v.askFileVisibility(file, public, revert)
	})

	remove := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() { v.deleteFile(*file) })
	remove.Importance = widget.DangerImportance

	download := widget.NewButtonWithIcon("Download…", theme.DownloadIcon(), func() {
		downloadFile(v.ui, *file, v.setStatus)
	})
	download.Importance = widget.HighImportance

	controls := []fyne.CanvasObject{
		widget.NewLabel("Visibility"), visibility,
		widget.NewSeparator(), copyName, copyDetails,
	}

	filePage(v.ui, v.pages, opened, "", controls, []fyne.CanvasObject{download}, remove)
}

// download is a row's own Download button.
func (v *accountView) download(item entry) {
	if item.kind == entryTemplate {
		downloadTemplate(v.ui, item.template, v.setStatus)
		return
	}

	downloadFile(v.ui, item.file, v.setStatus)
}

// fileRecord is the same thing as one tab-separated line, for a spreadsheet or
// a script.
func fileRecord(file apiclient.File) string {
	return fmt.Sprintf("%s\t%d\t%s\t%s", fileName(file), file.Version, visibilityOf(file.IsPublic), file.ID)
}

func (v *accountView) uploadFile() {
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

				v.afterLoad = "Uploaded " + path + "."
				v.reload()
			})
		})
	}, v.ui.window)
}

func (v *accountView) deleteFile(file apiclient.File) {
	name := fileName(file)

	confirm(v.ui, "Delete "+name+"?",
		fmt.Sprintf("This removes %q from the server, with every version of it. It cannot be undone.", name),
		"Delete", func(confirmed bool) {
			if !confirmed {
				return
			}

			v.setStatus("Deleting "+name+"…", statusPending)

			client := v.ui.client

			v.ui.background(func() {
				err := client.DeleteFile(file.ID)

				fyne.Do(func() {
					if err != nil {
						v.setStatus("The delete failed: "+messageOf(err), statusError)
						return
					}

					v.pages.back()
					v.afterLoad = "Deleted " + name + "."
					v.reload()
				})
			})
		})
}

// askFileVisibility confirms a change of visibility before it is sent (#376).
func (v *accountView) askFileVisibility(file *apiclient.File, public bool, revert func()) {
	title, message, action := visibilityQuestion("file", fileName(*file), public)

	confirm(v.ui, title, message, action, func(confirmed bool) {
		if !confirmed {
			revert()
			return
		}
		v.setFileVisibility(file, public, revert)
	})
}

// setFileVisibility sends the change. The page stays: its dropdown already
// shows the new state, and the list behind it reloads. A refusal puts the
// dropdown back.
func (v *accountView) setFileVisibility(file *apiclient.File, public bool, revert func()) {
	name := fileName(*file)
	id := file.ID

	v.setStatus("Changing the visibility of "+name+"…", statusPending)

	client := v.ui.client

	v.ui.background(func() {
		err := client.SetFilePublic(id, public)

		fyne.Do(func() {
			if err != nil {
				revert()
				v.setStatus("The visibility did not change: "+messageOf(err), statusError)
				return
			}

			file.IsPublic = public
			if public {
				v.afterLoad = name + " is public: it is on the Web tab now."
			} else {
				v.afterLoad = name + " is private again."
			}
			v.reload()
		})
	})
}
