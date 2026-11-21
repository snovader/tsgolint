package main

import (
	"github.com/microsoft/TypeScript/tsc/shim/bundled"
	"github.com/microsoft/TypeScript/tsc/shim/pnp"
	"github.com/microsoft/TypeScript/tsc/shim/tspath"
	"github.com/microsoft/TypeScript/tsc/shim/vfs"
	"github.com/microsoft/TypeScript/tsc/shim/vfs/cachedvfs"
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

type projectFileSystem struct {
	currentDirectory string
	fs               vfs.FS
}

// Each PnP installation needs its own discovery root and case-sensitive archive
// identities. Ordinary projects retain the host's filesystem casing behavior.
type projectFileSystems struct {
	base             vfs.FS
	currentDirectory string
	currentManifest  string
	pnpApi           *pnp.PnpApi
	overrides        map[string]string
	scopes           map[string]*projectFileSystem
	directories      map[string]*projectFileSystem
}

func newProjectFileSystems(currentDirectory string, overrides map[string]string) *projectFileSystems {
	fs := cachedvfs.From(osvfs.FS())
	api := pnp.InitPnpApi(fs, currentDirectory)
	manifest := ""
	if api != nil {
		manifest = api.GetManifestPath()
	}
	return &projectFileSystems{
		base:             fs,
		currentDirectory: currentDirectory,
		currentManifest:  manifest,
		pnpApi:           api,
		overrides:        overrides,
		scopes:           make(map[string]*projectFileSystem),
		directories:      make(map[string]*projectFileSystem),
	}
}

func (p *projectFileSystems) forFile(file tspath.RootedFilePath) *projectFileSystem {
	directory := tspath.GetDirectoryPath(file.AsString())
	if scope := p.directories[directory]; scope != nil {
		return scope
	}

	api := p.pnpApi
	if api == nil {
		api = pnp.InitPnpApi(p.base, directory)
		p.pnpApi = api
	} else {
		api = api.ForPath(directory)
	}

	manifest := ""
	root := p.currentDirectory
	if api != nil {
		manifestRoot := tspath.RootedDirectoryPathFromAbsolute(tspath.GetDirectoryPath(api.GetManifestPath()))
		// ForPath falls back to its original API when a file has no PnP scope.
		if tspath.CaseSensitive.ContainsFilePath(manifestRoot, file) {
			manifest = api.GetManifestPath()
			if manifest != p.currentManifest {
				root = manifestRoot.AsString()
			}
		}
	}
	if manifest == "" && p.currentManifest != "" {
		root = directory
	}

	scope := p.scopes[manifest]
	if scope == nil {
		fs := p.base
		if manifest != "" {
			fs = pnpvfs.From(fs)
		}
		if len(p.overrides) > 0 {
			fs = newOverlayFS(fs, p.overrides)
		}
		scope = &projectFileSystem{currentDirectory: root, fs: bundled.WrapFS(cachedvfs.From(fs))}
		p.scopes[manifest] = scope
	}
	p.directories[directory] = scope
	return scope
}
