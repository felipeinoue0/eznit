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

	content := u.tabs.Items[0].Content

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
// not expect, and the panel names the code rather than the dial error.
func TestUnreachablePanelNamesAnUnexpectedAnswer(t *testing.T) {
	u := newTestUI(t)

	u.showUnreachable("http://127.0.0.1:4321", &apiclient.APIError{Status: 503, Message: "maintenance"})

	if got := panelText(u.tabs.Items[0].Content); !strings.Contains(got, "answered 503") {
		t.Errorf("the panel says %q, want the status the server answered", got)
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
