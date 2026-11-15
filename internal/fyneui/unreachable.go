//go:build fyne

package fyneui

import (
	"fmt"

	"fyne.io/fyne/v2"
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
}

func newUnreachableView(u *ui, address string, cause error) *unreachableView {
	return &unreachableView{ui: u, address: address, cause: cause}
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

	return container.NewCenter(container.NewGridWrap(
		fyne.NewSize(unreachableWidth, body.MinSize().Height),
		container.NewPadded(body),
	))
}

// unreachableReason is one sentence: what happened, and what to do next. An
// answer with a status code is the server saying something the request did not
// expect, so it is worth naming the code and saying "wait".
func unreachableReason(err error) string {
	if status := apiErrorStatus(err); status != 0 {
		return fmt.Sprintf("The server answered %d. Try again in a moment.", status)
	}

	return "The server did not answer. Start it, or type another address above."
}
