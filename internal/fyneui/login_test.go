//go:build fyne

package fyneui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

			// The cell has to fit the tab as well, or the window clips the card
			// and not one of its own rows.
			if room := view.ui.tabs.Items[0].Content.Size(); size.Height > room.Height {
				t.Errorf("the card needs %.0f px of height but the tab gives it %.0f",
					size.Height, room.Height)
			}
		})
	}
}

// Signup says what an account needs before it asks: an address, a password
// typed twice, and no minimum length. Log in asks for neither of the last two,
// so it shows no hint.
func TestLoginShowsWhatSignupNeeds(t *testing.T) {
	signup := newTestLogin(t, true)

	if !holds(signup.ui.tabs.Items[0].Content, signup.hint) {
		t.Fatal("the signup form does not show what an account needs")
	}

	if !strings.Contains(strings.ToLower(signup.hint.Text), "password") ||
		!strings.Contains(strings.ToLower(signup.hint.Text), "twice") {
		t.Errorf("the hint = %q, want it to name the password typed twice", signup.hint.Text)
	}

	login := newTestLogin(t, false)

	if holds(login.ui.tabs.Items[0].Content, login.hint) {
		t.Error("the log-in form shows the signup requirements")
	}
}

// A signup that answers OK but whose sign-in is refused says what to do. The
// server's "invalid credentials" is true, and it is what the reader called a
// bug: the address already had an account, and signup does not say so.
func TestLoginExplainsAFailedSignInAfterSignup(t *testing.T) {
	server, seen := accountServer(t, http.StatusOK, http.StatusUnauthorized)

	view := newTestLogin(t, true)
	fillAndSubmit(t, view, server)

	if *seen != [2]int{1, 1} {
		t.Fatalf("the form called signup %d and login %d times, want one each", seen[0], seen[1])
	}

	if strings.Contains(view.status.Text, "invalid credentials") {
		t.Errorf("status = %q, want the server's message replaced", view.status.Text)
	}

	if !strings.Contains(view.status.Text, "already has an account") {
		t.Errorf("status = %q, want it to name the likely cause and the way out", view.status.Text)
	}
}

// A plain log-in failure keeps the server's own message: the tailored line is
// for the signup path, where the password that failed is the one just typed.
func TestLoginKeepsTheServerMessageForAPlainLogin(t *testing.T) {
	server, seen := accountServer(t, http.StatusOK, http.StatusUnauthorized)

	view := newTestLogin(t, false)
	fillAndSubmit(t, view, server)

	if *seen != [2]int{0, 1} {
		t.Fatalf("the form called signup %d and login %d times, want only a login", seen[0], seen[1])
	}

	if view.status.Text != "invalid credentials" {
		t.Errorf("status = %q, want the server's message", view.status.Text)
	}
}

// accountServer stands in for the API: signup and login answer with the status
// given, and the pair of call counts says which path ran.
func accountServer(t *testing.T, signup, login int) (*httptest.Server, *[2]int) {
	t.Helper()

	seen := &[2]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/signup":
			seen[0]++
			w.WriteHeader(signup)
			_, _ = w.Write([]byte(`{"status_code":200,"message":"user created!"}`))
		case "/v1/login":
			seen[1]++
			w.WriteHeader(login)
			_, _ = w.Write([]byte(`{"status_code":401,"error":"invalid credentials"}`))
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(server.Close)

	return server, seen
}

// fillAndSubmit types an account and the server into the form and waits for the
// submit to finish, which is one round trip against the test server.
func fillAndSubmit(t *testing.T, view *loginView, server *httptest.Server) {
	t.Helper()

	view.api.SetText(server.URL)
	view.email.SetText("taken@example.test")
	view.pass.SetText("test123")
	view.confirm.SetText("test123")

	view.submit()

	deadline := time.Now().Add(5 * time.Second)
	for view.busy && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}

	if view.busy {
		t.Fatal("the form is still waiting for an answer")
	}
}

// holds reports whether root contains target anywhere in its object tree.
func holds(root, target fyne.CanvasObject) bool {
	found := false

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, _ fyne.Position) {
		found = found || object == target
	})

	return found
}
