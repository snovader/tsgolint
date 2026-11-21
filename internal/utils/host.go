package utils

import (
	"github.com/microsoft/TypeScript/tsc/shim/ast"
	"github.com/microsoft/TypeScript/tsc/shim/compiler"
	"github.com/microsoft/TypeScript/tsc/shim/core"

	"github.com/microsoft/TypeScript/tsc/shim/parser"
	"github.com/microsoft/TypeScript/tsc/shim/pnp"
	"github.com/microsoft/TypeScript/tsc/shim/tsoptions"
	"github.com/microsoft/TypeScript/tsc/shim/tspath"
	"github.com/microsoft/TypeScript/tsc/shim/vfs"
	"github.com/microsoft/TypeScript/tsc/shim/vfs/cachedvfs"
	"github.com/typescript-eslint/tsgolint/internal/collections"
)

var _ compiler.CompilerHost = (*compilerHost)(nil)

type compilerHost struct {
	compiler.CompilerHost
}

func NewCachedFSCompilerHost(currentDirectory string, fs vfs.FS, defaultLibraryPath tspath.RootedDirectoryPath, extendedConfigCache tsoptions.ExtendedConfigCache, trace func(msg *ast.DiagnosticsMessage, args ...any)) compiler.CompilerHost {
	return NewCompilerHost(currentDirectory, cachedvfs.From(fs), defaultLibraryPath, extendedConfigCache, trace)
}

func NewCompilerHost(currentDirectory string, fs vfs.FS, defaultLibraryPath tspath.RootedDirectoryPath, extendedConfigCache tsoptions.ExtendedConfigCache, trace func(msg *ast.DiagnosticsMessage, args ...any)) compiler.CompilerHost {
	return &compilerHost{compiler.NewCompilerHost(fs, defaultLibraryPath, extendedConfigCache, pnp.InitPnpApi(fs, currentDirectory), trace, nil)}
}

var sourceFileCache collections.SyncMap[SourceFileCacheKey, *ast.SourceFile]

type SourceFileCacheKey struct {
	opts       ast.SourceFileParseOptions
	text       string
	scriptKind core.ScriptKind
}

func GetSourceFileCacheKey(opts ast.SourceFileParseOptions, text string, scriptKind core.ScriptKind) SourceFileCacheKey {
	return SourceFileCacheKey{
		opts:       opts,
		text:       text,
		scriptKind: scriptKind,
	}
}

func (h *compilerHost) GetSourceFile(opts ast.SourceFileParseOptions) *ast.SourceFile {
	text, ok := h.FS().ReadFile(opts.FileName)
	if !ok {
		return nil
	}

	scriptKind := core.GetScriptKindFromFileName(opts.FileName)
	if scriptKind == core.ScriptKindUnknown {
		panic("Unknown script kind for file  " + opts.FileName.AsString())
	}

	key := GetSourceFileCacheKey(opts, text, scriptKind)

	if cached, ok := sourceFileCache.Load(key); ok {
		return cached
	}

	sourceFile := parser.ParseSourceFile(opts, text, scriptKind)
	result, _ := sourceFileCache.LoadOrStore(key, sourceFile)
	return result
}
