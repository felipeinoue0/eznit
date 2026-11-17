//go:build fyne

package fyneui

import (
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/templateapply"
)

// The tree is the folder a template writes: every folder a file sits in, nested
// as on disk, folders before files and each alphabetical.
func TestFolderTreeKeepsTheStructure(t *testing.T) {
	tree := newFolderTree([]templateapply.File{
		{Path: "z.md"},
		{Path: "src/app/main.go"},
		{Path: "src/app/util.go"},
		{Path: "src/README.md"},
		{Path: "a.md"},
		{Path: ".github/workflows/ci.yml"},
	})

	tests := []struct {
		node string
		want string
	}{
		{"", ".github,src,a.md,z.md"},
		{".github", ".github/workflows"},
		{".github/workflows", ".github/workflows/ci.yml"},
		{"src", "src/app,src/README.md"},
		{"src/app", "src/app/main.go,src/app/util.go"},
	}

	for _, tt := range tests {
		if got := strings.Join(tree.children[tt.node], ","); got != tt.want {
			t.Errorf("children of %q = %q, want %q", tt.node, got, tt.want)
		}
	}

	if !tree.isFolder("src/app") || tree.isFolder("src/app/main.go") {
		t.Error("a folder and a file are told apart wrongly")
	}
	if _, ok := tree.files["src/app/main.go"]; !ok {
		t.Error("a file is missing from the tree's files")
	}
}

// A preview shows text, names what it cannot show, and says when it stops.
func TestPreviewText(t *testing.T) {
	long := strings.Repeat("line\n", previewLines+10)

	tests := []struct {
		name    string
		content []byte
		more    bool
		want    string
		notWant string
	}{
		{"text", []byte("hello\nworld"), false, "hello\nworld", "stops here"},
		{"empty", nil, false, "empty", ""},
		{"binary", []byte{0x7f, 'E', 'L', 'F', 0, 1}, false, "binary", ""},
		{"invalid utf-8", []byte{0xff, 0xfe, 'a'}, false, "binary", ""},
		{"cut inside a character", append([]byte("caf"), 0xc3), true, "caf", "binary"},
		{"more on the server", []byte("start"), true, "stops here", ""},
		{"too many lines", []byte(long), false, "stops here", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := previewText(tt.content, tt.more)
			if !strings.Contains(got, tt.want) {
				t.Errorf("previewText() = %q, want it to contain %q", got, tt.want)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Errorf("previewText() = %q, want no %q", got, tt.notWant)
			}
		})
	}
}

// A visibility change says what it will do, in both directions.
func TestVisibilityQuestion(t *testing.T) {
	title, message, action := visibilityQuestion("template", "starter", true)
	if !strings.Contains(title, "public") || action != "Make public" || !strings.Contains(message, "without an account") {
		t.Errorf("to public: %q / %q / %q", title, message, action)
	}

	title, message, action = visibilityQuestion("file", "notes.txt", false)
	if !strings.Contains(title, "private") || action != "Make private" || !strings.Contains(message, "only you") {
		t.Errorf("to private: %q / %q / %q", title, message, action)
	}
}
