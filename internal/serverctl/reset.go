package serverctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The reset's own refusals. Each is a sentence for a person: a reset that cannot
// be done safely is refused before anything is erased.
var (
	// ErrNoConfiguration is a folder with no .env: there is no server here to
	// reset, and inventing a configuration to erase would be absurd.
	ErrNoConfiguration = errors.New("this folder has no " + EnvFileName)

	// ErrRemoteDatabase is a configuration whose database is on another
	// machine. A reset erases this folder's own database only.
	ErrRemoteDatabase = errors.New("the database is not on this machine")
)

// dropScript empties the database through the folder's own migrate service: the
// same image, network, credentials and secret as the migrations, so the reset
// erases exactly the database the API reads. `drop -f` removes every table,
// type and the migration history; the next Start applies the migrations again.
//
// It is run with `sh -c` inside the container, so the password is read from
// the secret there and never passes through this process.
const dropScript = `exec /migrate -path /migrations -database "postgres://${DB_USER}:$(cat /run/secrets/db_password)@db:${DB_PORT}/${DB_NAME}?sslmode=disable" drop -f`

// ResetResult is what a reset erased, and what it left alone on purpose.
type ResetResult struct {
	// Dropped is true when the database was emptied.
	Dropped bool

	// Uploads is the upload folder, when the configuration names one.
	Uploads string
	// Removed is how many entries were deleted from it.
	Removed int
	// UploadsKept is true when the folder lies outside the server folder: it
	// is then left untouched, because a reset never deletes outside the folder
	// it was pointed at.
	UploadsKept bool
}

// Reset erases a local server's data so it starts from zero: every account,
// file and template. The configuration (.env) and the database password are
// kept, so Start brings the server back, empty. No container volume is removed
// — `down --volumes` would also take every other named volume the compose file
// declares.
//
// The server must be stopped first. ctx bounds the container runtime, and out
// receives its output as it arrives.
func Reset(ctx context.Context, dir string, out io.Writer) (ResetResult, error) {
	return preparer{
		ctx:   ctx,
		dir:   dir,
		out:   out,
		probe: 750 * time.Millisecond,
		wait:  time.Minute,
		poll:  250 * time.Millisecond,
		reach: reachable,
		look:  exec.LookPath,
	}.reset()
}

func (p preparer) reset() (ResetResult, error) {
	if p.ctx == nil {
		p.ctx = context.Background()
	}

	env, err := LoadEnv(filepath.Join(p.dir, EnvFileName))
	if errors.Is(err, os.ErrNotExist) {
		return ResetResult{}, ErrNoConfiguration
	}
	if err != nil {
		return ResetResult{}, err
	}

	address := databaseAddress(env)
	if !address.isLocal() {
		return ResetResult{}, fmt.Errorf("%w (%s)", ErrRemoteDatabase, address)
	}

	composeFile := p.composeFile()
	compose, hasRuntime := detectCompose(p.look)

	switch {
	case composeFile == "":
		return ResetResult{}, ErrNoCompose
	case !hasRuntime:
		return ResetResult{}, ErrNoRuntime
	case !p.hasSecret():
		return ResetResult{}, ErrNoPassword
	}

	environ := append(os.Environ(), env.ChildEnv()...)

	// The database has to answer to be emptied: bring the folder's own up if
	// it is not running.
	if !p.reach(address.host, address.port, p.probe) {
		if err := compose.run(p.ctx, p.dir, environ, p.out, "up", "-d", "db"); err != nil {
			return ResetResult{}, err
		}

		if !p.waitFor(address) {
			return ResetResult{}, fmt.Errorf("%w (%s)", ErrNoAnswer, address)
		}
	}

	if err := compose.run(p.ctx, p.dir, environ, p.out,
		"run", "--rm", "--entrypoint", "sh", "migrate", "-c", dropScript); err != nil {
		return ResetResult{}, err
	}

	result := ResetResult{Dropped: true}

	uploads, inside, err := uploadsFolder(p.dir, env)
	if err != nil {
		return result, err
	}

	result.Uploads = uploads
	if uploads == "" {
		return result, nil
	}
	if !inside {
		result.UploadsKept = true
		return result, nil
	}

	removed, err := emptyFolder(uploads)
	result.Removed = removed
	if err != nil {
		return result, fmt.Errorf("the database was emptied, but the uploads were not: %w", err)
	}

	return result, nil
}

// uploadsFolder resolves API_UPLOADS against the folder and reports whether it
// lies strictly inside it. An empty setting is "".
func uploadsFolder(dir string, env *EnvFile) (string, bool, error) {
	value := strings.TrimSpace(env.Value("API_UPLOADS", ""))
	if value == "" {
		return "", false, nil
	}

	root, err := filepath.Abs(dir)
	if err != nil {
		return "", false, err
	}

	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)

	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path, false, nil
	}

	inside := relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))

	return path, inside, nil
}

// emptyFolder deletes what is in a folder and keeps the folder itself, so the
// API finds it where its configuration says. A folder that does not exist is
// already empty.
func emptyFolder(path string) (int, error) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}

	return removed, nil
}
