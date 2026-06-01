package interceptor

import (
	"testing"

	"twin/internal/executor"
)

// ── classify() ────────────────────────────────────────────────────────────────

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   FailureKind
	}{
		{
			name: "gcc error",
			stderr: `main.c: In function 'main':
main.c:6:5: error: 'x' undeclared (first use in this function)
    6 |     x = 1;
      |     ^`,
			want: KindGCC,
		},
		{
			name: "clang error same pattern",
			stderr: `test.cpp:10:3: error: use of undeclared identifier 'foo'
    foo();
    ^~~`,
			want: KindGCC,
		},
		{
			name: "go build error",
			stderr: `# github.com/example/app
./main.go:12:2: undefined: Foobar`,
			want: KindGo,
		},
		{
			name: "go run error",
			stderr: `./main.go:8:14: cannot use "hello" (untyped string constant) as int value`,
			want: KindGo,
		},
		{
			name: "python traceback",
			stderr: `Traceback (most recent call last):
  File "script.py", line 5, in <module>
    result = 1 / 0
ZeroDivisionError: division by zero`,
			want: KindPython,
		},
		{
			name: "rust cargo error",
			stderr: `error[E0425]: cannot find value ` + "`" + `x` + "`" + ` in this scope
  --> src/main.rs:4:13
   |
 4 |     println!("{}", x);
   |                    ^ not found in this scope
cargo: error`,
			want: KindRust,
		},
		{
			name:   "unknown / shell failure",
			stderr: `ls: cannot access '/nonexistent': No such file or directory`,
			want:   KindUnknown,
		},
		{
			name:   "empty stderr",
			stderr: ``,
			want:   KindUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify([]byte(tc.stderr))
			if got != tc.want {
				t.Errorf("classify() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── Intercept() ───────────────────────────────────────────────────────────────

func TestIntercept_NilOnSuccess(t *testing.T) {
	result := &executor.Result{ExitCode: 0, Stderr: []byte("some output")}
	if got := Intercept(result); got != nil {
		t.Errorf("expected nil for exit 0, got %+v", got)
	}
}

func TestIntercept_NilOnEmptyStderr(t *testing.T) {
	result := &executor.Result{ExitCode: 1, Stderr: []byte{}}
	if got := Intercept(result); got != nil {
		t.Errorf("expected nil for empty stderr, got %+v", got)
	}
}

func TestIntercept_ReturnsEventOnFailure(t *testing.T) {
	stderr := []byte("main.c:3:1: error: expected ';' before '}'")
	result := &executor.Result{ExitCode: 1, Stderr: stderr}

	event := Intercept(result)
	if event == nil {
		t.Fatal("expected non-nil ErrorEvent")
	}
	if event.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", event.ExitCode)
	}
	if event.Kind != KindGCC {
		t.Errorf("Kind = %q, want %q", event.Kind, KindGCC)
	}
	if string(event.Stderr) != string(stderr) {
		t.Errorf("Stderr bytes not preserved")
	}
}
