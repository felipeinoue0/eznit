//go:build fyne

package fyneui

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
)

// folderTree is a template as the folder it writes (#376): every directory a
// file sits in, so a person sees the structure the template keeps instead of a
// flat list of paths. Node ids are the slash paths; "" is the root.
type folderTree struct {
	children map[string][]string
	files    map[string]templateapply.File
}

// newFolderTree builds the tree from the files a template writes — its own
// files and its commands at their target paths, which is what Apply puts on
// disk.
func newFolderTree(files []templateapply.File) folderTree {
	tree := folderTree{
		children: map[string][]string{"": nil},
		files:    make(map[string]templateapply.File, len(files)),
	}

	for _, file := range files {
		clean := path.Clean(strings.TrimPrefix(file.Path, "/"))
		tree.files[clean] = file

		// Link the file to its folder, and each folder to its own, up to the
		// root. A folder already linked is skipped by contains.
		for child := clean; ; {
			parent := path.Dir(child)
			if parent == "." {
				parent = ""
			}

			if !contains(tree.children[parent], child) {
				tree.children[parent] = append(tree.children[parent], child)
			}

			if parent == "" {
				break
			}
			child = parent
		}
	}

	// Folders first, then files, each alphabetically: the order a file
	// manager uses.
	for id := range tree.children {
		kids := tree.children[id]
		sort.Slice(kids, func(i, j int) bool {
			iDir, jDir := tree.isFolder(kids[i]), tree.isFolder(kids[j])
			if iDir != jDir {
				return iDir
			}
			return kids[i] < kids[j]
		})
	}

	return tree
}

func contains(items []string, item string) bool {
	for _, existing := range items {
		if existing == item {
			return true
		}
	}
	return false
}

// isFolder reports whether a node holds others: every folder was created as
// some path's parent, and a file never is one.
func (t folderTree) isFolder(id string) bool {
	_, ok := t.children[id]
	return ok
}

// widget is the tree as Fyne draws it, every folder open, calling show with
// whichever file is selected.
func (t folderTree) widget(show func(file templateapply.File)) *widget.Tree {
	tree := widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID { return t.children[id] },
		t.isFolder,
		newTreeNode,
		t.updateNode,
	)

	tree.OnSelected = func(id widget.TreeNodeID) {
		if file, ok := t.files[id]; ok && show != nil {
			show(file)
		}
	}

	tree.OpenAllBranches()

	return tree
}

// newTreeNode is one row of the tree: an icon and the name.
func newTreeNode(bool) fyne.CanvasObject {
	label := widget.NewLabel("")
	label.Truncation = fyne.TextTruncateEllipsis

	return container.NewBorder(nil, nil, widget.NewIcon(theme.FileIcon()), nil, label)
}

// updateNode names a row and gives it the icon of what it is.
func (t folderTree) updateNode(id widget.TreeNodeID, branch bool, object fyne.CanvasObject) {
	row, ok := object.(*fyne.Container)
	if !ok || len(row.Objects) != 2 {
		return
	}

	label, _ := row.Objects[0].(*widget.Label)
	icon, _ := row.Objects[1].(*widget.Icon)
	if label == nil || icon == nil {
		return
	}

	label.SetText(path.Base(id))

	switch {
	case branch:
		icon.SetResource(theme.FolderIcon())
	case t.files[id].IsExecutable:
		icon.SetResource(theme.ComputerIcon())
	default:
		icon.SetResource(theme.FileIcon())
	}
}

// templateFilePreview is what the preview pane shows for one file of a
// template: its text, or a sentence for bytes that are not text.
func templateFilePreview(file templateapply.File) string {
	header := file.Path
	if file.IsExecutable {
		header += " (executable)"
	}

	if file.Kind == templates.FileKindBinary {
		return fmt.Sprintf("%s\n\nA binary file — there is no text to preview.", header)
	}

	return header + "\n\n" + previewText([]byte(file.Content), false)
}
