package templatefolder

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/templates"
)

// The walk is shared by the CLI and the desktop window; the CLI's own suite
// covers the limits in depth. This pins the shape both callers rely on:
// forward-slash paths, skipped dependency folders, and the text/binary split.
func TestCollect(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, data []byte, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
	}
	write("AGENTS.md", []byte("# {{PROJECT_NAME}}\n"), 0o644)
	write("scripts/setup.sh", []byte("#!/bin/sh\n"), 0o755)
	write("logo.bin", []byte{0x00, 0x01, 0xff}, 0o644)
	write("node_modules/dep/index.js", []byte("x"), 0o644)
	write(".git/HEAD", []byte("ref"), 0o644)

	files, skipped, err := Collect(root)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none", skipped)
	}

	byPath := map[string]string{}
	for _, file := range files {
		byPath[file.Path] = file.Kind
		if file.Path == "scripts/setup.sh" && runtime.GOOS != "windows" && !file.IsExecutable {
			t.Errorf("scripts/setup.sh lost its executable bit")
		}
	}
	want := map[string]string{
		"AGENTS.md":        templates.FileKindText,
		"scripts/setup.sh": templates.FileKindText,
		"logo.bin":         templates.FileKindBinary,
	}
	if len(byPath) != len(want) {
		t.Fatalf("collected %v, want exactly %v", byPath, want)
	}
	for path, kind := range want {
		if byPath[path] != kind {
			t.Errorf("%s kind = %q, want %q", path, byPath[path], kind)
		}
	}
}
