// Package templatefolder turns a folder on disk into the files of a template:
// the walk, the limits and the text/binary split that `eznit template publish`
// and the desktop window's "Publish folder…" share, so the two cannot publish
// the same folder differently.
package templatefolder

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
	"github.com/fiwon123/eznit/pkg/helper"
)

// The limits of a template, the same ones the API enforces. The CLI and the
// desktop window check them too, so a file the server would refuse is refused
// here first, with the name of the file in the message.
const (
	// The text limits and the binary limits are separate: a text file is
	// configuration and small, a binary file is an image or a compiled
	// helper and is allowed to be bigger. The template as a whole is capped
	// once, counting both.
	MaxFileBytes      = 256 * 1024
	MaxTextTotalBytes = 4 * 1024 * 1024
	MaxBinaryBytes    = 10 * 1024 * 1024
	MaxTotalBytes     = 32 * 1024 * 1024
	MaxFiles          = 200
	MaxVariableName   = 64
)

// skipFolders are the folders a template never carries: the version control
// metadata and the dependency trees are not part of a project structure.
var skipFolders = map[string]bool{
	".git":          true,
	".svn":          true,
	".hg":           true,
	"node_modules":  true,
	"vendor":        true,
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".next":         true,
	".turbo":        true,
}

// Collect walks a folder and returns the files a template
// holds: the text files, with the path they take inside the project.
//
// The names of the files that were left out come back too: a file that is
// skipped has to be named, or the template is quietly incomplete.
func Collect(root string) ([]templateapply.File, []string, error) {
	var files []templateapply.File
	var skipped []string

	total := 0
	textTotal := 0

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			// The root itself is never skipped, only the folders
			// inside it.
			if path != root && skipFolders[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		// The path is stored with forward slashes: it is applied on
		// machines that are not this one.
		relative = filepath.ToSlash(relative)

		// A symlink is not followed: it can point outside the folder, and
		// a template is made of the files themselves. It is named, so the
		// author knows the folder holds more than the template does.
		if entry.Type()&fs.ModeSymlink != 0 {
			skipped = append(skipped, relative)
			return nil
		}

		safe, ok := helper.SafeRelPath(relative)
		if !ok {
			return fmt.Errorf("the path %q cannot be used in a template", relative)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		// Nothing in a template is bigger than the binary cap, so a file
		// over it is refused before it is read into memory.
		if info.Size() > MaxBinaryBytes {
			skipped = append(skipped, relative)
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// A binary file travels as base64 and its bytes are written to
		// disk by the server; a text file travels as it is. A file with
		// a NUL byte or a byte that is not valid UTF-8 is binary: a text
		// column would refuse it, and the trip would mangle it.
		if IsBinary(content) {
			total += len(content)
			if total > MaxTotalBytes {
				return fmt.Errorf("the folder holds more than %d bytes", MaxTotalBytes)
			}

			files = append(files, templateapply.File{
				Path:          safe,
				Kind:          templates.FileKindBinary,
				ContentBase64: base64.StdEncoding.EncodeToString(content),
				Bytes:         content,
				IsExecutable:  isExecutable(info.Mode()),
			})

			return nil
		}

		if len(content) > MaxFileBytes {
			skipped = append(skipped, relative)
			return nil
		}

		total += len(content)
		if total > MaxTotalBytes {
			return fmt.Errorf("the folder holds more than %d bytes", MaxTotalBytes)
		}

		textTotal += len(content)
		if textTotal > MaxTextTotalBytes {
			return fmt.Errorf("the folder holds more than %d bytes of text", MaxTextTotalBytes)
		}

		files = append(files, templateapply.File{
			Path:         safe,
			Kind:         templates.FileKindText,
			Content:      string(content),
			IsExecutable: isExecutable(info.Mode()),
		})

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	if len(files) > MaxFiles {
		return nil, nil, fmt.Errorf("the folder holds %d files, a template holds %d", len(files), MaxFiles)
	}

	sort.Strings(skipped)

	return files, skipped, nil
}

// IsBinary reports whether a content is not text: it holds a NUL byte, or a
// byte that is not valid UTF-8. Either one is refused by a text column.
func IsBinary(content []byte) bool {
	return bytes.IndexByte(content, 0) != -1 || !utf8.Valid(content)
}

// isExecutable reports whether a file mode says the file is executable.
func isExecutable(mode fs.FileMode) bool {
	return mode&0111 != 0
}
