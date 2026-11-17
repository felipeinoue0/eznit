//go:build fyne

package fyneui

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
)

// loginCard is the size of the sign-in card. It is fixed rather than fitted so
// the form does not stretch across a wide window; the card keeps its own
// minimum, and a test asserts both modes fit inside it — and inside the 600 px
// the tab has at the window's default 980x640.
const (
	loginWidth  float32 = 440
	loginHeight float32 = 540
)

// loginView is the sign-in and account-creation form. It is one form with a
// mode: the fields stay put when the mode changes, so a typed address is not
// lost. The server is not one of its fields (#376): it is chosen on the
// Settings tab, and the card names it with a way there.
type loginView struct {
	ui      *ui
	server  *widget.Label
	email   *widget.Entry
	pass    *widget.Entry
	confirm *widget.Entry
	hint    *widget.Label
	status  *widget.Label
	signup  bool
	busy    bool

	// card is the padded body, so a status that wraps can lay the card out
	// again and the layout test can measure what it holds.
	card fyne.CanvasObject
}

func newLoginView(u *ui, message string, kind statusKind) *loginView {
	view := &loginView{
		ui:      u,
		server:  oneLine(u.currentAddress()),
		email:   widget.NewEntry(),
		pass:    widget.NewPasswordEntry(),
		confirm: widget.NewPasswordEntry(),
		hint:    signupHint(),
		status:  widget.NewLabel(""),
	}

	view.setStatus(message, kind)

	view.email.SetPlaceHolder("you@example.com")
	view.pass.SetPlaceHolder("your password")
	view.confirm.SetPlaceHolder("the same password again")

	// Enter submits from any field: the form's own button is gone, and a
	// person typing a password expects the key to finish the job.
	for _, entry := range []*widget.Entry{view.email, view.pass, view.confirm} {
		entry.OnSubmitted = func(string) { view.submit() }
	}

	return view
}

// signupHint is what an account needs, in the words the form uses: an address
// and a password typed twice. One line, and no rule the form does not ask for —
// the API refuses an empty password and a mismatch, and nothing else (#324).
func signupHint() *widget.Label {
	label := muted("Any email, and a password typed twice.")
	label.Alignment = fyne.TextAlignCenter
	label.Wrapping = fyne.TextWrapWord

	return label
}

func (l *loginView) content() fyne.CanvasObject {
	logo := canvas.NewImageFromResource(logoResource())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(84, 84))

	title := widget.NewLabelWithStyle("eznit", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	title.Selectable = true

	subtitle := widget.NewLabelWithStyle("Your project's setup, in one place you host.",
		fyne.TextAlignCenter, fyne.TextStyle{})
	subtitle.Selectable = true

	// The server is named, not asked for: Settings is where it changes, and
	// the button goes there.
	change := widget.NewButton("Change in Settings", l.ui.openSettings)
	change.Importance = widget.LowImportance
	serverRow := container.NewBorder(nil, nil, nil, change, l.server)

	rows := []formRow{
		{caption: "Server", input: serverRow},
		{caption: "Email", input: l.email},
		{caption: "Password", input: l.pass},
	}
	if l.signup {
		rows = append(rows, formRow{caption: "Confirm", input: l.confirm})
	}

	submit := widget.NewButton(l.submitLabel(), l.submit)
	submit.Importance = widget.HighImportance

	mode := widget.NewButton(l.modeLabel(), l.toggleMode)
	mode.Importance = widget.LowImportance

	l.status.Alignment = fyne.TextAlignCenter
	l.status.Wrapping = fyne.TextWrapWord
	l.status.Selectable = true

	body := []fyne.CanvasObject{
		container.NewCenter(logo),
		title,
		subtitle,
		widget.NewSeparator(),
		formRows(rows...),
	}

	// What an account needs, said before the form asks for it: the server
	// refuses nothing a person could not have known in advance (#324).
	if l.signup {
		body = append(body, l.hint)
	}

	body = append(body, submit, container.NewCenter(mode), l.status)

	l.card = container.NewPadded(container.NewVBox(body...))

	// One primary action on the page, the width of the card, and the mode
	// switch under it: a form with one way forward is a form a person finishes.
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(loginWidth, loginHeight), l.card))
}

// submitLabel is the primary action's verb for the current mode.
func (l *loginView) submitLabel() string {
	if l.signup {
		return "Create account"
	}

	return "Log in"
}

// modeLabel is what the secondary action would do.
func (l *loginView) modeLabel() string {
	if l.signup {
		return "Back to log in"
	}

	return "Create an account"
}

func (l *loginView) toggleMode() {
	if l.busy {
		return
	}

	l.signup = !l.signup
	l.setStatus("", statusIdle)
	l.ui.setAccountTab(l.content())
}

// setStatus writes the line under the form, in the colour its kind earns: a
// refusal is the theme's error colour, so "something needs checking" is visible
// before the sentence is read (#337).
func (l *loginView) setStatus(text string, kind statusKind) {
	l.status.Importance = statusImportance(kind)
	l.status.SetText(text)

	relayout(l.card)
}

func (l *loginView) submit() {
	if l.busy {
		return
	}

	email := strings.TrimSpace(l.email.Text)
	password := l.pass.Text

	switch {
	case email == "":
		l.setStatus("Enter your email.", statusError)
		return
	case password == "":
		l.setStatus("Enter your password.", statusError)
		return
	case l.signup && password != l.confirm.Text:
		l.setStatus("Passwords do not match.", statusError)
		return
	}

	source := l.ui.address.source
	if l.ui.local {
		source = localSource
	}

	base, err := apiclient.ValidateBaseURL(l.ui.currentAddress())
	if err != nil {
		l.setStatus(err.Error()+" Choose the server in Settings.", statusError)
		return
	}

	l.busy = true
	l.setStatus("Talking to "+base+"…", statusPending)

	signup := l.signup
	confirm := l.confirm.Text

	l.ui.background(func() {
		client := apiclient.New(base)

		var token string
		var err error
		// signedUp is what the follow-up failure means: signup answered OK, so
		// the address is either new or taken, and the login that follows is
		// the only step that can refuse.
		signedUp := false

		if signup {
			err = client.Signup(email, password, confirm)
			if err == nil {
				signedUp = true
				token, err = client.Login(email, password)
			}
		} else {
			token, err = client.Login(email, password)
		}

		fyne.Do(func() {
			l.busy = false

			if err != nil {
				// The tailored line is for a refused pair after a signup —
				// that is the case the server cannot name. Anything else (a
				// server that went away between the two calls, a 5xx) gets
				// its own sentence, because "already has an account" would be
				// false (#337).
				if signedUp && apiErrorStatus(err) == http.StatusUnauthorized {
					l.setStatus(signupNotSignedIn, statusError)

					return
				}

				l.setStatus(loginMessage(base, err), statusError)
				return
			}

			if token == "" {
				l.setStatus("The server answered without a token.", statusError)
				return
			}

			if err := config.Set("token", token); err != nil {
				l.setStatus("The session could not be saved: "+err.Error(), statusError)
				return
			}

			l.ui.address = apiChoice{url: base, source: source}
			l.ui.client = apiclient.New(base).WithToken(token)
			l.ui.showMain()
		})
	})
}

// signupNotSignedIn is what a refused sign-in after a signup says. The API
// answers "user created!" for an address that already has an account — on
// purpose, so signup cannot be used to discover one — and then the login with
// the password just typed is what fails. "invalid credentials" is true and
// useless; this names the likely case and the way out. It is used for a 401
// only: a signup followed by a server that went away gets its own sentence.
const signupNotSignedIn = "That email already has an account. Log in with its password."

// loginMessage is the one line a failed sign-in gets. A refused pair is named
// plainly — the server refuses to say which half was wrong, so neither does the
// card — and an instance that could not be reached is named by its address
// rather than by the dial error behind it (#337).
func loginMessage(base string, err error) string {
	if apiErr, ok := err.(*apiclient.APIError); ok {
		if apiErr.Status == http.StatusUnauthorized {
			return "Wrong email or password."
		}

		if apiErr.Message != "" {
			return apiErr.Message
		}

		return fmt.Sprintf("The server answered %d.", apiErr.Status)
	}

	if errors.Is(err, apiclient.ErrUnreachable) {
		return "Cannot reach " + base + "."
	}

	return err.Error()
}

// apiErrorStatus is the status an error carried, or 0 when the request never
// got an answer. A refusal and a server that is not there are different
// answers, and only one of them says anything about the session.
func apiErrorStatus(err error) int {
	apiErr, ok := err.(*apiclient.APIError)
	if !ok {
		return 0
	}

	return apiErr.Status
}

// messageOf is the one line a person needs: the host's message when there is
// one, the error otherwise.
func messageOf(err error) string {
	if apiErr, ok := err.(*apiclient.APIError); ok {
		if apiErr.Message != "" {
			return apiErr.Message
		}

		return fmt.Sprintf("The server answered %d.", apiErr.Status)
	}

	return err.Error()
}
