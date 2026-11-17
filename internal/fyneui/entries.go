//go:build fyne

package fyneui

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/domain/templates"
)

// The Web and Workspace tabs share one shape (#373): a filter, a list of whole
// rows with a Download button on the right, and a detail page with Back. A
// table selected one cell at a time, and a cell said nothing the row did not.

// The filter's three choices. "All" lists templates first, then files.
const (
	showAll       = "All"
	showTemplates = "Templates"
	showFiles     = "Files"
)

// newShowFilter is the All / Templates / Files choice, as a dropdown (#376).
func newShowFilter(changed func(string)) *widget.Select {
	choice := widget.NewSelect([]string{showAll, showTemplates, showFiles}, nil)
	choice.SetSelected(showAll)
	choice.OnChanged = changed

	return choice
}

// pageInset is the margin every page keeps from the window's edges (#376): a
// button flush against the frame reads as part of the frame.
const pageInset float32 = 12

// inset gives a page its margins.
func inset(content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(pageInset, pageInset, pageInset, pageInset), content)
}

// heading is a section's title: bold, selectable, one line.
func heading(text string) *widget.Label {
	label := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	label.Selectable = true

	return label
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
// which the account tab needs and Explore — public only — does not.
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

// sharedFileEntry is a row of Explore's shared files.
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

// detailPage is one opened row (#376): Back, the name and the primary actions
// along the top, the body in the middle, and anything destructive alone at the
// bottom-left — apart from the action a person came for.
func detailPage(title string, back func(), body fyne.CanvasObject, primary []fyne.CanvasObject, danger ...fyne.CanvasObject) fyne.CanvasObject {
	backButton := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), back)

	name := heading(title)
	name.Truncation = fyne.TextTruncateEllipsis

	top := container.NewVBox(
		container.NewBorder(nil, nil, backButton, container.NewHBox(primary...), name),
		widget.NewSeparator(),
	)

	var bottom fyne.CanvasObject
	if len(danger) > 0 {
		bottom = container.NewVBox(widget.NewSeparator(), container.NewHBox(danger...))
	}

	return container.NewBorder(top, bottom, nil, nil, body)
}

// visibilityChoices are the two states a file or a template can be in, as the
// Visibility dropdowns name them. Private comes first: it is the default.
var visibilityChoices = []string{"Private", "Public"}

func visibilityChoice(public bool) string {
	if public {
		return "Public"
	}
	return "Private"
}

// newVisibilitySelect is the Visibility dropdown of something the account
// owns. A change is only a request: ask decides, and the dropdown goes back
// to what it was unless the change is confirmed and lands.
func newVisibilitySelect(public bool, ask func(public bool, revert func())) *widget.Select {
	var choice *widget.Select
	quiet := false

	choice = widget.NewSelect(visibilityChoices, nil)
	choice.SetSelected(visibilityChoice(public))

	current := choice.Selected
	choice.OnChanged = func(selected string) {
		if quiet || selected == current {
			return
		}

		previous := current
		ask(selected == "Public", func() {
			quiet = true
			choice.SetSelected(previous)
			quiet = false
		})
	}

	return choice
}

// confirmWidth is how wide a confirmation is: its sentence wraps inside it.
const confirmWidth float32 = 460

// confirm asks before something changes, in a dialog whose text can be selected
// and copied (#376): the stock confirmation paints its message as text that
// cannot be.
func confirm(u *ui, title, message, action string, done func(bool)) *dialog.ConfirmDialog {
	body := newProse(message)

	popup := dialog.NewCustomConfirm(title, action, "Cancel", body, done, u.window)
	popup.Show()
	popup.Resize(fyne.NewSize(confirmWidth, popup.MinSize().Height))

	return popup
}

// previewLimit is how much of a file a preview reads: enough to recognise it,
// little enough that a large file does not stall the page.
const previewLimit = 32 << 10

// previewLines caps a preview's height in lines, for the same reason.
const previewLines = 400

// previewText is what a preview shows for content: the text, cut to its first
// lines, or a sentence when the bytes are not text.
func previewText(content []byte, more bool) string {
	if len(content) == 0 {
		return "This file is empty."
	}

	// A cut can land inside a character: up to three bytes of one may hang
	// off the end of a read that stopped early.
	if more {
		for trim := 0; trim < utf8.UTFMax-1 && !utf8.Valid(content); trim++ {
			content = content[:len(content)-1]
		}
	}

	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return "This is a binary file — there is no text to preview."
	}

	text := string(content)
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > previewLines {
		text = strings.Join(lines[:previewLines], "")
		more = true
	}

	if more {
		text = strings.TrimRight(text, "\n") + "\n\n… (the preview stops here; the download has the whole file)"
	}

	return text
}

// newPreview is the box a preview is shown in: monospaced, selectable, and
// scrolling both ways so a long line or a long file stays inside it.
func newPreview(text string) (*widget.Label, fyne.CanvasObject) {
	label := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	label.Selectable = true

	scroll := container.NewScroll(label)
	scroll.SetMinSize(fyne.NewSize(200, 80))

	return label, scroll
}

// fileFacts is a file's details: one fact per line, every one selectable. The
// visibility is a dropdown on the owner's page, so it is a line only on the
// Explore's.
func fileFacts(file apiclient.File, owner string) *prose {
	lines := []string{
		"Name: " + fileName(file),
		fmt.Sprintf("Version: %d", file.Version),
	}
	if owner != "" {
		lines = append(lines, "Visibility: "+visibilityOf(file.IsPublic), "Shared by: "+owner)
	}
	lines = append(lines, "ID: "+file.ID)

	return newProse(strings.Join(lines, "\n"))
}

// filePage is one file's page (#376): its details with any controls the page
// adds under them, and a preview of its start so a person knows what a
// download holds. The preview is read in the background; owner is empty on
// Workspace, where the file is the reader's own. The page is pushed onto
// pages and returned.
func filePage(u *ui, pages *pager, file apiclient.File, owner string, controls []fyne.CanvasObject, primary []fyne.CanvasObject, danger ...fyne.CanvasObject) fyne.CanvasObject {
	details := container.NewVBox(heading("Details"), fileFacts(file, owner))
	if len(controls) > 0 {
		details.Add(container.NewHBox(controls...))
	}

	preview, box := newPreview("Loading a preview…")
	body := container.NewBorder(
		container.NewVBox(details, widget.NewSeparator(), heading("Preview")),
		nil, nil, nil, box)

	page := detailPage(fileName(file), pages.back, body, primary, danger...)

	// Shown before the preview is asked for, so its answer finds the page.
	pages.push(page)

	client := u.client
	u.background(func() {
		content, more, err := client.PreviewFile(file.ID, previewLimit)

		fyne.Do(func() {
			if len(pages.stack.Objects) == 0 || pages.stack.Objects[0] != page {
				return
			}

			if err != nil {
				preview.SetText("No preview: " + messageOf(err))
				return
			}

			preview.SetText(previewText(content, more))
		})
	})

	return page
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
