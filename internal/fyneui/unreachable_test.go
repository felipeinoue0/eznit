//go:build fyne

package fyneui

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
)

// The panel says which server it could not reach, in one sentence about what
// happened, and offers both ways out (#335).
func TestUnreachablePanelNamesTheAddress(t *testing.T) {
	u := newTestUI(t)

	address := "http://127.0.0.1:4321"
	u.showUnreachable(address, fmt.Errorf("%w: dial tcp %s: connect: connection refused", apiclient.ErrUnreachable, address))

	content := u.accountTab.Content

	if findLabel(content, "Cannot reach this server") == nil {
		t.Error("the panel has no headline")
	}

	if findLabel(content, address) == nil {
		t.Errorf("the panel does not name the address %q", address)
	}

	if got := panelText(content); !strings.Contains(got, "The server did not answer") {
		t.Errorf("the panel says %q, want one line about what happened", got)
	}

	tryAgain := findButton(content, "Try again")
	if tryAgain == nil {
		t.Fatal("the panel has no Try again")
	}
	if tryAgain.OnTapped == nil {
		t.Error("Try again runs nothing")
	}

	if findButton(content, "Sign out") == nil {
		t.Error("the panel has no Sign out")
	}
}

// An answer with a status code is the server saying something the request did
// not expect, and the advice depends on which side of 500 it is: a 5xx is worth
// retrying, a 4xx usually means the address is not an eznit instance.
func TestUnreachablePanelNamesAnUnexpectedAnswer(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{"a server in trouble", 503, "answered 503. Try again in a moment"},
		{"a wrong address", 404, "answered 404. Check the address above"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newTestUI(t)

			u.showUnreachable("http://127.0.0.1:4321", &apiclient.APIError{Status: tt.status})

			if got := panelText(u.accountTab.Content); !strings.Contains(got, tt.want) {
				t.Errorf("the panel says %q, want %q", got, tt.want)
			}
		})
	}
}

// The address bar stays usable while the panel is up, and its answers — a Test,
// a refused address — land in the panel's own line: the library's status line
// is in a container the tab no longer shows.
func TestUnreachablePanelAnswersTheAddressBar(t *testing.T) {
	u := newTestUI(t)

	u.showUnreachable("http://127.0.0.1:4321", fmt.Errorf("%w: no route to host", apiclient.ErrUnreachable))

	content := u.accountTab.Content

	if !holds(content, u.account.bar.entry) {
		t.Fatal("the panel's address bar is not on screen")
	}

	u.account.bar.message("No answer from http://127.0.0.1:4321", statusError)

	if got := panelText(content); !strings.Contains(got, "No answer from http://127.0.0.1:4321") {
		t.Errorf("the panel says %q, want the bar's answer", got)
	}
}

// The panel is what the tab shows instead of the library, so it has to fit the
// smallest window and keep its words inside their boxes.
func TestUnreachablePanelFitsTheWindow(t *testing.T) {
	for _, size := range windowSizes() {
		t.Run(fmt.Sprintf("%.0fx%.0f", size.Width, size.Height), func(t *testing.T) {
			u := newTestUI(t)

			u.showUnreachable("http://a-very-long-server-name.example.com:4000",
				fmt.Errorf("%w: no route to host", apiclient.ErrUnreachable))

			layOut(u.window, size)
			assertNoTextOverlap(t, u.window.Content())
			assertTextFits(t, u.window.Content())
		})
	}
}

// panelText is every word the tab draws, joined: the panel mixes labels and
// wrapped prose, and a test that asserted on one widget would miss the other.
func panelText(root fyne.CanvasObject) string {
	var texts []string

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, _ fyne.Position) {
		switch typed := object.(type) {
		case *widget.Label:
			texts = append(texts, typed.Text)
		case *prose:
			texts = append(texts, typed.Text)
		}
	})

	return strings.Join(texts, "\n")
}
