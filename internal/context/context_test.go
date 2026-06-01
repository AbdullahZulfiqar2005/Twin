package context

import (
	"os"
	"strings"
	"testing"

	"twin/internal/interceptor"
)

// ── parseWithRegex ────────────────────────────────────────────────────────────

func TestParseGCC(t *testing.T) {
	stderr := `main.c: In function 'main':
main.c:6:5: error: 'x' undeclared (first use in this function)
main.c:6:5: note: each undeclared identifier is reported only once
lib/util.c:12:3: error: conflicting types for 'helper'`

	refs := parseWithRegex(reGCC, stderr)

	want := []FileRef{
		{Path: "main.c", Line: 6},
		{Path: "lib/util.c", Line: 12},
	}
	assertRefs(t, "GCC", refs, want)
}

func TestParseGo(t *testing.T) {
	stderr := `# github.com/example/app
./cmd/main.go:14:2: undefined: Config
./internal/server/server.go:88:15: cannot use resp (variable of type *http.Response) as io.Reader`

	refs := parseWithRegex(reGo, stderr)

	want := []FileRef{
		{Path: "./cmd/main.go", Line: 14},
		{Path: "./internal/server/server.go", Line: 88},
	}
	assertRefs(t, "Go", refs, want)
}

func TestParsePython(t *testing.T) {
	stderr := `Traceback (most recent call last):
  File "app/main.py", line 23, in run
    result = compute(data)
  File "app/compute.py", line 7, in compute
    return x / y
ZeroDivisionError: division by zero`

	refs := parseWithRegex(rePython, stderr)

	want := []FileRef{
		{Path: "app/main.py", Line: 23},
		{Path: "app/compute.py", Line: 7},
	}
	assertRefs(t, "Python", refs, want)
}

func TestParseRust(t *testing.T) {
	stderr := `error[E0425]: cannot find value ` + "`" + `x` + "`" + ` in this scope
  --> src/main.rs:4:13
   |
 4 |     println!("{}", x);
   |                    ^ not found in this scope

error[E0308]: mismatched types
  --> src/lib.rs:19:5`

	refs := parseWithRegex(reRust, stderr)

	want := []FileRef{
		{Path: "src/main.rs", Line: 4},
		{Path: "src/lib.rs", Line: 19},
	}
	assertRefs(t, "Rust", refs, want)
}

// ── dedup ─────────────────────────────────────────────────────────────────────

func TestDedup(t *testing.T) {
	input := []FileRef{
		{Path: "main.c", Line: 6},
		{Path: "main.c", Line: 6}, // duplicate
		{Path: "main.c", Line: 9}, // same file, different line — keep
		{Path: "util.c", Line: 1},
	}

	got := dedup(input)

	if len(got) != 3 {
		t.Fatalf("dedup: want 3 unique refs, got %d: %+v", len(got), got)
	}
	if got[0] != input[0] || got[1] != input[2] || got[2] != input[3] {
		t.Errorf("dedup order or content wrong: %+v", got)
	}
}

// ── window ────────────────────────────────────────────────────────────────────

func makeSource(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = strings.Repeat("x", 10) // filler content
	}
	return strings.Join(lines, "\n")
}

func TestWindow_CentreInMiddle(t *testing.T) {
	src := makeSource(100)
	got := window(src, 50, 5)
	// Should contain lines 45–55 (11 lines)
	if !strings.Contains(got, "  45 |") || !strings.Contains(got, "  55 |") {
		t.Errorf("window did not contain expected line numbers:\n%s", got)
	}
}

func TestWindow_NearStart(t *testing.T) {
	src := makeSource(30)
	got := window(src, 2, 5)
	// Should start at line 1 (clamped), not a negative index
	if !strings.Contains(got, "   1 |") {
		t.Errorf("window near start should include line 1:\n%s", got)
	}
}

func TestWindow_NearEnd(t *testing.T) {
	src := makeSource(10)
	got := window(src, 9, 5)
	// Should end at line 10 (clamped), not panic
	if !strings.Contains(got, "  10 |") {
		t.Errorf("window near end should include last line:\n%s", got)
	}
}

func TestWindow_ZeroLine(t *testing.T) {
	src := makeSource(50)
	got := window(src, 0, contextLines)
	// Should return top 2*contextLines lines starting at line 1
	if !strings.Contains(got, "   1 |") {
		t.Errorf("zero-line window should start at line 1:\n%s", got)
	}
}

// ── Extract() integration test ────────────────────────────────────────────────

func TestExtract_ReadsRealFile(t *testing.T) {
	// Write a small fake C source file to a temp directory.
	dir := t.TempDir()

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(oldCwd)

	srcPath := "main.c"
	src := `#include <stdio.h>
int main() {
    x = 1; // error: x undeclared
    return 0;
}
`
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatalf("setup: write temp file: %v", err)
	}

	// Craft a GCC-style stderr that references the temp file at line 3.
	stderr := srcPath + `:3:5: error: 'x' undeclared (first use in this function)`

	event := &interceptor.ErrorEvent{
		Kind:     interceptor.KindGCC,
		ExitCode: 1,
		Stderr:   []byte(stderr),
	}

	payload, err := Extract(event)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(payload.Refs) == 0 {
		t.Fatal("Extract(): expected at least one file reference")
	}
	if payload.Refs[0].Path != srcPath {
		t.Errorf("Refs[0].Path = %q, want %q", payload.Refs[0].Path, srcPath)
	}
	if payload.Refs[0].Line != 3 {
		t.Errorf("Refs[0].Line = %d, want 3", payload.Refs[0].Line)
	}

	key := srcPath + ":3"
	snip, ok := payload.Snippets[key]
	if !ok {
		t.Fatalf("no snippet for key %q; snippets: %v", key, payload.Snippets)
	}
	if !strings.Contains(snip, "x = 1") {
		t.Errorf("snippet does not contain error line content:\n%s", snip)
	}
}

func TestExtract_UnknownKindNoRefs(t *testing.T) {
	event := &interceptor.ErrorEvent{
		Kind:     interceptor.KindUnknown,
		ExitCode: 127,
		Stderr:   []byte("command not found: foobar"),
	}

	payload, err := Extract(event)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}
	if len(payload.Refs) != 0 {
		t.Errorf("unknown kind should produce zero refs, got %d", len(payload.Refs))
	}
	if payload.Stderr != string(event.Stderr) {
		t.Error("payload.Stderr not preserved")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertRefs(t *testing.T, label string, got, want []FileRef) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d refs, want %d\ngot:  %+v\nwant: %+v", label, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Path != want[i].Path || got[i].Line != want[i].Line {
			t.Errorf("%s ref[%d]: got {%q, %d}, want {%q, %d}",
				label, i, got[i].Path, got[i].Line, want[i].Path, want[i].Line)
		}
	}
}
