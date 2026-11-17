//go:build fyne

package fyneui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/fiwon123/eznit/internal/config"
)

// The mode is this machine unless a run said otherwise: an explicit flag or
// environment address is not "this machine", and a remembered choice is.
func TestLocalModeDefaultsToThisMachine(t *testing.T) {
	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("GUI_API_URL", "")

	if !localMode("") {
		t.Error("a fresh window is not set to this machine")
	}

	if err := config.Set(localModeKey, "false"); err != nil {
		t.Fatal(err)
	}

	if localMode("") {
		t.Error("a remembered manual choice was ignored")
	}

	t.Setenv("GUI_API_URL", "http://from-env.test:4000")

	if localMode("") {
		t.Error("GUI_API_URL did not arrive unchecked")
	}

	if err := config.Set(localModeKey, "true"); err != nil {
		t.Fatal(err)
	}

	if localMode("http://from-flag.test:4000") {
		t.Error("the -api flag did not arrive unchecked")
	}
}

// Settings holds the one address bar (#376). It offers no Sign out — that is
// on the account screen — and it shows the connected address.
func TestSettingsHasTheAddressBar(t *testing.T) {
	u := newTestUI(t)

	if u.settings.bar == nil {
		t.Fatal("Settings has no address bar")
	}
	if u.settings.bar.signOut != nil {
		t.Error("the Settings bar offers Sign out")
	}
	if got := u.settings.bar.entry.Text; got != u.address.url {
		t.Errorf("Settings address = %q, want the connected one %q", got, u.address.url)
	}
}

// A local bar shows this machine's address, keeps the field shut, and names the
// source so the choice is never silent.
func TestLocalBarShowsThisMachine(t *testing.T) {
	u := newTestUI(t)

	u.local = true
	u.address = apiChoice{url: u.localAddress(), source: localSource}
	u.settings.bar.sync()

	if !u.settings.bar.local.Checked {
		t.Error("the checkbox is not checked for the local mode")
	}

	if !u.settings.bar.entry.Disabled() {
		t.Error("the address field is open while the bar is local")
	}

	if got := u.settings.bar.entry.Text; got != u.localAddress() {
		t.Errorf("address = %q, want %q", got, u.localAddress())
	}

	if got := u.settings.bar.source.Text; !strings.Contains(got, localSource) {
		t.Errorf("source = %q, want %q", got, localSource)
	}
}

// The address field shows a whole address even when the window is dragged to
// its narrowest. It used to share a row with the "Server" caption and every
// action, so the field was the leftover of the bar — 203 px at the window's
// own minimum, which clips a long address (#325).
func TestAddressFieldShowsAWholeAddress(t *testing.T) {
	const address = "http://a-very-long-server-name.example.com:4000"

	tabs := []struct {
		name  string
		index int
		bar   func(*ui) *addressBar
	}{
		{"settings", 1, func(u *ui) *addressBar { return u.settings.bar }},
	}

	for _, tab := range tabs {
		t.Run(tab.name, func(t *testing.T) {
			u := newTestUI(t)
			u.local = false
			u.address = apiChoice{url: address, source: "the address you entered on another machine"}

			u.tabs.SelectIndex(tab.index)
			bar := tab.bar(u)
			bar.sync()

			u.window.Content().Resize(fyne.NewSize(980, 640))
			relayout(u.window.Content())

			// The window can be dragged down to what its content needs; that
			// is the narrowest a person can make it.
			narrow := u.window.Content().MinSize().Width
			u.window.Content().Resize(fyne.NewSize(narrow, 640))
			relayout(u.window.Content())

			need := fyne.MeasureText(bar.entry.Text, theme.TextSize(), fyne.TextStyle{}).Width +
				2*theme.Padding()

			if got := bar.entry.Size().Width; got < need {
				t.Errorf("at %.0f px of window the address field is %.0f px wide, needs %.0f for %q",
					narrow, got, need, bar.entry.Text)
			}
		})
	}
}
