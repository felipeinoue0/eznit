//go:build fyne

package fyneui

// background runs fn off the UI goroutine and keeps the window's tally open
// until fn returns. fn still does its widget work through fyne.Do; what is new
// is that the whole piece — the request and the UI work it ends in — can be
// waited for.
//
// That is what a test needs, because the test driver runs fyne.Do *inline on
// the calling goroutine*: a worker's widget work happens on the worker's
// goroutine, and can overlap the test goroutine reading the same widgets.
// Settling before reading removes the overlap, and with it the reports that
// were only a test's own timing (#333).
func (u *ui) background(fn func()) {
	u.work.Add(1)

	go func() {
		defer u.work.Done()

		fn()
	}()
}

// settle blocks until every piece of background work has finished.
//
// It is for tests: nothing on the UI goroutine may call it while work is in
// flight, because that would be work waiting for itself. The one background
// loop that never ends — the Server tab's health poll — is deliberately not
// counted here, or settling would never return.
func (u *ui) settle() {
	u.work.Wait()
}
