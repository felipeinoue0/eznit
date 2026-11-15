//go:build fyne

package fyneui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// newTestLogin builds the sign-in form in the Files tab, which is where it
// lives: the login form is that tab's content when there is no token.
func newTestLogin(t *testing.T, signup bool) *loginView {
	t.Helper()

	u := newTestUI(t)

	view := newLoginView(u, "")
	view.signup = signup
	u.setFilesTab(view.content())

	return view
}

// One way forward: exactly one primary button, whatever the mode. The mode
// switch and "This machine" are secondary.
func TestLoginHasOnePrimaryAction(t *testing.T) {
	for _, signup := range []bool{false, true} {
		name := "log in"
		if signup {
			name = "create account"
		}

		t.Run(name, func(t *testing.T) {
			view := newTestLogin(t, signup)

			primary := 0
			for _, button := range allButtons(view.ui.tabs.Items[0].Content) {
				if button.Importance == widget.HighImportance {
					primary++
				}
			}

			if primary != 1 {
				t.Errorf("the form has %d primary buttons, want exactly one", primary)
			}
		})
	}
}

// The local choice belongs with the address field it fills in — under it,
// inside the same cell, so the field keeps the card's width — not on a row of
// its own apart from the form.
func TestLoginPutsThisMachineWithTheServerField(t *testing.T) {
	view := newTestLogin(t, false)
	view.local.SetChecked(false)
	view.ui.window.Content().Resize(fyne.NewSize(980, 640))
	relayout(view.ui.window.Content())

	if !sharesParent(view.ui.tabs.Items[0].Content, view.api, view.local.row) {
		t.Error("'Use this machine' is not in the Server field's cell")
	}

	if view.local.row.Position().Y <= view.api.Position().Y {
		t.Error("'Use this machine' is beside the Server field; it belongs under it")
	}

	if view.api.Size().Width <= view.local.row.MinSize().Width {
		t.Errorf("the address field is %.0f px wide, no wider than the choice beside it",
			view.api.Size().Width)
	}

	view.local.SetChecked(true)

	if !view.api.Disabled() {
		t.Error("the address field is open while the form is set to this machine")
	}

	if got := view.api.Text; got != view.ui.localAddress() {
		t.Errorf("address = %q, want this machine's %q", got, view.ui.localAddress())
	}
}

// The address field is wide enough for the address a person types: at 176 px
// it cut off a LAN address on one row beside "Use this machine" (#325).
func TestLoginServerFieldShowsTheWholeAddress(t *testing.T) {
	const address = "http://192.168.100.200:4000"

	view := newTestLogin(t, false)
	view.local.SetChecked(false)
	view.api.SetText(address)

	view.ui.window.Content().Resize(fyne.NewSize(980, 640))
	relayout(view.ui.window.Content())

	need := fyne.MeasureText(address, theme.TextSize(), fyne.TextStyle{}).Width + 2*theme.Padding()

	if got := view.api.Size().Width; got < need {
		t.Errorf("the address field is %.0f px wide, needs %.0f for %q", got, need, address)
	}
}

// The status is the card's line: centred and wrapped, so an error reads as one
// sentence under the form instead of a long line off the edge.
func TestLoginStatusIsCentredAndWrapped(t *testing.T) {
	view := newTestLogin(t, false)

	view.setStatus("The server at http://a-very-long-server-name.example.com:4000 did not answer.")

	if view.status.Alignment != fyne.TextAlignCenter {
		t.Errorf("status alignment = %v, want centre", view.status.Alignment)
	}

	if view.status.Wrapping == fyne.TextWrapOff {
		t.Error("the status does not wrap, so a long error would run off the card")
	}

	if view.status.Text == "" {
		t.Error("the status is empty after being set")
	}
}

// Enter in any field submits: the form's button is gone, and the keyboard is
// what a person expects to finish a login.
func TestLoginEntriesSubmitOnEnter(t *testing.T) {
	view := newTestLogin(t, false)

	for name, entry := range map[string]*widget.Entry{
		"email":    view.email,
		"password": view.pass,
		"confirm":  view.confirm,
		"server":   view.api,
	} {
		if entry.OnSubmitted == nil {
			t.Errorf("the %s field does not submit on Enter", name)
		}
	}

	// An empty address is refused before anything touches the network, which is
	// enough to prove the handler is the form's own submit.
	view.email.SetText("")
	view.api.SetText("http://127.0.0.1:4321")
	view.email.OnSubmitted("")

	if view.status.Text == "" {
		t.Error("Enter ran nothing: the status is still empty")
	}
}

// Both modes fit the card, including a long status line: a card that clips its
// own form is worse than one that scrolls.
func TestLoginCardFitsBothModes(t *testing.T) {
	long := "The server at http://a-very-long-server-name.example.com:4000 did not answer when asked whether it was up."

	for _, signup := range []bool{false, true} {
		name := "log in"
		if signup {
			name = "create account"
		}

		t.Run(name, func(t *testing.T) {
			view := newTestLogin(t, signup)
			view.setStatus(long)

			// The window lays the card out before it is measured; do what a
			// window does.
			view.ui.window.Content().Resize(fyne.NewSize(980, 640))
			relayout(view.ui.window.Content())

			size := view.card.MinSize()
			if size.Width > loginWidth || size.Height > loginHeight {
				t.Errorf("the card needs %.0fx%.0f but its cell is %.0fx%.0f",
					size.Width, size.Height, loginWidth, loginHeight)
			}
		})
	}
}
