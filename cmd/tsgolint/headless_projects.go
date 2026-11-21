package main

import (
	"fmt"
	"log"
	"slices"

	"github.com/microsoft/TypeScript/tsc/shim/ast"
	"github.com/microsoft/TypeScript/tsc/shim/tspath"
	"github.com/typescript-eslint/tsgolint/internal/linter"
	"github.com/typescript-eslint/tsgolint/internal/rule"
	"github.com/typescript-eslint/tsgolint/internal/utils"
)

type headlessProject struct {
	*projectFileSystem
	files       []string
	fileConfigs map[tspath.PathKey][]headlessRule
	workload    linter.Workload
}

func (p *headlessProject) configuredRules(sourceFile *ast.SourceFile, allRules map[string]rule.Rule) []linter.ConfiguredRule {
	config := p.fileConfigs[sourceFile.PathKey()]
	rules := make([]linter.ConfiguredRule, len(config))
	for i, configured := range config {
		r, ok := allRules[configured.Name]
		if !ok {
			panic(fmt.Sprintf("unknown rule: %v", configured.Name))
		}
		rules[i] = linter.ConfiguredRule{
			Name: r.Name,
			Run: func(ctx rule.RuleContext) rule.RuleListeners {
				return r.Run(ctx, configured.Options)
			},
		}
	}
	return rules
}

func prepareHeadlessProjects(cwd string, payload *headlessPayload, debug bool) []*headlessProject {
	fileSystems := newProjectFileSystems(cwd, payload.SourceOverrides)
	byScope := make(map[*projectFileSystem]*headlessProject)
	var projects []*headlessProject
	cwdPath := tspath.RootedDirectoryPathFromAbsolute(cwd)
	for _, config := range payload.Configs {
		for _, filePath := range config.FilePaths {
			file := tspath.ToRootedFilePath(tspath.NormalizeSlashes(filePath), cwdPath)
			scope := fileSystems.forFile(file)
			project := byScope[scope]
			if project == nil {
				project = &headlessProject{
					projectFileSystem: scope,
					fileConfigs:       make(map[tspath.PathKey][]headlessRule),
					workload:          linter.Workload{Programs: make(map[string][]string)},
				}
				byScope[scope] = project
				projects = append(projects, project)
			}
			project.files = append(project.files, file.AsString())
			project.fileConfigs[scope.fs.CaseSensitivity().PathKey(file.AsPath())] = config.Rules
		}
	}

	for _, project := range projects {
		project.assignFiles(debug)
	}
	return projects
}

func (p *headlessProject) assignFiles(debug bool) {
	resolver := utils.NewTsConfigResolver(p.fs, p.currentDirectory)
	for file, tsconfig := range resolver.FindTsConfigParallel(p.files) {
		if tsconfig == "" {
			p.workload.UnmatchedFiles = append(p.workload.UnmatchedFiles, file)
		} else {
			p.workload.Programs[tsconfig] = append(p.workload.Programs[tsconfig], file)
		}
		if debug {
			log.Printf("Got tsconfig for file %s: %s", file, tsconfig)
		}
	}
	for _, files := range p.workload.Programs {
		slices.SortFunc(files, func(a, b string) int { return len(b) - len(a) })
	}
	if debug {
		log.Printf("Done assigning files in %s. Total programs: %d. Unmatched files: %d", p.currentDirectory, len(p.workload.Programs), len(p.workload.UnmatchedFiles))
	}
}
