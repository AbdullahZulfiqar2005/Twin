package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"twin/internal/interceptor"
	"twin/internal/llm"
)

// SandboxResult holds the verification outcomes from the containerized trial run.
type SandboxResult struct {
	Success    bool   // True if the command executed successfully (exit code 0)
	ExitCode   int    // Actual exit code inside the container
	Stderr     string // Error output captured from the compilation/run
	Info       string // Helpful status description (e.g. "Verified in gcc:latest")
}

// ErrDockerNotAvailable is returned when the docker CLI is not found or the daemon is inactive.
var ErrDockerNotAvailable = errors.New("docker is not running or available on this system")

// IsDockerAvailable checks if the local Docker daemon is active and running.
func IsDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// VerifyFix clones the active project into a temporary sandbox directory inside the workspace,
// applies the proposed patches (or executes the proposed system command), and re-runs the original command
// in a tailored Docker container. It returns the result of this trial run.
func VerifyFix(failedCmd string, args []string, kind interceptor.FailureKind, fix *llm.Fix) (*SandboxResult, error) {
	if !IsDockerAvailable() {
		return nil, ErrDockerNotAvailable
	}

	// 1. Resolve tailored Docker image based on compiler/language kind
	image := getDockerImage(kind)

	// 2. Create local sandbox workspace directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}

	sandboxParentDir := filepath.Join(cwd, ".twin_sandbox")
	if err := os.MkdirAll(sandboxParentDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create sandbox registry: %w", err)
	}

	tempSandboxDir, err := os.MkdirTemp(sandboxParentDir, "trial-")
	if err != nil {
		return nil, fmt.Errorf("failed to create unique sandbox directory: %w", err)
	}
	defer func() {
		// Clean up the temporary sandbox folder
		_ = os.RemoveAll(tempSandboxDir)
	}()

	// 3. Copy files to the sandbox, ignoring large metadata and VCS folders
	if err := copyDir(cwd, tempSandboxDir); err != nil {
		return nil, fmt.Errorf("failed to clone workspace to sandbox: %w", err)
	}

	// 4. Apply fix actions inside the cloned sandbox
	if fix.IsCommand {
		// Executing a command setup: we run it inside the container first
		setupArgs := []string{
			"run", "--rm",
			"-v", tempSandboxDir + ":/app",
			"-w", "/app",
			image,
			"/bin/sh", "-c", fix.Command,
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		
		cmd := exec.CommandContext(ctx, "docker", setupArgs...)
		var stderrBuf strings.Builder
		cmd.Stderr = &stderrBuf
		
		if err := cmd.Run(); err != nil {
			var exitErr *exec.ExitError
			exitCode := 1
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
			return &SandboxResult{
				Success:  false,
				ExitCode: exitCode,
				Stderr:   fmt.Sprintf("Proposed setup command failed:\n%s", stderrBuf.String()),
				Info:     fmt.Sprintf("Setup command failed in %s", image),
			}, nil
		}
	} else {
		// Applying patches in the sandbox directory
		for _, p := range fix.Patches {
			if err := applyPatch(tempSandboxDir, p); err != nil {
				return &SandboxResult{
					Success:  false,
					ExitCode: 1,
					Stderr:   fmt.Sprintf("Failed to apply patch in sandbox: %v", err),
					Info:     "Patch application failed in sandbox",
				}, nil
			}
		}
	}

	// 5. Re-run the failed command inside the containerized sandbox to verify the fix
	execArgs := []string{
		"run", "--rm",
		"-v", tempSandboxDir + ":/app",
		"-w", "/app",
		image,
		failedCmd,
	}
	execArgs = append(execArgs, args...)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", execArgs...)
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		exitCode := 1
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return &SandboxResult{
			Success:  false,
			ExitCode: exitCode,
			Stderr:   stderrBuf.String(),
			Info:     fmt.Sprintf("Command failed in sandbox (%s)", image),
		}, nil
	}

	return &SandboxResult{
		Success:  true,
		ExitCode: 0,
		Info:     fmt.Sprintf("Successfully verified in %s", image),
	}, nil
}

// getDockerImage maps the error kind to a suitable tailored container image.
func getDockerImage(kind interceptor.FailureKind) string {
	switch kind {
	case interceptor.KindGCC:
		return "gcc:latest"
	case interceptor.KindGo:
		return "golang:latest"
	case interceptor.KindPython:
		return "python:3-slim"
	case interceptor.KindRust:
		return "rust:latest"
	default:
		return "ubuntu:latest"
	}
}

// copyDir recursively clones the files in src to dst, skipping large/hidden folders.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		// Skip VCS, temporary files, local build binaries, and output binary
		if name == ".git" || name == ".twin_sandbox" || name == "twin" || name == "node_modules" || name == "target" || strings.HasPrefix(name, ".") && name != ".gitignore" {
			continue
		}
		srcPath := filepath.Join(src, name)
		dstPath := filepath.Join(dst, name)
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile transfers binary/text file contents preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// applyPatch executes search/replace inside the cloned sandbox workspace.
func applyPatch(sandboxDir string, p llm.Patch) error {
	targetPath := filepath.Join(sandboxDir, p.FilePath)

	// Directory traversal guard
	absSandbox, err := filepath.Abs(sandboxDir)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absTarget, absSandbox) {
		return fmt.Errorf("security violation: path %s escapes sandbox context", p.FilePath)
	}

	contentBytes, err := os.ReadFile(absTarget)
	if err != nil {
		return fmt.Errorf("failed to read target: %w", err)
	}

	content := string(contentBytes)
	if !strings.Contains(content, p.SearchBlock) {
		return fmt.Errorf("search block not found in %s", p.FilePath)
	}

	patched := strings.Replace(content, p.SearchBlock, p.ReplaceBlock, 1)

	info, err := os.Stat(absTarget)
	if err != nil {
		return err
	}

	if err := os.WriteFile(absTarget, []byte(patched), info.Mode()); err != nil {
		return fmt.Errorf("failed to write patched content: %w", err)
	}

	return nil
}
