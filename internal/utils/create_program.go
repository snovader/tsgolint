package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/microsoft/TypeScript/tsc/shim/ast"
	"github.com/microsoft/TypeScript/tsc/shim/bundled"
	"github.com/microsoft/TypeScript/tsc/shim/compiler"
	"github.com/microsoft/TypeScript/tsc/shim/core"
	"github.com/microsoft/TypeScript/tsc/shim/tsoptions"
	"github.com/microsoft/TypeScript/tsc/shim/tspath"
	"github.com/microsoft/TypeScript/tsc/shim/vfs"
	"github.com/typescript-eslint/tsgolint/internal/diagnostic"
)

func CreateCompilerHost(cwd string, fs vfs.FS) compiler.CompilerHost {
	defaultLibraryPath := bundled.LibPath()
	return NewCompilerHost(cwd, fs, defaultLibraryPath, nil, nil)
}

func enhanceHelpDiagnosticMessage(msg string) string {
	if strings.Contains(msg, "Please remove it from your configuration.") {
		return msg + "\n" + "See https://github.com/oxc-project/tsgolint/issues/351 for more information."

	}
	return msg
}

func CreateProgram(singleThreaded bool, fs vfs.FS, cwd string, tsconfigPath string, host compiler.CompilerHost, suppressProgramDiagnostics bool) (*compiler.Program, []diagnostic.Internal, error) {
	resolvedConfigFile := tspath.ToRootedFilePath(tsconfigPath, tspath.RootedDirectoryPathFromAbsolute(cwd))
	resolvedConfigPath := resolvedConfigFile.AsString()
	if !fs.FileExists(resolvedConfigFile) {
		return nil, nil, fmt.Errorf("couldn't read tsconfig at %v", resolvedConfigPath)
	}

	configParseResult, diagnostics := tsoptions.GetParsedCommandLineOfConfigFile(resolvedConfigFile, &core.CompilerOptions{}, nil, tsoptions.NewDefaultParseConfigHost(fs), nil)

	if len(diagnostics) > 0 {
		internalDiags := make([]diagnostic.Internal, len(diagnostics))
		for i, d := range diagnostics {
			loc := d.Loc()
			filePath := resolvedConfigPath
			if d.File() != nil && d.File().FileName().AsString() != "" {
				filePath = d.File().FileName().AsString()
			}
			internalDiags[i] = diagnostic.Internal{
				Range:       core.NewTextRange(loc.Pos(), loc.End()),
				Id:          "tsconfig-error",
				Description: "Invalid tsconfig",
				Help:        GetDiagnosticMessage(d),
				FilePath:    &filePath,
			}
		}
		return nil, internalDiags, nil
	}

	configErrors := configParseDiagnostics(configParseResult)
	if len(configErrors) > 0 && !suppressProgramDiagnostics {
		internalDiags := make([]diagnostic.Internal, len(configErrors))
		for i, e := range configErrors {
			loc := e.Loc()
			filePath := resolvedConfigPath
			if e.File() != nil && e.File().FileName().AsString() != "" {
				filePath = e.File().FileName().AsString()
			}
			internalDiags[i] = diagnostic.Internal{
				Range:       core.NewTextRange(loc.Pos(), loc.End()),
				Id:          "tsconfig-error",
				Description: "Invalid tsconfig",
				Help:        GetDiagnosticMessage(e),
				FilePath:    &filePath,
			}
		}
		return nil, internalDiags, nil
	}

	opts := compiler.ProgramOptions{
		ProgramConfig: compiler.ProgramConfig{
			Config:                      configParseResult,
			SingleThreaded:              core.TSTrue,
			UseSourceOfProjectReference: true,
		},
		ProgramHosts: compiler.ProgramHosts{Host: host},
		// TODO: custom checker pool
		// CreateCheckerPool: func(p *compiler.Program) compiler.CheckerPool {},
	}
	if !singleThreaded {
		opts.SingleThreaded = core.TSFalse
	}
	program, err := compiler.NewProgram(context.Background(), opts)
	if err != nil {
		return nil, nil, err
	}
	if program == nil {
		return nil, nil, errors.New("couldn't create program")
	}

	program_diagnostics := program.GetProgramDiagnostics()
	if len(program_diagnostics) > 0 && !suppressProgramDiagnostics {
		// Convert ast.Diagnostic to diagnostic.Internal
		internalDiags := make([]diagnostic.Internal, len(program_diagnostics))
		for i, d := range program_diagnostics {
			loc := d.Loc()
			filePath := resolvedConfigPath
			if d.File() != nil && d.File().FileName().AsString() != "" {
				filePath = d.File().FileName().AsString()
			}
			internalDiags[i] = diagnostic.Internal{
				Range:       core.NewTextRange(loc.Pos(), loc.End()),
				Id:          "tsconfig-error",
				Description: "Invalid tsconfig",
				Help:        enhanceHelpDiagnosticMessage(GetDiagnosticMessage(d)),
				FilePath:    &filePath,
			}
		}
		return nil, internalDiags, nil
	}

	// TODO: report syntactic diagnostics?

	program.BindSourceFiles()

	return program, nil, nil
}

func CreateInferredProjectProgram(singleThreaded bool, fs vfs.FS, cwd string, host compiler.CompilerHost, fileNames []string) (*compiler.Program, []diagnostic.Internal, error) {
	opts := compiler.ProgramOptions{
		ProgramConfig: compiler.ProgramConfig{
			Config: tsoptions.NewParsedCommandLine(
				&core.CompilerOptions{
					AllowJs:                    core.TSTrue,
					Module:                     core.ModuleKindESNext,
					ModuleResolution:           core.ModuleResolutionKindBundler,
					Target:                     core.ScriptTargetES2022,
					Jsx:                        core.JsxEmitReactJSX,
					AllowImportingTsExtensions: core.TSTrue,
					StrictNullChecks:           core.TSTrue,
					StrictFunctionTypes:        core.TSTrue,
					SourceMap:                  core.TSTrue,
					ESModuleInterop:            core.TSTrue,
					AllowNonTsExtensions:       core.TSTrue,
					ResolveJsonModule:          core.TSTrue,
				},
				Map(fileNames, func(name string) tspath.RootedFilePath {
					return tspath.ToRootedFilePath(name, tspath.RootedDirectoryPathFromAbsolute(cwd))
				}),
				nil,
				tspath.RootedDirectoryPathFromAbsolute(cwd),
				fs.CaseSensitivity(),
			),
			SingleThreaded: core.TSTrue,
		},
		ProgramHosts: compiler.ProgramHosts{Host: host},
	}
	if !singleThreaded {
		opts.SingleThreaded = core.TSFalse
	}
	program, err := compiler.NewProgram(context.Background(), opts)
	if err != nil {
		return nil, nil, err
	}

	// TODO: report syntactic diagnostics?

	program.BindSourceFiles()
	return program, nil, nil
}

// Syntax errors are more useful than the config-validation errors they cause.
func configParseDiagnostics(config *tsoptions.ParsedCommandLine) []*ast.Diagnostic {
	if config.ConfigFile != nil {
		if diagnostics := config.ConfigFile.SourceFile.Diagnostics(); len(diagnostics) > 0 {
			return diagnostics[:1]
		}
	}
	return config.Errors
}
