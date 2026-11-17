//go:build fyne

package fyneui

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/domain/templates"
)

// windowSizes are the sizes a person actually gets, plus the smallest the
// window lets itself be today.
func windowSizes() []fyne.Size {
	return []fyne.Size{fyne.NewSize(800, 520), fyne.NewSize(980, 640), fyne.NewSize(1024, 720)}
}

// layOut lays a window's content out at a size and then settles the rows whose
// text wraps: Fyne measures such a row before it knows its width, so the second
// pass is what a real window does while it is being resized.
func layOut(window fyne.Window, size fyne.Size) {
	window.Resize(size)
	window.Content().Resize(size)
	relayout(window.Content())
}

// placed is one laid-out object: its absolute position and size.
type placed struct {
	name string
	pos  fyne.Position
	size fyne.Size
}

// walkTree visits every container and its children, accumulating positions.
// A widget's renderer is not descended into: only the objects this window
// builds itself are interesting, and a table or a button keeps its cells and
// its label to itself.
func walkTree(object fyne.CanvasObject, offset fyne.Position, visit func(fyne.CanvasObject, fyne.Position)) {
	pos := fyne.NewPos(offset.X+object.Position().X, offset.Y+object.Position().Y)
	visit(object, pos)

	switch typed := object.(type) {
	case *fyne.Container:
		for _, child := range typed.Objects {
			walkTree(child, pos, visit)
		}
	case *widget.Card:
		// A card's content is this window's own object; its renderer only adds
		// the background and the padding around it.
		if typed.Content != nil {
			walkTree(typed.Content, pos, visit)
		}
	case *container.Scroll:
		// A scroll's content is this window's own object too: a page body
		// that scrolls is still text that can overlap.
		if typed.Content != nil {
			walkTree(typed.Content, pos, visit)
		}
	case *container.Split:
		// A split is a widget, not a container, but its two halves are this
		// window's own objects and are laid out inside it.
		walkTree(typed.Leading, pos, visit)
		walkTree(typed.Trailing, pos, visit)
	}
}

// textBoxes are the laid-out labels, prose and buttons: everything that draws
// words and can therefore paint over another thing that draws words.
func textBoxes(root fyne.CanvasObject) []placed {
	var boxes []placed

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, pos fyne.Position) {
		if !object.Visible() || object.Size().Width <= 0 || object.Size().Height <= 0 {
			return
		}

		switch typed := object.(type) {
		case *widget.Label:
			boxes = append(boxes, placed{name: excerpt("label", typed.Text), pos: pos, size: object.Size()})
		case *prose:
			boxes = append(boxes, placed{name: excerpt("prose", typed.Text), pos: pos, size: object.Size()})
		case *widget.Button:
			boxes = append(boxes, placed{name: excerpt("button", typed.Text), pos: pos, size: object.Size()})
		}
	})

	return boxes
}

// excerpt keeps a failure message readable: the point is which widget, not the
// whole paragraph it holds.
func excerpt(kind, text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 40 {
		text = text[:37] + "…"
	}

	if text == "" {
		return kind + " (empty)"
	}

	return fmt.Sprintf("%s %q", kind, text)
}

// intersects reports whether two laid-out boxes share any area. Half a pixel is
// tolerated: Fyne rounds positions, and a rounding error is not an overlap.
func intersects(a, b placed) bool {
	const tolerance float32 = 0.5

	return a.pos.X+a.size.Width-tolerance > b.pos.X &&
		b.pos.X+b.size.Width-tolerance > a.pos.X &&
		a.pos.Y+a.size.Height-tolerance > b.pos.Y &&
		b.pos.Y+b.size.Height-tolerance > a.pos.Y
}

// assertNoTextOverlap is the guard for the whole class of defects: no two
// pieces of text are allowed to occupy the same pixels, at any supported size.
func assertNoTextOverlap(t *testing.T, root fyne.CanvasObject) {
	t.Helper()

	boxes := textBoxes(root)

	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			if intersects(boxes[i], boxes[j]) {
				t.Errorf("%s and %s overlap: %v %v and %v %v",
					boxes[i].name, boxes[j].name,
					boxes[i].pos, boxes[i].size, boxes[j].pos, boxes[j].size)
			}
		}
	}
}

// assertTextFits proves the other half: a box can be free of its neighbours and
// still be too short, which is how a wrapped line ends up drawn over the row
// below. prose.requiredSize is the height its text needs at the width it was
// given — measured independently of MinSize, so a MinSize that under-reports
// cannot hide behind its own answer.
func assertTextFits(t *testing.T, root fyne.CanvasObject) {
	t.Helper()

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, _ fyne.Position) {
		label, ok := object.(*prose)
		if !ok || !label.Visible() {
			return
		}

		want := label.requiredSize()
		size := label.Size()

		if size.Height < want.Height-1.5 {
			t.Errorf("%s needs %.1f px of height at its width but its row gave it %.1f",
				excerpt("prose", label.Text), want.Height, size.Height)
		}

		if size.Width < want.Width-1.5 {
			t.Errorf("%s needs %.1f px of width but its row gave it %.1f",
				excerpt("prose", label.Text), want.Width, size.Width)
		}
	})
}

// allButtons is every button this window builds itself, laid out or not.
func allButtons(root fyne.CanvasObject) []*widget.Button {
	var buttons []*widget.Button

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, _ fyne.Position) {
		if button, ok := object.(*widget.Button); ok {
			buttons = append(buttons, button)
		}
	})

	return buttons
}

// sharesParent reports whether two objects sit directly in the same container.
func sharesParent(root, first, second fyne.CanvasObject) bool {
	shared := false

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, _ fyne.Position) {
		container, ok := object.(*fyne.Container)
		if !ok {
			return
		}

		hasFirst, hasSecond := false, false
		for _, child := range container.Objects {
			hasFirst = hasFirst || child == first
			hasSecond = hasSecond || child == second
		}

		shared = shared || (hasFirst && hasSecond)
	})

	return shared
}

// findLabel returns the first label in the tree with the given text.
func findLabel(root fyne.CanvasObject, text string) *widget.Label {
	var found *widget.Label

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, _ fyne.Position) {
		label, ok := object.(*widget.Label)
		if ok && label.Text == text && found == nil {
			found = label
		}
	})

	return found
}

// findButton returns the first button in the tree with the given label.
func findButton(root fyne.CanvasObject, text string) *widget.Button {
	var found *widget.Button

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, _ fyne.Position) {
		button, ok := object.(*widget.Button)
		if ok && button.Text == text && found == nil {
			found = button
		}
	})

	return found
}

// placeOf finds one object's laid-out box by identity.
func placeOf(root, target fyne.CanvasObject) (placed, bool) {
	var found placed

	ok := false
	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, pos fyne.Position) {
		if object == target {
			found = placed{pos: pos, size: object.Size()}
			ok = true
		}
	})

	return found, ok
}

// assertDotCentredOn checks the dot's centre against the message's box. A
// status dot that floats above its text was the reported defect; a pixel and a
// half is rounding, not a layout.
func assertDotCentredOn(t *testing.T, root fyne.CanvasObject, dot, message fyne.CanvasObject) {
	t.Helper()

	dotBox, ok := placeOf(root, dot)
	if !ok {
		t.Fatal("the status dot is not in the laid-out tree")
	}

	messageBox, ok := placeOf(root, message)
	if !ok {
		t.Fatal("the status message is not in the laid-out tree")
	}

	dotCentre := dotBox.pos.Y + dotBox.size.Height/2
	messageCentre := messageBox.pos.Y + messageBox.size.Height/2

	if difference := dotCentre - messageCentre; difference > 1.5 || difference < -1.5 {
		t.Errorf("the dot's centre is at %.1f and the message's at %.1f — %.1f px apart",
			dotCentre, messageCentre, difference)
	}
}

// TestAccountTabTextDoesNotOverlap is My Account at its widest content: a file
// name long enough to wrap on its page, a long error at the bottom and a long
// address in the header — on the list and on the file's page.
func TestAccountTabTextDoesNotOverlap(t *testing.T) {
	for _, size := range windowSizes() {
		t.Run(fmt.Sprintf("%.0fx%.0f", size.Width, size.Height), func(t *testing.T) {
			view := newTestAccountView(t, apiChoice{
				url:    "http://a-very-long-server-name.example.com:4000",
				source: "the address you entered on another machine",
			})

			file := apiclient.File{
				ID:      "6b1f0e1a",
				Name:    "a-file-with-a-name-long-enough-to-need-a-second-line",
				Ext:     "txt",
				Version: 12,
			}
			view.files = []apiclient.File{file}
			view.ready = true
			view.fill()
			view.updateActions()
			view.setStatus("The upload failed: the server at http://a-very-long-server-name.example.com:4000 answered 413 — the file is larger than this instance accepts.", statusError)

			layOut(view.ui.window, size)
			assertNoTextOverlap(t, view.ui.window.Content())
			assertTextFits(t, view.ui.window.Content())

			view.openFile(file)

			layOut(view.ui.window, size)
			assertNoTextOverlap(t, view.ui.window.Content())
			assertTextFits(t, view.ui.window.Content())
		})
	}
}

// TestWebTabTextDoesNotOverlap covers a template's page on the Web tab: a
// description that wraps, a long error line and the header's address.
func TestWebTabTextDoesNotOverlap(t *testing.T) {
	for _, size := range windowSizes() {
		t.Run(fmt.Sprintf("%.0fx%.0f", size.Width, size.Height), func(t *testing.T) {
			u := newTestUI(t)
			u.local = false
			u.address = apiChoice{
				url:    "http://a-very-long-server-name.example.com:4000",
				source: "the address you entered on another machine",
			}
			u.web.bar.sync()

			detail := sampleDetail(t)
			detail.Description = strings.Repeat("A description long enough that it has to wrap at every supported window size. ", 4)

			u.web.templates = []templates.TemplateData{detail.TemplateData}
			u.web.fill()
			u.web.setStatus("Cannot load what is public on http://a-very-long-server-name.example.com:4000 — the server did not answer, or that address is not an eznit instance.", statusError)

			layOut(u.window, size)
			assertNoTextOverlap(t, u.window.Content())
			assertTextFits(t, u.window.Content())

			page := newTemplatePage(u, u.web.setStatus)
			u.web.pages.push(page.content(detail.Name, u.web.pages.back))
			page.show(detail)

			layOut(u.window, size)
			assertNoTextOverlap(t, u.window.Content())
			assertTextFits(t, u.window.Content())
		})
	}
}

// TestEntryRowTextDoesNotOverlap: a row's name and info end in an ellipsis
// rather than run under its Download button. A label that does not truncate
// asks for the width of its whole text, so the measure is the row's minimum: it
// must not grow with the name, or one long file name widens the window.
func TestEntryRowTextDoesNotOverlap(t *testing.T) {
	_ = newTestUI(t)

	short := newEntryRow()
	short.set(entry{kind: entryFile, name: "a.txt", info: "File · version 1 · private"}, func() {})

	long := newEntryRow()
	long.set(entry{
		kind: entryFile,
		name: strings.Repeat("a-very-long-file-name-", 8) + ".txt",
		info: "File · version 12 · shared by " + strings.Repeat("someone-with-a-long-address", 4) + "@example.com",
	}, func() {})

	if got, want := long.MinSize().Width, short.MinSize().Width; got > want+0.5 {
		t.Errorf("a long name makes the row ask for %.0f px, a short one %.0f: the text is not truncated", got, want)
	}

	for _, width := range []float32{320, 640} {
		long.Resize(fyne.NewSize(width, long.MinSize().Height))
		assertNoTextOverlap(t, test.WidgetRenderer(long).Objects()[0])
	}
}

// TestServerTabTextDoesNotOverlap is the reported defect: the health line, the
// folder path and the note all lived where they could be drawn over the row
// below or over the buttons beside them.
func TestServerTabTextDoesNotOverlap(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\nDB_HOST=\"127.0.0.1\"\nDB_PORT=\"5432\"\nDB_USER=\"root\"\nDB_NAME=\"eznit\"\n")

	for _, size := range windowSizes() {
		t.Run(fmt.Sprintf("%.0fx%.0f", size.Width, size.Height), func(t *testing.T) {
			view := newTestServerView(t, dir)
			layOut(view.ui.window, size)

			view.folder.SetText("C:/Users/someone/Documents/a very deep folder name that will not fit on one line/eznit")
			view.health.SetText("Not answering at http://127.0.0.1:4000 yet — see the log for what the server said")
			view.setNote("The server could not start: it could not reach the database at 127.0.0.1:5432, so there is nothing listening on the API port yet.")
			layOut(view.ui.window, size)

			assertNoTextOverlap(t, view.built)
			assertTextFits(t, view.built)

			// The note is the header's last row: it must end above the panes,
			// not be drawn over them.
			note, ok := placeOf(view.built, view.note)
			if !ok {
				t.Fatal("the note is not in the laid-out tree")
			}

			panes, ok := placeOf(view.built, view.panes)
			if !ok {
				t.Fatal("the tab panes are not in the laid-out tree")
			}

			if note.pos.Y+note.size.Height > panes.pos.Y+0.5 {
				t.Errorf("the note ends at %.1f but the panes begin at %.1f", note.pos.Y+note.size.Height, panes.pos.Y)
			}
		})
	}
}

// TestServerConfigurationTextDoesNotOverlap is the other half of the same
// defect: the password state is the action row's middle, and a long one used to
// run under the buttons.
func TestServerConfigurationTextDoesNotOverlap(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\nDB_HOST=\"127.0.0.1\"\nDB_PORT=\"5432\"\nDB_USER=\"root\"\nDB_NAME=\"eznit\"\n")

	for _, size := range windowSizes() {
		t.Run(fmt.Sprintf("%.0fx%.0f", size.Width, size.Height), func(t *testing.T) {
			view := newTestServerView(t, dir)
			view.panes.SelectIndex(1)
			layOut(view.ui.window, size)

			view.secret.SetText("Database password: set in secrets/db_password.txt (mode -rw-------), and the server has not read it since the file changed")
			relayout(view.configScrollPane)
			layOut(view.ui.window, size)

			assertNoTextOverlap(t, view.configScrollPane)
			assertNoTextOverlap(t, view.configScroll.Content)
			assertTextFits(t, view.configScrollPane)
		})
	}
}

// TestStatusDotsAreCentred keeps the dot on the text's baseline rather than
// above it, on every tab that has one.
func TestStatusDotsAreCentred(t *testing.T) {
	size := fyne.NewSize(980, 640)

	t.Run("account", func(t *testing.T) {
		view := newTestAccountView(t, apiChoice{url: "http://127.0.0.1:4000", source: "the default"})
		view.setStatus("3 file(s) on http://127.0.0.1:4000.", statusOK)
		layOut(view.ui.window, size)

		assertDotCentredOn(t, view.ui.window.Content(), view.dot, view.status)
	})

	t.Run("web", func(t *testing.T) {
		u := newTestUI(t)
		u.window.SetContent(u.web.built)
		u.web.setStatus("4 template(s) on http://127.0.0.1:4000.", statusOK)
		layOut(u.window, size)

		assertDotCentredOn(t, u.window.Content(), u.web.dot, u.web.status)
	})

	t.Run("server", func(t *testing.T) {
		dir := bundleDir(t, "API_PORT=\"4000\"\n")
		view := newTestServerView(t, dir)
		layOut(view.ui.window, size)

		assertDotCentredOn(t, view.built, view.dot, view.status)
	})
}

// TestServerHeaderActionsShareOneEdge is the "organised and aligned" guard: the
// two header rows group their buttons differently, but the buttons share one
// height and each row ends at the same right edge.
func TestServerHeaderActionsShareOneEdge(t *testing.T) {
	view := newTestServerView(t, bundleDir(t, "API_PORT=\"4000\"\n"))
	layOut(view.ui.window, fyne.NewSize(980, 640))

	buttons := []*widget.Button{view.start, view.stop, view.open, view.use, view.choose, view.reset}

	statusRow, ok := placeOf(view.built, view.open)
	if !ok {
		t.Fatal("the status row's actions are not laid out")
	}

	for _, button := range buttons {
		box, ok := placeOf(view.built, button)
		if !ok {
			t.Fatalf("the %q button is not in the laid-out header", button.Text)
		}

		if delta := box.size.Height - statusRow.size.Height; delta > 1 || delta < -1 {
			t.Errorf("%q is %.1f px taller or shorter than the row", button.Text, delta)
		}
	}

	// Each row's last button is that row's right edge; the two must agree.
	right := statusRow.pos.X + statusRow.size.Width

	folderRow, ok := placeOf(view.built, view.reset)
	if !ok {
		t.Fatal("the folder row's actions are not laid out")
	}

	if gap := folderRow.pos.X + folderRow.size.Width - right; gap > 1 || gap < -1 {
		t.Errorf("the folder row ends %.1f px from the status row's right edge", gap)
	}
}
