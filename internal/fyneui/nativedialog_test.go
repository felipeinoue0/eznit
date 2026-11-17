//go:build fyne

package fyneui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
)

// fakePicker stands in for the system dialogs (#392). It answers every call
// with path and err, after release is closed when there is one.
type fakePicker struct {
	path    string
	err     error
	release chan struct{}
	calls   atomic.Int32
}

func (f *fakePicker) answer() (string, error) {
	f.calls.Add(1)

	if f.release != nil {
		<-f.release
	}

	return f.path, f.err
}

func (f *fakePicker) openFile(string) (string, error)                 { return f.answer() }
func (f *fakePicker) openFolder(string) (string, error)               { return f.answer() }
func (f *fakePicker) saveFile(string, string, string) (string, error) { return f.answer() }

// overlay is the dialog Fyne drew over the window, or nil.
func overlay(u *ui) fyne.CanvasObject {
	return u.window.Canvas().Overlays().Top()
}

// A folder chosen in the system dialog is used, and no Fyne dialog is drawn.
func TestBrowseUsesTheSystemDialog(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	chosen := bundleDir(t, "API_PORT=\"4001\"\n")
	view.ui.picker = &fakePicker{path: chosen}

	view.chooseFolder()
	view.ui.settle()

	if got := view.folderPath(); got != chosen {
		t.Errorf("the folder is %q, want the chosen %q", got, chosen)
	}
	if overlay(view.ui) != nil {
		t.Error("a Fyne dialog was drawn although the system one answered")
	}
}

// A cancel in the system dialog changes nothing and draws nothing.
func TestBrowseCancelledChangesNothing(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\n")
	view := newTestServerView(t, dir)
	view.ui.picker = &fakePicker{}

	view.chooseFolder()
	view.ui.settle()

	if got := view.folderPath(); got != dir {
		t.Errorf("a cancel moved the folder to %q", got)
	}
	if overlay(view.ui) != nil {
		t.Error("a cancel drew a Fyne dialog")
	}
}

// A system dialog that cannot be shown falls back to Fyne's, so the choice is
// still possible.
func TestBrowseFallsBackWhenTheSystemDialogFails(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	view.ui.picker = &fakePicker{err: errors.New("zenity: not found")}

	view.chooseFolder()
	view.ui.settle()

	if overlay(view.ui) == nil {
		t.Error("no dialog at all: the fallback to Fyne's was not shown")
	}
}

// While a system dialog is open the window is still clickable; a second click
// is ignored instead of stacking a second dialog.
func TestOneSystemDialogAtATime(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	fake := &fakePicker{release: make(chan struct{})}
	view.ui.picker = fake

	view.chooseFolder()
	if !view.ui.picking {
		t.Fatal("no system dialog is marked open")
	}
	view.chooseFolder()

	close(fake.release)
	view.ui.settle()

	if got := fake.calls.Load(); got != 1 {
		t.Errorf("%d system dialogs were opened, want 1", got)
	}
	if view.ui.picking {
		t.Error("the window still thinks a dialog is open after it closed")
	}
}

// A template bundle saved through the system dialog is written by the window
// itself: the dialog hands back a path, not a writer.
func TestSaveBundleWritesTheChosenPath(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	path := filepath.Join(t.TempDir(), "my-starter"+bundleExtension)
	view.ui.picker = &fakePicker{path: path}

	var status string
	saveBundle(view.ui, sampleDetail(t), func(text string, _ statusKind) { status = text })
	view.ui.settle()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the bundle was not written: %v", err)
	}
	if !strings.Contains(string(data), "my-starter") {
		t.Errorf("the bundle does not hold the template: %.80q", data)
	}
	if !strings.Contains(status, "Saved "+path) {
		t.Errorf("status = %q, want it to name the saved path", status)
	}
}
