//go:build fyne

package fyneui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/domain/templates"
)

// The Web and My Account tabs share one shape (#373): a filter, a list of whole
// rows with a Download button on the right, and a detail page with Back. A
// table selected one cell at a time, and a cell said nothing the row did not.

// The filter's three choices. "All" lists templates first, then files.
const (
	showAll       = "All"
	showTemplates = "Templates"
	showFiles     = "Files"
)

// newShowFilter is the Templates / Files / All choice, laid out in one line.
func newShowFilter(changed func(string)) *widget.RadioGroup {
	group := widget.NewRadioGroup([]string{showAll, showTemplates, showFiles}, nil)
	group.Horizontal = true
	group.Required = true
	group.SetSelected(showAll)
	group.OnChanged = changed

	return group
}

type entryKind int

const (
	entryTemplate entryKind = iota
	entryFile
)

// entry is one row: a template or a file, the one line that describes it, and
// what the row's buttons need.
type entry struct {
	kind entryKind
	name string
	info string

	template templates.TemplateData
	file     apiclient.File
	// owner names who shared a file, when the API said.
	owner string
}

// fileName is the name a file was uploaded under. The extension is read
// without a leading dot: rows replaced before #358 stored ".txt".
func fileName(file apiclient.File) string {
	ext := strings.TrimLeft(file.Ext, ".")
	if ext == "" {
		return file.Name
	}
	return file.Name + "." + ext
}

func visibilityOf(public bool) string {
	if public {
		return "shared"
	}
	return "private"
}

// templateEntry is a template row. withVisibility adds "private"/"shared",
// which the account tab needs and the Web tab — public only — does not.
func templateEntry(item templates.TemplateData, withVisibility bool) entry {
	info := fmt.Sprintf("Template · %s · version %d · %d file(s)", item.Kind, item.Version, item.FileCount)
	if withVisibility {
		info += " · " + visibilityOf(item.IsPublic)
	}

	return entry{kind: entryTemplate, name: item.Name, info: info, template: item}
}

// ownFileEntry is a row of the account's own files.
func ownFileEntry(file apiclient.File) entry {
	return entry{
		kind: entryFile,
		name: fileName(file),
		info: fmt.Sprintf("File · version %d · %s", file.Version, visibilityOf(file.IsPublic)),
		file: file,
	}
}

// sharedFileEntry is a row of the Web tab's shared files.
func sharedFileEntry(file apiclient.SharedFile) entry {
	return entry{
		kind:  entryFile,
		name:  fileName(file.File),
		info:  fmt.Sprintf("File · version %d · shared by %s", file.Version, sharedBy(file)),
		file:  file.File,
		owner: file.OwnerEmail,
	}
}

// sharedBy names the owner when the API sent the address — only to a caller
// with a session, so a visitor never gets the server's list of accounts.
func sharedBy(file apiclient.SharedFile) string {
	if file.OwnerEmail != "" {
		return file.OwnerEmail
	}
	return "someone on this server"
}

// filterEntries keeps what the filter asks for, templates first.
func filterEntries(show string, items []templates.TemplateData, files []entry, toEntry func(templates.TemplateData) entry) []entry {
	out := make([]entry, 0, len(items)+len(files))

	if show != showFiles {
		for _, item := range items {
			out = append(out, toEntry(item))
		}
	}
	if show != showTemplates {
		out = append(out, files...)
	}

	return out
}

// entryRow is one row of the list: the kind's icon, the name over one muted
// line, and Download at the right edge. The whole row is the selection.
type entryRow struct {
	widget.BaseWidget

	icon     *widget.Icon
	name     *widget.Label
	info     *widget.Label
	download *widget.Button
}

func newEntryRow() *entryRow {
	row := &entryRow{
		icon:     widget.NewIcon(theme.FileIcon()),
		name:     widget.NewLabel(""),
		info:     widget.NewLabel(""),
		download: widget.NewButtonWithIcon("Download", theme.DownloadIcon(), nil),
	}

	// A long name ends in an ellipsis instead of pushing the button out.
	row.name.TextStyle = fyne.TextStyle{Bold: true}
	row.name.Truncation = fyne.TextTruncateEllipsis
	// The info line is the row's content, not a caption: it keeps the
	// regular colour, because the low-importance grey all but disappears on
	// the light theme's background.
	row.info.Truncation = fyne.TextTruncateEllipsis

	row.ExtendBaseWidget(row)

	return row
}

func (r *entryRow) CreateRenderer() fyne.WidgetRenderer {
	text := container.NewVBox(r.name, r.info)
	button := container.NewCenter(r.download)

	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, r.icon, button, text))
}

func (r *entryRow) set(item entry, download func()) {
	if item.kind == entryTemplate {
		r.icon.SetResource(theme.DocumentIcon())
	} else {
		r.icon.SetResource(theme.FileIcon())
	}

	r.name.SetText(item.name)
	r.info.SetText(item.info)
	r.download.OnTapped = download
}

// entryList is the list both tabs show. Tapping a row opens it; the row does
// not stay selected, so tapping it again after Back opens it again.
type entryList struct {
	items []entry
	list  *widget.List

	open     func(entry)
	download func(entry)
}

func newEntryList(open, download func(entry)) *entryList {
	l := &entryList{open: open, download: download}

	l.list = widget.NewList(
		func() int { return len(l.items) },
		func() fyne.CanvasObject { return newEntryRow() },
		func(id widget.ListItemID, object fyne.CanvasObject) {
			row, ok := object.(*entryRow)
			if !ok || id < 0 || id >= len(l.items) {
				return
			}

			item := l.items[id]
			row.set(item, func() { l.download(item) })
		},
	)

	l.list.OnSelected = func(id widget.ListItemID) {
		l.list.Unselect(id)

		if id >= 0 && id < len(l.items) {
			l.open(l.items[id])
		}
	}

	return l
}

func (l *entryList) set(items []entry) {
	l.items = items
	l.list.UnselectAll()
	l.list.Refresh()
}

// pager holds one page at a time: the list, or the detail it opened.
type pager struct {
	home  fyne.CanvasObject
	stack *fyne.Container
}

func newPager(home fyne.CanvasObject) *pager {
	return &pager{home: home, stack: container.NewStack(home)}
}

func (p *pager) push(page fyne.CanvasObject) {
	p.stack.Objects = []fyne.CanvasObject{page}
	p.stack.Refresh()
}

func (p *pager) back() {
	p.push(p.home)
}

// atHome reports whether the list is the page on show.
func (p *pager) atHome() bool {
	return len(p.stack.Objects) == 1 && p.stack.Objects[0] == p.home
}

// detailPage is one opened row: Back and the name at the top, the body in the
// middle, and the row's actions along the bottom edge.
func detailPage(title string, back func(), body fyne.CanvasObject, actions ...fyne.CanvasObject) fyne.CanvasObject {
	backButton := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), back)

	heading := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	heading.Truncation = fyne.TextTruncateEllipsis
	heading.Selectable = true

	top := container.NewVBox(container.NewBorder(nil, nil, backButton, nil, heading), widget.NewSeparator())

	var bottom fyne.CanvasObject
	if len(actions) > 0 {
		bottom = container.NewPadded(container.NewBorder(nil, nil, nil, container.NewHBox(actions...), nil))
	}

	return container.NewBorder(top, bottom, nil, nil, body)
}

// fileFacts is a file's detail body: one fact per line, every one selectable.
func fileFacts(file apiclient.File, owner string) *prose {
	lines := []string{
		"Name: " + fileName(file),
		fmt.Sprintf("Version: %d", file.Version),
		"Visibility: " + visibilityOf(file.IsPublic),
	}
	if owner != "" {
		lines = append(lines, "Shared by: "+owner)
	}
	lines = append(lines, "ID: "+file.ID)

	return newProse(strings.Join(lines, "\n"))
}

// downloadFile saves a file where the person chooses, with its own name filled
// in.
func downloadFile(u *ui, file apiclient.File, report func(string, statusKind)) {
	name := fileName(file)

	save := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			report("The destination could not be chosen: "+err.Error(), statusError)
			return
		}
		if writer == nil {
			return
		}
		path := writer.URI().Path()
		writer.Close()

		report("Downloading "+name+"…", statusPending)

		client := u.client
		u.background(func() {
			saved, err := client.DownloadFile(file.ID, path)

			fyne.Do(func() {
				if err != nil {
					report("The download failed: "+messageOf(err), statusError)
					return
				}
				report("Saved to "+saved+".", statusOK)
			})
		})
	}, u.window)
	save.SetFileName(name)
	save.Show()
}

// downloadTemplate is a template row's Download: the template fetched whole
// and saved as the bundle `eznit template apply --from-file` reads.
func downloadTemplate(u *ui, item templates.TemplateData, report func(string, statusKind)) {
	report("Loading "+item.Name+"…", statusPending)

	client := u.client
	u.background(func() {
		detail, err := client.GetTemplate(item.ID.String())

		fyne.Do(func() {
			if err != nil {
				report("The template could not be loaded: "+messageOf(err), statusError)
				return
			}
			report("Choose where to save "+item.Name+".", statusIdle)
			saveBundle(u, detail, report)
		})
	})
}

// saveBundle writes a template as the offline bundle the CLI reads, so the
// window and `template export` produce the same file.
func saveBundle(u *ui, detail *templates.TemplateDetail, report func(string, statusKind)) {
	save := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			report("The destination could not be chosen: "+err.Error(), statusError)
			return
		}
		if writer == nil {
			return
		}

		path := writer.URI().Path()

		if err := exportBundle(writer, detail); err != nil {
			writer.Close()
			report("The bundle could not be written: "+err.Error(), statusError)
			return
		}

		if err := writer.Close(); err != nil {
			report("The bundle was written but could not be closed: "+err.Error(), statusError)
			return
		}

		report(fmt.Sprintf("Saved %s. Apply it with `eznit template apply --from-file %s`.", path, path), statusOK)
	}, u.window)

	save.SetFileName(bundleFilename(detail))
	save.SetFilter(storage.NewExtensionFileFilter([]string{bundleExtension}))
	save.Show()
}
