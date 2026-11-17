//go:build fyne

package fyneui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// newTestLogin builds the sign-in form on My Account, which is where it
// lives: the login form is that tab's content when there is no token.
func newTestLogin(t *testing.T, signup bool) *loginView {
	t.Helper()

	u := newTestUI(t)

	view := newLoginView(u, "", statusIdle)
	view.signup = signup
	u.setLoginCard(view.content())
	u.showScreen(u.loginScreen)

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
			for _, button := range allButtons(view.ui.loginBody.Objects[0]) {
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

// The card names the server it signs in to and sends a person to Settings to
// change it (#376): the address is chosen in one place only.
func TestLoginNamesTheServerAndLinksToSettings(t *testing.T) {
	view := newTestLogin(t, false)
	view.ui.local = false
	view.ui.address = apiChoice{url: "http://192.168.100.200:4000", source: "the address you entered"}

	card := newLoginView(view.ui, "", statusIdle)
	view.ui.setLoginCard(card.content())

	if card.server.Text != "http://192.168.100.200:4000" || !card.server.Selectable {
		t.Errorf("server line = %q (selectable %v), want the current address", card.server.Text, card.server.Selectable)
	}

	for _, entry := range []string{"Connect", "Test"} {
		if findButton(view.ui.loginBody.Objects[0], entry) != nil {
			t.Errorf("the card still has %q: the address belongs to Settings", entry)
		}
	}

	change := findButton(view.ui.loginBody.Objects[0], "Change in Settings")
	if change == nil {
		t.Fatal("the card has no way to Settings")
	}

	test.Tap(change)

	if !view.ui.screenShown(view.ui.settingsScreen) || view.ui.settings.sections.Selected() != view.ui.settings.serverSection {
		t.Error("Change in Settings did not open the Settings tab")
	}
}

// The status is the card's line: centred and wrapped, so an error reads as one
// sentence under the form instead of a long line off the edge.
func TestLoginStatusIsCentredAndWrapped(t *testing.T) {
	view := newTestLogin(t, false)

	view.setStatus("The server at http://a-very-long-server-name.example.com:4000 did not answer.", statusError)

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
	} {
		if entry.OnSubmitted == nil {
			t.Errorf("the %s field does not submit on Enter", name)
		}
	}

	// An empty address is refused before anything touches the network, which is
	// enough to prove the handler is the form's own submit.
	view.email.SetText("")
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
			view.setStatus(long, statusError)

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
			if room := view.ui.loginBody.Objects[0].Size(); size.Height > room.Height {
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

	if !holds(signup.ui.loginBody.Objects[0], signup.hint) {
		t.Fatal("the signup form does not show what an account needs")
	}

	if !strings.Contains(strings.ToLower(signup.hint.Text), "password") ||
		!strings.Contains(strings.ToLower(signup.hint.Text), "twice") {
		t.Errorf("the hint = %q, want it to name the password typed twice", signup.hint.Text)
	}

	login := newTestLogin(t, false)

	if holds(login.ui.loginBody.Objects[0], login.hint) {
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

// A plain log-in failure is named plainly: the server's "invalid credentials"
// is the API's word, not one a person reads, and it is the same either way
// (#337).
func TestLoginNamesAWrongPairPlainly(t *testing.T) {
	server, seen := accountServer(t, http.StatusOK, http.StatusUnauthorized)

	view := newTestLogin(t, false)
	fillAndSubmit(t, view, server)

	if *seen != [2]int{0, 1} {
		t.Fatalf("the form called signup %d and login %d times, want only a login", seen[0], seen[1])
	}

	if view.status.Text != "Wrong email or password." {
		t.Errorf("status = %q, want the one line a refused pair gets", view.status.Text)
	}

	if view.status.Importance != widget.DangerImportance {
		t.Errorf("status importance = %v, want the error colour", view.status.Importance)
	}
}

// A wrong answer is drawn in red, so it is visible before it is read. The card
// used to say everything in the same grey (#337).
func TestLoginErrorsAreRed(t *testing.T) {
	view := newTestLogin(t, true)

	view.email.SetText("someone@example.test")
	view.pass.SetText("one")
	view.confirm.SetText("another")
	view.submit()

	if got := view.status.Text; got != "Passwords do not match." {
		t.Errorf("status = %q, want the mismatch named in one line", got)
	}

	if view.status.Importance != widget.DangerImportance {
		t.Errorf("status importance = %v, want the error colour", view.status.Importance)
	}

	// A message that is not a failure is ordinary text again.
	view.setStatus("Talking to http://127.0.0.1:4000…", statusPending)

	if view.status.Importance == widget.DangerImportance {
		t.Error("the status is still red for a message that is not a failure")
	}
}

// An instance that cannot be reached is named by its address: the dial error
// behind it is not a sentence a person reads (#337).
func TestLoginNamesAnUnreachableServer(t *testing.T) {
	err := fmt.Errorf("%w: dial tcp 127.0.0.1:4000: connect: connection refused", apiclient.ErrUnreachable)

	if got := loginMessage("http://127.0.0.1:4000", err); got != "Cannot reach http://127.0.0.1:4000." {
		t.Errorf("loginMessage() = %q, want the address named in one line", got)
	}
}

// A signup followed by a server that went away is not "that email already has
// an account": the tailored line is for a refused pair, and anything else said
// about the account would be a guess (#337).
func TestLoginAfterSignupSaysWhatWentWrong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/signup":
			_, _ = w.Write([]byte(`{"status_code":200,"message":"user created!"}`))
		case "/v1/login":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	view := newTestLogin(t, true)
	fillAndSubmit(t, view, server)

	if strings.Contains(view.status.Text, "already has an account") {
		t.Errorf("status = %q, want the failure the server gave, not a guess about the account", view.status.Text)
	}

	if !strings.Contains(view.status.Text, "503") {
		t.Errorf("status = %q, want the status the server answered", view.status.Text)
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

	// The card signs in to the window's server, which Settings chooses.
	view.ui.local = false
	view.ui.address = apiChoice{url: server.URL, source: "the address you entered"}
	view.email.SetText("taken@example.test")
	view.pass.SetText("test123")
	view.confirm.SetText("test123")

	view.submit()

	// Wait through the window's own tally, not by polling `busy`: the test
	// driver runs fyne.Do inline, so the worker writes that field on its own
	// goroutine and the poll is a data race (#333).
	view.ui.settle()

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
