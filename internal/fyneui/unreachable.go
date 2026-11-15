//go:build fyne

package fyneui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// unreachableWidth is how wide the panel is: a sentence and two buttons read as
// one decision, and a card that stretched across the window would read as an
// error page rather than a state the tab is in.
const unreachableWidth float32 = 460

// unreachableView is the Files tab when the server did not answer. It is not a
// sign-out: the token is kept, because a server that is not there has said
// nothing about it, so the way forward is Try again and not Log in.
//
// It exists because a failed listing used to leave the library in place with a
// status line under it: a table with no rows reads as "signed in, nothing
// here", which is exactly what the window could not know (#335).
type unreachableView struct {
	ui      *ui
	address string
	cause   error

	// status and dot are the tab's message line and its colour while the panel
	// is up. They are the panel's own: the library's line lives in a container
	// the tab no longer shows, so a Test or a refused address would report into
	// a label nobody can see.
	status *prose
	dot    *canvas.Circle

	// built is the panel as laid out, so a message that changes its height can
	// lay it out again.
	built fyne.CanvasObject
}

func newUnreachableView(u *ui, address string, cause error) *unreachableView {
	view := &unreachableView{
		ui:      u,
		address: address,
		cause:   cause,
		status:  newProse(""),
	}

	view.dot = canvas.NewCircle(statusColour(u.app.Settings().ThemeVariant(), statusIdle))

	return view
}

// content is the panel: what failed, what to do about it, and the second way
// out for a person who would rather be signed out of it.
func (v *unreachableView) content() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("Cannot reach this server",
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Selectable = true

	// The address is one line that ellipsizes: it is a value to check and to
	// copy, and the field above it holds the whole of it.
	address := oneLine(v.address)
	address.Alignment = fyne.TextAlignLeading

	reason := newProse(unreachableReason(v.cause))

	tryAgain := widget.NewButtonWithIcon("Try again", theme.ViewRefreshIcon(), v.ui.showMain)
	tryAgain.Importance = widget.HighImportance

	signOut := widget.NewButtonWithIcon("Sign out", theme.LogoutIcon(), v.ui.signOut)
	signOut.Importance = widget.LowImportance

	body := container.NewVBox(
		title,
		address,
		reason,
		widget.NewSeparator(),
		container.NewHBox(tryAgain, signOut),
	)

	// The grid wrap gives the card its width; its height is a floor, so a
	// sentence that wraps past it still fits and the card grows instead.
	v.built = container.NewCenter(container.NewGridWrap(
		fyne.NewSize(unreachableWidth, body.MinSize().Height),
		container.NewPadded(body),
	))

	return v.built
}

// setStatus writes the panel's message line and colours the dot, the way the
// library's bar does: a refusal here is as much of an error as it is there.
func (v *unreachableView) setStatus(text string, kind statusKind) {
	v.status.Importance = statusImportance(kind)
	v.status.SetText(text)

	v.dot.FillColor = statusColour(v.ui.app.Settings().ThemeVariant(), kind)
	v.dot.Refresh()

	// A longer message, or a card that grew with it, is a taller panel than the
	// one that was measured.
	relayout(v.built)
}

// unreachableReason is one sentence: what happened, and what to do next. An
// answer with a status code is the server saying something the request did not
// expect, and the advice depends on which side of 500 it is — a 5xx is worth
// retrying, a 4xx usually means the address is not an eznit instance.
func unreachableReason(err error) string {
	switch status := apiErrorStatus(err); {
	case status >= 500:
		return fmt.Sprintf("The server answered %d. Try again in a moment.", status)
	case status != 0:
		return fmt.Sprintf("The server answered %d. Check the address above.", status)
	default:
		return "The server did not answer. Start it, or type another address above."
	}
}
