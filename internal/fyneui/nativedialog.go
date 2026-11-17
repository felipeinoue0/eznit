//go:build fyne

package fyneui

import (
	"errors"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/ncruces/zenity"
)

// picker chooses files and folders with the operating system's own dialogs
// (#392): Explorer's on Windows, the system panel on macOS, GTK's (through
// zenity, qarma or matedialog) on Linux. Fyne's dialog is drawn inside the
// window, carries none of the person's bookmarks or recent places, and looks
// like a different program; Fyne 2.8 swaps it for the system one only in a
// Flatpak build.
//
// Each method blocks until the dialog closes. A cancel is ("", nil). An error
// means the dialog could not be shown at all — no helper installed on a
// minimal Linux desktop, a headless session — and the caller falls back to
// Fyne's dialog, so a choice never becomes impossible.
type picker interface {
	openFile(title string) (string, error)
	openFolder(title string) (string, error)
	saveFile(title, name, extension string) (string, error)
}

// systemPicker is the picker Run wires in. Tests leave ui.picker nil and get
// Fyne's dialog, which the test driver can see and drive.
type systemPicker struct{}

func (systemPicker) openFile(title string) (string, error) {
	return chosen(zenity.SelectFile(zenity.Title(title)))
}

func (systemPicker) openFolder(title string) (string, error) {
	return chosen(zenity.SelectFile(zenity.Title(title), zenity.Directory()))
}

func (systemPicker) saveFile(title, name, extension string) (string, error) {
	options := []zenity.Option{zenity.Title(title), zenity.Filename(name), zenity.ConfirmOverwrite()}
	if extension != "" {
		options = append(options, zenity.FileFilter{Name: extension, Patterns: []string{"*" + extension}})
	}

	path, err := chosen(zenity.SelectFileSave(options...))
	if err != nil || path == "" {
		return path, err
	}

	// A save dialog lets the name lose its extension; the file is still the
	// bundle the CLI reads by its extension.
	if extension != "" && !strings.EqualFold(filepath.Ext(path), extension) {
		path += extension
	}

	return path, nil
}

// chosen turns zenity's cancel into the picker's ("", nil).
func chosen(path string, err error) (string, error) {
	if errors.Is(err, zenity.ErrCanceled) {
		return "", nil
	}

	return path, err
}

// pick shows one system dialog off the UI goroutine — it blocks until the
// person closes it — and hands the result back on the UI goroutine: done with
// the chosen path, nothing on a cancel, fallback when the system dialog could
// not be shown (or there is no picker, as in the tests).
//
// The window stays clickable while a system dialog is open, so a second
// choice is ignored until the first one closes rather than stacking dialogs.
func (u *ui) pick(choose func(picker) (string, error), done func(path string), fallback func()) {
	if u.picker == nil {
		fallback()
		return
	}

	if u.picking {
		return
	}
	u.picking = true

	system := u.picker
	u.background(func() {
		path, err := choose(system)

		fyne.Do(func() {
			u.picking = false

			switch {
			case err != nil:
				fyne.LogError("the system file dialog could not be shown; using the window's own", err)
				fallback()
			case path != "":
				done(path)
			}
		})
	})
}
