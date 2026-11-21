package utils

import (
	"slices"
	"testing"

	"github.com/microsoft/TypeScript/tsc/shim/tspath"
	"github.com/microsoft/TypeScript/tsc/shim/vfs"
	"github.com/microsoft/TypeScript/tsc/shim/vfs/osvfs"
)

type cachedEntriesFS struct {
	vfs.FS
	entries vfs.Entries
}

func (fs *cachedEntriesFS) GetAccessibleEntries(tspath.RootedDirectoryPath) vfs.Entries {
	return fs.entries
}

func TestOverlayDirectoryEntriesRemainIndependent(t *testing.T) {
	files := make([]string, 1, 3)
	files[0] = "base.ts"
	directories := make([]string, 1, 3)
	directories[0] = "base"
	base := &cachedEntriesFS{FS: osvfs.FS(), entries: vfs.Entries{Files: files, Directories: directories}}
	first := NewOverlayVFS(base, map[string]string{"/project/first.ts": "", "/project/first/file.ts": ""})
	second := NewOverlayVFS(base, map[string]string{"/project/second.ts": "", "/project/second/file.ts": ""})

	entries := first.GetAccessibleEntries("/project")
	second.GetAccessibleEntries("/project")

	if !slices.Equal(entries.Files, []string{"base.ts", "first.ts"}) {
		t.Errorf("another overlay changed the first file listing: %v", entries.Files)
	}
	if !slices.Equal(entries.Directories, []string{"base", "first"}) {
		t.Errorf("another overlay changed the first directory listing: %v", entries.Directories)
	}
}
