package main

import (
	"github.com/microsoft/TypeScript/tsc/shim/pnp"
	"github.com/microsoft/TypeScript/tsc/shim/vfs"
	"github.com/microsoft/TypeScript/tsc/shim/vfs/osvfs"
	"github.com/microsoft/TypeScript/tsc/shim/vfs/pnpvfs"
)

func createFileSystem(currentDirectory string) vfs.FS {
	fs := osvfs.FS()
	// PnP archives require case-sensitive identities. Preserve the host filesystem's
	// casing behavior when running in a regular node_modules project.
	if pnp.InitPnpApi(fs, currentDirectory) != nil {
		return pnpvfs.From(fs)
	}
	return fs
}
