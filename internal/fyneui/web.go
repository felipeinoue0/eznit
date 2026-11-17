//go:build fyne

package fyneui

import (
	"fmt"
	"net/http"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/domain/templates"
)

// webView is the Web tab (#373): everything anyone on the server made public —
// templates and files in one list. It needs no session: a person without an
// account can still download what was shared. With one, each shared file also
// names its owner.
type webView struct {
	ui *ui

	templates []templates.TemplateData
	files     []entry
	loaded    bool

	show    *widget.RadioGroup
	list    *entryList
	pages   *pager
	refresh *widget.Button

	// bar is the API address, without a session to end: that is My Account's.
	bar *addressBar

	status *prose
	dot    *canvas.Circle
	built  fyne.CanvasObject
}

// webIntro is the line above the list: what is here, and that an account is
// optional.
const webIntro = "What anyone on this server made public. No account is needed to download it; to share your own, open it on My Account."

func newWebView(u *ui) *webView {
	view := &webView{ui: u, status: newProse("")}

	view.list = newEntryList(view.open, view.download)
	view.refresh = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), view.reload)
	view.show = newShowFilter(func(string) { view.fill() })

	return view
}

func (v *webView) content() fyne.CanvasObject {
	v.bar = newAddressBar(v.ui, v.setStatus, false)
	v.bar.afterConnect = v.reload

	v.dot = canvas.NewCircle(statusColour(v.ui.app.Settings().ThemeVariant(), statusIdle))

	filters := container.NewHBox(muted("Show"), v.show, layout.NewSpacer(), v.refresh)
	top := container.NewVBox(container.NewPadded(newProse(webIntro)), container.NewPadded(filters), widget.NewSeparator())
	v.pages = newPager(container.NewBorder(top, nil, nil, nil, v.list.list))

	head := container.NewVBox(v.bar.content(), widget.NewSeparator())
	v.built = container.NewBorder(head, statusBar(v.dot, v.status), nil, nil, v.pages.stack)

	return v.built
}

func (v *webView) setStatus(text string, kind statusKind) {
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

// fill puts the loaded rows through the filter.
func (v *webView) fill() {
	show := showAll
	if v.show != nil {
		show = v.show.Selected
	}

	v.list.set(filterEntries(show, v.templates, v.files, func(item templates.TemplateData) entry {
		return templateEntry(item, false)
	}))
}

// reload lists the public templates and the shared files. The client is read
// on the UI side: a connect replaces u.client while a request may be running.
func (v *webView) reload() {
	client := v.ui.client
	address := client.BaseURL

	if v.pages != nil {
		v.pages.back()
	}
	v.setStatus("Loading what is public on "+address+"…", statusPending)

	v.ui.background(func() {
		items, templatesErr := client.ListTemplates(false, "")
		shared, filesErr := client.ListSharedFiles()

		fyne.Do(func() {
			if v.ui.client != client {
				return
			}

			// A token the API refuses is the same story as on My Account: the
			// login card goes there, and this list loads again without the
			// token — what is public needs none. Without a token there was no
			// session to end, so a refusal is only an error; that also stops
			// the reload from ending a session that is already gone.
			if apiErrorStatus(templatesErr) == http.StatusUnauthorized || apiErrorStatus(filesErr) == http.StatusUnauthorized {
				v.forget()

				if client.Token != "" {
					v.ui.forgetSession(sessionEnded)

					// Last, so nothing in this answer touches a widget
					// while the new request's answer can.
					v.reload()
					return
				}

				v.setStatus(address+" refused to list what is public without an account.", statusError)
				return
			}

			if templatesErr != nil && filesErr != nil {
				v.forget()
				v.setStatus("Cannot load what is public on "+address+".", statusError)
				return
			}

			// The catalogue lists the caller's private templates too; this
			// tab is what everyone can see.
			v.templates = make([]templates.TemplateData, 0, len(items))
			for _, item := range items {
				if item.IsPublic {
					v.templates = append(v.templates, item)
				}
			}

			v.files = make([]entry, 0, len(shared))
			for _, file := range shared {
				v.files = append(v.files, sharedFileEntry(file))
			}

			v.loaded = true
			v.fill()

			switch {
			case templatesErr != nil:
				v.setStatus("The templates could not be loaded from "+address+": "+messageOf(templatesErr), statusError)
			case filesErr != nil:
				v.setStatus("The shared files could not be loaded from "+address+": "+messageOf(filesErr), statusError)
			case len(v.templates)+len(v.files) == 0:
				v.setStatus("Nothing is public on "+address+" yet. Publish a template or share a file from My Account.", statusOK)
			default:
				v.setStatus(fmt.Sprintf("%d template(s) and %d file(s) on %s.", len(v.templates), len(v.files), address), statusOK)
			}
		})
	})
}

// ensureLoaded fetches the list the first time the tab is shown.
func (v *webView) ensureLoaded() {
	if !v.loaded {
		v.reload()
	}
}

// forget drops the list: a new server is a new list, and a sign-in or out
// changes whether the owners are named. The next time the tab is shown it
// loads again.
func (v *webView) forget() {
	v.loaded = false
	v.templates = nil
	v.files = nil
	v.list.set(nil)

	if v.pages != nil {
		v.pages.back()
	}
}

// open shows one row's page.
func (v *webView) open(item entry) {
	if item.kind == entryTemplate {
		openTemplate(v.ui, v.pages, item.template, v.setStatus)
		return
	}

	download := widget.NewButtonWithIcon("Download…", theme.DownloadIcon(), func() {
		downloadFile(v.ui, item.file, v.setStatus)
	})
	download.Importance = widget.HighImportance

	body := container.NewVScroll(container.NewPadded(fileFacts(item.file, sharedBy(apiclient.SharedFile{File: item.file, OwnerEmail: item.owner}))))
	v.pages.push(detailPage(item.name, v.pages.back, body, download))
}

// download is a row's own Download button.
func (v *webView) download(item entry) {
	if item.kind == entryTemplate {
		downloadTemplate(v.ui, item.template, v.setStatus)
		return
	}

	downloadFile(v.ui, item.file, v.setStatus)
}
