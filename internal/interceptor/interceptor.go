// Package interceptor implements FR-2: Stream Interception.
//
// It inspects the executor.Result (exit code + captured stderr) and decides:
//   (a) whether twin should engage the LLM pipeline at all, and
//   (b) what kind of failure occurred, so FR-3 knows which parser to run.
package interceptor

import (
	"regexp"
	"strings"

	"twin/internal/executor"
)

// FailureKind classifies the type of error twin observed.
// FR-3 uses this to pick the right stderr parser.
type FailureKind string

const (
	KindGCC    FailureKind = "gcc"    // GCC / Clang / G++ compiler errors
	KindGo     FailureKind = "go"     // Go build / run errors
	KindPython FailureKind = "python" // Python tracebacks
	KindRust   FailureKind = "rust"   // Rust / cargo errors
	KindShell  FailureKind = "shell"  // Generic shell command failure (no source file)
	KindUnknown FailureKind = "unknown"
)

// ErrorEvent is the structured output of FR-2.
// It carries everything the context extractor (FR-3) needs to start parsing.
type ErrorEvent struct {
	// Kind is the classified failure type.
	Kind FailureKind

	// ExitCode is the child's exit status.
	ExitCode int

	// Stderr is the raw captured stderr bytes.
	Stderr []byte
}

// heuristic patterns to identify the tool that produced the error.
var (
	reGCC    = regexp.MustCompile(`(?m):\d+:\d+:\s+(error|warning|note):`)
	reGo     = regexp.MustCompile(`(?m)^(#\s+\S+|\S+\.go:\d+:\d+:)`)
	rePython = regexp.MustCompile(`(?m)^Traceback \(most recent call last\):`)
	reRust   = regexp.MustCompile(`(?m)^error(\[E\d+\])?:`)
)

// Intercept examines a completed executor.Result and returns an ErrorEvent
// if twin should intervene, or nil if the command succeeded / produced no
// actionable stderr.
//
// FR-2 contract: trigger on non-zero exit code AND non-empty stderr.
func Intercept(result *executor.Result) *ErrorEvent {
	if result.ExitCode == 0 || len(result.Stderr) == 0 {
		return nil
	}

	return &ErrorEvent{
		Kind:     classify(result.Stderr),
		ExitCode: result.ExitCode,
		Stderr:   result.Stderr,
	}
}

// classify inspects the raw stderr bytes and returns the most likely FailureKind.
// Order matters: more specific patterns are checked first.
func classify(stderr []byte) FailureKind {
	s := string(stderr)

	switch {
	case rePython.MatchString(s):
		return KindPython
	case reRust.MatchString(s) && strings.Contains(s, "cargo"):
		return KindRust
	case reGo.MatchString(s):
		return KindGo
	case reGCC.MatchString(s):
		return KindGCC
	default:
		return KindUnknown
	}
}
