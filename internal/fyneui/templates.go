//go:build fyne

package fyneui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
)

// bundleExtension is what a template is written to: the file
// `eznit template apply --from-file` reads.
const bundleExtension = ".eznit"

// templatePage is one opened template, on Explore or on Workspace: its
// summary, its files as the folder they write with a preview of the selected
// one, its variables and commands, and the two actions that take it away —
// Save as bundle and Apply. On Workspace it also has a Visibility dropdown.
type templatePage struct {
	ui     *ui
	report func(text string, kind statusKind)

	detail *templates.TemplateDetail

	export *widget.Button
	apply  *widget.Button

	// owned pages carry the Visibility dropdown; changed runs after a change
	// lands, so the list behind the page can follow.
	owned   bool
	changed func(note string)

	// built is the page, so a description that wraps can lay it out again and
	// a late answer can tell whether its page is still the one on show.
	built fyne.CanvasObject

	summary    *prose
	visibility *fyne.Container
	variables  *prose
	commands   *prose

	// tree and preview are the Files tab: the folder on the left, the
	// selected file's text on the right.
	tree    *widget.Tree
	preview *widget.Label
	folder  folderTree

	// parts is the template in three tabs; each tab's label carries its count,
	// so a person can see that a template has variables without opening it.
	parts        *container.AppTabs
	filesTab     *container.TabItem
	variablesTab *container.TabItem
	commandsTab  *container.TabItem
}

func newTemplatePage(u *ui, report func(string, statusKind)) *templatePage {
	page := &templatePage{ui: u, report: report}

	page.export = widget.NewButtonWithIcon("Save as bundle…", theme.DownloadIcon(), page.exportSelected)
	page.apply = widget.NewButtonWithIcon("Apply…", theme.ConfirmIcon(), page.applySelected)
	page.apply.Importance = widget.HighImportance

	page.summary = selectableWrap()
	page.variables = selectableWrap()
	page.commands = selectableWrap()
	page.visibility = container.NewHBox()

	var previewBox fyne.CanvasObject
	page.preview, previewBox = newPreview("Select a file to preview it.")

	page.tree = newFolderTree(nil).widget(func(templateapply.File) {})
	split := container.NewHSplit(page.tree, previewBox)
	split.Offset = 0.35

	page.filesTab = container.NewTabItem("Files (0)", split)
	page.variablesTab = container.NewTabItem("Variables (0)", part(page.variables))
	page.commandsTab = container.NewTabItem("Commands (0)", part(page.commands))
	page.parts = container.NewAppTabs(page.filesTab, page.variablesTab, page.commandsTab)

	page.show(nil)

	return page
}

// content is the page under a Back button: the summary and, for the owner,
// the Visibility dropdown above the three tabs.
func (p *templatePage) content(title string, back func()) fyne.CanvasObject {
	top := container.NewVBox(p.summary, p.visibility)
	body := container.NewBorder(top, nil, nil, nil, p.parts)
	p.built = detailPage(title, back, body, []fyne.CanvasObject{p.export, p.apply})

	return p.built
}

// openTemplate shows a template's page and fetches the whole template into it.
// owned pages — Workspace's — can change the template's visibility; changed
// runs after one does.
func openTemplate(u *ui, pages *pager, item templates.TemplateData, report func(string, statusKind), owned bool, changed func(note string)) *templatePage {
	page := newTemplatePage(u, report)
	page.owned = owned
	page.changed = changed
	pages.push(page.content(item.Name, pages.back))

	id := item.ID.String()
	client := u.client

	report("Loading "+item.Name+"…", statusPending)

	u.background(func() {
		detail, err := client.GetTemplate(id)

		fyne.Do(func() {
			// Back may have been pressed, or another row opened, meanwhile.
			if pages.stack.Objects[0] != page.built {
				return
			}

			if err != nil {
				page.summary.SetText("This template could not be loaded.")
				report("The template could not be loaded: "+messageOf(err), statusError)
				return
			}

			page.show(detail)
			report(fmt.Sprintf("%s — %d file(s), %d command(s), %d variable(s).",
				detail.Name, len(detail.Files), len(detail.Commands), len(detail.Variables)), statusOK)
		})
	})

	return page
}

// part wraps one detail body in its own scroll: a template with forty files
// scrolls inside Files without moving Variables.
func part(body *prose) fyne.CanvasObject {
	return container.NewVScroll(container.NewPadded(body))
}

// selectableWrap is the one shape every value on this page takes: wrapped, so a
// long path can neither widen the window nor paint over its neighbour, and
// selectable, so it can be copied.
func selectableWrap() *prose {
	return newProse("")
}

// show fills the summary and the three tabs; without a template it says the
// page is loading and keeps the actions off.
func (p *templatePage) show(detail *templates.TemplateDetail) {
	p.detail = detail
	p.visibility.Objects = nil

	if detail == nil {
		p.summary.SetText("Loading the template…")
		p.variables.SetText("")
		p.commands.SetText("")
		p.parts.Hide()
		p.export.Disable()
		p.apply.Disable()
		relayout(p.built)

		return
	}

	summary := fmt.Sprintf("%s — %s, version %d — %s — by %s",
		detail.Name, detail.Kind, detail.Version, visibilityOf(detail.IsPublic), detail.OwnerEmail)
	if strings.TrimSpace(detail.Description) != "" {
		summary += "\n\n" + detail.Description
	}

	p.summary.SetText(summary)
	p.variables.SetText(linesOr(detail.Variables, variableLine, "None — this template asks nothing."))
	p.commands.SetText(linesOr(detail.Commands, commandLine, "No commands."))

	// The folder is what Apply writes: the files and the commands at their
	// target paths. A template that cannot be written still shows its own
	// files.
	files, err := templateapply.Files(detail)
	if err != nil {
		files = nil
		for _, file := range detail.Files {
			files = append(files, templateapply.File{Path: file.Path, Kind: file.Kind, Content: file.Content, IsExecutable: file.IsExecutable})
		}
	}

	p.folder = newFolderTree(files)
	p.tree.ChildUIDs = func(id widget.TreeNodeID) []widget.TreeNodeID { return p.folder.children[id] }
	p.tree.IsBranch = func(id widget.TreeNodeID) bool { return p.folder.isFolder(id) }
	p.tree.UpdateNode = p.folder.updateNode
	p.tree.OnSelected = func(id widget.TreeNodeID) {
		if file, ok := p.folder.files[id]; ok {
			p.preview.SetText(templateFilePreview(file))
		}
	}
	p.tree.Refresh()
	p.tree.OpenAllBranches()
	p.preview.SetText("Select a file to preview it.")

	if p.owned {
		p.visibility.Objects = []fyne.CanvasObject{
			widget.NewLabel("Visibility"),
			newVisibilitySelect(detail.IsPublic, p.askVisibility),
		}
	}
	p.visibility.Refresh()

	p.filesTab.Text = fmt.Sprintf("Files (%d)", len(files))
	p.variablesTab.Text = fmt.Sprintf("Variables (%d)", len(detail.Variables))
	p.commandsTab.Text = fmt.Sprintf("Commands (%d)", len(detail.Commands))
	p.parts.Show()
	p.parts.Refresh()

	p.export.Enable()
	p.apply.Enable()

	// A description is prose: it can wrap to more lines than the page was
	// measured for.
	relayout(p.built)
}

// askVisibility confirms a change of visibility before it is sent (#376).
func (p *templatePage) askVisibility(public bool, revert func()) {
	detail := p.detail
	if detail == nil {
		revert()
		return
	}

	title, message, action := visibilityQuestion("template", detail.Name, public)
	confirm(p.ui, title, message, action, func(confirmed bool) {
		if !confirmed {
			revert()
			return
		}
		p.setVisibility(public, revert)
	})
}

// setVisibility sends the change; a refusal puts the dropdown back.
func (p *templatePage) setVisibility(public bool, revert func()) {
	id := p.detail.ID.String()
	name := p.detail.Name
	client := p.ui.client

	p.report("Changing the visibility of "+name+"…", statusPending)

	p.ui.background(func() {
		updated, err := client.SetTemplateVisibility(id, public)

		fyne.Do(func() {
			if err != nil {
				revert()
				p.report("The visibility did not change: "+messageOf(err), statusError)
				return
			}

			note := fmt.Sprintf("%s is %s now (version %d).", name, visibilityOf(updated.IsPublic), updated.Version)
			p.show(updated)
			p.report(note, statusOK)

			// Last: the list's reload runs in the background, and nothing here
			// may touch a widget while its answer can (#333). It says the note
			// again when it lands, instead of a count.
			if p.changed != nil {
				p.changed(note)
			}
		})
	})
}

// visibilityQuestion is the confirmation a change of visibility asks: what
// will happen, to what, in words a person can copy.
func visibilityQuestion(kind, name string, public bool) (title, message, action string) {
	if public {
		return "Make " + name + " public?",
			fmt.Sprintf("Anyone who can reach this server will be able to see and download the %s %q from Explore, without an account.\n\nYou can make it private again later.", kind, name),
			"Make public"
	}

	return "Make " + name + " private?",
		fmt.Sprintf("The %s %q will leave Explore: only you will see it, in your Workspace. Copies people already downloaded are not affected.", kind, name),
		"Make private"
}

// linesOr renders a slice one per line, or the empty state when it has
// nothing, so a section that is empty says what that means instead of holding a
// dash nobody can read.
func linesOr[T any](items []T, line func(T) string, empty string) string {
	if len(items) == 0 {
		return empty
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, "• "+line(item))
	}

	return strings.Join(out, "\n")
}

func fileLine(file templates.TemplateFileData) string {
	line := fmt.Sprintf("%s — %s, %s", file.Path, file.Kind, byteSize(file.Size))
	if file.IsExecutable {
		line += ", executable"
	}

	return line
}

// variableLine is one variable in the detail pane: the key, the question the
// template asks, the default and whether an answer is required. The braces
// belong to the files a template writes; a person reads a key here.
func variableLine(variable templates.TemplateVariableData) string {
	parts := []string{variable.Key}

	if prompt := strings.TrimSpace(variable.Prompt); prompt != "" {
		parts = append(parts, prompt)
	}
	if strings.TrimSpace(variable.DefaultValue) != "" {
		parts = append(parts, "default "+variable.DefaultValue)
	}
	if variable.Required {
		parts = append(parts, "required")
	}

	return strings.Join(parts, " — ")
}

// promptOf is the question a variable asks, or its key when it has no prompt.
func promptOf(variable templates.TemplateVariableData) string {
	if strings.TrimSpace(variable.Prompt) == "" {
		return variable.Key
	}

	return variable.Prompt
}

// variableCaption is the line above one answer box: the prompt, the key it
// fills in when the prompt does not already say it, and the marker for a
// required answer. No braces: the key is a name here.
func variableCaption(variable templates.TemplateVariableData) string {
	caption := variable.Key
	if prompt := strings.TrimSpace(variable.Prompt); prompt != "" && prompt != variable.Key {
		caption = prompt + " (" + variable.Key + ")"
	}

	if variable.Required {
		caption += " *"
	}

	return caption
}

func commandLine(command templates.TemplateCommandData) string {
	return fmt.Sprintf("%s — %s, %s", command.TargetPath, command.Name, byteSize(command.Size))
}

// byteSize is a size a person reads: bytes under a kilobyte, then KiB and MiB.
func byteSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1f MiB", float64(size)/(1024*1024))
	}
}

// exportSelected saves the template on show as the offline bundle.
func (p *templatePage) exportSelected() {
	if p.detail == nil {
		p.report("The template is still loading.", statusWarn)
		return
	}

	saveBundle(p.ui, p.detail, p.report)
}

// exportBundle turns a template into the bundle format and writes it. It is
// the same pair `template export` uses, so the two cannot drift.
func exportBundle(w io.Writer, detail *templates.TemplateDetail) error {
	bundle, err := templates.NewBundle(detail)
	if err != nil {
		return err
	}

	return templates.WriteBundle(w, bundle)
}

// bundleFilename suggests a name: the slug, or the name with its spaces turned
// into dashes, and the extension the CLI reads.
func bundleFilename(detail *templates.TemplateDetail) string {
	base := strings.ToLower(strings.TrimSpace(detail.Slug))
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(detail.Name))
	}

	var kept strings.Builder

	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			kept.WriteRune(r)
		case r == ' ' || r == '.':
			kept.WriteRune('-')
		}
	}

	name := kept.String()
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	name = strings.Trim(name, "-")
	if name == "" {
		name = "template"
	}

	return name + bundleExtension
}

// applySelected writes the selected template into a folder the person picks:
// check what is there, ask the questions the template needs, then write.
//
// The order is the CLI's: the files are checked before anything is asked,
// because a person should not answer what a template wants to know for a write
// that cannot happen.
func (v *templatePage) applySelected() {
	detail := v.detail
	if detail == nil {
		v.report("The template is still loading.", statusWarn)
		return
	}

	files, err := templateapply.Files(detail)
	if err != nil {
		v.report("This template cannot be written: "+err.Error(), statusError)
		return
	}

	use := func(target string) {
		existing, err := templateapply.Existing(target, files)
		if err != nil {
			v.report("This template cannot be written into that folder: "+err.Error(), statusError)
			return
		}

		if len(existing) == 0 {
			v.askForValues(detail, files, target)
			return
		}

		list := selectableWrap()
		list.SetText("These files are already in " + target + " and would be overwritten:\n\n" + strings.Join(existing, "\n"))

		dialog.ShowCustomConfirm(
			fmt.Sprintf("Overwrite %d file(s)?", len(existing)),
			"Overwrite", "Cancel",
			list,
			func(confirmed bool) {
				if confirmed {
					v.askForValues(detail, files, target)
				}
			}, v.ui.window)
	}

	v.ui.pick(func(p picker) (string, error) { return p.openFolder("Choose where to apply the template") }, use, func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				v.report("The folder could not be chosen: "+err.Error(), statusError)
				return
			}
			if uri == nil {
				return
			}

			use(uri.Path())
		}, v.ui.window)
	})
}

// askForValues collects the answers a template needs: its declared variables
// and the placeholders its files use without declaring them. Nothing is written
// while a required answer is empty.
func (v *templatePage) askForValues(detail *templates.TemplateDetail, files []templateapply.File, target string) {
	undeclared := templateapply.Undeclared(files, detail.Variables)

	rows := make([]formRow, 0, len(detail.Variables)+len(undeclared))
	entries := make(map[string]*widget.Entry, len(detail.Variables)+len(undeclared))
	required := false

	for _, variable := range detail.Variables {
		entry := widget.NewEntry()
		entry.SetText(variable.DefaultValue)

		rows = append(rows, formRow{caption: variableCaption(variable), input: entry})
		entries[variable.Key] = entry
		required = required || variable.Required
	}

	for _, key := range undeclared {
		entry := widget.NewEntry()

		rows = append(rows, formRow{caption: key + " — not declared by the template", input: entry})
		entries[key] = entry
	}

	// The folder the files land in belongs above the questions: it is the thing
	// a person is agreeing to. The asterisk is explained once.
	hint := "The files are written into " + target + "."
	if required {
		hint += " Required fields are marked *."
	}

	input := formRows(rows...)

	// A template may ask many questions: the dialog keeps its height and the
	// form scrolls inside it.
	if len(rows) > 4 {
		scroll := container.NewVScroll(input)
		scroll.SetMinSize(fyne.NewSize(420, 220))
		input = scroll
	}

	title := widget.NewLabelWithStyle("Values for "+detail.Name,
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	status := selectableWrap()

	var popup *dialog.CustomDialog

	var content fyne.CanvasObject

	// showStatus resizes the dialog as well: a dialog takes its size from its
	// content once, so an error that arrives afterwards would be cut off.
	showStatus := func(message string) {
		status.SetText(message)
		relayout(content)
		popup.Resize(popup.MinSize())
	}

	write := widget.NewButton("Write", func() {
		values := make(map[string]string, len(entries))
		for key, entry := range entries {
			values[key] = strings.TrimSpace(entry.Text)
		}

		if key := missingAnswer(detail.Variables, values); key != "" {
			showStatus("The template asks for " + key + " and it has no answer yet.")
			return
		}

		if err := applyTemplate(target, files, values, true); err != nil {
			showStatus("The template was not written: " + err.Error())
			return
		}

		if popup != nil {
			popup.Hide()
		}

		v.showWritten(target, files)
	})
	write.Importance = widget.HighImportance

	actions := container.NewHBox(layout.NewSpacer(), write)

	content = container.NewVBox(title, newProse(hint), input, status, actions)

	popup = dialog.NewCustom("Apply "+detail.Name, "Cancel", content, v.ui.window)
	popup.Show()
}

// missingAnswer names the first required variable that has no answer, or "".
func missingAnswer(variables []templates.TemplateVariableData, values map[string]string) string {
	for _, variable := range variables {
		if variable.Required && strings.TrimSpace(values[variable.Key]) == "" {
			return variable.Key
		}
	}

	return ""
}

// applyTemplate fills in the answers and writes the files: the whole write
// path, apart from the dialogs that collect the answers.
func applyTemplate(target string, files []templateapply.File, values map[string]string, overwrite bool) error {
	for i := range files {
		if files[i].Kind == templates.FileKindBinary {
			continue
		}

		files[i].Content = templateapply.Substitute(files[i].Content, values)
	}

	return templateapply.Write(target, files, overwrite)
}

// showWritten names every path that was written, selectable, with a copy.
func (v *templatePage) showWritten(target string, files []templateapply.File) {
	paths := make([]string, 0, len(files))

	for _, file := range files {
		paths = append(paths, filepath.Join(target, filepath.FromSlash(file.Path)))
	}

	list := selectableWrap()
	list.SetText(strings.Join(paths, "\n"))

	copyPaths := widget.NewButton("Copy paths", func() {
		if !v.ui.copyToClipboard(strings.Join(paths, "\n")) {
			list.SetText(list.Text + "\n\n(This window has no clipboard available.)")
		}
	})

	body := container.NewBorder(nil,
		container.NewHBox(copyPaths), nil, nil,
		container.NewVScroll(list))

	dialog.ShowCustom(fmt.Sprintf("Wrote %d file(s)", len(paths)), "Close", body, v.ui.window)

	v.report(fmt.Sprintf("Wrote %d file(s) into %s.", len(paths), target), statusOK)
}
