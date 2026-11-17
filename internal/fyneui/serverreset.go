//go:build fyne

package fyneui

import (
	"context"
	"errors"
	"fmt"

	"fyne.io/fyne/v2"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/serverctl"
)

// resetQuestion is the confirmation a reset asks (#379): what is erased, what is
// kept, in which folder — in words a person can select and copy.
func resetQuestion(folder string, running bool) string {
	message := fmt.Sprintf("This erases everything the local server in\n%s\nholds: every account, every file and every template. It cannot be undone.\n\n"+
		"Kept: the configuration (.env) and the database password, so Start brings the server back, empty.", folder)

	if running {
		message += "\n\nThe server is running: it is stopped first."
	}

	return message
}

// askReset asks before the local server's data is erased.
func (v *serverView) askReset() {
	if v.preparing {
		v.report("The local server is busy with another step — wait for it to finish.", statusWarn)
		return
	}

	confirm(v.ui, "Reset the local server?", resetQuestion(v.folderPath(), v.running()), "Reset", func(confirmed bool) {
		if confirmed {
			v.resetData()
		}
	})
}

// resetData stops the server when it runs, then erases its data through
// serverctl.Reset, with the runtime's output in the step's modal.
func (v *serverView) resetData() {
	if v.preparing {
		return
	}

	v.preparing = true
	v.stepTitle = "Resetting the local server…"
	v.setNote("")
	v.refresh()

	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	progress := v.showProgress("Resetting the local server",
		"Stopping the server if it runs, then emptying its database and its uploads.", cancel)

	folder := v.folderPath()
	running := v.running()
	supervisor := v.server()

	stepDone := make(chan struct{})
	v.stepDone = stepDone

	v.ui.background(func() {
		defer close(stepDone)

		var result serverctl.ResetResult
		var err error

		if running {
			err = supervisor.Stop(stopTimeout)
		}
		if err == nil {
			result, err = v.wipe(ctx, folder, progress)
		}
		cancel()

		fyne.DoAndWait(func() { v.afterReset(folder, result, err, progress) })
	})
}

// afterReset says what the reset did, and logs out a session that belonged to
// the server it erased: its account no longer exists.
func (v *serverView) afterReset(folder string, result serverctl.ResetResult, err error, progress *databaseProgress) {
	progress.Close()
	v.preparing = false
	v.stepTitle = ""

	if err != nil {
		message := "The reset did not finish: " + err.Error()
		if errors.Is(err, context.Canceled) {
			message = "The reset was cancelled."
		}
		if result.Dropped {
			message += " The database was emptied already."
		}

		v.report(message, statusError)
		v.setNote(message)
		v.refresh()

		return
	}

	note := resetNote(folder, result)
	v.report(note, statusOK)
	v.setNote(note)
	v.refresh()

	// Last: logging out reloads Explore, which runs in the background.
	if v.ui.sessionOnLocalServer() {
		v.ui.endSession("The local server was reset, so your account there is gone. Create a new one.", false)
		v.ui.reloadExploreLast()
	}
}

// resetNote is the one line a finished reset gets.
func resetNote(folder string, result serverctl.ResetResult) string {
	note := "The local server in " + folder + " was reset: its database is empty"

	switch {
	case result.Uploads == "":
		note += "."
	case result.UploadsKept:
		note += ". The uploads folder " + result.Uploads + " is outside the server folder, so it was left as it is."
	default:
		note += fmt.Sprintf(" and %d item(s) were removed from its uploads.", result.Removed)
	}

	return note + " Start it to use it again."
}

// sessionOnLocalServer reports whether the window's session was issued by the
// local server — the one a reset erases.
func (u *ui) sessionOnLocalServer() bool {
	if u.client == nil || u.client.Token == "" {
		return false
	}

	return u.local || apiclient.NormalizeBaseURL(u.client.BaseURL) == u.localAddress()
}

// report writes Settings' status line, where the reset was asked for.
func (v *serverView) report(text string, kind statusKind) {
	if v.ui.settings != nil {
		v.ui.settings.setStatus(text, kind)
	}
}
