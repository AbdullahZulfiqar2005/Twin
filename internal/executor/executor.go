// Package executor implements FR-1: Process Execution.
//
// It accepts a command + args, spawns the child process, pipes stdout and stdin
// directly to the terminal (so normal usage is completely uninterrupted), and
// captures stderr for FR-2 (stream interception) to consume later.
package executor

import (
	"bytes"
	"io"
	"os"
	"os/exec"
)

// Result holds everything twin needs after a child process finishes.
type Result struct {
	// ExitCode is the child's exit status (0 = success).
	ExitCode int

	// Stderr is the raw bytes written to the child's stderr stream.
	// Kept in memory for FR-2 (error detection) and FR-3 (context extraction).
	Stderr []byte
}

// Run spawns `name` with `args` as a child process.
//
// FR-1 requirements met:
//   - stdout  → os.Stdout  (direct pipe, unmodified)
//   - stdin   → os.Stdin   (direct pipe, allows interactive programs)
//   - stderr  → captured into Result.Stderr (intercepted for FR-2)
//   - exit code is always returned; a non-zero code is NOT an error from
//     twin's perspective — it is normal data for FR-2 to act on.
func Run(name string, args []string) (*Result, error) {
	cmd := exec.Command(name, args...)

	// Pipe stdin and stdout directly — NFR-1: overhead must be < 50 ms.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	// Tee stderr: write to the terminal AND capture into a buffer.
	// The user still sees errors in real time; twin also gets them for analysis.
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	err := cmd.Run()

	result := &Result{
		Stderr: stderrBuf.Bytes(),
	}

	if err != nil {
		// *exec.ExitError means the child ran but exited non-zero.
		// That is expected behaviour for twin — capture the code and continue.
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		// If the command failed to start (e.g., executable not found, permission denied),
		// capture the error message, write it to stderr, and return as a status code 127
		// so the LLM pipeline can intercept and suggest the correct tool or spelling.
		errMsg := err.Error()
		_, _ = os.Stderr.WriteString("twin: execution error: " + errMsg + "\n")

		result.ExitCode = 127
		result.Stderr = []byte(errMsg)
		return result, nil
	}

	// err == nil → exit code 0
	result.ExitCode = 0
	return result, nil
}
