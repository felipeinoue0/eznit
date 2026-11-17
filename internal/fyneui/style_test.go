//go:build fyne

package fyneui

import (
	"testing"

	"fyne.io/fyne/v2/widget"
)

// One primary action is what makes a tab read as a page rather than a pile of
// buttons. The Local server tab has two because its two panes are two views with one
// action each — Start on Output, Save configuration on Configuration — and the
// Save button's importance is load-bearing for the #235 pixel guard.
func TestTabsHaveOnePrimaryAction(t *testing.T) {
	u := newTestUI(t)

	// My Account's is Upload file…; the Web tab adds nothing, and its rows'
	// Download buttons are plain so a list of them does not shout.
	for name, want := range map[string]int{"My Account": 1, "Web": 0} {
		primary := 0

		root := u.account.built
		if name == "Web" {
			root = tabNamed(t, u, name).Content
		}

		for _, button := range allButtons(root) {
			if !button.Hidden && button.Importance == widget.HighImportance {
				primary++
			}
		}

		if primary != want {
			t.Errorf("the %s tab has %d primary buttons, want %d", name, primary, want)
		}
	}

	for _, button := range allButtons(tabNamed(t, u, "About").Content) {
		if !button.Hidden && button.Importance == widget.HighImportance {
			t.Errorf("the About tab offers a primary action %q; it is a page, not a form", button.Text)
		}
	}
}
