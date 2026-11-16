//go:build fyne

package fyneui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
)

// The Shared tab and the publish form draw at the sizes a person gets, in both
// themes, without the window having to grow (#367, #368).
func TestSharedTabAndPublishFormFitTheirWindow(t *testing.T) {
	sizes := []fyne.Size{fyne.NewSize(980, 640), fyne.NewSize(800, 520)}
	themes := map[string]fyne.Theme{"light": theme.LightTheme(), "dark": theme.DarkTheme()}

	for name, th := range themes {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s/%.0fx%.0f", name, size.Width, size.Height), func(t *testing.T) {
				u := newTestUI(t)
				u.app.Settings().SetTheme(th)
				u.window.Resize(size)

				u.shared.items = []apiclient.SharedFile{
					{File: apiclient.File{ID: "a1", Name: "gitconfig", Ext: "txt", Version: 2, IsPublic: true}, OwnerEmail: "ana@example.com"},
					{File: apiclient.File{ID: "a2", Name: "zshrc", Version: 1, IsPublic: true}},
				}
				u.shared.table.Refresh()
				u.shared.setStatus("2 shared file(s) on http://127.0.0.1:4000.", statusOK)
				u.tabs.Select(tabNamed(t, u, "Shared"))
				u.window.Content().Resize(size)

				if min := u.window.Content().MinSize(); min.Width > size.Width || min.Height > size.Height {
					t.Errorf("the Shared tab needs %.0fx%.0f, the window has %.0fx%.0f", min.Width, min.Height, size.Width, size.Height)
				}
				capture(t, u, fmt.Sprintf("shared-%s-%.0fx%.0f.png", name, size.Width, size.Height))

				files := []templateapply.File{
					{Path: "AGENTS.md", Kind: templates.FileKindText, Content: "# {{PROJECT_NAME}} — {{AUTHOR}}"},
					{Path: "Makefile", Kind: templates.FileKindText, Content: "all:"},
				}
				u.tabs.Select(tabNamed(t, u, "Templates"))
				u.templates.showPublishForm("/home/ana/projects/go-starter", files, []string{"big.iso"})
				capture(t, u, fmt.Sprintf("publish-%s-%.0fx%.0f.png", name, size.Width, size.Height))
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
