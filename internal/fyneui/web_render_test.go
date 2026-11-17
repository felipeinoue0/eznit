//go:build fyne

package fyneui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
)

// Every page of the two lists draws at the sizes a person gets, in both themes,
// without the window having to grow (#373): the Web list and a template's page,
// My Account's list and a file's page, and the publish form.
func TestListsAndPagesFitTheirWindow(t *testing.T) {
	sizes := []fyne.Size{fyne.NewSize(980, 640), fyne.NewSize(800, 520)}
	themes := map[string]fyne.Theme{"light": theme.LightTheme(), "dark": theme.DarkTheme()}

	for name, th := range themes {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s/%.0fx%.0f", name, size.Width, size.Height), func(t *testing.T) {
				u := newTestUI(t)
				u.app.Settings().SetTheme(th)
				u.window.Resize(size)

				fits := func(what string) {
					t.Helper()
					u.window.Content().Resize(size)
					if min := u.window.Content().MinSize(); min.Width > size.Width || min.Height > size.Height {
						t.Errorf("%s needs %.0fx%.0f, the window has %.0fx%.0f", what, min.Width, min.Height, size.Width, size.Height)
					}
				}
				suffix := fmt.Sprintf("%s-%.0fx%.0f.png", name, size.Width, size.Height)

				detail := sampleDetail(t)
				shared := []apiclient.SharedFile{
					{File: apiclient.File{ID: "a1", Name: "gitconfig", Ext: "txt", Version: 2, IsPublic: true}, OwnerEmail: "ana@example.com"},
					{File: apiclient.File{ID: "a2", Name: "zshrc", Version: 1, IsPublic: true}},
				}

				u.web.templates = []templates.TemplateData{detail.TemplateData}
				u.web.files = nil
				for _, file := range shared {
					u.web.files = append(u.web.files, sharedFileEntry(file))
				}
				u.web.fill()
				u.web.setStatus("1 template(s) and 2 file(s) on http://127.0.0.1:4000.", statusOK)
				u.tabs.Select(tabNamed(t, u, "Explore"))
				fits("the Web list")
				capture(t, u, "web-"+suffix)

				page := newTemplatePage(u, u.web.setStatus)
				u.web.pages.push(page.content(detail.Name, u.web.pages.back))
				page.show(detail)
				fits("a template's page")
				capture(t, u, "web-template-"+suffix)

				u.account.files = []apiclient.File{shared[0].File, {ID: "b2", Name: "notes", Ext: "md", Version: 4}}
				u.account.templates = []templates.TemplateData{detail.TemplateData}
				u.account.ready = true
				u.account.fill()
				u.account.updateActions()
				u.account.setStatus("1 template(s) and 2 file(s) on http://127.0.0.1:4000.", statusOK)
				u.setWorkspace(u.account.content())
				u.account.fill()
				u.addWorkspaceTab()
				u.tabs.Select(u.workspaceTab)
				fits("My Account")
				capture(t, u, "account-"+suffix)

				u.account.openFile(u.account.files[0])
				u.settle()
				fits("a file's page")
				capture(t, u, "account-file-"+suffix)

				files := []templateapply.File{
					{Path: "AGENTS.md", Kind: templates.FileKindText, Content: "# {{PROJECT_NAME}} — {{AUTHOR}}"},
					{Path: "Makefile", Kind: templates.FileKindText, Content: "all:"},
				}
				u.account.pages.back()
				u.account.showPublishForm("/home/ana/projects/go-starter", files, []string{"big.iso"})
				capture(t, u, "publish-"+suffix)
			})
		}
	}
}

func capture(t *testing.T, u *ui, file string) {
	t.Helper()

	captured := u.window.Canvas().Capture()
	if captured == nil {
		t.Fatal("the canvas captured nothing")
	}
	if got := colourCount(captured); got < 4 {
		t.Errorf("%s has %d colours — the widgets did not draw", file, got)
	}
	if dir := os.Getenv("EZNIT_CAPTURE_DIR"); dir != "" {
		if err := writePNG(filepath.Join(dir, file), captured); err != nil {
			t.Fatal(err)
		}
	}
}

// Settings draws each of its sections — Server, Local server, Reset — at the
// sizes a person gets, in both themes, without the window having to grow
// (#379). The local server panel is the tallest thing in the window.
func TestSettingsFitsItsWindow(t *testing.T) {
	sizes := []fyne.Size{fyne.NewSize(980, 640), fyne.NewSize(800, 520)}
	themes := map[string]fyne.Theme{"light": theme.LightTheme(), "dark": theme.DarkTheme()}

	for name, th := range themes {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s/%.0fx%.0f", name, size.Width, size.Height), func(t *testing.T) {
				u := newTestUI(t)
				u.app.Settings().SetTheme(th)
				u.server = newServerView(u, bundleDir(t, "API_PORT=\"4000\"\n"))
				u.settings = newSettingsView(u)
				u.settingsScreen = u.screen("Settings", theme.SettingsIcon(), u.settings.content())
				u.window.Resize(size)
				u.showScreen(u.settingsScreen)

				for _, section := range []*container.TabItem{u.settings.serverSection, u.settings.localSection, u.settings.resetSection} {
					u.settings.showSection(section)
					u.window.Content().Resize(size)

					if min := u.window.Content().MinSize(); min.Width > size.Width || min.Height > size.Height {
						t.Errorf("Settings → %s needs %.0fx%.0f, the window has %.0fx%.0f", section.Text, min.Width, min.Height, size.Width, size.Height)
					}
					capture(t, u, fmt.Sprintf("settings-%s-%s-%.0fx%.0f.png", strings.ReplaceAll(strings.ToLower(section.Text), " ", "-"), name, size.Width, size.Height))
				}
			})
		}
	}
}
