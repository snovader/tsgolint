package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/go-json-experiment/json"

	"github.com/microsoft/TypeScript/tsc/shim/ast"
	"github.com/microsoft/TypeScript/tsc/shim/core"
	"github.com/microsoft/TypeScript/tsc/shim/tspath"
	"github.com/typescript-eslint/tsgolint/internal/diagnostic"
	"github.com/typescript-eslint/tsgolint/internal/linter"
	"github.com/typescript-eslint/tsgolint/internal/rule"
	"github.com/typescript-eslint/tsgolint/internal/utils"
)

type headlessOptions struct {
	traceOut       string
	cpuprofOut     string
	heapOut        string
	allocsOut      string
	fix            bool
	fixSuggestions bool
	debugTimings   bool
}

var suppressProgramDiagnostics = sync.OnceValue(func() bool {
	return os.Getenv("OXLINT_TSGOLINT_DANGEROUSLY_SUPPRESS_PROGRAM_DIAGNOSTICS") == "true"
})

func parseHeadlessOptions(args []string) (*headlessOptions, error) {
	var opts headlessOptions
	var debug string

	flag.StringVar(&opts.traceOut, "trace", "", "file to put trace to")
	flag.StringVar(&opts.cpuprofOut, "cpuprof", "", "file to put cpu profiling to")
	flag.StringVar(&opts.heapOut, "heap", "", "file to put heap profiling to")
	flag.StringVar(&opts.allocsOut, "allocs", "", "file to put allocs profiling to")
	flag.BoolVar(&opts.fix, "fix", false, "generate fixes for code problems")
	flag.BoolVar(&opts.fixSuggestions, "fix-suggestions", false, "generate suggestions for code problems")
	flag.StringVar(&debug, "debug", "", "enable debug output options")

	if err := flag.CommandLine.Parse(args); err != nil {
		return nil, err
	}

	debugTimings, err := parseDebugTimings(debug)
	if err != nil {
		return nil, err
	}
	opts.debugTimings = debugTimings

	return &opts, nil
}

type headlessRange struct {
	Pos int `json:"pos"`
	End int `json:"end"`
}

func headlessRangeFromRange(r core.TextRange) *headlessRange {
	if !r.IsValid() {
		return nil
	}
	return &headlessRange{
		Pos: r.Pos(),
		End: r.End(),
	}
}

type headlessRuleMessage struct {
	Id          string `json:"id"`
	Description string `json:"description"`
	Help        string `json:"help,omitempty"`
}

func headlessRuleMessageFromRuleMessage(msg rule.RuleMessage) headlessRuleMessage {
	return headlessRuleMessage{
		Id:          msg.Id,
		Description: msg.Description,
		Help:        msg.Help,
	}
}

type headlessFix struct {
	Text  string        `json:"text"`
	Range headlessRange `json:"range"`
}
type headlessSuggestion struct {
	Message headlessRuleMessage `json:"message"`
	Fixes   []headlessFix       `json:"fixes"`
}

func headlessFixesFromRuleFixes(fixes []rule.RuleFix) []headlessFix {
	headlessFixes := make([]headlessFix, len(fixes))
	for i, fix := range fixes {
		headlessFixes[i] = headlessFix{
			Text:  fix.Text,
			Range: *headlessRangeFromRange(fix.Range),
		}
	}
	return headlessFixes
}

// Diagnostic kind discriminator
type headlessDiagnosticKind uint8

const (
	headlessDiagnosticKindRule headlessDiagnosticKind = iota
	headlessDiagnosticKindTsconfig
)

// A labeled span of source code. Useful for highlighting additional info related to a diagnostic in the context
// of the source code.
type headlessLabeledRange struct {
	// The text label associated with this range.
	Label string `json:"label"`
	// The range in the source file that this label applies to.
	Range headlessRange `json:"range"`
}

type headlessDiagnostic struct {
	Kind          headlessDiagnosticKind `json:"kind"`
	Range         *headlessRange         `json:"range,omitempty"`
	Message       headlessRuleMessage    `json:"message"`
	FilePath      *string                `json:"file_path"`
	LabeledRanges []headlessLabeledRange `json:"labeled_ranges,omitempty"`

	// Only for kind="rule"
	Rule        *string              `json:"rule,omitempty"`
	Fixes       []headlessFix        `json:"fixes,omitempty"`
	Suggestions []headlessSuggestion `json:"suggestions,omitempty"`
}

type headlessMessageType uint8

const (
	headlessMessageTypeError headlessMessageType = iota
	headlessMessageTypeDiagnostic
	headlessMessageTypeTiming
)

type headlessMessagePayloadError struct {
	Error string `json:"error"`
}

type headlessTimingPayload struct {
	Rules []headlessRuleTiming `json:"rules"`
}

type headlessRuleTiming struct {
	RuleName string `json:"rule_name"`
	Duration uint64 `json:"duration"`
	Calls    uint64 `json:"calls"`
}

func headlessTimingPayloadFromRecords(records []linter.RuleTimingRecord) headlessTimingPayload {
	rules := make([]headlessRuleTiming, len(records))
	for i, record := range records {
		rules[i] = headlessRuleTiming{
			RuleName: record.RuleName,
			Duration: uint64(record.Duration),
			Calls:    record.Calls,
		}
	}
	return headlessTimingPayload{Rules: rules}
}

// Unified diagnostic type for channel
type anyDiagnostic struct {
	ruleDiagnostic     *rule.RuleDiagnostic
	internalDiagnostic *diagnostic.Internal
}

func ruleToAny(d rule.RuleDiagnostic) anyDiagnostic {
	return anyDiagnostic{ruleDiagnostic: &d}
}

func internalToAny(d diagnostic.Internal) anyDiagnostic {
	return anyDiagnostic{internalDiagnostic: &d}
}

func writeMessage(w io.Writer, messageType headlessMessageType, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var header [5]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(payloadBytes)))
	header[4] = byte(messageType)
	w.Write(header[:])
	w.Write(payloadBytes)
	return nil
}

func writeErrorMessage(text string) error {
	return writeMessage(os.Stdout, headlessMessageTypeError, headlessMessagePayloadError{
		Error: text,
	})
}

func runHeadless(args []string) int {
	logLevel := utils.GetLogLevel()
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	if logLevel == utils.LogLevelDebug {
		log.Printf("Starting tsgolint")
	}

	opts, err := parseHeadlessOptions(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing options: %v", err)
		return 1
	}

	if cleanup, err := setupProfiling(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error starting profiling: %v", err)
		return 1
	} else {
		defer cleanup()
	}

	cwd, err := os.Getwd()
	if err != nil {
		writeErrorMessage(fmt.Sprintf("error getting current directory: %v", err))
		return 1
	}

	jsonPayload, err := io.ReadAll(os.Stdin)
	if err != nil {
		writeErrorMessage(fmt.Sprintf("error reading from stdin: %v", err))
		return 1
	}

	payload, err := deserializePayload(jsonPayload)
	if err != nil {
		writeErrorMessage(fmt.Sprintf("error parsing config: %v", err))
		return 1
	}

	projects := prepareHeadlessProjects(tspath.NormalizePath(cwd), payload, logLevel == utils.LogLevelDebug)

	allRulesByName := make(map[string]rule.Rule, len(allRules))
	for _, r := range allRules {
		allRulesByName[r.Name] = r
	}

	var wg sync.WaitGroup

	diagnosticsChan := make(chan anyDiagnostic, 4096)

	// Handle all diagnostics
	wg.Go(func() {
		w := bufio.NewWriterSize(os.Stdout, 4096*100)
		defer w.Flush()
		for d := range diagnosticsChan {
			var hd headlessDiagnostic

			if d.ruleDiagnostic != nil {
				// Rule diagnostic
				rd := d.ruleDiagnostic
				filePath := rd.SourceFile.FileName().AsString()
				hd = headlessDiagnostic{
					Kind:          headlessDiagnosticKindRule,
					Range:         headlessRangeFromRange(rd.Range),
					Rule:          &rd.RuleName,
					Message:       headlessRuleMessageFromRuleMessage(rd.Message),
					Fixes:         nil,
					Suggestions:   nil,
					FilePath:      &filePath,
					LabeledRanges: nil,
				}

				if len(rd.LabeledRanges) > 0 {
					hd.LabeledRanges = make([]headlessLabeledRange, len(rd.LabeledRanges))
					for i, labeledRange := range rd.LabeledRanges {
						hd.LabeledRanges[i] = headlessLabeledRange{
							Label: labeledRange.Label,
							Range: *headlessRangeFromRange(labeledRange.Range),
						}
					}
				}

				if opts.fix {
					hd.Fixes = headlessFixesFromRuleFixes(rd.Fixes())
				}
				if opts.fixSuggestions {
					suggestions := rd.GetSuggestions()
					hd.Suggestions = make([]headlessSuggestion, len(suggestions))
					for i, suggestion := range suggestions {
						hd.Suggestions[i] = headlessSuggestion{
							Message: headlessRuleMessageFromRuleMessage(suggestion.Message),
							Fixes:   headlessFixesFromRuleFixes(suggestion.Fixes()),
						}
					}
				}
			} else if d.internalDiagnostic != nil {
				// Internal diagnostic (tsconfig, type error, etc.)
				internalDiagnostic := d.internalDiagnostic

				hd = headlessDiagnostic{
					Kind:  headlessDiagnosticKindTsconfig,
					Range: headlessRangeFromRange(internalDiagnostic.Range),
					Rule:  nil, // Internal diagnostics don't have a rule
					Message: headlessRuleMessage{
						Id:          internalDiagnostic.Id,
						Description: internalDiagnostic.Description,
						Help:        internalDiagnostic.Help,
					},
					Fixes:       nil,
					Suggestions: nil,
					FilePath:    internalDiagnostic.FilePath,
				}
			}

			writeMessage(w, headlessMessageTypeDiagnostic, hd)
			if w.Available() < 4096 {
				w.Flush()
			}
		}
	})

	var timingStore *linter.RuleTimingStore
	if opts.debugTimings {
		timingStore = linter.NewRuleTimingStore()
	}

	if logLevel == utils.LogLevelDebug {
		log.Printf("Running Linter")
	}

	for _, project := range projects {
		err = linter.RunLinter(linter.RunLinterOptions{
			LogLevel:         logLevel,
			CurrentDirectory: project.currentDirectory,
			Workload:         project.workload,
			Workers:          runtime.GOMAXPROCS(0),
			FS:               project.fs,
			GetRulesForFile: func(sourceFile *ast.SourceFile) []linter.ConfiguredRule {
				return project.configuredRules(sourceFile, allRulesByName)
			},
			OnRuleDiagnostic:     func(d rule.RuleDiagnostic) { diagnosticsChan <- ruleToAny(d) },
			OnInternalDiagnostic: func(d diagnostic.Internal) { diagnosticsChan <- internalToAny(d) },
			Fixes: linter.Fixes{
				Fix:            opts.fix,
				FixSuggestions: opts.fixSuggestions,
			},
			TypeErrors: linter.TypeErrors{
				ReportSyntactic: payload.ReportSyntactic,
				ReportSemantic:  payload.ReportSemantic,
			},
			SuppressProgramDiagnostics: suppressProgramDiagnostics(),
			TimingStore:                timingStore,
		})

		if err != nil {
			break
		}
	}

	close(diagnosticsChan)
	if err != nil {
		log.Printf("ERROR: Linter failed: %v", err)
		writeErrorMessage(fmt.Sprintf("error running linter: %v", err))
		return 1
	}

	wg.Wait()

	if opts.debugTimings {
		if err := writeMessage(os.Stdout, headlessMessageTypeTiming, headlessTimingPayloadFromRecords(timingStore.Collect())); err != nil {
			log.Printf("ERROR: failed to write timing output: %v", err)
			return 1
		}
	}

	if logLevel == utils.LogLevelDebug {
		log.Printf("Linting Complete")
	}

	return 0
}
