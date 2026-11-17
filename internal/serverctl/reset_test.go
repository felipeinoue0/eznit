package serverctl

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeUploads sets API_UPLOADS in the folder's configuration and puts a user's
// file under it.
func writeUploads(t *testing.T, dir, setting, under string) string {
	t.Helper()

	env := "DB_HOST=\"127.0.0.1\"\nDB_PORT=\"55432\"\nAPI_UPLOADS=\"" + setting + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, EnvFileName), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(under, "user-1", "file-1", "1_notes.txt")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	return file
}

// A reset empties the database through the folder's migrate service and the
// uploads inside the folder, and keeps the configuration and the password.
func TestResetEmptiesTheDatabaseAndTheUploads(t *testing.T) {
	dir := bundleFor(t, true, true)
	file := writeUploads(t, dir, "./uploads", filepath.Join(dir, "uploads"))
	logFile := fakeRuntime(t, "docker", 0)

	p, _, _ := newPreparer(t, dir, func(int) bool { return true })

	result, err := p.reset()
	if err != nil {
		t.Fatalf("reset() = %v", err)
	}

	if !result.Dropped || result.Removed != 1 || result.UploadsKept {
		t.Errorf("reset() = %+v, want the database dropped and one upload entry removed", result)
	}

	got := commands(t, logFile)
	if len(got) != 1 || !strings.HasPrefix(got[0], "compose run --rm --entrypoint sh migrate -c ") || !strings.Contains(got[0], "drop -f") {
		t.Errorf("the runtime was asked for %q, want one drop through migrate", got)
	}
	for _, line := range got {
		if strings.Contains(line, "down") || strings.Contains(line, "volume") {
			t.Errorf("the reset touched volumes: %q", line)
		}
	}

	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Error("the uploaded file is still there")
	}
	if info, err := os.Stat(filepath.Join(dir, "uploads")); err != nil || !info.IsDir() {
		t.Error("the uploads folder itself was removed; the API expects it")
	}
	for _, kept := range []string{EnvFileName, filepath.FromSlash(SecretPath)} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Errorf("%s was not kept: %v", kept, err)
		}
	}
}

// A database that is not running is started first: it must answer to be
// emptied.
func TestResetStartsTheDatabaseFirst(t *testing.T) {
	dir := bundleFor(t, true, true)
	logFile := fakeRuntime(t, "docker", 0)

	p, _, _ := newPreparer(t, dir, func(call int) bool { return call > 1 })

	if _, err := p.reset(); err != nil {
		t.Fatalf("reset() = %v", err)
	}

	got := commands(t, logFile)
	if len(got) != 2 || got[0] != "compose up -d db" || !strings.Contains(got[1], "drop -f") {
		t.Errorf("the runtime was asked for %q, want up, then drop", got)
	}
}

// Uploads outside the folder are never deleted: the reset only erases what
// belongs to the folder it was pointed at.
func TestResetKeepsUploadsOutsideTheFolder(t *testing.T) {
	dir := bundleFor(t, true, true)
	outside := t.TempDir()
	file := writeUploads(t, dir, outside, outside)
	fakeRuntime(t, "docker", 0)

	p, _, _ := newPreparer(t, dir, func(int) bool { return true })

	result, err := p.reset()
	if err != nil {
		t.Fatalf("reset() = %v", err)
	}
	if !result.UploadsKept || result.Removed != 0 {
		t.Errorf("reset() = %+v, want the outside folder kept", result)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("a file outside the folder was deleted: %v", err)
	}
}

// The folder itself, or its parent, is never treated as the uploads folder.
func TestUploadsFolderMustBeInside(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		setting string
		inside  bool
	}{
		{"./uploads", true},
		{"uploads/deeper", true},
		{".", false},
		{"..", false},
		{"../elsewhere", false},
		{"./uploads/../..", false},
	}

	for _, tt := range tests {
		env := &EnvFile{}
		env.Set("API_UPLOADS", tt.setting)

		_, inside, err := uploadsFolder(dir, env)
		if err != nil {
			t.Fatal(err)
		}
		if inside != tt.inside {
			t.Errorf("API_UPLOADS=%q inside = %v, want %v", tt.setting, inside, tt.inside)
		}
	}
}

// What cannot be reset safely is refused before anything runs.
func TestResetRefusesWhatItCannotDoSafely(t *testing.T) {
	logFile := fakeRuntime(t, "docker", 0)

	remote := bundleFor(t, true, true)
	if err := os.WriteFile(filepath.Join(remote, EnvFileName), []byte("DB_HOST=\"db.example.com\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	empty := t.TempDir()

	tests := []struct {
		name string
		dir  string
		want error
	}{
		{"no configuration", empty, ErrNoConfiguration},
		{"a database elsewhere", remote, ErrRemoteDatabase},
		{"no compose file", bundleFor(t, false, true), ErrNoCompose},
		{"no password", bundleFor(t, true, false), ErrNoPassword},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _, _ := newPreparer(t, tt.dir, func(int) bool { return true })

			if _, err := p.reset(); !errors.Is(err, tt.want) {
				t.Errorf("reset() = %v, want %v", err, tt.want)
			}
		})
	}

	if got := commands(t, logFile); len(got) != 0 {
		t.Errorf("a refused reset still ran %q", got)
	}
}

// A failed drop is reported, and nothing else is erased after it.
func TestResetReportsAFailedDrop(t *testing.T) {
	dir := bundleFor(t, true, true)
	file := writeUploads(t, dir, "./uploads", filepath.Join(dir, "uploads"))
	fakeRuntime(t, "docker", 1)

	p, _, _ := newPreparer(t, dir, func(int) bool { return true })

	if _, err := p.reset(); err == nil {
		t.Fatal("a failed drop reported success")
	}
	if _, err := os.Stat(file); err != nil {
		t.Error("the uploads were erased although the database was not")
	}
}
