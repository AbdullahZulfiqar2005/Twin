// Package context implements FR-3: Context Extraction.
//
// It parses the stderr captured by FR-2 to find file paths and line numbers,
// reads those files, and assembles a Payload for the LLM (FR-4).
package context

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"twin/internal/interceptor"
)

// contextLines is the number of lines to include above and below an error line.
const contextLines = 10

// FileRef is a single file:line pair found inside stderr.
type FileRef struct {
	Path string
	Line int // 1-based; 0 means "file referenced but no specific line"
}

// Payload is the structured data sent to the LLM (FR-4).
type Payload struct {
	// Kind is the classified failure type forwarded from FR-2.
	Kind interceptor.FailureKind

	// Stderr is the full raw error output.
	Stderr string

	// Refs are all the file:line pairs parsed from stderr (deduplicated).
	Refs []FileRef

	// Snippets maps "path:line" → a windowed excerpt of the source file
	// centred around the error line.  Files that could not be read are omitted.
	Snippets map[string]string
}

// ── Regex patterns ────────────────────────────────────────────────────────────

var (
	// GCC / Clang:  path/to/file.c:12:5: error: ...
	// Intentionally excludes "note:" — note lines are secondary diagnostics
	// that repeat the same file:line as the primary error, causing duplicates.
	reGCC = regexp.MustCompile(`(?m)^([^:\s][^:]*\.[a-zA-Z]+):(\d+):\d+:\s+(?:error|warning):`)

	// Go build:     ./pkg/foo.go:34:12: undefined: Bar
	// [^\n:] prevents the pattern from consuming across line boundaries when a
	// "# pkg" comment line precedes the actual .go path on the next line.
	reGo = regexp.MustCompile(`(?m)^([^\n:\s][^\n:]*\.go):(\d+):\d+:`)

	// Python traceback:
	//   File "path/to/script.py", line 42, in something
	rePython = regexp.MustCompile(`(?m)^\s+File "([^"]+)", line (\d+)`)

	// Rust / cargo:
	//   --> src/main.rs:10:5
	reRust = regexp.MustCompile(`(?m)-->\s+([^:\s][^:]*\.rs):(\d+):\d+`)
)

// ── Public API ────────────────────────────────────────────────────────────────

// Extract is the FR-3 entry point.  It receives the ErrorEvent from FR-2,
// parses stderr for file references, reads those files, and returns a Payload.
func Extract(event *interceptor.ErrorEvent) (*Payload, error) {
	stderr := string(event.Stderr)

	refs, err := parse(event.Kind, stderr)
	if err != nil {
		return nil, fmt.Errorf("context: parse: %w", err)
	}

	snippets := readSnippets(refs)

	return &Payload{
		Kind:     event.Kind,
		Stderr:   stderr,
		Refs:     refs,
		Snippets: snippets,
	}, nil
}

// ── Parser dispatcher ─────────────────────────────────────────────────────────

func parse(kind interceptor.FailureKind, stderr string) ([]FileRef, error) {
	var raw []FileRef

	switch kind {
	case interceptor.KindGCC:
		raw = parseWithRegex(reGCC, stderr)
	case interceptor.KindGo:
		raw = parseWithRegex(reGo, stderr)
	case interceptor.KindPython:
		raw = parseWithRegex(rePython, stderr)
	case interceptor.KindRust:
		raw = parseWithRegex(reRust, stderr)
	default:
		// For shell / unknown errors there are no source files to read.
		// Return an empty slice — the LLM will work from stderr alone.
		return nil, nil
	}

	return dedup(raw), nil
}

// parseWithRegex applies a compiled pattern to stderr.
// The pattern must have two capture groups: (filepath)(line-number).
func parseWithRegex(re *regexp.Regexp, stderr string) []FileRef {
	matches := re.FindAllStringSubmatch(stderr, -1)
	refs := make([]FileRef, 0, len(matches))

	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		path := strings.TrimSpace(m[1])
		line, err := strconv.Atoi(m[2])
		if err != nil {
			line = 0
		}
		// Skip files that clearly don't exist on disk (e.g. <built-in>).
		if strings.HasPrefix(path, "<") {
			continue
		}
		refs = append(refs, FileRef{Path: path, Line: line})
	}

	return refs
}

// ── File reading ──────────────────────────────────────────────────────────────

// readSnippets attempts to read each referenced file and extract a windowed
// excerpt around the error line.  Files that cannot be read are silently skipped
// — the LLM will still receive the raw stderr and can work with that.
func readSnippets(refs []FileRef) map[string]string {
	snippets := make(map[string]string, len(refs))

	for _, ref := range refs {
		key := fmt.Sprintf("%s:%d", ref.Path, ref.Line)
		if _, already := snippets[key]; already {
			continue
		}

		content, err := os.ReadFile(ref.Path)
		if err != nil {
			// File may be a system header or outside the project — skip quietly.
			continue
		}

		snippets[key] = window(string(content), ref.Line, contextLines)
	}

	return snippets
}

// window extracts lines [center-radius .. center+radius] from src.
// Lines are 1-based.  If center == 0, the full file is returned (capped at
// 2*contextLines lines so we don't flood the LLM context).
func window(src string, center, radius int) string {
	lines := strings.Split(src, "\n")
	total := len(lines)

	if center == 0 {
		// No specific line — return the top of the file.
		end := 2 * radius
		if end > total {
			end = total
		}
		return annotate(lines[:end], 1)
	}

	start := center - 1 - radius // convert to 0-based, then go back radius
	if start < 0 {
		start = 0
	}
	end := center - 1 + radius + 1 // +1 because slice end is exclusive
	if end > total {
		end = total
	}

	return annotate(lines[start:end], start+1)
}

// annotate prepends "lineNum | " to each line, matching what editors show.
// firstLine is the 1-based number of lines[0].
func annotate(lines []string, firstLine int) string {
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "%4d | %s\n", firstLine+i, l)
	}
	return b.String()
}

// ── Deduplication ─────────────────────────────────────────────────────────────

// dedup removes duplicate FileRef entries, keeping first occurrence.
// A duplicate is defined as the same path+line pair.
func dedup(refs []FileRef) []FileRef {
	seen := make(map[string]bool, len(refs))
	out := make([]FileRef, 0, len(refs))

	for _, r := range refs {
		key := fmt.Sprintf("%s:%d", r.Path, r.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}

	return out
}
