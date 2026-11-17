//go:build fyne

package fyneui

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
	"github.com/fiwon123/eznit/internal/templatefolder"
)

// Publishing from the window is `eznit template publish <folder>` with a form
// instead of prompts (#367): the same folder walk (templatefolder.Collect), the
// same request, the same rule that a placeholder without a default must be
// answered when the template is applied.

// publishSignedOut is what the button says without a session. Browsing and
// applying need no account; publishing puts something on the server under one.
const publishSignedOut = "Log in to publish a template. Browsing and applying need no account."

// publishForm is what a person fills in; buildPublishRequest turns it and the
// collected files into the API's request.
type publishForm struct {
	name        string
	kind        string
	description string
	public      bool
	// defaults holds one entry per placeholder the files use. An empty
	// default makes the question required when the template is applied.
	defaults map[string]string
}

// buildPublishRequest is the request the CLI would send for the same folder
// and answers. The variables come out in the order the placeholders were
// found, which is the order Undeclared reports them in.
func buildPublishRequest(form publishForm, files []templateapply.File) (templates.PublishRequest, error) {
	name := strings.TrimSpace(form.name)
	if name == "" {
		return templates.PublishRequest{}, errors.New("a template needs a name")
	}

	kind := form.kind
	if kind == "" {
		kind = templates.KindGeneric
	}
	if !templates.IsKind(kind) {
		return templates.PublishRequest{}, fmt.Errorf("%q is not a kind eznit knows", kind)
	}

	if len(files) == 0 {
		return templates.PublishRequest{}, errors.New("the folder holds no file to publish")
	}

	request := templates.PublishRequest{
		Name:        name,
		Description: strings.TrimSpace(form.description),
		Kind:        kind,
		IsPublic:    form.public,
	}

	for _, file := range files {
		request.Files = append(request.Files, templates.PublishFile{
			Path:          file.Path,
			Kind:          file.Kind,
			Content:       file.Content,
			ContentBase64: file.ContentBase64,
			IsExecutable:  file.IsExecutable,
		})
	}

	for _, key := range templateapply.Undeclared(files, nil) {
		value := strings.TrimSpace(form.defaults[key])
		request.Variables = append(request.Variables, templates.PublishVariable{
			Key:          key,
			Prompt:       key,
			DefaultValue: value,
			Required:     value == "",
		})
	}

	return request, nil
}

// publishFolder asks for a folder, reads it off the UI thread, and opens the
// form. Without a session it says where to sign in instead.
func (v *accountView) publishFolder() {
	if v.ui.client.Token == "" {
		v.setStatus(publishSignedOut, statusWarn)
		return
	}

	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			v.setStatus("The folder could not be chosen: "+err.Error(), statusError)
			return
		}
		if uri == nil {
			return
		}

		root := uri.Path()
		v.setStatus("Reading "+root+"…", statusPending)

		v.ui.background(func() {
			files, skipped, err := templatefolder.Collect(root)

			fyne.Do(func() {
				if err != nil {
					v.setStatus("This folder cannot be published: "+err.Error(), statusError)
					return
				}
				if len(files) == 0 {
					v.setStatus("The folder "+root+" holds no file to publish.", statusWarn)
					return
				}

				v.setStatus(fmt.Sprintf("%d file(s) read from %s.", len(files), root), statusOK)
				v.showPublishForm(root, files, skipped)
			})
		})
	}, v.ui.window)
}

// publishDialog is the open form's controls. It is returned so a test can fill
// it in and press Publish: Fyne's dialog keeps its tree to itself.
type publishDialog struct {
	name        *widget.Entry
	kind        *widget.Select
	description *widget.Entry
	visibility  *widget.Select
	defaults    map[string]*widget.Entry
	send        *widget.Button
	status      *prose
}

// showPublishForm is the CLI's prompts as one dialog: name, kind, description,
// visibility, and a default for each placeholder.
func (v *accountView) showPublishForm(root string, files []templateapply.File, skipped []string) *publishDialog {
	name := widget.NewEntry()
	name.SetText(filepath.Base(root))

	kind := widget.NewSelect(templates.Kinds, nil)
	kind.SetSelected(templates.KindGeneric)

	// The description is prose (#376): a few lines on show, wrapped, and a
	// scrollbar of its own past them, so a long one never grows the dialog.
	description := widget.NewMultiLineEntry()
	description.Wrapping = fyne.TextWrapWord
	description.SetMinRowsVisible(4)
	description.SetPlaceHolder("What a project gets when it applies this")

	// Private is the default: publishing to everyone is a choice to make.
	visibility := widget.NewSelect(visibilityChoices, nil)
	visibility.SetSelected("Private")

	rows := []formRow{
		{caption: "Name *", input: name},
		{caption: "Kind", input: kind},
		{caption: "Description", input: description},
		{caption: "Visibility", input: visibility},
	}

	keys := templateapply.Undeclared(files, nil)
	defaults := make(map[string]*widget.Entry, len(keys))
	for _, key := range keys {
		entry := widget.NewEntry()
		entry.SetPlaceHolder("empty = asked every time")
		rows = append(rows, formRow{caption: "{{" + key + "}} default", input: entry})
		defaults[key] = entry
	}

	// The form scrolls inside the dialog: with the description's four lines
	// and a question per placeholder it is taller than a small window, and a
	// dialog that runs off the screen hides its own Publish button.
	scroll := container.NewVScroll(formRows(rows...))
	scroll.SetMinSize(fyne.NewSize(460, 220))
	input := fyne.CanvasObject(scroll)

	summary := fmt.Sprintf("%d file(s) from %s.", len(files), root)
	if len(keys) > 0 {
		summary += fmt.Sprintf(" The files use %d placeholder(s); each becomes a question when the template is applied.", len(keys))
	}
	if len(skipped) > 0 {
		summary += "\n\nLeft out (a symlink, or bigger than a template allows): " + strings.Join(skipped, ", ")
	}

	status := selectableWrap()

	var popup *dialog.CustomDialog
	var content fyne.CanvasObject

	showStatus := func(message string) {
		status.SetText(message)
		relayout(content)
		popup.Resize(popup.MinSize())
	}

	var send *widget.Button
	send = widget.NewButton("Publish", func() {
		form := publishForm{
			name:        name.Text,
			kind:        kind.Selected,
			description: description.Text,
			public:      visibility.Selected == "Public",
			defaults:    make(map[string]string, len(defaults)),
		}
		for key, entry := range defaults {
			form.defaults[key] = entry.Text
		}

		request, err := buildPublishRequest(form, files)
		if err != nil {
			showStatus(capitalize(err.Error()) + ".")
			return
		}

		send.Disable()
		showStatus("Publishing…")

		client := v.ui.client
		v.ui.background(func() {
			detail, err := client.PublishTemplate(request)

			fyne.Do(func() {
				if err != nil {
					send.Enable()
					if apiErrorStatus(err) == http.StatusUnauthorized {
						popup.Hide()
						v.setStatus(sessionEnded, statusError)
						v.ui.forgetSession(sessionEnded)
						return
					}
					showStatus("Not published: " + messageOf(err))
					return
				}

				popup.Hide()
				visibility := "private — only you can see it"
				if detail.IsPublic {
					visibility = "public"
				}
				v.afterLoad = fmt.Sprintf("Published %s (v%d, %s). Apply it with: eznit template apply %s",
					detail.Name, detail.Version, visibility, detail.ID)
				v.reload()
			})
		})
	})
	send.Importance = widget.HighImportance

	content = container.NewBorder(container.NewVBox(newProse(summary)), status, nil, nil, input)

	// Cancel and Publish side by side at the bottom, where a dialog's
	// answers are: Publish used to sit on a line of its own above Cancel.
	popup = dialog.NewCustomWithoutButtons("Publish a template", content, v.ui.window)
	cancel := widget.NewButton("Cancel", popup.Hide)
	popup.SetButtons([]fyne.CanvasObject{cancel, send})
	popup.Show()
	popup.Resize(fyne.NewSize(560, popup.MinSize().Height))

	return &publishDialog{name: name, kind: kind, description: description, visibility: visibility, defaults: defaults, send: send, status: status}
}

func capitalize(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
